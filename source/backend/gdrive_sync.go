package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GDriveSyncConfig stores configuration and runtime status for two-way Google Drive synchronization
type GDriveSyncConfig struct {
	Enabled         bool     `json:"enabled"`
	LocalDir        string   `json:"local_dir"`
	DriveURL        string   `json:"drive_url"`
	DriveFolderID   string   `json:"drive_folder_id"`
	AccessToken     string   `json:"access_token"`
	APIKey          string   `json:"api_key"`
	IntervalMinutes int      `json:"interval_minutes"` // 15, 30, 60, 360, 720, 1440, 0 = manual
	Direction       string   `json:"direction"`        // "two_way", "upload_only", "download_only"
	LastSyncTime    string   `json:"last_sync_time"`
	LastStatus      string   `json:"last_status"`      // "idle", "syncing", "success", "error"
	LastLog         []string `json:"last_log"`
	FilesSynced     int      `json:"files_synced"`
	IsSyncing       bool     `json:"is_syncing"`
}

type gdriveRawItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	Size         string `json:"size"`
	MD5Checksum  string `json:"md5Checksum"`
	ModifiedTime string `json:"modifiedTime"` // RFC3339
}

type gdriveFileListResponse struct {
	NextPageToken string          `json:"nextPageToken"`
	Files         []gdriveRawItem `json:"files"`
}

type gdriveSyncFile struct {
	ID           string
	Name         string
	RelPath      string // relative path with forward slashes e.g. "Folder/Sub/file.jpg"
	ParentID     string
	Size         int64
	MD5Checksum  string
	ModifiedTime time.Time
}

type localSyncFile struct {
	Name         string
	RelPath      string // forward slashes e.g. "Folder/Sub/file.jpg"
	FullPath     string
	ParentRelDir string // "Folder/Sub" or ""
	Size         int64
	ModTime      time.Time
}

var (
	gdriveSyncLock   sync.Mutex
	gdriveConfigLock sync.RWMutex
	currentGDriveCfg GDriveSyncConfig
	gdriveLogHistory []string
	gdriveHTTPClient = &http.Client{
		Timeout: 300 * time.Second, // 5 min timeout per request
	}
)

const (
	gdriveConfigPrimaryPath  = "/etc/r3p/gdrive_sync.json"
	gdriveConfigFallbackPath = "/tmp/r3p_gdrive_sync.json"
	maxGDriveLogLines        = 100
)

func getGDriveConfigPath() string {
	if _, err := os.Stat("/etc/r3p"); err == nil {
		return gdriveConfigPrimaryPath
	}
	_ = os.MkdirAll("/etc/r3p", 0755)
	if _, err := os.Stat("/etc/r3p"); err == nil {
		return gdriveConfigPrimaryPath
	}
	return gdriveConfigFallbackPath
}

func initGDriveSync() {
	currentGDriveCfg = GDriveSyncConfig{
		Enabled:         false,
		LocalDir:        "/mnt/sda1/GDrive",
		DriveURL:        "",
		DriveFolderID:   "",
		AccessToken:     "",
		APIKey:          "",
		IntervalMinutes: 30,
		Direction:       "two_way",
		LastSyncTime:    "",
		LastStatus:      "idle",
		LastLog:         []string{"Hệ thống đồng bộ Google Drive 2 chiều sẵn sàng."},
		FilesSynced:     0,
		IsSyncing:       false,
	}

	loadGDriveConfigFromFile()
	go startGDriveScheduler()
}

func appendGDriveLog(msg string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)

	gdriveConfigLock.Lock()
	defer gdriveConfigLock.Unlock()

	gdriveLogHistory = append(gdriveLogHistory, entry)
	if len(gdriveLogHistory) > maxGDriveLogLines {
		gdriveLogHistory = gdriveLogHistory[len(gdriveLogHistory)-maxGDriveLogLines:]
	}
	currentGDriveCfg.LastLog = make([]string, len(gdriveLogHistory))
	copy(currentGDriveCfg.LastLog, gdriveLogHistory)
}

