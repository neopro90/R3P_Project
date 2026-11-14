package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
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
	AccessToken        string                `json:"access_token"`
	RefreshToken       string                `json:"refresh_token"`
	ClientID           string                `json:"client_id"`
	ClientSecret       string                `json:"client_secret"`
	APIKey             string                `json:"api_key"`
	TokenExpiresAt     int64                 `json:"token_expires_at,omitempty"`
	TokenLastRefreshed string                `json:"token_last_refreshed,omitempty"`
	TokenAuthMode      string                `json:"token_auth_mode,omitempty"`
	LastRefreshError   string                `json:"last_refresh_error,omitempty"`
	IntervalMinutes    int                   `json:"interval_minutes"` // 15, 30, 60, 360, 720, 1440, 0 = manual
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
	gdriveTokenMu     sync.Mutex
	currentGDriveCfg  GDriveSyncConfig
	gdriveLogHistory  []string
	systemTimeLoc     *time.Location
	systemTimeLocOnce sync.Once
	gdriveHTTPClient  = &http.Client{
		Timeout: 300 * time.Second, // 5 min timeout per request
	}
	gdriveOAuthTokenEndpoint        = "https://oauth2.googleapis.com/token"
	gdrivePlaygroundRefreshEndpoint = "https://developers.google.com/oauthplayground/refreshAccessToken"
	gdriveAPIBaseURL                = "https://www.googleapis.com"
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

	gdriveConfigLock.Lock()
	defer gdriveConfigLock.Unlock()

	var loaded GDriveSyncConfig
	if settingsDBLoadJSON(bucketSettings, "gdrive_sync", path, &loaded) {
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
		currentGDriveCfg.TokenExpiresAt = loaded.TokenExpiresAt
		currentGDriveCfg.TokenLastRefreshed = loaded.TokenLastRefreshed
		currentGDriveCfg.TokenAuthMode = loaded.TokenAuthMode
		currentGDriveCfg.LastRefreshError = loaded.LastRefreshError
		if currentGDriveCfg.TokenAuthMode == "" {
			if strings.TrimSpace(currentGDriveCfg.ClientID) != "" && strings.TrimSpace(currentGDriveCfg.ClientSecret) != "" {
				currentGDriveCfg.TokenAuthMode = "custom_oauth"
			} else if strings.TrimSpace(currentGDriveCfg.RefreshToken) != "" {
				currentGDriveCfg.TokenAuthMode = "playground"
			}
		}
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
	return settingsDBSaveJSON(bucketSettings, "gdrive_sync", getGDriveConfigPath(), 0600, currentGDriveCfg)
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
	re := regexp.MustCompile(`(?:folders/|id=|/d/)([a-zA-Z0-9_-]{8,128})`)
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		return matches[1]
	}

	// If already a clean raw folder ID
	if safeGDriveFolderIDPattern.MatchString(input) {
		return input
	}

	return ""
}

type gdriveTokenRefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

func formatGDriveOAuthError(oauthErr, errDesc string, customOAuth bool) string {
	combined := strings.TrimSpace(oauthErr)
	if errDesc != "" && !strings.Contains(strings.ToLower(combined), strings.ToLower(errDesc)) {
		if combined != "" {
			combined += ": " + errDesc
		} else {
			combined = errDesc
		}
	}
	lower := strings.ToLower(combined)
	if strings.Contains(lower, "invalid_grant") {
		if !customOAuth {
			return "Refresh Token đã hết hạn hoặc bị Google thu hồi (invalid_grant). Token mặc định của OAuth Playground tự hết hạn sau 24 giờ — hãy bật 'Use your own OAuth credentials' trong Google OAuth Playground và nhập Client ID & Client Secret để giữ kết nối lâu dài."
		}
		return "Refresh Token đã hết hạn hoặc bị Google thu hồi (invalid_grant). Nếu ứng dụng Google Cloud đang ở chế độ 'Testing', token sẽ tự hết hạn sau 7 ngày — hãy chuyển app sang 'In production' hoặc tạo Refresh Token mới."
	}
	if strings.Contains(lower, "invalid_client") || strings.Contains(lower, "unauthorized_client") {
		return "Client ID hoặc Client Secret không khớp với Refresh Token (" + combined + ")."
	}
	if combined == "" {
		return "Phản hồi OAuth từ Google không hợp lệ hoặc không có Access Token"
	}
	return combined
}

