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
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

// GDriveFolderMapping defines an individual synchronization pairing
type GDriveFolderMapping struct {
	ID            string `json:"id"`
	LocalDir      string `json:"local_dir"`
	DriveURL      string `json:"drive_url"`
	DriveFolderID string `json:"drive_folder_id"`
	Enabled       bool   `json:"enabled"`
	Name          string `json:"name,omitempty"`
}

// GDriveSyncConfig stores configuration and runtime status for two-way Google Drive synchronization
type GDriveSyncConfig struct {
	Enabled         bool                  `json:"enabled"`
	LocalDir        string                `json:"local_dir"`
	DriveURL        string                `json:"drive_url"`
	DriveFolderID   string                `json:"drive_folder_id"`
	Folders         []GDriveFolderMapping `json:"folders"`
	AccessToken     string                `json:"access_token"`
	RefreshToken    string                `json:"refresh_token"`
	ClientID        string                `json:"client_id"`
	ClientSecret    string                `json:"client_secret"`
	APIKey          string                `json:"api_key"`
	IntervalMinutes int                   `json:"interval_minutes"` // 15, 30, 60, 360, 720, 1440, 0 = manual
	Direction       string                `json:"direction"`        // "two_way", "upload_only", "download_only"
	LastSyncTime    string                `json:"last_sync_time"`
	LastStatus      string                `json:"last_status"`      // "idle", "syncing", "success", "error"
	LastLog         []string              `json:"last_log"`
	FilesSynced     int                   `json:"files_synced"`
	IsSyncing       bool                  `json:"is_syncing"`
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
	gdriveSyncLock    sync.Mutex
	gdriveConfigLock  sync.RWMutex
	currentGDriveCfg  GDriveSyncConfig
	gdriveLogHistory  []string
	systemTimeLoc     *time.Location
	systemTimeLocOnce sync.Once
	gdriveHTTPClient  = &http.Client{
		Timeout: 300 * time.Second, // 5 min timeout per request
	}
)

const (
	gdriveConfigPrimaryPath  = "/etc/r3p/gdrive_sync.json"
	gdriveConfigFallbackPath = "/tmp/r3p_gdrive_sync.json"
	maxGDriveLogLines        = 100
)

// getSystemTimeLocation returns the router's local timezone (e.g., Asia/Ho_Chi_Minh / ICT)
func getSystemTimeLocation() *time.Location {
	systemTimeLocOnce.Do(func() {
		// 1. Check UCI zonename: uci get system.@system[0].zonename
		if out, err := exec.Command("uci", "get", "system.@system[0].zonename").Output(); err == nil {
			zName := strings.TrimSpace(string(out))
			if zName != "" {
				if loc, err := time.LoadLocation(zName); err == nil {
					systemTimeLoc = loc
					return
				}
			}
		}

		// 2. Check "date +%z" from system shell (e.g. "+0700")
		if out, err := exec.Command("date", "+%z").Output(); err == nil {
			s := strings.TrimSpace(string(out))
			if len(s) == 5 && (s[0] == '+' || s[0] == '-') {
				sign := 1
				if s[0] == '-' {
					sign = -1
				}
				hours, _ := strconv.Atoi(s[1:3])
				mins, _ := strconv.Atoi(s[3:5])
				offsetSec := sign * ((hours * 3600) + (mins * 60))
				systemTimeLoc = time.FixedZone("SYSTEM", offsetSec)
				return
			}
		}

		// 3. Fallback to time.Local
		systemTimeLoc = time.Local
	})
	if systemTimeLoc == nil {
		return time.Local
	}
	return systemTimeLoc
}

func getSystemTimeNow() time.Time {
	return time.Now().In(getSystemTimeLocation())
}

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
		Folders:         []GDriveFolderMapping{},
		AccessToken:     "",
		RefreshToken:    "",
		ClientID:        "",
		ClientSecret:    "",
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
	timestamp := getSystemTimeNow().Format("2006-01-02 15:04:05")
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
		currentGDriveCfg.Folders = loaded.Folders
		currentGDriveCfg.AccessToken = loaded.AccessToken
		currentGDriveCfg.RefreshToken = loaded.RefreshToken
		currentGDriveCfg.ClientID = loaded.ClientID
		currentGDriveCfg.ClientSecret = loaded.ClientSecret
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

	// Auto-migrate single folder config to Folders array if Folders is empty
	if len(currentGDriveCfg.Folders) == 0 && (currentGDriveCfg.LocalDir != "" || currentGDriveCfg.DriveURL != "") {
		fID := currentGDriveCfg.DriveFolderID
		if fID == "" {
			fID = extractGDriveFolderID(currentGDriveCfg.DriveURL)
		}
		currentGDriveCfg.Folders = []GDriveFolderMapping{
			{
				ID:            "folder-1",
				LocalDir:      currentGDriveCfg.LocalDir,
				DriveURL:      currentGDriveCfg.DriveURL,
				DriveFolderID: fID,
				Enabled:       true,
				Name:          filepath.Base(currentGDriveCfg.LocalDir),
			},
		}
	}
}