func loadGDriveConfigFromFile() {
	path := getGDriveConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	gdriveConfigLock.Lock()
	defer gdriveConfigLock.Unlock()

	var loaded GDriveSyncConfig
	if err := json.Unmarshal(data, &loaded); err == nil {
		currentGDriveCfg.Enabled = loaded.Enabled
		if loaded.LocalDir != "" {
			currentGDriveCfg.LocalDir = loaded.LocalDir
		}
		currentGDriveCfg.DriveURL = loaded.DriveURL
		currentGDriveCfg.DriveFolderID = loaded.DriveFolderID
		currentGDriveCfg.AccessToken = loaded.AccessToken
		currentGDriveCfg.APIKey = loaded.APIKey
		if loaded.IntervalMinutes > 0 {
			currentGDriveCfg.IntervalMinutes = loaded.IntervalMinutes
		}
		if loaded.Direction != "" {
			currentGDriveCfg.Direction = loaded.Direction
		}
		currentGDriveCfg.LastSyncTime = loaded.LastSyncTime
		currentGDriveCfg.LastStatus = loaded.LastStatus
		currentGDriveCfg.FilesSynced = loaded.FilesSynced
		if len(loaded.LastLog) > 0 {
			gdriveLogHistory = loaded.LastLog
			currentGDriveCfg.LastLog = loaded.LastLog
		}
	}
}

func saveGDriveConfigToFile() error {
	path := getGDriveConfigPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	gdriveConfigLock.RLock()
	data, err := json.MarshalIndent(currentGDriveCfg, "", "  ")
	gdriveConfigLock.RUnlock()

	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// extractGDriveFolderID parses folder ID from Google Drive URL formats
func extractGDriveFolderID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	// Regex for /folders/{id} or /d/{id} or id={id}
	re := regexp.MustCompile(`(?:folders/|id=|/d/)([a-zA-Z0-9_-]{15,})`)
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		return matches[1]
	}

	// If already a clean raw folder ID (15+ chars, no slashes or queries)
	if len(input) >= 15 && !strings.Contains(input, "/") && !strings.Contains(input, "?") {
		return input
	}

	return input
}

func startGDriveScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		gdriveConfigLock.RLock()
		enabled := currentGDriveCfg.Enabled
		interval := currentGDriveCfg.IntervalMinutes
		lastTimeStr := currentGDriveCfg.LastSyncTime
		isSyncing := currentGDriveCfg.IsSyncing
		folderID := currentGDriveCfg.DriveFolderID
		token := currentGDriveCfg.AccessToken
		gdriveConfigLock.RUnlock()

		if !enabled || interval <= 0 || isSyncing || (folderID == "" && token == "") {
			continue
		}

		shouldRun := false
		if lastTimeStr == "" {
			shouldRun = true
		} else {
			lastTime, err := time.Parse("2006-01-02 15:04:05", lastTimeStr)
			if err != nil || time.Since(lastTime) >= time.Duration(interval)*time.Minute {
				shouldRun = true
			}
		}

		if shouldRun {
			log.Println("[GDrive Sync] Auto-schedule triggered sync interval:", interval, "minutes")
			go runGDriveSyncNow()
		}
	}
}