func parseGDriveOAuthResponse(statusCode int, bodyBytes []byte, customOAuth bool) (res *gdriveTokenRefreshResult, permErr error, retryErr error) {
	if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return nil, nil, fmt.Errorf("HTTP %d từ Google OAuth", statusCode)
	}

	var top struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		StatusCode       int    `json:"Status-Code"`
		Success          bool   `json:"success"`
		Response         *struct {
			StatusCode  int    `json:"Status-Code"`
			MessageBody string `json:"message-body"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(bodyBytes, &top); err != nil {
		if statusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d: %s", statusCode, strings.TrimSpace(string(bodyBytes))), nil
		}
		return nil, fmt.Errorf("lỗi giải mã JSON OAuth: %v", err), nil
	}

	if top.AccessToken != "" {
		return &gdriveTokenRefreshResult{
			AccessToken:  top.AccessToken,
			RefreshToken: top.RefreshToken,
			ExpiresIn:    top.ExpiresIn,
		}, nil, nil
	}

	// Google OAuth2 Playground embeds the raw Google token response as base64 in Response.message-body
	if top.Response != nil && strings.TrimSpace(top.Response.MessageBody) != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(top.Response.MessageBody))
		if decErr != nil {
			decoded, decErr = base64.RawStdEncoding.DecodeString(strings.TrimSpace(top.Response.MessageBody))
		}
		if decErr == nil {
			var inner struct {
				AccessToken      string `json:"access_token"`
				RefreshToken     string `json:"refresh_token"`
				ExpiresIn        int64  `json:"expires_in"`
				Error            string `json:"error"`
				ErrorDescription string `json:"error_description"`
			}
			if json.Unmarshal(decoded, &inner) == nil {
				if inner.AccessToken != "" {
					return &gdriveTokenRefreshResult{
						AccessToken:  inner.AccessToken,
						RefreshToken: inner.RefreshToken,
						ExpiresIn:    inner.ExpiresIn,
					}, nil, nil
				}
				if inner.Error != "" || inner.ErrorDescription != "" {
					return nil, fmt.Errorf("%s", formatGDriveOAuthError(inner.Error, inner.ErrorDescription, customOAuth)), nil
				}
			}
		}
		if top.Response.StatusCode == 429 || top.Response.StatusCode >= 500 {
			return nil, nil, fmt.Errorf("Google OAuth tạm thời bận (HTTP %d)", top.Response.StatusCode)
		}
	}

	if top.StatusCode == 429 || top.StatusCode >= 500 {
		return nil, nil, fmt.Errorf("Google Playground tạm thời bận (Status-Code %d)", top.StatusCode)
	}
	if top.Error != "" || top.ErrorDescription != "" {
		return nil, fmt.Errorf("%s", formatGDriveOAuthError(top.Error, top.ErrorDescription, customOAuth)), nil
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d từ Google OAuth", statusCode), nil
	}
	return nil, nil, fmt.Errorf("không nhận được Access Token từ Google OAuth")
}

func doGDriveTokenRequest(authClient *http.Client, refreshToken, clientID, clientSecret string) (*gdriveTokenRefreshResult, error) {
	customOAuth := clientID != "" && clientSecret != ""
	var lastErr error

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		var req *http.Request
		var err error
		if customOAuth {
			form := url.Values{}
			form.Set("grant_type", "refresh_token")
			form.Set("refresh_token", refreshToken)
			form.Set("client_id", clientID)
			form.Set("client_secret", clientSecret)
			req, err = http.NewRequest("POST", gdriveOAuthTokenEndpoint, strings.NewReader(form.Encode()))
			if err == nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Accept", "application/json")
			}
		} else {
			payload, _ := json.Marshal(map[string]string{
				"token_uri":     gdriveOAuthTokenEndpoint,
				"refresh_token": refreshToken,
			})
			req, err = http.NewRequest("POST", gdrivePlaygroundRefreshEndpoint, bytes.NewReader(payload))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				req.Header.Set("Origin", "https://developers.google.com")
				req.Header.Set("Referer", "https://developers.google.com/oauthplayground/")
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
			}
		}
		if err != nil {
			return nil, err
		}

		resp, doErr := authClient.Do(req)
		if doErr != nil {
			lastErr = doErr
			continue
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()

		parsed, permErr, retryErr := parseGDriveOAuthResponse(resp.StatusCode, bodyBytes, customOAuth)
		if parsed != nil {
			return parsed, nil
		}
		if permErr != nil {
			return nil, permErr
		}
		lastErr = retryErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("không thể kết nối tới dịch vụ làm mới Google OAuth")
	}
	return nil, lastErr
}

// refreshGDriveToken requests a new access token from Google OAuth2 using the refresh token.
func refreshGDriveToken() (string, error) {
	return refreshGDriveTokenWithOverride("", true, nil, nil, nil)
}

func refreshGDriveTokenWithPrevious(previousFailedToken string) (string, error) {
	return refreshGDriveTokenWithOverride(previousFailedToken, false, nil, nil, nil)
}

func refreshGDriveTokenWithOverride(previousFailedToken string, force bool, overrideRefresh, overrideClientID, overrideClientSecret *string) (string, error) {
	gdriveTokenMu.Lock()
	defer gdriveTokenMu.Unlock()

	hasRefreshOverride := false
	var nextRefreshOverride string
	if overrideRefresh != nil {
		if v := strings.TrimSpace(*overrideRefresh); !strings.HasPrefix(v, "••••") {
			nextRefreshOverride = v
			hasRefreshOverride = true
		}
	}
	hasClientIDOverride := false
	var nextClientIDOverride string
	if overrideClientID != nil {
		if v := strings.TrimSpace(*overrideClientID); !strings.HasPrefix(v, "••••") {
			nextClientIDOverride = v
			hasClientIDOverride = true
		}
	}
	hasClientSecretOverride := false
	var nextClientSecretOverride string
	if overrideClientSecret != nil {
		if v := strings.TrimSpace(*overrideClientSecret); !strings.HasPrefix(v, "••••") {
			nextClientSecretOverride = v
			hasClientSecretOverride = true
		}
	}

	gdriveConfigLock.RLock()
	if !force && !hasRefreshOverride && !hasClientIDOverride && !hasClientSecretOverride &&
		currentGDriveCfg.AccessToken != "" &&
		currentGDriveCfg.AccessToken != previousFailedToken &&
		currentGDriveCfg.TokenExpiresAt > 0 &&
		time.Now().Unix() < currentGDriveCfg.TokenExpiresAt-60 {
		tok := currentGDriveCfg.AccessToken
		gdriveConfigLock.RUnlock()
		return tok, nil
	}
	refreshToken := strings.TrimSpace(currentGDriveCfg.RefreshToken)
	clientID := strings.TrimSpace(currentGDriveCfg.ClientID)
	clientSecret := strings.TrimSpace(currentGDriveCfg.ClientSecret)
	gdriveConfigLock.RUnlock()

	if hasRefreshOverride {
		refreshToken = nextRefreshOverride
	}
	if hasClientIDOverride {
		clientID = nextClientIDOverride
	}
	if hasClientSecretOverride {
		clientSecret = nextClientSecretOverride
	}

	authMode := "playground"
	if clientID != "" && clientSecret != "" {
		authMode = "custom_oauth"
	}

	if refreshToken == "" {
		err := fmt.Errorf("chưa cấu hình Refresh Token")
		gdriveConfigLock.Lock()
		currentGDriveCfg.TokenAuthMode = authMode
		currentGDriveCfg.LastRefreshError = err.Error()
		_ = saveGDriveConfigToFileLocked()
		gdriveConfigLock.Unlock()
		return "", err
	}

	authClient := &http.Client{Timeout: 15 * time.Second}
	res, err := doGDriveTokenRequest(authClient, refreshToken, clientID, clientSecret)
	if err != nil {
		gdriveConfigLock.Lock()
		currentGDriveCfg.TokenAuthMode = authMode
		currentGDriveCfg.LastRefreshError = err.Error()
		_ = saveGDriveConfigToFileLocked()
		gdriveConfigLock.Unlock()
		appendGDriveLog("❌ [TOKEN REFRESH] " + err.Error())
		return "", err
	}

	expiresIn := res.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3599
	}

	gdriveConfigLock.Lock()
	currentGDriveCfg.AccessToken = res.AccessToken
	if hasRefreshOverride {
		currentGDriveCfg.RefreshToken = refreshToken
	}
	if strings.TrimSpace(res.RefreshToken) != "" {
		currentGDriveCfg.RefreshToken = strings.TrimSpace(res.RefreshToken)
	}
	if hasClientIDOverride {
		currentGDriveCfg.ClientID = clientID
	}
	if hasClientSecretOverride {
		currentGDriveCfg.ClientSecret = clientSecret
	}
	currentGDriveCfg.TokenExpiresAt = time.Now().Unix() + expiresIn
	currentGDriveCfg.TokenLastRefreshed = getSystemTimeNow().Format("2006-01-02 15:04:05")
	currentGDriveCfg.TokenAuthMode = authMode
	currentGDriveCfg.LastRefreshError = ""
	_ = saveGDriveConfigToFileLocked()
	gdriveConfigLock.Unlock()

	srcLabel := "Google Playground"
	if authMode == "custom_oauth" {
		srcLabel = "Custom Google OAuth2"
	}
	appendGDriveLog(fmt.Sprintf("🔄 [TOKEN REFRESH] Đã cấp mới Access Token thành công qua %s (hiệu lực ~%d phút).", srcLabel, expiresIn/60))
	return res.AccessToken, nil
}

func isGDriveAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 401") || strings.Contains(s, "Invalid Credentials") || strings.Contains(s, "UNAUTHENTICATED")
}

func retryOnGDriveAuth(token *string, refreshToken string, op func(tok string) error) error {
	err := op(*token)
	if err == nil || refreshToken == "" || !isGDriveAuthError(err) {
		return err
	}
	appendGDriveLog("  ⚠️ Access Token đã hết hạn trong phiên đồng bộ! Đang tự động cấp lại...")
	newToken, refErr := refreshGDriveTokenWithPrevious(*token)
	if refErr != nil {
		return fmt.Errorf("%w (refresh thất bại: %v)", err, refErr)
	}
	*token = newToken
	return op(*token)
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
			now := getSystemTimeNow()
			lastTime, err := time.ParseInLocation("2006-01-02 15:04:05", lastTimeStr, now.Location())
			if err != nil || now.Sub(lastTime) >= time.Duration(interval)*time.Minute {
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
	tokenExpiresAt := currentGDriveCfg.TokenExpiresAt
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

	// Proactively refresh token if empty OR within 2 minutes of known expiry
	needPreRefresh := token == "" || (tokenExpiresAt > 0 && time.Now().Unix() >= tokenExpiresAt-120)
	if needPreRefresh && refreshToken != "" {
		if token == "" {
			appendGDriveLog("🔑 Access Token trống, đang dùng Refresh Token để lấy token mới...")
		} else {
			appendGDriveLog("🔑 Access Token sắp hết hạn, chủ động gia hạn trước khi đồng bộ...")
		}
		newToken, err := refreshGDriveTokenWithPrevious(token)
		if err != nil {
			if token == "" {
				appendGDriveLog("❌ LỖI gia hạn Token: " + err.Error())
				finishGDriveSync("error", 0)
				return
			}
			appendGDriveLog("⚠️ Gia hạn sớm không thành công, tiếp tục thử với token hiện tại: " + err.Error())
		} else {
			token = newToken
		}
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

		// 1. Recursive scan of Google Drive folder hierarchy via BFS (auto-refreshes 401 once)
		appendGDriveLog(fmt.Sprintf("  🔍 Quét cây thư mục Google Drive (%s)...", folderID))
		var driveFolders map[string]string
		var driveFiles map[string]gdriveSyncFile
		err := retryOnGDriveAuth(&token, refreshToken, func(tok string) error {
			var scanErr error
			driveFolders, driveFiles, scanErr = scanGDriveTree(tok, folderID)
			return scanErr
		})

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
					destPath, ok := safeJoinUnderBase(localDir, relPath)
					if !ok {
						appendGDriveLog(fmt.Sprintf("    ⚠️ Bỏ qua đường dẫn Drive không an toàn: %s", relPath))
						continue
					}
					_ = os.MkdirAll(filepath.Dir(destPath), 0755)

					appendGDriveLog(fmt.Sprintf("  ⬇️ [DOWNLOAD] Tải tệp mới: %s (%d bytes)", relPath, dFile.Size))
					dlErr := retryOnGDriveAuth(&token, refreshToken, func(tok string) error {
						return downloadGDriveFile(tok, dFile.ID, destPath, dFile.ModifiedTime)
					})
					if dlErr != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi tải '%s': %v", relPath, dlErr))
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
					targetFolderID, err := ensureGDriveFolderPath(&token, refreshToken, folderID, lFile.ParentRelDir, driveFolders)
					if err != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi tạo thư mục đám mây cho '%s': %v", relPath, err))
						continue
					}

					appendGDriveLog(fmt.Sprintf("  ⬆️ [UPLOAD] Tải tệp mới lên Drive: %s (%d bytes)", relPath, lFile.Size))
					upErr := retryOnGDriveAuth(&token, refreshToken, func(tok string) error {
						return uploadGDriveMultipart(tok, targetFolderID, lFile.FullPath, lFile.Name)
					})
					if upErr != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi tải lên '%s': %v", relPath, upErr))
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
					patchErr := retryOnGDriveAuth(&token, refreshToken, func(tok string) error {
						return patchGDriveFile(tok, dFile.ID, lFile.FullPath)
					})
					if patchErr != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi cập nhật lên Drive '%s': %v", relPath, patchErr))
					} else {
						folderSynced++
						appendGDriveLog(fmt.Sprintf("    ✅ Đã cập nhật lên Drive: %s", relPath))
					}
				} else if diff < -3*time.Second {
					appendGDriveLog(fmt.Sprintf("  ⬇️ [UPDATE LOCAL] Tệp Drive mới hơn: %s", relPath))
					destPath := lFile.FullPath
					dlErr := retryOnGDriveAuth(&token, refreshToken, func(tok string) error {
						return downloadGDriveFile(tok, dFile.ID, destPath, dFile.ModifiedTime)
					})
					if dlErr != nil {
						appendGDriveLog(fmt.Sprintf("    ❌ Lỗi cập nhật về máy '%s': %v", relPath, dlErr))
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
	st, err := os.Lstat(filePath)
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return "", fmt.Errorf("từ chối đọc symlink hoặc file không hợp lệ: %s", filepath.Base(filePath))
	}
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
			safeName, nameOK := sanitizeGDrivePathSegment(item.Name)
			if !nameOK {
				continue
			}
			var itemRelPath string
			if curr.relPath == "" {
				itemRelPath = safeName
			} else {
				itemRelPath = curr.relPath + "/" + safeName
			}

			if item.MimeType == "application/vnd.google-apps.folder" {
				driveFolders[itemRelPath] = item.ID
				queue = append(queue, queueItem{
					folderID: item.ID,
					relPath:  itemRelPath,
				})
			} else if strings.HasPrefix(item.MimeType, "application/vnd.google-apps.") {
				// Native Google Docs/Sheets/Slides cannot be downloaded with alt=media
				continue
			} else {
				sz, _ := strconv.ParseInt(item.Size, 10, 64)
				driveFiles[itemRelPath] = gdriveSyncFile{
					ID:           item.ID,
					Name:         safeName,
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

func sanitizeGDrivePathSegment(name string) (string, bool) {
	cleaned := strings.TrimSpace(strings.ReplaceAll(name, "\x00", ""))
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "..") {
		return "", false
	}
	return cleaned, true
}

func safeJoinUnderBase(baseDir, relPath string) (string, bool) {
	cleanBase := filepath.Clean(baseDir)
	joined := filepath.Clean(filepath.Join(cleanBase, filepath.FromSlash(relPath)))
	rel, err := filepath.Rel(cleanBase, joined)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return joined, true
}

func queryGDriveFolderPage(token, folderID, pageToken string) (*gdriveFileListResponse, error) {
	apiURL := fmt.Sprintf(
		"%s/drive/v3/files?q='%s'+in+parents+and+trashed=false&fields=nextPageToken,files(id,name,mimeType,size,md5Checksum,modifiedTime)&pageSize=1000",
		strings.TrimRight(gdriveAPIBaseURL, "/"),
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&listResp); err != nil {
		return nil, err
	}
	return &listResp, nil
}

// queryGDriveFolderItems queries all direct children of folderID
func queryGDriveFolderItems(token, folderID string) ([]gdriveRawItem, error) {
	var results []gdriveRawItem
	pageToken := ""

	for {
		listResp, err := queryGDriveFolderPage(token, folderID, pageToken)
		if err != nil {
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
		if info.Mode()&os.ModeSymlink != 0 {
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
func ensureGDriveFolderPath(token *string, refreshToken, rootFolderID, relPath string, driveFolders map[string]string) (string, error) {
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
			var newID string
			err := retryOnGDriveAuth(token, refreshToken, func(tok string) error {
				var createErr error
				newID, createErr = createGDriveFolder(tok, currParentID, seg)
				return createErr
			})
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
	req, err := http.NewRequest("POST", strings.TrimRight(gdriveAPIBaseURL, "/")+"/drive/v3/files", bytes.NewReader(reqBody))
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
	apiURL := fmt.Sprintf("%s/drive/v3/files/%s?alt=media", strings.TrimRight(gdriveAPIBaseURL, "/"), url.PathEscape(fileID))
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
	fileInfo, err := os.Lstat(localPath)
	if err != nil {
		return err
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("từ chối tải lên symlink hoặc file không hợp lệ: %s", fileName)
	}

	pr, pw := io.Pipe()
	defer pr.Close()
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

	uploadURL := strings.TrimRight(gdriveAPIBaseURL, "/") + "/upload/drive/v3/files?uploadType=multipart"
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
	st, err := os.Lstat(localPath)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return fmt.Errorf("từ chối tải lên symlink hoặc file không hợp lệ: %s", filepath.Base(localPath))
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	uploadURL := fmt.Sprintf("%s/upload/drive/v3/files/%s?uploadType=media", strings.TrimRight(gdriveAPIBaseURL, "/"), url.PathEscape(fileID))
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

func validateGDriveLocalDir(raw string) (string, error) {
	dir := strings.TrimSpace(raw)
	if dir == "" || strings.ContainsAny(dir, "\\:\x00") || !strings.HasPrefix(dir, "/") {
		return "", fmt.Errorf("Thư mục cục bộ phải bắt đầu bằng /mnt/ hoặc /overlay/gdrive")
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("Đường dẫn thư mục cục bộ không hợp lệ")
	}
	allowed := (clean != "/mnt" && strings.HasPrefix(clean, "/mnt/")) ||
		clean == "/overlay/gdrive" || strings.HasPrefix(clean, "/overlay/gdrive/")
	if !allowed {
		return "", fmt.Errorf("Thư mục đồng bộ cục bộ chỉ cho phép trong /mnt/<tên> hoặc /overlay/gdrive")
	}
	for cur := clean; cur != "/" && cur != "."; cur = filepath.ToSlash(filepath.Dir(cur)) {
		if st, err := os.Lstat(cur); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Không cho phép đồng bộ qua symbolic link (%s)", cur)
		}
	}
	return clean, nil
}

func maskGDriveSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}

var safeGDriveFolderIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,128}$`)