func saveGDriveConfigToFileLocked() error {
	path := getGDriveConfigPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	data, err := json.MarshalIndent(currentGDriveCfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func saveGDriveConfigToFile() error {
	gdriveConfigLock.RLock()
	defer gdriveConfigLock.RUnlock()
	return saveGDriveConfigToFileLocked()
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

// refreshGDriveToken requests a new access token from Google OAuth2 using the refresh token
func refreshGDriveToken() (string, error) {
	gdriveConfigLock.RLock()
	refreshToken := strings.TrimSpace(currentGDriveCfg.RefreshToken)
	clientID := strings.TrimSpace(currentGDriveCfg.ClientID)
	clientSecret := strings.TrimSpace(currentGDriveCfg.ClientSecret)
	gdriveConfigLock.RUnlock()

	if refreshToken == "" {
		return "", fmt.Errorf("chưa cấu hình Refresh Token")
	}

	authClient := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Method 1: If user configured custom GCP Client ID & Secret, use standard OAuth2 endpoint
	if clientID != "" && clientSecret != "" {
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refreshToken)
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)

		req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if resp, err := authClient.Do(req); err == nil {
				defer resp.Body.Close()
				bodyBytes, _ := io.ReadAll(resp.Body)
				if resp.StatusCode == http.StatusOK {
					var res struct {
						AccessToken string `json:"access_token"`
					}
					if err := json.Unmarshal(bodyBytes, &res); err == nil && res.AccessToken != "" {
						gdriveConfigLock.Lock()
						currentGDriveCfg.AccessToken = res.AccessToken
						_ = saveGDriveConfigToFileLocked()
						gdriveConfigLock.Unlock()
						appendGDriveLog("🔄 [TOKEN REFRESH] Đã cấp mới Access Token thành công qua Custom Google OAuth2.")
						return res.AccessToken, nil
					}
				}
			}
		}
	}

	// Method 2: Google OAuth 2.0 Playground refresh endpoint (primary method for Playground tokens)
	playgroundPayload, _ := json.Marshal(map[string]string{
		"token_uri":     "https://oauth2.googleapis.com/token",
		"refresh_token": refreshToken,
	})
	pgReq, err := http.NewRequest("POST", "https://developers.google.com/oauthplayground/refreshAccessToken", bytes.NewReader(playgroundPayload))
	if err == nil {
		pgReq.Header.Set("Content-Type", "application/json")
		pgReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
		if resp, err := authClient.Do(pgReq); err == nil {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusOK {
				var pgRes struct {
					Success     bool   `json:"success"`
					AccessToken string `json:"access_token"`
				}
				if err := json.Unmarshal(bodyBytes, &pgRes); err == nil && pgRes.AccessToken != "" {
					gdriveConfigLock.Lock()
					currentGDriveCfg.AccessToken = pgRes.AccessToken
					_ = saveGDriveConfigToFileLocked()
					gdriveConfigLock.Unlock()
					appendGDriveLog("🔄 [TOKEN REFRESH] Đã cấp mới Access Token thành công qua Google Playground.")
					return pgRes.AccessToken, nil
				}
			}
		}
	}

	return "", fmt.Errorf("không thể làm mới Access Token: vui lòng kiểm tra lại Refresh Token từ Google Playground hoặc cấu hình Client ID & Client Secret riêng")
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
		refreshToken := currentGDriveCfg.RefreshToken
		gdriveConfigLock.RUnlock()

		if !enabled || interval <= 0 || isSyncing || (folderID == "" && token == "" && refreshToken == "") {
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

// runGDriveSyncNow performs the recursive two-way sync operation for all configured folders
func runGDriveSyncNow() {
	if !gdriveSyncLock.TryLock() {
		appendGDriveLog("⚠️ Quá trình đồng bộ đang chạy, bỏ qua yêu cầu trùng lặp.")
		return
	}
	defer gdriveSyncLock.Unlock()

	gdriveConfigLock.Lock()
	currentGDriveCfg.IsSyncing = true
	currentGDriveCfg.LastStatus = "syncing"

	// Prepare list of active folders to sync
	var activeFolders []GDriveFolderMapping
	for _, f := range currentGDriveCfg.Folders {
		if f.Enabled && f.LocalDir != "" {
			fID := f.DriveFolderID
			if fID == "" {
				fID = extractGDriveFolderID(f.DriveURL)
				f.DriveFolderID = fID
			}
			if fID != "" {
				activeFolders = append(activeFolders, f)
			}
		}
	}

	// Fallback to root LocalDir / DriveFolderID if Folders slice is empty
	if len(activeFolders) == 0 && currentGDriveCfg.LocalDir != "" {
		fID := currentGDriveCfg.DriveFolderID
		if fID == "" {
			fID = extractGDriveFolderID(currentGDriveCfg.DriveURL)
		}
		if fID != "" {
			activeFolders = append(activeFolders, GDriveFolderMapping{
				ID:            "folder-default",
				LocalDir:      currentGDriveCfg.LocalDir,
				DriveFolderID: fID,
				DriveURL:      currentGDriveCfg.DriveURL,
				Enabled:       true,
			})
		}
	}

	token := strings.TrimSpace(currentGDriveCfg.AccessToken)
	refreshToken := strings.TrimSpace(currentGDriveCfg.RefreshToken)
	direction := currentGDriveCfg.Direction
	if direction == "" {
		direction = "two_way"
	}
	gdriveConfigLock.Unlock()

	if len(activeFolders) == 0 {
		appendGDriveLog("❌ LỖI: Chưa có thư mục nào được bật hoặc thiếu Google Drive Folder ID / URL.")
		finishGDriveSync("error", 0)
		return
	}

	appendGDriveLog(fmt.Sprintf("🚀 [BẮT ĐẦU] Khởi động đồng bộ Google Drive 2 chiều (%d thư mục)...", len(activeFolders)))

	// Auto-refresh token if empty but refresh token is present
	if token == "" && refreshToken != "" {
		appendGDriveLog("🔑 Access Token trống, đang dùng Refresh Token để lấy token mới...")
		newToken, err := refreshGDriveToken()
		if err != nil {
			appendGDriveLog("❌ LỖI gia hạn Token: " + err.Error())
			finishGDriveSync("error", 0)
			return
		}
		token = newToken
	}

	if token == "" {
		appendGDriveLog("❌ LỖI: Cần cung cấp Access Token hoặc Refresh Token để truy cập Google Drive API.")
		finishGDriveSync("error", 0)
		return
	}

	totalSynced := 0

	for idx, folderMap := range activeFolders {
		localDir := folderMap.LocalDir
		folderID := folderMap.DriveFolderID

		appendGDriveLog(fmt.Sprintf("📂 [%d/%d] Đang đồng bộ thư mục: %s <---> Google Drive (%s)...", idx+1, len(activeFolders), localDir, folderID))

		// Ensure local dir exists
		if err := os.MkdirAll(localDir, 0755); err != nil {
			appendGDriveLog(fmt.Sprintf("  ❌ LỖI tạo thư mục cục bộ '%s': %v", localDir, err))
			continue
		}

		// 1. Recursive scan of Google Drive folder hierarchy via BFS
		appendGDriveLog(fmt.Sprintf("  🔍 Quét cây thư mục Google Drive (%s)...", folderID))
		driveFolders, driveFiles, err := scanGDriveTree(token, folderID)
		if err != nil {
			// If 401 Unauthorized or expired token, attempt automatic token refresh if refreshToken is available
			errStr := err.Error()
			if (strings.Contains(errStr, "401") || strings.Contains(errStr, "Invalid Credentials")) && refreshToken != "" {
				appendGDriveLog("  ⚠️ Access Token đã hết hạn! Đang tự động cấp lại qua Refresh Token...")
				newToken, refErr := refreshGDriveToken()
				if refErr == nil {
					token = newToken
					driveFolders, driveFiles, err = scanGDriveTree(token, folderID)
				}
			}
		}

		if err != nil {
			appendGDriveLog(fmt.Sprintf("  ❌ LỖI truy vấn Google Drive: %v", err))
			continue
		}
		appendGDriveLog(fmt.Sprintf("  ☁️ Google Drive: %d thư mục con, %d tệp.", len(driveFolders)-1, len(driveFiles)))

		// 2. Recursive scan of local directory
		localDirs, localFiles, err := scanLocalTree(localDir)
		if err != nil {
			appendGDriveLog(fmt.Sprintf("  ❌ LỖI đọc thư mục cục bộ '%s': %v", localDir, err))
			continue
		}
		appendGDriveLog(fmt.Sprintf("  💻 Thư mục cục bộ: %d thư mục con, %d tệp.", len(localDirs), len(localFiles)))

		folderSynced := 0

		// Action A: Files on Google Drive but not local -> Download
		if direction == "two_way" || direction == "download_only" {
			for relPath, dFile := range driveFiles {
				if _, exists := localFiles[relPath]; !exists {
					destPath := filepath.Join(localDir, filepath.FromSlash(relPath))
					_ = os.MkdirAll(filepath.Dir(destPath), 0755)

					appendGDriveLog(fmt.Sprintf("  ⬇️ [DOWNLOAD] Tải tệp mới: %s (%d bytes)", relPath, dFile.Size))
					if err := downloadGDriveFile(token, dFile.ID, destPath, dFile.ModifiedTime); err != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi tải '%s': %v", relPath, err))
					} else {
						folderSynced++
						appendGDriveLog(fmt.Sprintf("    ✅ Đã tải về thành công: %s", relPath))
					}
				}
			}
		}

		// Action B: Files locally but not on Google Drive -> Upload
		if direction == "two_way" || direction == "upload_only" {
			for relPath, lFile := range localFiles {
				if _, exists := driveFiles[relPath]; !exists {
					targetFolderID, err := ensureGDriveFolderPath(token, folderID, lFile.ParentRelDir, driveFolders)
					if err != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi tạo thư mục đám mây cho '%s': %v", relPath, err))
						continue
					}

					appendGDriveLog(fmt.Sprintf("  ⬆️ [UPLOAD] Tải tệp mới lên Drive: %s (%d bytes)", relPath, lFile.Size))
					if err := uploadGDriveMultipart(token, targetFolderID, lFile.FullPath, lFile.Name); err != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi tải lên '%s': %v", relPath, err))
					} else {
						folderSynced++
						appendGDriveLog(fmt.Sprintf("    ✅ Đã tải lên Google Drive thành công: %s", relPath))
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

				diff := lFile.ModTime.Sub(dFile.ModifiedTime)
				if lFile.Size == dFile.Size && diff >= -3*time.Second && diff <= 3*time.Second {
					continue
				}

				lHash, _ := getFileMD5(lFile.FullPath)
				if dFile.MD5Checksum != "" && lHash != "" && strings.EqualFold(dFile.MD5Checksum, lHash) {
					_ = os.Chtimes(lFile.FullPath, dFile.ModifiedTime, dFile.ModifiedTime)
					continue
				}

				if diff > 3*time.Second {
					appendGDriveLog(fmt.Sprintf("  ⬆️ [UPDATE DRIVE] Tệp cục bộ mới hơn: %s", relPath))
					if err := patchGDriveFile(token, dFile.ID, lFile.FullPath); err != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi cập nhật lên Drive '%s': %v", relPath, err))
					} else {
						folderSynced++
						appendGDriveLog(fmt.Sprintf("    ✅ Đã cập nhật lên Drive: %s", relPath))
					}
				} else if diff < -3*time.Second {
					appendGDriveLog(fmt.Sprintf("  ⬇️ [UPDATE LOCAL] Tệp Drive mới hơn: %s", relPath))
					destPath := lFile.FullPath
					if err := downloadGDriveFile(token, dFile.ID, destPath, dFile.ModifiedTime); err != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi cập nhật về máy '%s': %v", relPath, err))
					} else {
						folderSynced++
						appendGDriveLog(fmt.Sprintf("    ✅ Đã cập nhật về máy: %s", relPath))
					}
				}
			}
		}

		totalSynced += folderSynced
	}

	appendGDriveLog(fmt.Sprintf("🏁 [HOÀN TẤT] Đồng bộ xong %d cặp thư mục. Tổng số tệp xử lý/cập nhật: %d tệp.", len(activeFolders), totalSynced))
	finishGDriveSync("success", totalSynced)
}