// runGDriveSyncNow performs the recursive two-way sync operation
func runGDriveSyncNow() {
	if !gdriveSyncLock.TryLock() {
		appendGDriveLog("⚠️ Quá trình đồng bộ đang chạy, bỏ qua yêu cầu trùng lặp.")
		return
	}
	defer gdriveSyncLock.Unlock()

	gdriveConfigLock.Lock()
	currentGDriveCfg.IsSyncing = true
	currentGDriveCfg.LastStatus = "syncing"
	localDir := currentGDriveCfg.LocalDir
	folderID := currentGDriveCfg.DriveFolderID
	if folderID == "" {
		folderID = extractGDriveFolderID(currentGDriveCfg.DriveURL)
		currentGDriveCfg.DriveFolderID = folderID
	}
	token := strings.TrimSpace(currentGDriveCfg.AccessToken)
	direction := currentGDriveCfg.Direction
	if direction == "" {
		direction = "two_way"
	}
	gdriveConfigLock.Unlock()

	appendGDriveLog("🚀 [BẮT ĐẦU] Khởi động đồng bộ Google Drive 2 chiều (Đệ quy đa thư mục)...")
	appendGDriveLog(fmt.Sprintf("📂 Thư mục cục bộ: %s | Chế độ: %s", localDir, direction))

	if folderID == "" {
		appendGDriveLog("❌ LỖI: Chưa có Google Drive Folder ID hoặc URL hợp lệ.")
		finishGDriveSync("error", 0)
		return
	}

	if token == "" {
		appendGDriveLog("❌ LỖI: Cần cung cấp Access Token để truy cập Google Drive API.")
		finishGDriveSync("error", 0)
		return
	}

	// Ensure local dir exists
	if err := os.MkdirAll(localDir, 0755); err != nil {
		appendGDriveLog(fmt.Sprintf("❌ LỖI tạo thư mục cục bộ: %v", err))
		finishGDriveSync("error", 0)
		return
	}

	// 1. Recursive scan of Google Drive folder hierarchy via BFS
	appendGDriveLog("🔍 Đang kết nối Google Drive và quét toàn bộ cây thư mục & tệp...")
	driveFolders, driveFiles, err := scanGDriveTree(token, folderID)
	if err != nil {
		appendGDriveLog(fmt.Sprintf("❌ LỖI truy vấn Google Drive API: %v", err))
		finishGDriveSync("error", 0)
		return
	}
	appendGDriveLog(fmt.Sprintf("☁️ Tìm thấy %d thư mục con và %d tệp trên Google Drive.", len(driveFolders)-1, len(driveFiles)))

	// 2. Recursive scan of local directory
	appendGDriveLog(fmt.Sprintf("📁 Đang quét danh sách thư mục & tệp tại %s...", localDir))
	localDirs, localFiles, err := scanLocalTree(localDir)
	if err != nil {
		appendGDriveLog(fmt.Sprintf("❌ LỖI đọc thư mục cục bộ: %v", err))
		finishGDriveSync("error", 0)
		return
	}
	appendGDriveLog(fmt.Sprintf("💻 Tìm thấy %d thư mục con và %d tệp tại thư mục cục bộ.", len(localDirs), len(localFiles)))

	syncedCount := 0

	// 3. Two-way reconciliation

	// Action A: Files on Google Drive but not local -> Download
	if direction == "two_way" || direction == "download_only" {
		for relPath, dFile := range driveFiles {
			if _, exists := localFiles[relPath]; !exists {
				destPath := filepath.Join(localDir, filepath.FromSlash(relPath))
				_ = os.MkdirAll(filepath.Dir(destPath), 0755)

				appendGDriveLog(fmt.Sprintf("⬇️ [DOWNLOAD] Tải tệp mới: %s (%d bytes)", relPath, dFile.Size))
				if err := downloadGDriveFile(token, dFile.ID, destPath, dFile.ModifiedTime); err != nil {
					appendGDriveLog(fmt.Sprintf("  ❌ Lỗi tải '%s': %v", relPath, err))
				} else {
					syncedCount++
					appendGDriveLog(fmt.Sprintf("  ✅ Đã tải về thành công: %s", relPath))
				}
			}
		}
	}

	// Action B: Files locally but not on Google Drive -> Upload
	if direction == "two_way" || direction == "upload_only" {
		for relPath, lFile := range localFiles {
			if _, exists := driveFiles[relPath]; !exists {
				// Ensure remote parent folder exists
				targetFolderID, err := ensureGDriveFolderPath(token, folderID, lFile.ParentRelDir, driveFolders)
				if err != nil {
					appendGDriveLog(fmt.Sprintf("  ❌ Lỗi tạo thư mục đám mây cho '%s': %v", relPath, err))
					continue
				}

				appendGDriveLog(fmt.Sprintf("⬆️ [UPLOAD] Tải tệp mới lên Drive: %s (%d bytes)", relPath, lFile.Size))
				if err := uploadGDriveMultipart(token, targetFolderID, lFile.FullPath, lFile.Name); err != nil {
					appendGDriveLog(fmt.Sprintf("  ❌ Lỗi tải lên '%s': %v", relPath, err))
				} else {
					syncedCount++
					appendGDriveLog(fmt.Sprintf("  ✅ Đã tải lên Google Drive thành công: %s", relPath))
				}
			}
		}
	}

	// Action C: Files existing in BOTH local and Drive -> Compare MD5 / timestamps
	if direction == "two_way" {
		for relPath, lFile := range localFiles {
			dFile, exists := driveFiles[relPath]
			if !exists {
				continue
			}

			// If size and timestamps match closely (< 3s), skip expensive MD5 hash
			diff := lFile.ModTime.Sub(dFile.ModifiedTime)
			if lFile.Size == dFile.Size && diff >= -3*time.Second && diff <= 3*time.Second {
				continue
			}

			// Check MD5 checksum
			lHash, _ := getFileMD5(lFile.FullPath)
			if dFile.MD5Checksum != "" && lHash != "" && strings.EqualFold(dFile.MD5Checksum, lHash) {
				// Exact content match, synchronize modification timestamp
				_ = os.Chtimes(lFile.FullPath, dFile.ModifiedTime, dFile.ModifiedTime)
				continue
			}

			// Different content: newer wins
			if diff > 3*time.Second {
				// Local file is newer -> Update Google Drive
				appendGDriveLog(fmt.Sprintf("⬆️ [UPDATE DRIVE] Tệp cục bộ mới hơn, cập nhật lên Drive: %s", relPath))
				if err := patchGDriveFile(token, dFile.ID, lFile.FullPath); err != nil {
					appendGDriveLog(fmt.Sprintf("  ❌ Lỗi cập nhật lên Drive '%s': %v", relPath, err))
				} else {
					syncedCount++
					appendGDriveLog(fmt.Sprintf("  ✅ Đã cập nhật lên Drive: %s", relPath))
				}
			} else if diff < -3*time.Second {
				// Remote Drive file is newer -> Download to local
				appendGDriveLog(fmt.Sprintf("⬇️ [UPDATE LOCAL] Tệp Drive mới hơn, tải bản mới về máy: %s", relPath))
				destPath := lFile.FullPath
				if err := downloadGDriveFile(token, dFile.ID, destPath, dFile.ModifiedTime); err != nil {
					appendGDriveLog(fmt.Sprintf("  ❌ Lỗi cập nhật về máy '%s': %v", relPath, err))
				} else {
					syncedCount++
					appendGDriveLog(fmt.Sprintf("  ✅ Đã cập nhật về máy: %s", relPath))
				}
			}
		}
	}

	appendGDriveLog(fmt.Sprintf("🏁 [HOÀN TẤT] Đồng bộ xong. Tổng số tệp xử lý/cập nhật: %d tệp.", syncedCount))
	finishGDriveSync("success", syncedCount)
}