func isValidGDriveDirection(d string) bool {
	return d == "two_way" || d == "upload_only" || d == "download_only"
}

func publicGDriveConfig(cfg GDriveSyncConfig) GDriveSyncConfig {
	out := cfg
	if cfg.Folders != nil {
		out.Folders = make([]GDriveFolderMapping, len(cfg.Folders))
		copy(out.Folders, cfg.Folders)
	}
	if cfg.LastLog != nil {
		out.LastLog = make([]string, len(cfg.LastLog))
		copy(out.LastLog, cfg.LastLog)
	}
	out.AccessToken = maskGDriveSecret(cfg.AccessToken)
	out.RefreshToken = maskGDriveSecret(cfg.RefreshToken)
	out.ClientSecret = maskGDriveSecret(cfg.ClientSecret)
	out.APIKey = maskGDriveSecret(cfg.APIKey)
	return out
}

func handleGDriveSync(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gdriveConfigLock.RLock()
		cfg := currentGDriveCfg
		gdriveConfigLock.RUnlock()
		writeJSON(w, http.StatusOK, true, publicGDriveConfig(cfg))

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

		if incoming.Direction != nil {
			dirVal := strings.TrimSpace(*incoming.Direction)
			if dirVal != "" && !isValidGDriveDirection(dirVal) {
				writeJSON(w, http.StatusBadRequest, false, "Hướng đồng bộ không hợp lệ (chỉ hỗ trợ two_way, upload_only, download_only)")
				return
			}
		}

		if incoming.DriveFolderID != nil {
			fid := strings.TrimSpace(*incoming.DriveFolderID)
			if fid != "" && !safeGDriveFolderIDPattern.MatchString(fid) {
				writeJSON(w, http.StatusBadRequest, false, "Google Drive Folder ID không đúng định dạng")
				return
			}
		}

		if incoming.Folders != nil && len(*incoming.Folders) > 32 {
			writeJSON(w, http.StatusBadRequest, false, "Tối đa 32 cặp thư mục đồng bộ Google Drive")
			return
		}

		gdriveConfigLock.Lock()
		next := currentGDriveCfg
		if incoming.Enabled != nil {
			next.Enabled = *incoming.Enabled
		}
		if incoming.LocalDir != nil && *incoming.LocalDir != "" {
			cleanLocal, dirErr := validateGDriveLocalDir(*incoming.LocalDir)
			if dirErr != nil {
				gdriveConfigLock.Unlock()
				writeJSON(w, http.StatusBadRequest, false, dirErr.Error())
				return
			}
			next.LocalDir = cleanLocal
		}
		if incoming.DriveURL != nil {
			next.DriveURL = strings.TrimSpace(*incoming.DriveURL)
			extractedID := extractGDriveFolderID(next.DriveURL)
			if extractedID != "" {
				if !safeGDriveFolderIDPattern.MatchString(extractedID) {
					gdriveConfigLock.Unlock()
					writeJSON(w, http.StatusBadRequest, false, "URL Google Drive không chứa Folder ID hợp lệ")
					return
				}
				next.DriveFolderID = extractedID
			} else if next.DriveURL == "" && (incoming.DriveFolderID == nil || strings.TrimSpace(*incoming.DriveFolderID) == "") {
				next.DriveFolderID = ""
			}
		}
		if incoming.DriveFolderID != nil {
			fid := strings.TrimSpace(*incoming.DriveFolderID)
			if fid != "" {
				next.DriveFolderID = fid
			} else if incoming.DriveURL == nil || strings.TrimSpace(*incoming.DriveURL) == "" {
				next.DriveFolderID = ""
			}
		}
		if incoming.Folders != nil {
			cleanFolders := make([]GDriveFolderMapping, 0, len(*incoming.Folders))
			for idx, f := range *incoming.Folders {
				cleanLocal, dirErr := validateGDriveLocalDir(f.LocalDir)
				if dirErr != nil {
					gdriveConfigLock.Unlock()
					writeJSON(w, http.StatusBadRequest, false, dirErr.Error())
					return
				}
				f.LocalDir = cleanLocal
				f.DriveURL = strings.TrimSpace(f.DriveURL)
				f.DriveFolderID = strings.TrimSpace(f.DriveFolderID)
				if f.DriveFolderID == "" && f.DriveURL != "" {
					f.DriveFolderID = extractGDriveFolderID(f.DriveURL)
				}
				if f.DriveFolderID != "" && !safeGDriveFolderIDPattern.MatchString(f.DriveFolderID) {
					gdriveConfigLock.Unlock()
					writeJSON(w, http.StatusBadRequest, false, "Folder ID trong danh sách thư mục đồng bộ không hợp lệ")
					return
				}
				if f.ID == "" {
					f.ID = fmt.Sprintf("folder-%d-%d", time.Now().Unix(), idx+1)
				}
				cleanFolders = append(cleanFolders, f)
			}
			next.Folders = cleanFolders

			// Update primary LocalDir and DriveURL for backwards compatibility
			if len(cleanFolders) > 0 {
				next.LocalDir = cleanFolders[0].LocalDir
				next.DriveURL = cleanFolders[0].DriveURL
				next.DriveFolderID = cleanFolders[0].DriveFolderID
			}
		}
		if incoming.AccessToken != nil {
			if v := strings.TrimSpace(*incoming.AccessToken); !strings.HasPrefix(v, "••••") {
				next.AccessToken = v
				next.TokenExpiresAt = 0
				next.LastRefreshError = ""
			}
		}
		if incoming.RefreshToken != nil {
			if v := strings.TrimSpace(*incoming.RefreshToken); !strings.HasPrefix(v, "••••") {
				next.RefreshToken = v
				next.LastRefreshError = ""
			}
		}
		if incoming.ClientID != nil {
			v := strings.TrimSpace(*incoming.ClientID)
			if !strings.HasPrefix(v, "••••") {
				next.ClientID = v
				next.LastRefreshError = ""
			}
		}
		if incoming.ClientSecret != nil {
			if v := strings.TrimSpace(*incoming.ClientSecret); !strings.HasPrefix(v, "••••") {
				next.ClientSecret = v
				next.LastRefreshError = ""
			}
		}
		if strings.TrimSpace(next.ClientID) != "" && strings.TrimSpace(next.ClientSecret) != "" {
			next.TokenAuthMode = "custom_oauth"
		} else if strings.TrimSpace(next.RefreshToken) != "" {
			next.TokenAuthMode = "playground"
		} else {
			next.TokenAuthMode = ""
		}
		if incoming.APIKey != nil {
			if v := strings.TrimSpace(*incoming.APIKey); !strings.HasPrefix(v, "••••") {
				next.APIKey = v
			}
		}
		if incoming.IntervalMinutes != nil && *incoming.IntervalMinutes >= 0 {
			next.IntervalMinutes = *incoming.IntervalMinutes
		}
		if incoming.Direction != nil && strings.TrimSpace(*incoming.Direction) != "" {
			next.Direction = strings.TrimSpace(*incoming.Direction)
		}
		currentGDriveCfg = next
		cfg := publicGDriveConfig(currentGDriveCfg)
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
		RefreshToken *string `json:"refresh_token"`
		ClientID     *string `json:"client_id"`
		ClientSecret *string `json:"client_secret"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&payload)
	}

	newToken, err := refreshGDriveTokenWithOverride(
		"",
		true,
		payload.RefreshToken,
		payload.ClientID,
		payload.ClientSecret,
	)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, "Lỗi làm mới Access Token: "+err.Error())
		return
	}

	gdriveConfigLock.RLock()
	exp := currentGDriveCfg.TokenExpiresAt
	mode := currentGDriveCfg.TokenAuthMode
	lastRef := currentGDriveCfg.TokenLastRefreshed
	gdriveConfigLock.RUnlock()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"message":              "Đã làm mới Access Token thành công!",
		"access_token":         maskGDriveSecret(newToken),
		"token_expires_at":     exp,
		"token_auth_mode":      mode,
		"token_last_refreshed": lastRef,
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