func finishGDriveSync(status string, syncedCount int) {
	gdriveConfigLock.Lock()
	currentGDriveCfg.IsSyncing = false
	currentGDriveCfg.LastStatus = status
	currentGDriveCfg.LastSyncTime = getSystemTimeNow().Format("2006-01-02 15:04:05")
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
			return nil, nil, err
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
			Enabled         *bool                  `json:"enabled"`
			LocalDir        *string                `json:"local_dir"`
			DriveURL        *string                `json:"drive_url"`
			DriveFolderID   *string                `json:"drive_folder_id"`
			Folders         *[]GDriveFolderMapping `json:"folders"`
			AccessToken     *string                `json:"access_token"`
			RefreshToken    *string                `json:"refresh_token"`
			ClientID        *string                `json:"client_id"`
			ClientSecret    *string                `json:"client_secret"`
			APIKey          *string                `json:"api_key"`
			IntervalMinutes *int                   `json:"interval_minutes"`
			Direction       *string                `json:"direction"`
		}

		if err := json.NewDecoder(io.LimitReader(r.Body, 32768)).Decode(&incoming); err != nil {
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
		if incoming.Folders != nil {
			var cleanFolders []GDriveFolderMapping
			for idx, f := range *incoming.Folders {
				f.LocalDir = strings.TrimSpace(f.LocalDir)
				f.DriveURL = strings.TrimSpace(f.DriveURL)
				f.DriveFolderID = strings.TrimSpace(f.DriveFolderID)
				if f.DriveFolderID == "" && f.DriveURL != "" {
					f.DriveFolderID = extractGDriveFolderID(f.DriveURL)
				}
				if f.ID == "" {
					f.ID = fmt.Sprintf("folder-%d-%d", time.Now().Unix(), idx+1)
				}
				cleanFolders = append(cleanFolders, f)
			}
			currentGDriveCfg.Folders = cleanFolders

			// Update primary LocalDir and DriveURL for backwards compatibility
			if len(cleanFolders) > 0 {
				currentGDriveCfg.LocalDir = cleanFolders[0].LocalDir
				currentGDriveCfg.DriveURL = cleanFolders[0].DriveURL
				currentGDriveCfg.DriveFolderID = cleanFolders[0].DriveFolderID
			}
		}
		if incoming.AccessToken != nil {
			currentGDriveCfg.AccessToken = strings.TrimSpace(*incoming.AccessToken)
		}
		if incoming.RefreshToken != nil {
			currentGDriveCfg.RefreshToken = strings.TrimSpace(*incoming.RefreshToken)
		}
		if incoming.ClientID != nil {
			currentGDriveCfg.ClientID = strings.TrimSpace(*incoming.ClientID)
		}
		if incoming.ClientSecret != nil {
			currentGDriveCfg.ClientSecret = strings.TrimSpace(*incoming.ClientSecret)
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

func handleGDriveSyncRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "Method not allowed")
		return
	}

	var payload struct {
		RefreshToken string `json:"refresh_token"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&payload)
	}

	if payload.RefreshToken != "" {
		gdriveConfigLock.Lock()
		currentGDriveCfg.RefreshToken = strings.TrimSpace(payload.RefreshToken)
		if payload.ClientID != "" {
			currentGDriveCfg.ClientID = strings.TrimSpace(payload.ClientID)
		}
		if payload.ClientSecret != "" {
			currentGDriveCfg.ClientSecret = strings.TrimSpace(payload.ClientSecret)
		}
		_ = saveGDriveConfigToFileLocked()
		gdriveConfigLock.Unlock()
	}

	newToken, err := refreshGDriveToken()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, "Lỗi làm mới Access Token: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"message":      "Đã làm mới Access Token thành công!",
		"access_token": newToken,
	})
}

func handleGDriveSyncClearLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "Method not allowed")
		return
	}

	gdriveConfigLock.Lock()
	gdriveLogHistory = []string{}
	currentGDriveCfg.LastLog = []string{}
	_ = saveGDriveConfigToFileLocked()
	gdriveConfigLock.Unlock()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"message": "Đã xóa toàn bộ nhật ký đồng bộ thành công!",
	})
}