func finishGDriveSync(status string, syncedCount int) {
	gdriveConfigLock.Lock()
	currentGDriveCfg.IsSyncing = false
	currentGDriveCfg.LastStatus = status
	currentGDriveCfg.LastSyncTime = time.Now().Format("2006-01-02 15:04:05")
	if syncedCount > 0 {
		currentGDriveCfg.FilesSynced += syncedCount
	}
	gdriveConfigLock.Unlock()

	_ = saveGDriveConfigToFile()
}

func parseRFC3339Time(str string) time.Time {
	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, str)
		if err != nil {
			return time.Now().UTC()
		}
	}
	return t.UTC()
}

func getFileMD5(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// scanGDriveTree performs BFS traversal to discover all subfolders and files
func scanGDriveTree(token string, rootFolderID string) (map[string]string, map[string]gdriveSyncFile, error) {
	driveFolders := make(map[string]string)
	driveFolders[""] = rootFolderID
	driveFiles := make(map[string]gdriveSyncFile)

	type queueItem struct {
		folderID string
		relPath  string
	}
	queue := []queueItem{{folderID: rootFolderID, relPath: ""}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		items, err := queryGDriveFolderItems(token, curr.folderID)
		if err != nil {
			appendGDriveLog(fmt.Sprintf("⚠️ Lỗi quét thư mục Drive '%s': %v", curr.relPath, err))
			continue
		}

		for _, item := range items {
			var itemRelPath string
			if curr.relPath == "" {
				itemRelPath = item.Name
			} else {
				itemRelPath = curr.relPath + "/" + item.Name
			}

			if item.MimeType == "application/vnd.google-apps.folder" {
				driveFolders[itemRelPath] = item.ID
				queue = append(queue, queueItem{
					folderID: item.ID,
					relPath:  itemRelPath,
				})
			} else {
				sz, _ := strconv.ParseInt(item.Size, 10, 64)
				driveFiles[itemRelPath] = gdriveSyncFile{
					ID:           item.ID,
					Name:         item.Name,
					RelPath:      itemRelPath,
					ParentID:     curr.folderID,
					Size:         sz,
					MD5Checksum:  item.MD5Checksum,
					ModifiedTime: parseRFC3339Time(item.ModifiedTime),
				}
			}
		}
	}

	return driveFolders, driveFiles, nil
}

// queryGDriveFolderItems queries all direct children of folderID
func queryGDriveFolderItems(token, folderID string) ([]gdriveRawItem, error) {
	var results []gdriveRawItem
	pageToken := ""

	for {
		apiURL := fmt.Sprintf(
			"https://www.googleapis.com/drive/v3/files?q='%s'+in+parents+and+trashed=false&fields=nextPageToken,files(id,name,mimeType,size,md5Checksum,modifiedTime)&pageSize=1000",
			url.QueryEscape(folderID),
		)
		if pageToken != "" {
			apiURL += "&pageToken=" + url.QueryEscape(pageToken)
		}

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		resp, err := gdriveHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var listResp gdriveFileListResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, err
		}

		results = append(results, listResp.Files...)
		if listResp.NextPageToken == "" {
			break
		}
		pageToken = listResp.NextPageToken
	}

	return results, nil
}

// scanLocalTree traverses local directory and indexes subfolders and files
func scanLocalTree(localDir string) (map[string]bool, map[string]localSyncFile, error) {
	localDirs := make(map[string]bool)
	localFiles := make(map[string]localSyncFile)

	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == localDir {
			return nil
		}
		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".tmp") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			localDirs[rel] = true
			return nil
		}

		parentRel := filepath.ToSlash(filepath.Dir(rel))
		if parentRel == "." {
			parentRel = ""
		}

		localFiles[rel] = localSyncFile{
			Name:         info.Name(),
			RelPath:      rel,
			FullPath:     path,
			ParentRelDir: parentRel,
			Size:         info.Size(),
			ModTime:      info.ModTime().UTC(),
		}
		return nil
	})

	return localDirs, localFiles, err
}

// ensureGDriveFolderPath creates missing directories on Google Drive along relPath
func ensureGDriveFolderPath(token, rootFolderID, relPath string, driveFolders map[string]string) (string, error) {
	relPath = strings.Trim(filepath.ToSlash(relPath), "/")
	if relPath == "" || relPath == "." {
		return rootFolderID, nil
	}

	if id, ok := driveFolders[relPath]; ok && id != "" {
		return id, nil
	}

	segments := strings.Split(relPath, "/")
	currParentID := rootFolderID
	currPath := ""

	for _, seg := range segments {
		if currPath == "" {
			currPath = seg
		} else {
			currPath = currPath + "/" + seg
		}

		if id, ok := driveFolders[currPath]; ok && id != "" {
			currParentID = id
		} else {
			newID, err := createGDriveFolder(token, currParentID, seg)
			if err != nil {
				return "", fmt.Errorf("không thể tạo thư mục '%s': %w", currPath, err)
			}
			driveFolders[currPath] = newID
			currParentID = newID
			appendGDriveLog(fmt.Sprintf("📁 [TẠO THƯ MỤC DRIVE] Đã tạo thư mục đám mây: %s", currPath))
		}
	}

	return currParentID, nil
}

// createGDriveFolder creates a single folder on Google Drive
func createGDriveFolder(token, parentID, folderName string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"name":     folderName,
		"mimeType": "application/vnd.google-apps.folder",
		"parents":  []string{parentID},
	})
	req, err := http.NewRequest("POST", "https://www.googleapis.com/drive/v3/files", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := gdriveHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// downloadGDriveFile downloads a file from Google Drive and preserves timestamp
func downloadGDriveFile(token, fileID, destPath string, modTime time.Time) error {
	apiURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s?alt=media", url.PathEscape(fileID))
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := gdriveHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	tmpFile := destPath + ".gdrive.tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if err := os.Rename(tmpFile, destPath); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if !modTime.IsZero() {
		_ = os.Chtimes(destPath, modTime, modTime)
	}

	return nil
}

// uploadGDriveMultipart uploads a new file to Google Drive using multipart/related
func uploadGDriveMultipart(token, folderID, localPath, fileName string) error {
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	mpw := multipart.NewWriter(pw)

	go func() {
		var writeErr error
		defer func() {
			if writeErr != nil {
				pw.CloseWithError(writeErr)
			} else {
				pw.Close()
			}
		}()

		// Part 1: Metadata JSON
		hMeta := make(textproto.MIMEHeader)
		hMeta.Set("Content-Type", "application/json; charset=UTF-8")
		partMeta, err := mpw.CreatePart(hMeta)
		if err != nil {
			writeErr = err
			return
		}

		metaJSON, _ := json.Marshal(map[string]interface{}{
			"name":    fileName,
			"parents": []string{folderID},
		})
		if _, err := partMeta.Write(metaJSON); err != nil {
			writeErr = err
			return
		}

		// Part 2: Media content
		hMedia := make(textproto.MIMEHeader)
		mimeType := mime.TypeByExtension(filepath.Ext(fileName))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		hMedia.Set("Content-Type", mimeType)
		partMedia, err := mpw.CreatePart(hMedia)
		if err != nil {
			writeErr = err
			return
		}

		f, err := os.Open(localPath)
		if err != nil {
			writeErr = err
			return
		}
		defer f.Close()

		if _, err := io.Copy(partMedia, f); err != nil {
			writeErr = err
			return
		}

		writeErr = mpw.Close()
	}()

	uploadURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart"
	req, err := http.NewRequest("POST", uploadURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/related; boundary=%s", mpw.Boundary()))

	resp, err := gdriveHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	_ = fileInfo
	return nil
}

// patchGDriveFile updates an existing file on Google Drive with new local content
func patchGDriveFile(token, fileID, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	uploadURL := fmt.Sprintf("https://www.googleapis.com/upload/drive/v3/files/%s?uploadType=media", url.PathEscape(fileID))
	req, err := http.NewRequest("PATCH", uploadURL, f)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	mimeType := mime.TypeByExtension(filepath.Ext(localPath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", mimeType)

	resp, err := gdriveHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// Handlers for API endpoints

func handleGDriveSync(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gdriveConfigLock.RLock()
		cfg := currentGDriveCfg
		gdriveConfigLock.RUnlock()
		writeJSON(w, http.StatusOK, true, cfg)

	case http.MethodPost, http.MethodPut:
		var incoming struct {
			Enabled         *bool   `json:"enabled"`
			LocalDir        *string `json:"local_dir"`
			DriveURL        *string `json:"drive_url"`
			DriveFolderID   *string `json:"drive_folder_id"`
			AccessToken     *string `json:"access_token"`
			APIKey          *string `json:"api_key"`
			IntervalMinutes *int    `json:"interval_minutes"`
			Direction       *string `json:"direction"`
		}

		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&incoming); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "JSON payload không hợp lệ: "+err.Error())
			return
		}

		gdriveConfigLock.Lock()
		if incoming.Enabled != nil {
			currentGDriveCfg.Enabled = *incoming.Enabled
		}
		if incoming.LocalDir != nil && *incoming.LocalDir != "" {
			currentGDriveCfg.LocalDir = strings.TrimSpace(*incoming.LocalDir)
		}
		if incoming.DriveURL != nil {
			currentGDriveCfg.DriveURL = strings.TrimSpace(*incoming.DriveURL)
			extractedID := extractGDriveFolderID(currentGDriveCfg.DriveURL)
			if extractedID != "" {
				currentGDriveCfg.DriveFolderID = extractedID
			}
		}
		if incoming.DriveFolderID != nil && *incoming.DriveFolderID != "" {
			currentGDriveCfg.DriveFolderID = strings.TrimSpace(*incoming.DriveFolderID)
		}
		if incoming.AccessToken != nil {
			currentGDriveCfg.AccessToken = strings.TrimSpace(*incoming.AccessToken)
		}
		if incoming.APIKey != nil {
			currentGDriveCfg.APIKey = strings.TrimSpace(*incoming.APIKey)
		}
		if incoming.IntervalMinutes != nil && *incoming.IntervalMinutes >= 0 {
			currentGDriveCfg.IntervalMinutes = *incoming.IntervalMinutes
		}
		if incoming.Direction != nil && *incoming.Direction != "" {
			currentGDriveCfg.Direction = *incoming.Direction
		}
		cfg := currentGDriveCfg
		gdriveConfigLock.Unlock()

		if err := saveGDriveConfigToFile(); err != nil {
			appendGDriveLog("⚠️ Lỗi lưu cấu hình ra tệp: " + err.Error())
		} else {
			appendGDriveLog("⚙️ Đã lưu cấu hình Google Drive Sync thành công.")
		}

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": "Đã lưu cài đặt đồng bộ Google Drive thành công!",
			"config":  cfg,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "Phương thức không được hỗ trợ")
	}
}

func handleGDriveSyncRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "Method not allowed")
		return
	}

	gdriveConfigLock.RLock()
	isSyncing := currentGDriveCfg.IsSyncing
	gdriveConfigLock.RUnlock()

	if isSyncing {
		writeJSON(w, http.StatusConflict, false, "Tiến trình đồng bộ đang được thực hiện.")
		return
	}

	go runGDriveSyncNow()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"status":  "started",
		"message": "Đã bắt đầu tiến trình đồng bộ 2 chiều với Google Drive!",
	})
}
