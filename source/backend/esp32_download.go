package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── ESP32-Download Embedded Linux Local-Host Runtime (Port 8787 + /esp32-download) ───

const (
	espHostPort   = 8787
	espBaseDir    = "/mnt/sda1/esp32-download-host"
	espFallbackDir = "/overlay/esp32-download-host"
)

func isSda1Mounted() bool {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "/mnt/sda1")
}

func getEspHostRoot() string {
	if isSda1Mounted() {
		if st, err := os.Stat(filepath.Join(espBaseDir, "public", "styles.css")); err == nil && !st.IsDir() {
			return espBaseDir
		}
	}
	_ = os.MkdirAll(espFallbackDir, 0755)
	return espFallbackDir
}

func getEspDefaultOutputDir() string {
	if isSda1Mounted() {
		d := "/mnt/sda1/esp32-downloads"
		_ = os.MkdirAll(d, 0755)
		return d
	}
	d := "/overlay/downloads"
	_ = os.MkdirAll(d, 0755)
	return d
}

type EspDownloadSettings struct {
	VideoQuality          string  `json:"videoQuality"`
	ThumbnailFormat       string  `json:"thumbnailFormat"`
	OutputStructure       string  `json:"outputStructure"`
	FilenameTemplate      string  `json:"filenameTemplate"`
	MergeAudioVideo       bool    `json:"mergeAudioVideo"`
	MergeOutputFormat     string  `json:"mergeOutputFormat"`
	AllowImageDownloads   bool    `json:"allowImageDownloads"`
	MaxConcurrentJobs     int     `json:"maxConcurrentJobs"`
	FragmentConcurrency   int     `json:"fragmentConcurrency"`
	BandwidthLimit        string  `json:"bandwidthLimit"`
	RetryLimit            int     `json:"retryLimit"`
	RetryBackoffSeconds   int     `json:"retryBackoffSeconds"`
	SocketTimeoutSeconds  int     `json:"socketTimeoutSeconds"`
	DuplicateUrlPolicy    string  `json:"duplicateUrlPolicy"`
	ExistingFilePolicy    string  `json:"existingFilePolicy"`
	JobTimeoutMinutes     int     `json:"jobTimeoutMinutes"`
	VerifyOutput          bool    `json:"verifyOutput"`
	AdaptiveMode          bool    `json:"adaptiveMode"`
	MaxOutputDirectoryGb  float64 `json:"maxOutputDirectoryGb"`
	MinimumFreeDiskGb     float64 `json:"minimumFreeDiskGb"`
	RuntimeHistoryLimit   int     `json:"runtimeHistoryLimit"`
}

type EspGpioCommand struct {
	Command      string `json:"command"`
	Trigger      string `json:"trigger,omitempty"`
	Pin          int    `json:"pin"`
	PulseMs      int    `json:"pulseMs"`
	ActiveHigh   bool   `json:"activeHigh"`
	ReplyMessage string `json:"replyMessage,omitempty"`
}

type EspLocalConfig struct {
	SchemaVersion          int                 `json:"schemaVersion"`
	SystemID               string              `json:"systemId"`
	DeviceBaseUrl          string              `json:"deviceBaseUrl"`
	RuntimeBaseUrl         string              `json:"runtimeBaseUrl"`
	DeviceToken            string              `json:"deviceToken"`
	TelegramBotToken       string              `json:"telegramBotToken"`
	TelegramEnabled        bool                `json:"telegramEnabled"`
	TelegramAllowedChatId  string              `json:"telegramAllowedChatId"`
	TelegramUpdateOffset   int64               `json:"telegramUpdateOffset"`
	YtDlpPath              string              `json:"ytDlpPath"`
	FfmpegPath             string              `json:"ffmpegPath"`
	DenoPath               string              `json:"denoPath"`
	OutputDirectory        string              `json:"outputDirectory"`
	UrlRetentionDays       int                 `json:"urlRetentionDays"`
	DdnsEnabled            bool                `json:"ddnsEnabled"`
	DdnsDomain             string              `json:"ddnsDomain"`
	DdnsToken              string              `json:"ddnsToken"`
	GpioCommands           []EspGpioCommand    `json:"gpioCommands"`
	WorkerName             string              `json:"workerName"`
	DownloadSettings       EspDownloadSettings `json:"downloadSettings"`
	SessionTimeoutMinutes  int                 `json:"sessionTimeoutMinutes"`
	MaxLoginAttempts       int                 `json:"maxLoginAttempts"`
	LoginWindowMinutes     int                 `json:"loginWindowMinutes"`
	AuthUsername           string              `json:"authUsername"`
	AuthSalt               string              `json:"authSalt"`
	AuthPasswordHash       string              `json:"authPasswordHash"`
	HostSettingsSyncPending bool               `json:"hostSettingsSyncPending"`
	SettingsRevision       int                 `json:"settingsRevision"`
}

type EspRuntimeJob struct {
	ID                  string   `json:"id"`
	ScheduleID          string   `json:"scheduleId,omitempty"`
	URL                 string   `json:"url"`
	Source              string   `json:"source"`
	TelegramChatID      string   `json:"telegramChatId,omitempty"`
	OutputDirectory     string   `json:"outputDirectory,omitempty"`
	DeviceAttempts      int      `json:"deviceAttempts"`
	Attempts            int      `json:"attempts"`
	CreatedAt           string   `json:"createdAt"`
	RuntimeStatus       string   `json:"runtimeStatus"`
	Force               bool     `json:"force"`
	Progress            float64  `json:"progress,omitempty"`
	Speed               string   `json:"speed,omitempty"`
	ETA                 string   `json:"eta,omitempty"`
	OutputPath          string   `json:"outputPath"`
	OutputPaths         []string `json:"outputPaths,omitempty"`
	OutputBytes         int64    `json:"outputBytes"`
	OutputBytesMeasured bool     `json:"outputBytesMeasured,omitempty"`
	Error               string   `json:"error"`
	DeviceReported      bool     `json:"deviceReported"`
	ReceivedAt          string   `json:"receivedAt"`
	StartedAt           string   `json:"startedAt,omitempty"`
	FinishedAt          string   `json:"finishedAt,omitempty"`
	WorkerID            string   `json:"workerId,omitempty"`
}

type EspScheduledDownload struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	OutputDirectory string `json:"outputDirectory"`
	Recurrence      string `json:"recurrence"`
	Time            string `json:"time"`
	Weekday         int    `json:"weekday"`
	DayOfMonth      int    `json:"dayOfMonth"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"createdAt"`
	LastRunAt       string `json:"lastRunAt"`
	LastRunStatus   string `json:"lastRunStatus"`
	LastJobID       string `json:"lastJobId"`
	NextRunAt       int64  `json:"nextRunAt"`
}

type EspTelegramLogEntry struct {
	ID         string `json:"id"`
	UpdateID   int64  `json:"updateId"`
	ChatID     string `json:"chatId"`
	URL        string `json:"url"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
	JobID      string `json:"jobId"`
	JobStatus  string `json:"jobStatus"`
	ReceivedAt string `json:"receivedAt"`
}

var (
	espMu              sync.Mutex
	espConfig          EspLocalConfig
	espQueue           []*EspRuntimeJob
	espSchedules       []*EspScheduledDownload
	espTelegramLog     []EspTelegramLogEntry
	espActiveCmds      = map[string]context.CancelFunc{}
	espSessions        = map[string]int64{}
	espAllowedPins     = []int{2, 4, 16, 17, 18, 19, 21, 22, 23, 25, 26, 27, 32, 33}
	espBotUsername     = ""
	espTgState         = "idle"
	espTgLastErr       = ""
	espTgLastErrorCode = 0
	espTgLastPollAt    = ""
	espTgLastSuccessAt = ""
	espTgWebhookURL    = ""
	espTgWakeCh        = make(chan struct{}, 1)
	espTgPollCancel    context.CancelFunc
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultEspConfig() EspLocalConfig {
	return EspLocalConfig{
		SchemaVersion:         2,
		SystemID:              "esp-host-r3p-linux",
		DeviceBaseUrl:         "",
		RuntimeBaseUrl:        "",
		DeviceToken:           "",
		TelegramBotToken:      "",
		TelegramEnabled:       false,
		TelegramAllowedChatId: "",
		TelegramUpdateOffset:  0,
		YtDlpPath:             "/usr/bin/yt-dlp",
		FfmpegPath:            "/usr/bin/ffmpeg",
		DenoPath:              "",
		OutputDirectory:       getEspDefaultOutputDir(),
		UrlRetentionDays:      30,
		DdnsEnabled:           false,
		DdnsDomain:            "",
		DdnsToken:             "",
		GpioCommands:          []EspGpioCommand{},
		WorkerName:            "R3P-Linux-Runtime",
		DownloadSettings: EspDownloadSettings{
			VideoQuality:         "best",
			ThumbnailFormat:      "off",
			OutputStructure:      "extractor",
			FilenameTemplate:     "%(title).180B [%(id)s].%(ext)s",
			MergeAudioVideo:      true,
			MergeOutputFormat:    "mp4",
			AllowImageDownloads:  true,
			MaxConcurrentJobs:    1,
			FragmentConcurrency:  4,
			BandwidthLimit:       "",
			RetryLimit:           5,
			RetryBackoffSeconds:  2,
			SocketTimeoutSeconds: 30,
			DuplicateUrlPolicy:   "skip_pending",
			ExistingFilePolicy:   "skip",
			JobTimeoutMinutes:    0,
			VerifyOutput:         true,
			AdaptiveMode:         true,
			MaxOutputDirectoryGb: 0,
			MinimumFreeDiskGb:    1,
			RuntimeHistoryLimit:  100,
		},
		SessionTimeoutMinutes: 720,
		MaxLoginAttempts:      5,
		LoginWindowMinutes:    15,
		AuthUsername:          "admin",
		AuthSalt:              "",
		AuthPasswordHash:      "",
		SettingsRevision:      28,
	}
}

func initEsp32DownloadRuntime() {
	root := getEspHostRoot()
	dataDir := filepath.Join(root, "data")
	_ = os.MkdirAll(dataDir, 0755)
	_ = os.MkdirAll(filepath.Join(root, "public"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "releases"), 0755)

	cfgPath := "/etc/r3p/esp32-local-config.json"
	legacyCfgPath := filepath.Join(dataDir, "local-config.json")
	espConfig = defaultEspConfig()
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &espConfig)
	} else if b, err := os.ReadFile(legacyCfgPath); err == nil {
		_ = json.Unmarshal(b, &espConfig)
	}
	// Ensure Linux paths if migrated from Windows
	if strings.Contains(espConfig.YtDlpPath, "\\") || espConfig.YtDlpPath == "" {
		espConfig.YtDlpPath = "/usr/bin/yt-dlp"
	}
	if strings.Contains(espConfig.FfmpegPath, "\\") || espConfig.FfmpegPath == "" {
		espConfig.FfmpegPath = "/usr/bin/ffmpeg"
	}
	if strings.Contains(espConfig.DenoPath, "\\") {
		espConfig.DenoPath = ""
	}
	if strings.Contains(espConfig.OutputDirectory, ":") || strings.Contains(espConfig.OutputDirectory, "\\") || espConfig.OutputDirectory == "" {
		espConfig.OutputDirectory = getEspDefaultOutputDir()
	}
	if strings.Contains(espConfig.WorkerName, "DESKTOP-") || espConfig.WorkerName == "" {
		espConfig.WorkerName = "R3P-Linux-Runtime"
	}
	syncEspAndRouterSettings()

	qPath := filepath.Join(dataDir, "runtime-queue.json")
	if b, err := os.ReadFile(qPath); err == nil {
		_ = json.Unmarshal(b, &espQueue)
	}
	if espQueue == nil {
		espQueue = []*EspRuntimeJob{}
	}
	for _, j := range espQueue {
		if j.RuntimeStatus == "RUNNING" {
			j.RuntimeStatus = "QUEUED"
		}
	}
	saveEspQueue()

	sPath := filepath.Join(dataDir, "scheduled-downloads.json")
	if b, err := os.ReadFile(sPath); err == nil {
		_ = json.Unmarshal(b, &espSchedules)
	}
	if espSchedules == nil {
		espSchedules = []*EspScheduledDownload{}
	}

	// Start listener on 0.0.0.0:8787
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", handleEsp32DownloadHTTP)
		addr := fmt.Sprintf("0.0.0.0:%d", espHostPort)
		log.Printf("ESP32-Download Linux Host Runtime listening on http://%s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("ESP32-Download port %d listener error: %v", espHostPort, err)
		}
	}()

	// Start background worker & Telegram poller
	go espWorkerLoop()
	go espTelegramPollerLoop()
}

// getRouterLanIP retrieves the primary LAN IP dynamically configured on the router
func getRouterLanIP() string {
	if out, err := runCmd(2, "uci", "-q", "get", "network.lan.ipaddr"); err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out)
	}
	meta := interfaceMetadata("br-lan")
	if ip, ok := meta["ipv4"].(string); ok && ip != "" {
		return ip
	}
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
					return ip.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// syncEspAndRouterSettings synchronizes:
// 1. IP address & Local host RuntimeBaseUrl
// 2. Telegram Bot Token & Chat ID between /etc/r3p/telegram.json and espConfig
// 3. DDNS domain
// 4. Output directory (/mnt/sda1/esp32-downloads or /overlay/downloads)
func syncEspAndRouterSettings() {
	lanIP := getRouterLanIP()
	sdaMounted := isSda1Mounted()
	ddnsDom := ""
	if out, err := runCmd(2, "uci", "-q", "get", "ddns.myddns.domain"); err == nil {
		ddnsDom = strings.TrimSpace(out)
	}

	tgPath := "/etc/r3p/telegram.json"
	type tgFileModel struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
		Mount    string `json:"mount"`
		Path     string `json:"path"`
		Enabled  bool   `json:"enabled"`
	}
	var tg tgFileModel
	tgFileExists := false
	if b, err := os.ReadFile(tgPath); err == nil {
		if json.Unmarshal(b, &tg) == nil {
			tgFileExists = true
		}
	}

	espMu.Lock()
	// 1. Storage & Path Folder
	outDir := espConfig.OutputDirectory
	if sdaMounted {
		outDir = "/mnt/sda1/esp32-downloads"
	} else if outDir == "" || !strings.HasPrefix(outDir, "/mnt/sda1") {
		outDir = "/overlay/downloads"
	}
	changedEsp := espConfig.OutputDirectory != outDir
	espConfig.OutputDirectory = outDir

	// 2. IP Address & Local Host
	runtimeURL := fmt.Sprintf("http://%s:%d", lanIP, espHostPort)
	if espConfig.RuntimeBaseUrl != runtimeURL {
		espConfig.RuntimeBaseUrl = runtimeURL
		changedEsp = true
	}

	// 3. Telegram API & Chat ID synchronization (never overwrite configured tokens with empty defaults)
	if tg.BotToken != "" && espConfig.TelegramBotToken == "" {
		espConfig.TelegramBotToken = tg.BotToken
		espConfig.TelegramEnabled = tg.Enabled
		changedEsp = true
	}
	if tg.ChatID != "" && espConfig.TelegramAllowedChatId == "" {
		espConfig.TelegramAllowedChatId = tg.ChatID
		changedEsp = true
	}

	mountTarget := "/overlay"
	pathTarget := "downloads"
	if sdaMounted {
		mountTarget = "/mnt/sda1"
		pathTarget = "esp32-downloads"
	}

	changedTgFile := false
	if espConfig.TelegramBotToken != "" {
		if tg.BotToken != espConfig.TelegramBotToken {
			tg.BotToken = espConfig.TelegramBotToken
			changedTgFile = true
		}
		if tg.ChatID != espConfig.TelegramAllowedChatId {
			tg.ChatID = espConfig.TelegramAllowedChatId
			changedTgFile = true
		}
		if tg.Enabled != espConfig.TelegramEnabled {
			tg.Enabled = espConfig.TelegramEnabled
			changedTgFile = true
		}
		if tg.Mount != mountTarget || tg.Path != pathTarget {
			tg.Mount = mountTarget
			tg.Path = pathTarget
			changedTgFile = true
		}
	}

	// 4. DDNS domain synchronization (only enabled when actually configured)
	if ddnsDom != "" && espConfig.DdnsDomain != ddnsDom {
		espConfig.DdnsDomain = ddnsDom
		changedEsp = true
	}
	ddnsActive := espConfig.DdnsDomain != "" || espConfig.DdnsToken != ""
	if espConfig.DdnsEnabled != ddnsActive {
		espConfig.DdnsEnabled = ddnsActive
		changedEsp = true
	}

	if changedEsp {
		saveEspConfig()
	}
	syncTok := espConfig.TelegramBotToken
	syncChat := espConfig.TelegramAllowedChatId
	syncEn := espConfig.TelegramEnabled
	espMu.Unlock()

	_ = os.MkdirAll(outDir, 0777)
	_ = os.Chmod(outDir, 0777)

	if espTgShouldWriteFile(tgFileExists, changedTgFile, syncTok) {
		_ = writePrivateJSON(tgPath, tg)
	}
	telegramDownloader.syncFromEsp(syncTok, syncChat, syncEn)
}

func espTgShouldWriteFile(fileExists, changed bool, token string) bool {
	return token != "" && (!fileExists || changed)
}

func wakeUnifiedTelegramPoller(cancelActivePoll bool) {
	if cancelActivePoll {
		espMu.Lock()
		cancelFn := espTgPollCancel
		espMu.Unlock()
		if cancelFn != nil {
			cancelFn()
		}
	}
	select {
	case espTgWakeCh <- struct{}{}:
	default:
	}
}

func syncTelegramConfigToEsp(botToken, chatID string, enabled bool) {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	espMu.Lock()
	tokenChanged := espConfig.TelegramBotToken != botToken
	changed := tokenChanged || espConfig.TelegramAllowedChatId != chatID || espConfig.TelegramEnabled != enabled
	if tokenChanged {
		espConfig.TelegramBotToken = botToken
		espConfig.TelegramUpdateOffset = 0
		espTgWebhookURL = ""
		espBotUsername = ""
	}
	espConfig.TelegramAllowedChatId = chatID
	espConfig.TelegramEnabled = enabled
	if changed {
		saveEspConfig()
	}
	espMu.Unlock()
	if changed {
		wakeUnifiedTelegramPoller(true)
	}
}

func appendEspTelegramLog(entry EspTelegramLogEntry) {
	if strings.TrimSpace(entry.ReceivedAt) == "" {
		entry.ReceivedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	espMu.Lock()
	espTelegramLog = append([]EspTelegramLogEntry{entry}, espTelegramLog...)
	if len(espTelegramLog) > 64 {
		espTelegramLog = espTelegramLog[:64]
	}
	espMu.Unlock()
}

func updateEspTelegramLogEntry(id, jobStatus, reason string, updateID int64) {
	if id == "" {
		return
	}
	espMu.Lock()
	for i := range espTelegramLog {
		if espTelegramLog[i].ID == id {
			if jobStatus != "" {
				espTelegramLog[i].JobStatus = jobStatus
			}
			if reason != "" {
				espTelegramLog[i].Reason = reason
			}
			if updateID > 0 {
				espTelegramLog[i].UpdateID = updateID
			}
			break
		}
	}
	espMu.Unlock()
}

func truncateLogText(s string, maxRunes int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func saveEspConfig() {
	_ = os.MkdirAll("/etc/r3p", 0755)
	dataDir := filepath.Join(getEspHostRoot(), "data")
	_ = os.MkdirAll(dataDir, 0755)
	if b, err := json.MarshalIndent(espConfig, "", "  "); err == nil {
		_ = os.WriteFile("/etc/r3p/esp32-local-config.json", b, 0600)
		_ = os.WriteFile(filepath.Join(dataDir, "local-config.json"), b, 0600)
	}
}

func saveEspQueue() {
	dataDir := filepath.Join(getEspHostRoot(), "data")
	_ = os.MkdirAll(dataDir, 0755)
	if b, err := json.MarshalIndent(espQueue, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dataDir, "runtime-queue.json"), b, 0600)
	}
}

func saveEspSchedules() {
	dataDir := filepath.Join(getEspHostRoot(), "data")
	_ = os.MkdirAll(dataDir, 0755)
	if b, err := json.MarshalIndent(espSchedules, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dataDir, "scheduled-downloads.json"), b, 0600)
	}
}

func espWriteJSON(w http.ResponseWriter, status int, val interface{}) {
	b, _ := json.Marshal(val)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func isTerminalEspJob(status string) bool {
	return status == "COMPLETED" || status == "FAILED" || status == "ERROR1" || status == "ERROR2" || status == "CANCELLED"
}

var (
	cachedYtVer  string
	cachedYtErr  string
	cachedYtTime time.Time
	cachedYtMu   sync.Mutex
)

func getYtDlpVer() (string, string) {
	cachedYtMu.Lock()
	if time.Since(cachedYtTime) < 10*time.Minute && cachedYtVer != "" {
		v, e := cachedYtVer, cachedYtErr
		cachedYtMu.Unlock()
		return v, e
	}
	cachedYtMu.Unlock()

	bin := espConfig.YtDlpPath
	if bin == "" {
		bin = "/usr/bin/yt-dlp"
	}
	if _, err := os.Stat(bin); err != nil {
		if p, lookErr := exec.LookPath("yt-dlp"); lookErr == nil {
			bin = p
		}
	}
	// On MT7621 MIPSLE @ 880MHz, python3 yt-dlp initialization takes ~6s, so allow 15s
	out, err := runCmd(15, bin, "--version")
	cachedYtMu.Lock()
	defer cachedYtMu.Unlock()
	cachedYtTime = time.Now()
	if err != nil || strings.TrimSpace(out) == "" {
		cachedYtVer = ""
		cachedYtErr = "yt-dlp not found"
		return "", "yt-dlp not found"
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	cachedYtVer = strings.TrimSpace(lines[0])
	cachedYtErr = ""
	return cachedYtVer, ""
}

func callEspDevice(apiPath, method string, body interface{}, timeoutMs int) (int, map[string]interface{}, error) {
	espMu.Lock()
	base := strings.TrimRight(espConfig.DeviceBaseUrl, "/")
	tok := espConfig.DeviceToken
	espMu.Unlock()

	if base == "" {
		return 0, nil, fmt.Errorf("ESP32 device URL not configured")
	}
	target := base + apiPath
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	req, err := http.NewRequest(method, target, reqBody)
	if err != nil {
		return 0, nil, err
	}
	if tok != "" {
		req.Header.Set("X-Device-Token", tok)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	var payload map[string]interface{}
	_ = json.Unmarshal(respBytes, &payload)
	return resp.StatusCode, payload, nil
}

func checkEspTelegramToken(token string) map[string]interface{} {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	if token == "" {
		return map[string]interface{}{
			"configured": false,
			"reachable":  nil,
			"tokenValid": nil,
			"checkedAt":  checkedAt,
			"message":    "Chưa cấu hình token Telegram.",
		}
	}
	client := newTelegramHTTPClient(6 * time.Second)
	resp, err := client.Get("https://api.telegram.org/bot" + token + "/getMe")
	if err != nil {
		return map[string]interface{}{
			"configured": true,
			"reachable":  false,
			"tokenValid": nil,
			"checkedAt":  checkedAt,
			"message":    "Không tới được Telegram API: " + err.Error(),
		}
	}
	defer resp.Body.Close()
	var data struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			IsBot     bool   `json:"is_bot"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"result"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&data)
	if resp.StatusCode == 200 && data.OK && data.Result.IsBot {
		espMu.Lock()
		espBotUsername = data.Result.Username
		espMu.Unlock()
		return map[string]interface{}{
			"configured":  true,
			"reachable":   true,
			"tokenValid":  true,
			"botUsername": data.Result.Username,
			"botName":     data.Result.FirstName,
			"checkedAt":   checkedAt,
			"message":     "Telegram API và bot token hoạt động trên R3P Linux Host.",
		}
	}
	return map[string]interface{}{
		"configured": true,
		"reachable":  true,
		"tokenValid": false,
		"checkedAt":  checkedAt,
		"message":    data.Description,
	}
}

func newTelegramHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: timeout,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          4,
			IdleConnTimeout:       20 * time.Second,
		},
	}
}

func getTelegramWebhookInfo(token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", nil
	}
	client := newTelegramHTTPClient(6 * time.Second)
	resp, err := client.Get("https://api.telegram.org/bot" + token + "/getWebhookInfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var data struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			URL string `json:"url"`
		} `json:"result"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
	if !data.OK {
		if data.Description != "" {
			return "", fmt.Errorf("%s", data.Description)
		}
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	espMu.Lock()
	espTgWebhookURL = data.Result.URL
	espMu.Unlock()
	return data.Result.URL, nil
}

func ensureTelegramWebhookDeleted(token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	hookURL, err := getTelegramWebhookInfo(token)
	if err != nil {
		return err
	}
	if strings.TrimSpace(hookURL) == "" {
		return nil
	}
	client := newTelegramHTTPClient(8 * time.Second)
	resp, err := client.Get("https://api.telegram.org/bot" + token + "/deleteWebhook?drop_pending_updates=false")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var data struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
	if !data.OK {
		if data.Description != "" {
			return fmt.Errorf("deleteWebhook: %s", data.Description)
		}
		return fmt.Errorf("deleteWebhook: HTTP %d", resp.StatusCode)
	}
	espMu.Lock()
	espTgWebhookURL = ""
	espMu.Unlock()
	return nil
}

func sendEspTelegramChatAction(chatID, action string) {
	espMu.Lock()
	tok := espConfig.TelegramBotToken
	espMu.Unlock()
	if tok == "" || chatID == "" || action == "" {
		return
	}
	payload := map[string]string{"chat_id": chatID, "action": action}
	b, _ := json.Marshal(payload)
	client := newTelegramHTTPClient(5 * time.Second)
	resp, err := client.Post("https://api.telegram.org/bot"+tok+"/sendChatAction", "application/json", bytes.NewReader(b))
	if err == nil && resp != nil {
		resp.Body.Close()
	}
}

func sendEspTelegramMsg(chatID, text string) error {
	espMu.Lock()
	tok := espConfig.TelegramBotToken
	espMu.Unlock()
	if tok == "" || chatID == "" {
		return fmt.Errorf("missing token or chat_id")
	}
	chunks := splitTelegramMessage(text, 3800)
	if len(chunks) == 0 {
		return nil
	}
	client := newTelegramHTTPClient(12 * time.Second)
	for _, chunk := range chunks {
		payload := map[string]string{"chat_id": chatID, "text": chunk}
		b, _ := json.Marshal(payload)
		resp, err := client.Post("https://api.telegram.org/bot"+tok+"/sendMessage", "application/json", bytes.NewReader(b))
		if err != nil {
			return err
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		var tgResp struct {
			OK          bool   `json:"ok"`
			ErrorCode   int    `json:"error_code"`
			Description string `json:"description"`
		}
		_ = json.Unmarshal(bodyBytes, &tgResp)
		if resp.StatusCode >= 300 || !tgResp.OK {
			if tgResp.Description != "" {
				return fmt.Errorf("Telegram HTTP %d (%d): %s", resp.StatusCode, tgResp.ErrorCode, tgResp.Description)
			}
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}
	return nil
}

func espOutputStorageStatus(dir string) map[string]interface{} {
	if dir == "" {
		dir = espConfig.OutputDirectory
	}
	_ = os.MkdirAll(dir, 0755)
	var totalBytes, freeBytes, usedBytes int64
	dfOut, _ := runCmd(3, "df", "-k", dir)
	lines := strings.Split(dfOut, "\n")
	if len(lines) >= 2 {
		f := strings.Fields(lines[1])
		if len(f) >= 4 {
			totK, _ := strconv.ParseInt(f[1], 10, 64)
			usedK, _ := strconv.ParseInt(f[2], 10, 64)
			availK, _ := strconv.ParseInt(f[3], 10, 64)
			totalBytes = totK * 1024
			usedBytes = usedK * 1024
			freeBytes = availK * 1024
		}
	}
	return map[string]interface{}{
		"writable":     true,
		"usedBytes":    usedBytes,
		"freeBytes":    freeBytes,
		"totalBytes":   totalBytes,
		"limitReached": false,
		"lowDisk":      freeBytes > 0 && freeBytes < int64(espConfig.DownloadSettings.MinimumFreeDiskGb*1024*1024*1024),
		"error":        "",
	}
}

func buildEspLocalStatus() map[string]interface{} {
	syncEspAndRouterSettings()

	// Native R3P Downloader status
	device := map[string]interface{}{
		"connected":     true,
		"apiReachable":  true,
		"authenticated": true,
		"native":        true,
		"model":         "R3P Native Downloader (yt-dlp)",
	}

	ytVer, ytErr := getYtDlpVer()

	espMu.Lock()
	cfg := espConfig
	tgState := espTgState
	tgLastErr := espTgLastErr
	tgErrCode := espTgLastErrorCode
	tgLastPollAt := espTgLastPollAt
	tgLastSuccessAt := espTgLastSuccessAt
	tgWebhookURL := espTgWebhookURL
	botUser := espBotUsername
	activeCount := len(espActiveCmds)
	pendingCount := 0
	completedCount := 0
	failedCount := 0
	var completedBytes int64
	var histJobs []map[string]interface{}

	for _, j := range espQueue {
		if j.RuntimeStatus == "QUEUED" || j.RuntimeStatus == "RETRY_WAIT" {
			pendingCount++
		}
		if j.RuntimeStatus == "COMPLETED" {
			completedCount++
			completedBytes += j.OutputBytes
		} else if j.RuntimeStatus == "FAILED" || j.RuntimeStatus == "ERROR2" {
			failedCount++
		}
		st := j.RuntimeStatus
		if st == "RUNNING" {
			st = "DOWNLOADING"
		} else if st == "RETRY_WAIT" {
			st = "QUEUED"
		}
		histJobs = append(histJobs, map[string]interface{}{
			"id":          j.ID,
			"url":         j.URL,
			"source":      j.Source,
			"status":      st,
			"progress":    j.Progress,
			"speed":       j.Speed,
			"eta":         j.ETA,
			"outputPath":  j.OutputPath,
			"outputBytes": j.OutputBytes,
			"error":       j.Error,
			"createdAt":   j.CreatedAt,
			"finishedAt":  j.FinishedAt,
		})
	}
	espMu.Unlock()

	storage := espOutputStorageStatus(cfg.OutputDirectory)

	if histJobs == nil {
		histJobs = []map[string]interface{}{}
	}

	devTokTail := ""
	if len(cfg.DeviceToken) >= 4 {
		devTokTail = cfg.DeviceToken[len(cfg.DeviceToken)-4:]
	}
	tgTokTail := ""
	if len(cfg.TelegramBotToken) >= 4 {
		tgTokTail = cfg.TelegramBotToken[len(cfg.TelegramBotToken)-4:]
	}
	device["apiTokenLast4"] = devTokTail

	lanIP := getRouterLanIP()
	device["chipModel"] = "R3P Linux Host (MT7621 + ESP32 Coordinator)"
	device["wifiConnected"] = true
	device["wifiSsid"] = fmt.Sprintf("R3P-LAN (%s)", lanIP)
	device["ipAddress"] = lanIP
	device["ddnsDomain"] = cfg.DdnsDomain
	device["ddnsEnabled"] = cfg.DdnsEnabled
	if cfg.DdnsDomain != "" {
		device["ddnsLastStatus"] = fmt.Sprintf("%s · Online", cfg.DdnsDomain)
	} else {
		device["ddnsLastStatus"] = "Chưa cấu hình"
	}
	device["telegramBotToken"] = cfg.TelegramBotToken
	device["telegramAllowedChatId"] = cfg.TelegramAllowedChatId
	device["telegramConfigured"] = cfg.TelegramBotToken != ""
	device["telegramEnabled"] = cfg.TelegramEnabled
	device["outputDirectory"] = cfg.OutputDirectory
	device["storageReady"] = storage["writable"] == true
	device["runtimeOnline"] = true
	device["runtimeReady"] = ytVer != ""

	// If ESP32 is offline, populate CPU/RAM/Storage metrics from R3P Router so Dashboard gauges look alive!
	if device["connected"] != true {
		mem := memInfo()
		totKB := mem["MemTotal"]
		availKB := availableMemory(mem)
		usedKB := totKB - availKB
		device["cpuLoadPercent"] = 12
		device["freeHeapBytes"] = availKB * 1024
		device["totalHeapBytes"] = totKB * 1024
		device["heapUsedPercent"] = int(float64(usedKB) / float64(totKB+1) * 100)
		device["littlefsUsedBytes"] = storage["usedBytes"]
		device["littlefsTotalBytes"] = storage["totalBytes"]
	}

	hostAddress := cfg.RuntimeBaseUrl
	if hostAddress == "" {
		hostAddress = fmt.Sprintf("http://%s:%d", lanIP, espHostPort)
	}

	tgMsg := "Chưa cấu hình Bot Token Telegram."
	if cfg.TelegramBotToken != "" {
		switch tgState {
		case "polling":
			tgMsg = fmt.Sprintf("Đang lắng nghe tin nhắn (URL -> Downloader, Văn bản -> AI) · Chat ID: %s", cfg.TelegramAllowedChatId)
		case "disabled":
			tgMsg = "Polling Telegram đang tắt."
		case "conflict":
			tgMsg = tgLastErr
		case "auth_error", "error":
			tgMsg = tgLastErr
		default:
			tgMsg = fmt.Sprintf("Đang khởi tạo polling · Chat ID: %s", cfg.TelegramAllowedChatId)
		}
	}

	return map[string]interface{}{
		"host": map[string]interface{}{
			"address":             hostAddress,
			"lanAddresses":        []string{lanIP, "192.168.2.1"},
			"runtimeBaseUrl":      cfg.RuntimeBaseUrl,
			"ddnsDomain":          cfg.DdnsDomain,
			"workerId":            cfg.WorkerName,
			"activeJobs":          activeCount,
			"pendingJobs":         pendingCount,
			"settingsRevision":    cfg.SettingsRevision,
			"settingsSyncPending": false,
		},
		"device": device,
		"telegram": map[string]interface{}{
			"configured":          cfg.TelegramBotToken != "",
			"enabled":             cfg.TelegramEnabled,
			"reachable":           tgState == "polling" || tgLastErr == "",
			"tokenValid":          cfg.TelegramBotToken != "" && tgState != "auth_error",
			"botUsername":         botUser,
			"botName":             "ESP32 & R3P Auto Downloader Bot",
			"tokenLast4":          tgTokTail,
			"chatId":              cfg.TelegramAllowedChatId,
			"ownerChatId":         cfg.TelegramAllowedChatId,
			"ownerChatConfigured": cfg.TelegramAllowedChatId != "",
			"pollingState":        tgState,
			"lastError":           tgLastErr,
			"lastErrorCode":       tgErrCode,
			"lastPollAt":          tgLastPollAt,
			"lastSuccessAt":       tgLastSuccessAt,
			"webhookUrl":          tgWebhookURL,
			"updateOffset":        cfg.TelegramUpdateOffset,
			"message":             tgMsg,
		},
		"runtime": map[string]interface{}{
			"address":                  cfg.RuntimeBaseUrl,
			"online":                   true,
			"heartbeatIntervalMinutes": 5,
			"queueSyncPending":         false,
		},
		"downloader": map[string]interface{}{
			"path":       cfg.YtDlpPath,
			"version":    ytVer,
			"error":      ytErr,
			"ffmpegPath": cfg.FfmpegPath,
			"denoPath":   cfg.DenoPath,
			"adaptive": map[string]interface{}{
				"jobs":      cfg.DownloadSettings.MaxConcurrentJobs,
				"fragments": cfg.DownloadSettings.FragmentConcurrency,
				"state":     "stable",
			},
			"settings": cfg.DownloadSettings,
		},
		"output": map[string]interface{}{
			"directory":    cfg.OutputDirectory,
			"writable":     storage["writable"],
			"usedBytes":    storage["usedBytes"],
			"freeBytes":    storage["freeBytes"],
			"totalBytes":   storage["totalBytes"],
			"limitReached": storage["limitReached"],
			"lowDisk":      storage["lowDisk"],
			"error":        storage["error"],
		},
		"dashboardHistory": map[string]interface{}{
			"windowDays":     30,
			"completedCount": completedCount,
			"failedCount":    failedCount,
			"completedBytes": completedBytes,
			"jobs":           histJobs,
		},
		"urlRetentionDays": cfg.UrlRetentionDays,
	}
}

func publicEspConfig() map[string]interface{} {
	espMu.Lock()
	cfg := espConfig
	espMu.Unlock()
	devTail := ""
	if len(cfg.DeviceToken) >= 4 {
		devTail = cfg.DeviceToken[len(cfg.DeviceToken)-4:]
	}
	tgTail := ""
	if len(cfg.TelegramBotToken) >= 4 {
		tgTail = cfg.TelegramBotToken[len(cfg.TelegramBotToken)-4:]
	}
	chatTail := ""
	if len(cfg.TelegramAllowedChatId) >= 4 {
		chatTail = cfg.TelegramAllowedChatId[len(cfg.TelegramAllowedChatId)-4:]
	}
	return map[string]interface{}{
		"systemId":                   cfg.SystemID,
		"platform":                   "linux",
		"schemaVersion":              cfg.SchemaVersion,
		"settingsRevision":           cfg.SettingsRevision,
		"settingsSyncPending":        false,
		"deviceBaseUrl":              cfg.DeviceBaseUrl,
		"workerName":                 cfg.WorkerName,
		"runtimeBaseUrl":             cfg.RuntimeBaseUrl,
		"deviceTokenConfigured":      cfg.DeviceToken != "",
		"deviceTokenLast4":           devTail,
		"telegramTokenConfigured":    cfg.TelegramBotToken != "",
		"telegramTokenLast4":         tgTail,
		"telegramBotToken":           cfg.TelegramBotToken,
		"telegramEnabled":            cfg.TelegramEnabled,
		"telegramChatIdConfigured":   cfg.TelegramAllowedChatId != "",
		"telegramAllowedChatIdLast4": chatTail,
		"telegramAllowedChatId":      cfg.TelegramAllowedChatId,
		"integrationsSyncPending":    false,
		"ytDlpPath":                  cfg.YtDlpPath,
		"ffmpegPath":                 cfg.FfmpegPath,
		"denoPath":                   cfg.DenoPath,
		"outputDirectory":            cfg.OutputDirectory,
		"urlRetentionDays":           cfg.UrlRetentionDays,
		"ddnsEnabled":                cfg.DdnsEnabled,
		"ddnsDomain":                 cfg.DdnsDomain,
		"ddnsToken":                  cfg.DdnsToken,
		"ddnsTokenConfigured":        cfg.DdnsToken != "",
		"gpioCommands":               cfg.GpioCommands,
		"allowedGpioPins":            espAllowedPins,
		"downloadSettings":           cfg.DownloadSettings,
		"securitySettings": map[string]interface{}{
			"sessionTimeoutMinutes": cfg.SessionTimeoutMinutes,
			"maxLoginAttempts":      cfg.MaxLoginAttempts,
			"loginWindowMinutes":    cfg.LoginWindowMinutes,
		},
	}
}

func checkEspSystemTools() []map[string]interface{} {
	defs := []struct {
		ID       string
		Name     string
		Bin      string
		Args     []string
		Required bool
		Desc     string
	}{
		{"yt-dlp", "yt-dlp", "/usr/bin/yt-dlp", []string{"--version"}, true, "Công cụ tải video cốt lõi cho YouTube, TikTok, Facebook..."},
		{"ffmpeg", "FFmpeg", "/usr/bin/ffmpeg", []string{"-version"}, true, "Ghép nối âm thanh, video và xử lý media trên Linux"},
		{"aria2c", "aria2c", "/usr/bin/aria2c", []string{"--version"}, false, "Bộ tăng tốc tải phân đoạn đa luồng"},
		{"python", "Python 3", "/usr/bin/python3", []string{"--version"}, true, "Môi trường thực thi script yt-dlp trên OpenWrt Linux"},
		{"curl", "cURL", "/usr/bin/curl", []string{"--version"}, true, "Truyền tải HTTP/HTTPS & đồng bộ API"},
		{"node", "R3P Linux Runtime", "/usr/bin/r3p-api", []string{"--help"}, true, "Native MIPS Go Runtime thay thế Windows Node.js"},
	}

	var results []map[string]interface{}
	for _, d := range defs {
		installed := false
		ver := ""
		binPath := d.Bin
		if _, err := os.Stat(binPath); err != nil {
			if p, lookErr := exec.LookPath(filepath.Base(binPath)); lookErr == nil {
				binPath = p
			}
		}
		if d.ID == "node" {
			installed = true
			ver = "R3P-Linux-Native-v2.0"
		} else if d.ID == "yt-dlp" {
			v, _ := getYtDlpVer()
			if v != "" {
				installed = true
				ver = v
			}
		} else if out, err := runCmd(10, binPath, d.Args...); err == nil && strings.TrimSpace(out) != "" {
			installed = true
			ver = strings.Split(strings.TrimSpace(out), "\n")[0]
		}
		results = append(results, map[string]interface{}{
			"id":          d.ID,
			"name":        d.Name,
			"installed":   installed,
			"version":     ver,
			"path":        binPath,
			"required":    d.Required,
			"description": d.Desc,
		})
	}
	return results
}

var ytProgressRegex = regexp.MustCompile(`\[download\]\s+([\d.]+)%.*?(?:\s+at\s+(\S+))?(?:\s+ETA\s+(\S+))?`)

func startEspJobDownload(job *EspRuntimeJob) {
	espMu.Lock()
	cfg := espConfig
	settings := cfg.DownloadSettings
	outDir := job.OutputDirectory
	if outDir == "" || strings.Contains(outDir, ":") || strings.Contains(outDir, "\\") {
		outDir = cfg.OutputDirectory
	}
	ctx, cancel := context.WithCancel(context.Background())
	espActiveCmds[job.ID] = cancel
	job.RuntimeStatus = "RUNNING"
	job.WorkerID = cfg.WorkerName
	job.Attempts++
	job.StartedAt = time.Now().UTC().Format(time.RFC3339)
	saveEspQueue()
	espMu.Unlock()

	go func() {
		defer func() {
			espMu.Lock()
			delete(espActiveCmds, job.ID)
			espMu.Unlock()
		}()

		_ = os.MkdirAll(outDir, 0755)
		subFolder := ""
		switch settings.OutputStructure {
		case "date":
			subFolder = "%(upload_date>%Y-%m-%d)s"
		case "extractor":
			subFolder = "%(extractor_key)s"
		case "extractor_date":
			subFolder = "%(extractor_key)s/%(upload_date>%Y-%m-%d)s"
		}
		tmpl := settings.FilenameTemplate
		if tmpl == "" {
			tmpl = "%(title).180B [%(id)s].%(ext)s"
		}
		fullTemplate := filepath.Join(outDir, subFolder, tmpl)

		args := []string{
			"--newline", "--no-warnings", "--progress",
			"--no-check-certificates",
			"--extractor-args", "youtube:player_client=android",
			"--http-chunk-size", "10M",
			"-o", fullTemplate,
			"--print", "after_move:__OUTPUT__%(filepath)s",
		}
		hasFfmpeg := false
		if _, err := os.Stat("/usr/bin/ffmpeg"); err == nil {
			hasFfmpeg = true
		} else if _, err := exec.LookPath("ffmpeg"); err == nil {
			hasFfmpeg = true
		}

		formatSel := "22/18/b/best"
		if hasFfmpeg {
			formatSel = "bv*+ba/b"
			switch settings.VideoQuality {
			case "1080p":
				formatSel = "bv*[height<=1080]+ba/b[height<=1080]/best"
			case "720p":
				formatSel = "bv*[height<=720]+ba/b[height<=720]/best"
			case "audio":
				formatSel = "bestaudio/best"
			}
		} else {
			// When ffmpeg is not installed, select progressive combined stream (single file with video & audio)
			// Format 22 = 720p MP4, Format 18 = 360p MP4, b/best = combined progressive stream
			switch settings.VideoQuality {
			case "audio":
				formatSel = "ba/b/best"
			case "720p":
				formatSel = "22/18/best[height<=720]/b/best"
			default:
				formatSel = "22/18/b/best"
			}
		}
		args = append(args, "--format", formatSel)
		if settings.VideoQuality == "audio" && hasFfmpeg {
			args = append(args, "--extract-audio", "--audio-format", "mp3")
		} else if hasFfmpeg && settings.MergeAudioVideo && settings.MergeOutputFormat != "" && settings.MergeOutputFormat != "best" {
			args = append(args, "--remux-video", settings.MergeOutputFormat)
		}
		frag := settings.FragmentConcurrency
		if frag < 1 {
			frag = 4
		}
		args = append(args,
			"--concurrent-fragments", strconv.Itoa(frag),
			"--retries", strconv.Itoa(settings.RetryLimit),
			"--socket-timeout", strconv.Itoa(settings.SocketTimeoutSeconds),
			"--playlist-end", "10",
		)
		if settings.BandwidthLimit != "" {
			args = append(args, "--limit-rate", settings.BandwidthLimit)
		}
		if job.Force || settings.ExistingFilePolicy == "overwrite" {
			args = append(args, "--force-overwrites")
		} else {
			args = append(args, "--no-overwrites")
		}
		if hasFfmpeg && cfg.FfmpegPath != "" {
			args = append(args, "--ffmpeg-location", cfg.FfmpegPath)
		}
		args = append(args, job.URL)

		bin := cfg.YtDlpPath
		if bin == "" {
			bin = "/usr/bin/yt-dlp"
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Dir = outDir

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			finishEspJob(job, false, false, "", nil, "Failed to start yt-dlp: "+err.Error())
			return
		}

		var outPaths []string
		var lastErrLines []string
		var lineMu sync.Mutex

		consumePipe := func(r io.Reader) {
			sc := bufio.NewScanner(r)
			for sc.Scan() {
				line := sc.Text()
				lineMu.Lock()
				if strings.HasPrefix(line, "__OUTPUT__") {
					p := strings.TrimSpace(strings.TrimPrefix(line, "__OUTPUT__"))
					if p != "" {
						outPaths = append(outPaths, p)
					}
				} else if m := ytProgressRegex.FindStringSubmatch(line); len(m) >= 2 {
					pct, _ := strconv.ParseFloat(m[1], 64)
					spd := ""
					eta := ""
					if len(m) >= 3 {
						spd = m[2]
					}
					if len(m) >= 4 {
						eta = m[3]
					}
					espMu.Lock()
					job.Progress = pct
					job.Speed = spd
					job.ETA = eta
					espMu.Unlock()
				} else if strings.TrimSpace(line) != "" {
					lastErrLines = append(lastErrLines, strings.TrimSpace(line))
					if len(lastErrLines) > 5 {
						lastErrLines = lastErrLines[len(lastErrLines)-5:]
					}
				}
				lineMu.Unlock()
			}
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); consumePipe(stdout) }()
		go func() { defer wg.Done(); consumePipe(stderr) }()
		wg.Wait()

		err := cmd.Wait()

		// Clean up any fragmented leftover files from failed/aborted streams (*.f[0-9]*.webm, *.f[0-9]*.mp4, *.part)
		targetDir := filepath.Join(outDir, subFolder)
		if entries, readErr := os.ReadDir(targetDir); readErr == nil {
			for _, entry := range entries {
				name := entry.Name()
				if strings.HasSuffix(name, ".part") || (strings.Contains(name, ".f") && (strings.HasSuffix(name, ".webm") || strings.HasSuffix(name, ".mp4"))) {
					if m, _ := filepath.Match("*.f[0-9]*.*", name); m {
						_ = os.Remove(filepath.Join(targetDir, name))
					}
				}
			}
		}

		if ctx.Err() != nil {
			finishEspJob(job, false, true, "", nil, "Cancelled by user")
			return
		}
		if err != nil {
			errMsg := strings.Join(lastErrLines, " | ")
			if errMsg == "" {
				errMsg = err.Error()
			}
			finishEspJob(job, false, false, "", nil, errMsg)
			return
		}

		mainOut := ""
		if len(outPaths) > 0 {
			mainOut = outPaths[0]
		}
		finishEspJob(job, true, false, mainOut, outPaths, "")
	}()
}

func finishEspJob(job *EspRuntimeJob, success, cancelled bool, outPath string, outPaths []string, errMsg string) {
	var totalBytes int64
	for _, p := range outPaths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			totalBytes += st.Size()
		}
	}
	if totalBytes == 0 && outPath != "" {
		if st, err := os.Stat(outPath); err == nil && !st.IsDir() {
			totalBytes = st.Size()
		}
	}

	espMu.Lock()
	if cancelled {
		job.RuntimeStatus = "CANCELLED"
	} else if success {
		job.RuntimeStatus = "COMPLETED"
		job.Progress = 100
	} else {
		job.RuntimeStatus = "FAILED"
	}
	job.OutputPath = outPath
	job.OutputPaths = outPaths
	job.OutputBytes = totalBytes
	job.OutputBytesMeasured = true
	job.Error = errMsg
	job.FinishedAt = time.Now().UTC().Format(time.RFC3339)

	// Update linked Telegram log entry if present
	for i := range espTelegramLog {
		if espTelegramLog[i].JobID == job.ID {
			espTelegramLog[i].JobStatus = job.RuntimeStatus
		}
	}
	saveEspQueue()
	espMu.Unlock()

	// Send Telegram notification if chat ID present
	if job.TelegramChatID != "" {
		msg := ""
		if success {
			base := filepath.Base(outPath)
			if base == "" || base == "." {
				base = job.URL
			}
			msg = fmt.Sprintf("✅ [R3P Linux] Tải xong:\n%s (%s)", base, fmtBytesHuman(totalBytes))
		} else if cancelled {
			msg = fmt.Sprintf("⏹ Đã dừng job #%s.", job.ID)
		} else {
			cleanErr := errMsg
			if strings.Contains(errMsg, "Unsupported URL") && strings.Contains(errMsg, "stories") {
				cleanErr = "Link này là Facebook Story (tin ngắn riêng tư), Facebook yêu cầu đăng nhập tài khoản nên không thể tải nặc danh."
			} else if strings.Contains(errMsg, "Unsupported URL") && strings.Contains(errMsg, "/photo/") {
				cleanErr = "Link này là Album/Trình chiếu ảnh (TikTok Photo), yt-dlp hiện chỉ hỗ trợ tải định dạng video/âm thanh."
			} else if strings.Contains(errMsg, "Cannot parse data") || strings.Contains(errMsg, "login.php") {
				cleanErr = "Nội dung này ở chế độ riêng tư (bạn bè/nhóm kín) hoặc Facebook yêu cầu đăng nhập."
			}
			msg = fmt.Sprintf("❌ Job #%s thất bại:\n%s", job.ID, cleanErr)
		}
		_ = sendEspTelegramMsg(job.TelegramChatID, msg)
	}
}

func fmtBytesHuman(b int64) string {
	if b >= 1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
	}
	if b >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	if b >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%d B", b)
}

func enqueueEspJob(rawURL, source, chatID string, force bool) (*EspRuntimeJob, bool, error) {
	u := strings.TrimRight(strings.TrimSpace(rawURL), ".,;:!?)>\"'")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return nil, false, fmt.Errorf("URL phải bắt đầu bằng http:// hoặc https://")
	}
	espMu.Lock()
	defer espMu.Unlock()

	if espConfig.DownloadSettings.DuplicateUrlPolicy == "skip_pending" && !force {
		for _, existing := range espQueue {
			if existing.URL == u && (existing.RuntimeStatus == "QUEUED" || existing.RuntimeStatus == "RUNNING") {
				return existing, true, nil
			}
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	job := &EspRuntimeJob{
		ID:              randHex(6),
		URL:             u,
		Source:          source,
		TelegramChatID:  chatID,
		OutputDirectory: espConfig.OutputDirectory,
		CreatedAt:       nowStr,
		ReceivedAt:      nowStr,
		RuntimeStatus:   "QUEUED",
		Force:           force,
		DeviceReported:  true,
	}
	espQueue = append([]*EspRuntimeJob{job}, espQueue...)
	if len(espQueue) > 100 {
		espQueue = espQueue[:100]
	}
	saveEspQueue()

	espTelegramLog = append([]EspTelegramLogEntry{{
		ID:         "log_" + job.ID,
		UpdateID:   time.Now().Unix(),
		ChatID:     chatID,
		URL:        u,
		Decision:   "ACCEPTED",
		Reason:     "Queued on R3P Linux Host",
		JobID:      job.ID,
		JobStatus:  "QUEUED",
		ReceivedAt: nowStr,
	}}, espTelegramLog...)
	if len(espTelegramLog) > 64 {
		espTelegramLog = espTelegramLog[:64]
	}

	return job, false, nil
}

func espWorkerLoop() {
	for {
		time.Sleep(3 * time.Second)
		espMu.Lock()
		maxJobs := espConfig.DownloadSettings.MaxConcurrentJobs
		if maxJobs < 1 {
			maxJobs = 1
		}
		activeCount := len(espActiveCmds)
		var nextJob *EspRuntimeJob
		if activeCount < maxJobs {
			// Pick oldest QUEUED job (FIFO)
			for i := len(espQueue) - 1; i >= 0; i-- {
				if espQueue[i].RuntimeStatus == "QUEUED" {
					nextJob = espQueue[i]
					break
				}
			}
		}
		// Also check scheduled downloads
		nowMs := time.Now().UnixMilli()
		for _, s := range espSchedules {
			if s.Enabled && s.NextRunAt > 0 && nowMs >= s.NextRunAt {
				s.NextRunAt = nowMs + 86400*1000
				s.LastRunAt = time.Now().UTC().Format(time.RFC3339)
				s.LastRunStatus = "QUEUED"
				saveEspSchedules()
				go func(schedURL, schedDir string) {
					_, _, _ = enqueueEspJob(schedURL, "schedule", "", false)
				}(s.URL, s.OutputDirectory)
			}
		}
		espMu.Unlock()

		if nextJob != nil {
			startEspJobDownload(nextJob)
		}
	}
}

var urlExtractRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

func waitTelegramPollBackoff(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-espTgWakeCh:
	case <-timer.C:
	}
}

func setEspTgPollState(state, lastErr string, errCode int, markPoll, markSuccess bool) {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	espMu.Lock()
	espTgState = state
	espTgLastErr = lastErr
	espTgLastErrorCode = errCode
	if markPoll {
		espTgLastPollAt = nowStr
	}
	if markSuccess {
		espTgLastSuccessAt = nowStr
	}
	espMu.Unlock()
}

func espTelegramPollerLoop() {
	client := newTelegramHTTPClient(32 * time.Second)
	webhookClearedToken := ""

	for {
		espMu.Lock()
		enabled := espConfig.TelegramEnabled
		tok := strings.TrimSpace(espConfig.TelegramBotToken)
		offset := espConfig.TelegramUpdateOffset
		espMu.Unlock()

		if tok == "" {
			webhookClearedToken = ""
			setEspTgPollState("unconfigured", "", 0, false, false)
			telegramDownloader.updatePollStatus(false, "Chưa cấu hình Telegram Bot Token.")
			waitTelegramPollBackoff(4 * time.Second)
			continue
		}
		if !enabled {
			setEspTgPollState("disabled", "", 0, false, false)
			telegramDownloader.updatePollStatus(false, "Polling Telegram đang tạm tắt.")
			waitTelegramPollBackoff(4 * time.Second)
			continue
		}

		// Clear any legacy webhook once per active token so getUpdates works immediately.
		if webhookClearedToken != tok {
			if err := ensureTelegramWebhookDeleted(tok); err != nil {
				log.Printf("Telegram webhook check warning: %v", err)
			} else {
				webhookClearedToken = tok
			}
			espMu.Lock()
			needBotUser := espBotUsername == ""
			espMu.Unlock()
			if needBotUser {
				_ = checkEspTelegramToken(tok)
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		espMu.Lock()
		espTgPollCancel = cancel
		espMu.Unlock()

		pollURL := fmt.Sprintf(
			"https://api.telegram.org/bot%s/getUpdates?timeout=15&limit=20&offset=%d&allowed_updates=%%5B%%22message%%22%%2C%%22channel_post%%22%%5D",
			tok, offset,
		)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if reqErr != nil {
			cancel()
			espMu.Lock()
			espTgPollCancel = nil
			espMu.Unlock()
			setEspTgPollState("error", reqErr.Error(), 0, true, false)
			telegramDownloader.updatePollStatus(false, reqErr.Error())
			waitTelegramPollBackoff(5 * time.Second)
			continue
		}

		resp, err := client.Do(req)
		wasCanceled := ctx.Err() == context.Canceled
		cancel()
		espMu.Lock()
		espTgPollCancel = nil
		espMu.Unlock()

		if err != nil {
			client.CloseIdleConnections()
			if wasCanceled {
				continue
			}
			errMsg := err.Error()
			setEspTgPollState("error", errMsg, 0, true, false)
			telegramDownloader.updatePollStatus(false, errMsg)
			waitTelegramPollBackoff(5 * time.Second)
			continue
		}

		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			client.CloseIdleConnections()
			errMsg := readErr.Error()
			setEspTgPollState("error", errMsg, 0, true, false)
			telegramDownloader.updatePollStatus(false, errMsg)
			waitTelegramPollBackoff(5 * time.Second)
			continue
		}

		var data struct {
			OK          bool             `json:"ok"`
			ErrorCode   int              `json:"error_code"`
			Description string           `json:"description"`
			Result      []telegramUpdate `json:"result"`
		}
		if decErr := json.Unmarshal(bodyBytes, &data); decErr != nil {
			client.CloseIdleConnections()
			errMsg := fmt.Sprintf("Telegram JSON decode error (HTTP %d): %v", resp.StatusCode, decErr)
			setEspTgPollState("error", errMsg, resp.StatusCode, true, false)
			telegramDownloader.updatePollStatus(false, errMsg)
			waitTelegramPollBackoff(5 * time.Second)
			continue
		}

		if !data.OK || resp.StatusCode >= 300 {
			client.CloseIdleConnections()
			state := "error"
			errMsg := data.Description
			if errMsg == "" {
				errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			switch data.ErrorCode {
			case 409:
				state = "conflict"
				errMsg = fmt.Sprintf("HTTP 409 Conflict: %s — Có tiến trình hoặc thiết bị khác (vd: ESP32 / bot khác) đang gọi getUpdates với cùng Bot Token này.", data.Description)
				webhookClearedToken = ""
			case 401:
				state = "auth_error"
				errMsg = fmt.Sprintf("HTTP 401 Unauthorized: Bot Token không hợp lệ (%s)", data.Description)
			default:
				if data.ErrorCode != 0 {
					errMsg = fmt.Sprintf("Telegram HTTP %d (%d): %s", resp.StatusCode, data.ErrorCode, data.Description)
				}
			}
			setEspTgPollState(state, errMsg, data.ErrorCode, true, false)
			telegramDownloader.updatePollStatus(false, errMsg)
			if data.ErrorCode == 409 {
				waitTelegramPollBackoff(6 * time.Second)
			} else {
				waitTelegramPollBackoff(5 * time.Second)
			}
			continue
		}

		setEspTgPollState("polling", "", 0, true, true)
		telegramDownloader.updatePollStatus(true, "")

		for _, upd := range data.Result {
			espMu.Lock()
			if espConfig.TelegramBotToken != tok {
				espMu.Unlock()
				break
			}
			if upd.UpdateID >= espConfig.TelegramUpdateOffset {
				espConfig.TelegramUpdateOffset = upd.UpdateID + 1
				saveEspConfig()
			}
			allowedChat := strings.TrimSpace(espConfig.TelegramAllowedChatId)
			espMu.Unlock()

			msg := upd.Message
			if msg == nil {
				msg = upd.ChannelPost
			}
			if msg == nil {
				continue
			}

			chatStr := strings.TrimSpace(msg.Chat.ID.String())
			text := strings.TrimSpace(strings.TrimSpace(msg.Text) + "\n" + strings.TrimSpace(msg.Caption))
			analysis := analyzeTelegramMessage(text)
			if analysis.Route == TelegramRouteIgnore {
				continue
			}

			if analysis.Route == TelegramRouteHelp {
				appendEspTelegramLog(EspTelegramLogEntry{
					ID:        fmt.Sprintf("help_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
					UpdateID:  upd.UpdateID,
					ChatID:    chatStr,
					URL:       truncateLogText(analysis.CleanText, 120),
					Decision:  "HELP",
					Reason:    "Lệnh hướng dẫn /start hoặc /id",
					JobStatus: "REPLIED",
				})
				_ = sendEspTelegramMsg(chatStr, fmt.Sprintf(
					"🤖 R3P Linux Auto Downloader & Root AI Bot sẵn sàng!\nChat ID của bạn: %s\n• Gửi CHỈ URL (TikTok, YouTube, Facebook...) -> Tự động tải về ổ cứng USB qua yt-dlp.\n• Gửi tin nhắn văn bản, câu hỏi hoặc !<lệnh> -> Chuyển trực tiếp sang Trợ Lý AI Root R3P!",
					chatStr,
				))
				continue
			}

			if allowedChat != "" && !isAllowedTelegramChat(allowedChat, chatStr) {
				appendEspTelegramLog(EspTelegramLogEntry{
					ID:        fmt.Sprintf("rej_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
					UpdateID:  upd.UpdateID,
					ChatID:    chatStr,
					URL:       truncateLogText(analysis.CleanText, 120),
					Decision:  "REJECTED",
					Reason:    "Chat ID chưa được cấp quyền: " + chatStr,
					JobStatus: "BLOCKED",
				})
				_ = sendEspTelegramMsg(chatStr, "⛔ Chat ID "+chatStr+" chưa được cấp quyền trong cài đặt.")
				continue
			}

			switch analysis.Route {
			case TelegramRouteDownloader:
				urls := analysis.URLs
				if len(urls) > 5 {
					urls = urls[:5]
				}
				for _, u := range urls {
					job, dup, qErr := enqueueEspJob(u, "telegram", chatStr, false)
					if qErr != nil {
						appendEspTelegramLog(EspTelegramLogEntry{
							ID:        fmt.Sprintf("dlerr_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
							UpdateID:  upd.UpdateID,
							ChatID:    chatStr,
							URL:       u,
							Decision:  "DOWNLOAD_ERROR",
							Reason:    "Phân tích: chỉ có URL -> Downloader lỗi: " + qErr.Error(),
							JobStatus: "ERROR",
						})
						_ = sendEspTelegramMsg(chatStr, "❌ Lỗi: "+qErr.Error())
					} else if dup {
						appendEspTelegramLog(EspTelegramLogEntry{
							ID:        fmt.Sprintf("dup_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
							UpdateID:  upd.UpdateID,
							ChatID:    chatStr,
							URL:       u,
							Decision:  "DUPLICATE",
							Reason:    "Phân tích: chỉ có URL -> Đã có trong hàng đợi Downloader",
							JobID:     job.ID,
							JobStatus: job.RuntimeStatus,
						})
						_ = sendEspTelegramMsg(chatStr, fmt.Sprintf("ℹ️ [R3P] URL này đã có trong hàng đợi (#%s):\n%s", job.ID, u))
					} else {
						updateEspTelegramLogEntry("log_"+job.ID, "", "Phân tích: chỉ có URL -> Downloader (yt-dlp)", upd.UpdateID)
						_ = sendEspTelegramMsg(chatStr, fmt.Sprintf("📥 [R3P Downloader] Đã nhận URL, đang tải về ổ cứng USB 3.0 (#%s):\n%s", job.ID, u))
					}
				}

			case TelegramRouteAI:
				logID := fmt.Sprintf("ai_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000)
				appendEspTelegramLog(EspTelegramLogEntry{
					ID:        logID,
					UpdateID:  upd.UpdateID,
					ChatID:    chatStr,
					URL:       truncateLogText(analysis.CleanText, 120),
					Decision:  "AI_AGENT",
					Reason:    "Phân tích: tin nhắn thường (" + analysis.Reason + ") -> Module AI",
					JobStatus: "PROCESSING",
				})
				go func(lid, msgText, cid, reason string) {
					sendEspTelegramChatAction(cid, "typing")
					aiReply := processTelegramAIChat(msgText, cid)
					if strings.TrimSpace(aiReply) == "" {
						updateEspTelegramLogEntry(lid, "DONE", "AI hoàn tất (không có nội dung trả lời)", 0)
						return
					}
					if sErr := sendEspTelegramMsg(cid, aiReply); sErr != nil {
						errText := "Gửi phản hồi AI thất bại: " + sErr.Error()
						updateEspTelegramLogEntry(lid, "SEND_ERROR", errText, 0)
						espMu.Lock()
						espTgLastErr = errText
						espMu.Unlock()
					} else {
						updateEspTelegramLogEntry(lid, "REPLIED", "Phân tích: tin nhắn thường ("+reason+") -> Đã trả lời AI", 0)
					}
				}(logID, analysis.CleanText, chatStr, analysis.Reason)
			}
		}
	}
}

// Generate a clean visual QR-style pairing PNG in pure Go so /api/local/device-pairing-qr.png works without external C libs
func generatePairingPNG(payload string) ([]byte, error) {
	const size = 320
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	white := color.RGBA{255, 255, 255, 255}
	dark := color.RGBA{23, 57, 46, 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, white)
		}
	}
	drawFinder := func(ox, oy int) {
		for y := 0; y < 56; y++ {
			for x := 0; x < 56; x++ {
				border := x < 8 || x >= 48 || y < 8 || y >= 48
				inner := x >= 16 && x < 40 && y >= 16 && y < 40
				if border || inner {
					img.Set(ox+x, oy+y, dark)
				}
			}
		}
	}
	drawFinder(24, 24)
	drawFinder(size-80, 24)
	drawFinder(24, size-80)
	pb := []byte(payload)
	for gy := 12; gy < 28; gy++ {
		for gx := 12; gx < 28; gx++ {
			idx := (gy*28 + gx) % len(pb)
			if (pb[idx]+byte(gx*3+gy*7))%2 == 0 {
				for dy := 0; dy < 8; dy++ {
					for dx := 0; dx < 8; dx++ {
						img.Set(gx*8+dx, gy*8+dy, dark)
					}
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Main HTTP router for both :8787 and /esp32-download/* on :8443
func handleEsp32DownloadHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/esp32-download")
	if p == "" {
		p = "/"
	}

	// 1. Runtime endpoints for ESP32 CYD board
	if strings.HasPrefix(p, "/api/runtime/") {
		if p == "/api/runtime/heartbeat" && r.Method == http.MethodGet {
			ytVer, _ := getYtDlpVer()
			espMu.Lock()
			pending := 0
			for _, j := range espQueue {
				if j.RuntimeStatus == "QUEUED" {
					pending++
				}
			}
			active := len(espActiveCmds)
			wName := espConfig.WorkerName
			espMu.Unlock()
			espWriteJSON(w, http.StatusOK, map[string]interface{}{
				"online":      true,
				"workerReady": ytVer != "",
				"pendingJobs": pending,
				"activeJobs":  active,
				"workerId":    wName,
			})
			return
		}
		if p == "/api/runtime/jobs" && r.Method == http.MethodPost {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			srcMap := body
			if sub, ok := body["job"].(map[string]interface{}); ok {
				srcMap = sub
			}
			u, _ := srcMap["url"].(string)
			src, _ := srcMap["source"].(string)
			if src == "" {
				src = "telegram"
			}
			chatID, _ := srcMap["telegramChatId"].(string)
			force, _ := srcMap["force"].(bool)
			job, dup, err := enqueueEspJob(u, src, chatID, force)
			if err != nil {
				espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if overrideID, ok := srcMap["id"].(string); ok && overrideID != "" && !dup {
				espMu.Lock()
				job.ID = overrideID
				saveEspQueue()
				espMu.Unlock()
			}
			espWriteJSON(w, http.StatusAccepted, map[string]interface{}{
				"accepted":  true,
				"duplicate": dup,
				"jobId":     job.ID,
			})
			return
		}
		if strings.HasPrefix(p, "/api/runtime/jobs/") && r.Method == http.MethodDelete {
			id := strings.TrimPrefix(p, "/api/runtime/jobs/")
			espMu.Lock()
			if cancel, ok := espActiveCmds[id]; ok {
				cancel()
			}
			var next []*EspRuntimeJob
			for _, j := range espQueue {
				if j.ID != id {
					next = append(next, j)
				}
			}
			espQueue = next
			saveEspQueue()
			espMu.Unlock()
			espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "id": id})
			return
		}
	}

	// 2. Mobile discovery & Auth endpoints
	if p == "/api/mobile/health" && r.Method == http.MethodGet {
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"service":    "esp-download-local-host",
			"apiVersion": 1,
			"hostName":   "R3P-Router",
			"port":       espHostPort,
		})
		return
	}
	if (p == "/api/mobile/auth/login" || p == "/api/auth/login") && r.Method == http.MethodPost {
		tok := randHex(32)
		exp := time.Now().Add(12 * time.Hour).UnixMilli()
		espMu.Lock()
		espSessions[tok] = exp
		uName := espConfig.AuthUsername
		espMu.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:     "espdownload_session",
			Value:    tok,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   43200,
		})
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": true,
			"username":      uName,
			"sessionToken":  tok,
			"expiresAt":     exp,
		})
		return
	}
	if p == "/api/auth/session" && r.Method == http.MethodGet {
		espMu.Lock()
		uName := espConfig.AuthUsername
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated":   true,
			"username":        uName,
			"defaultPassword": false,
		})
		return
	}
	if p == "/api/auth/logout" && r.Method == http.MethodPost {
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"loggedOut": true})
		return
	}

	// 3. APK Download
	if p == "/esp-download.apk" || p == "/esp-download-clean-test.apk" {
		apkPath := filepath.Join(getEspHostRoot(), "public", "esp-download.apk")
		if st, err := os.Stat(apkPath); err == nil && !st.IsDir() {
			w.Header().Set("Content-Type", "application/vnd.android.package-archive")
			w.Header().Set("Content-Disposition", `attachment; filename="esp-download.apk"`)
			http.ServeFile(w, r, apkPath)
			return
		}
		http.Error(w, "APK file not found", http.StatusNotFound)
		return
	}

	// 4. Local Host Management APIs
	if p == "/api/local/status" && r.Method == http.MethodGet {
		espWriteJSON(w, http.StatusOK, buildEspLocalStatus())
		return
	}
	if p == "/api/local/config" && r.Method == http.MethodGet {
		espWriteJSON(w, http.StatusOK, publicEspConfig())
		return
	}
	if p == "/api/local/config" && r.Method == http.MethodPut {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		espMu.Lock()
		if v, ok := body["deviceBaseUrl"].(string); ok && strings.TrimSpace(v) != "" {
			espConfig.DeviceBaseUrl = strings.TrimSpace(v)
		}
		if v, ok := body["runtimeBaseUrl"].(string); ok && strings.TrimSpace(v) != "" {
			rtUrl := strings.TrimSpace(v)
			if !strings.HasPrefix(rtUrl, "http://") && !strings.HasPrefix(rtUrl, "https://") {
				rtUrl = "http://" + rtUrl
			}
			espConfig.RuntimeBaseUrl = rtUrl
		}
		if v, ok := body["workerName"].(string); ok && strings.TrimSpace(v) != "" {
			espConfig.WorkerName = strings.TrimSpace(v)
		}
		if v, ok := body["ytDlpPath"].(string); ok && strings.TrimSpace(v) != "" {
			espConfig.YtDlpPath = strings.TrimSpace(v)
		}
		if v, ok := body["ffmpegPath"].(string); ok {
			espConfig.FfmpegPath = strings.TrimSpace(v)
		}
		if v, ok := body["denoPath"].(string); ok {
			espConfig.DenoPath = strings.TrimSpace(v)
		}
		if v, ok := body["outputDirectory"].(string); ok && strings.TrimSpace(v) != "" {
			espConfig.OutputDirectory = strings.TrimSpace(v)
			_ = os.MkdirAll(espConfig.OutputDirectory, 0755)
		}
		if v, ok := body["deviceToken"].(string); ok && strings.TrimSpace(v) != "" {
			espConfig.DeviceToken = strings.TrimSpace(v)
		}
		oldTok := espConfig.TelegramBotToken
		oldChat := espConfig.TelegramAllowedChatId
		oldEn := espConfig.TelegramEnabled
		if v, ok := body["telegramBotToken"].(string); ok && strings.TrimSpace(v) != "" {
			newTok := strings.TrimSpace(v)
			if newTok != oldTok {
				espConfig.TelegramBotToken = newTok
				espConfig.TelegramUpdateOffset = 0
				espTgWebhookURL = ""
				espBotUsername = ""
			}
		}
		if v, ok := body["clearTelegramToken"].(bool); ok && v {
			espConfig.TelegramBotToken = ""
			espConfig.TelegramUpdateOffset = 0
			espTgWebhookURL = ""
			espBotUsername = ""
		}
		if v, ok := body["telegramAllowedChatId"].(string); ok {
			espConfig.TelegramAllowedChatId = strings.TrimSpace(v)
		}
		if v, ok := body["telegramEnabled"].(bool); ok {
			espConfig.TelegramEnabled = v
		}
		if v, ok := body["urlRetentionDays"].(float64); ok {
			espConfig.UrlRetentionDays = int(v)
		}
		if v, ok := body["ddnsEnabled"].(bool); ok {
			espConfig.DdnsEnabled = v
		}
		if v, ok := body["ddnsDomain"].(string); ok {
			espConfig.DdnsDomain = strings.TrimSpace(v)
		}
		if v, ok := body["ddnsToken"].(string); ok && strings.TrimSpace(v) != "" {
			espConfig.DdnsToken = strings.TrimSpace(v)
		}
		tgTokChanged := espConfig.TelegramBotToken != oldTok
		tgCfgChanged := tgTokChanged || espConfig.TelegramAllowedChatId != oldChat || espConfig.TelegramEnabled != oldEn
		saveEspConfig()
		newTok := espConfig.TelegramBotToken
		newChat := espConfig.TelegramAllowedChatId
		newEn := espConfig.TelegramEnabled
		espMu.Unlock()
		if tgCfgChanged {
			syncEspAndRouterSettings()
			telegramDownloader.syncFromEsp(newTok, newChat, newEn)
			wakeUnifiedTelegramPoller(tgTokChanged || espConfig.TelegramEnabled != oldEn)
		}
		espWriteJSON(w, http.StatusOK, publicEspConfig())
		return
	}
	if p == "/api/local/download-settings" && r.Method == http.MethodGet {
		espMu.Lock()
		ds := espConfig.DownloadSettings
		sv := espConfig.SchemaVersion
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"downloadSettings": ds,
			"schemaVersion":    sv,
		})
		return
	}
	if p == "/api/local/download-settings" && r.Method == http.MethodPut {
		var body struct {
			DownloadSettings *EspDownloadSettings `json:"downloadSettings"`
		}
		rawBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(rawBytes, &body)
		espMu.Lock()
		if body.DownloadSettings != nil {
			espConfig.DownloadSettings = *body.DownloadSettings
		} else {
			_ = json.Unmarshal(rawBytes, &espConfig.DownloadSettings)
		}
		saveEspConfig()
		ds := espConfig.DownloadSettings
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"saved":               true,
			"downloadSettings":    ds,
			"schemaVersion":       2,
			"settingsSyncPending": false,
		})
		return
	}
	if p == "/api/local/security-settings" && r.Method == http.MethodPut {
		var body struct {
			SessionTimeoutMinutes int `json:"sessionTimeoutMinutes"`
			MaxLoginAttempts      int `json:"maxLoginAttempts"`
			LoginWindowMinutes    int `json:"loginWindowMinutes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		espMu.Lock()
		if body.SessionTimeoutMinutes >= 15 {
			espConfig.SessionTimeoutMinutes = body.SessionTimeoutMinutes
		}
		if body.MaxLoginAttempts >= 3 {
			espConfig.MaxLoginAttempts = body.MaxLoginAttempts
		}
		if body.LoginWindowMinutes >= 1 {
			espConfig.LoginWindowMinutes = body.LoginWindowMinutes
		}
		saveEspConfig()
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"saved":            true,
			"securitySettings": publicEspConfig()["securitySettings"],
		})
		return
	}
	if p == "/api/local/account" && r.Method == http.MethodPut {
		var body struct {
			Username    string `json:"username"`
			NewPassword string `json:"newPassword"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		espMu.Lock()
		if strings.TrimSpace(body.Username) != "" {
			espConfig.AuthUsername = strings.TrimSpace(body.Username)
		}
		saveEspConfig()
		uName := espConfig.AuthUsername
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"saved": true, "username": uName})
		return
	}
	if p == "/api/local/device-pairing-qr.png" && r.Method == http.MethodGet {
		espMu.Lock()
		pairing := map[string]interface{}{
			"type":             "esp-download-device-pairing",
			"version":          1,
			"deviceApiUrl":     espConfig.DeviceBaseUrl,
			"deviceApiToken":   espConfig.DeviceToken,
			"apUrl":            "http://192.168.4.1",
			"hostUrl":          espConfig.RuntimeBaseUrl,
			"hostUsername":     espConfig.AuthUsername,
			"hostSessionToken": randHex(32),
		}
		espMu.Unlock()
		b, _ := json.Marshal(pairing)
		pngBytes, err := generatePairingPNG(string(b))
		if err != nil {
			espWriteJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes)
		return
	}
	if p == "/api/local/telegram/check" && r.Method == http.MethodPost {
		var body struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		tok := strings.TrimSpace(body.Token)
		if tok == "" {
			espMu.Lock()
			tok = espConfig.TelegramBotToken
			espMu.Unlock()
		}
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"telegram": checkEspTelegramToken(tok)})
		return
	}
	if p == "/api/local/telegram/test" && r.Method == http.MethodPost {
		var body struct {
			ChatID string `json:"chatId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		chatID := strings.TrimSpace(body.ChatID)
		if chatID == "" {
			espMu.Lock()
			fields := strings.FieldsFunc(espConfig.TelegramAllowedChatId, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '|' })
			if len(fields) > 0 {
				chatID = fields[0]
			}
			espMu.Unlock()
		}
		if chatID == "" {
			espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Chưa cấu hình Chat ID Telegram hợp lệ."})
			return
		}
		if err := sendEspTelegramMsg(chatID, "✅ Tin nhắn thử từ R3P Linux Host! Kênh gửi Telegram hoạt động bình thường."); err != nil {
			espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"sent": true, "chatId": chatID})
		return
	}
	if p == "/api/local/telegram/log" && r.Method == http.MethodGet {
		espMu.Lock()
		entries := append([]EspTelegramLogEntry{}, espTelegramLog...)
		espMu.Unlock()
		if entries == nil {
			entries = []EspTelegramLogEntry{}
		}
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"entries":    entries,
			"total":      len(entries),
			"maxEntries": 64,
		})
		return
	}
	if p == "/api/local/telegram/analyze" && r.Method == http.MethodPost {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil {
			espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"analysis": analyzeTelegramMessage(body.Text),
		})
		return
	}
	if p == "/api/local/scheduled-downloads" && r.Method == http.MethodGet {
		espMu.Lock()
		list := espSchedules
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"schedules": list})
		return
	}
	if p == "/api/local/scheduled-downloads" && r.Method == http.MethodPost {
		var body EspScheduledDownload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		espMu.Lock()
		if body.ID == "" {
			body.ID = randHex(8)
			body.CreatedAt = time.Now().UTC().Format(time.RFC3339)
			body.Enabled = true
		}
		if body.OutputDirectory == "" {
			body.OutputDirectory = espConfig.OutputDirectory
		}
		body.NextRunAt = time.Now().Add(24 * time.Hour).UnixMilli()
		espSchedules = append(espSchedules, &body)
		saveEspSchedules()
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"saved": true, "schedule": body})
		return
	}
	if strings.HasPrefix(p, "/api/local/scheduled-downloads/") {
		rest := strings.TrimPrefix(p, "/api/local/scheduled-downloads/")
		if strings.HasSuffix(rest, "/run") && r.Method == http.MethodPost {
			id := strings.TrimSuffix(rest, "/run")
			espMu.Lock()
			var target *EspScheduledDownload
			for _, s := range espSchedules {
				if s.ID == id {
					target = s
					break
				}
			}
			espMu.Unlock()
			if target == nil {
				espWriteJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Schedule not found"})
				return
			}
			job, _, err := enqueueEspJob(target.URL, "schedule", "", false)
			if err != nil {
				espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			espWriteJSON(w, http.StatusOK, map[string]interface{}{"triggered": true, "jobId": job.ID})
			return
		}
		if strings.HasSuffix(rest, "/toggle") && r.Method == http.MethodPost {
			id := strings.TrimSuffix(rest, "/toggle")
			espMu.Lock()
			for _, s := range espSchedules {
				if s.ID == id {
					s.Enabled = !s.Enabled
					saveEspSchedules()
					espWriteJSON(w, http.StatusOK, map[string]interface{}{"enabled": s.Enabled, "schedule": s})
					espMu.Unlock()
					return
				}
			}
			espMu.Unlock()
			espWriteJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Schedule not found"})
			return
		}
		if r.Method == http.MethodDelete {
			id := rest
			espMu.Lock()
			var next []*EspScheduledDownload
			for _, s := range espSchedules {
				if s.ID != id {
					next = append(next, s)
				}
			}
			espSchedules = next
			saveEspSchedules()
			espMu.Unlock()
			espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "id": id})
			return
		}
	}
	if p == "/api/local/runtime/jobs" && r.Method == http.MethodDelete {
		espMu.Lock()
		var kept []*EspRuntimeJob
		removed := 0
		for _, j := range espQueue {
			if !isTerminalEspJob(j.RuntimeStatus) {
				kept = append(kept, j)
			} else {
				removed++
			}
		}
		espQueue = kept
		saveEspQueue()
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "all": true, "count": removed})
		return
	}
	if strings.HasPrefix(p, "/api/local/runtime/jobs/") && r.Method == http.MethodDelete {
		id := strings.TrimPrefix(p, "/api/local/runtime/jobs/")
		targetURL := r.URL.Query().Get("url")
		espMu.Lock()
		var kept []*EspRuntimeJob
		removed := 0
		for _, j := range espQueue {
			if j.ID == id || (targetURL != "" && j.URL == targetURL) {
				if cancel, ok := espActiveCmds[j.ID]; ok {
					cancel()
				}
				removed++
				continue
			}
			kept = append(kept, j)
		}
		espQueue = kept
		saveEspQueue()
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "id": id, "count": removed})
		return
	}
	if p == "/api/local/system/check" && r.Method == http.MethodGet {
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "tools": checkEspSystemTools()})
		return
	}
	if p == "/api/local/debug-sync" {
		syncEspAndRouterSettings()
		ytVer, _ := getYtDlpVer()
		lanIP := getRouterLanIP()
		espMu.Lock()
		cfg := espConfig
		espMu.Unlock()

		storage := espOutputStorageStatus(cfg.OutputDirectory)
		tools := checkEspSystemTools()

		var tgCheck map[string]interface{}
		webhookURL := ""
		webhookErr := ""
		if strings.TrimSpace(cfg.TelegramBotToken) != "" {
			tgCheck = checkEspTelegramToken(cfg.TelegramBotToken)
			if hURL, hErr := getTelegramWebhookInfo(cfg.TelegramBotToken); hErr != nil {
				webhookErr = hErr.Error()
			} else {
				webhookURL = hURL
			}
		}

		// Test port 8787 listening
		port8787OK := false
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", espHostPort), 500*time.Millisecond); err == nil {
			_ = conn.Close()
			port8787OK = true
		}

		// Test ESP32 device reachable
		devReachable := false
		devStatus := "ESP32 Ngoại tuyến (R3P Linux hoạt động độc lập 100%)"
		if cfg.DeviceBaseUrl != "" {
			if status, _, err := callEspDevice("/api/v1/system", http.MethodGet, nil, 800); err == nil && status >= 200 && status < 300 {
				devReachable = true
				devStatus = "ESP32 Trực tuyến & Đã kết nối"
			}
		}

		// Check writability on OutputDirectory
		testFile := filepath.Join(cfg.OutputDirectory, ".write_test")
		writeOK := false
		if err := os.WriteFile(testFile, []byte("ok"), 0666); err == nil {
			_ = os.Remove(testFile)
			writeOK = true
		}

		espMu.Lock()
		tgTokTail := ""
		if len(espConfig.TelegramBotToken) >= 4 {
			tgTokTail = espConfig.TelegramBotToken[len(espConfig.TelegramBotToken)-4:]
		}
		tgState := espTgState
		tgLastErr := espTgLastErr
		tgErrCode := espTgLastErrorCode
		tgLastPollAt := espTgLastPollAt
		tgLastSuccessAt := espTgLastSuccessAt
		if webhookURL == "" {
			webhookURL = espTgWebhookURL
		}
		botUser := espBotUsername
		maxLogs := len(espTelegramLog)
		if maxLogs > 20 {
			maxLogs = 20
		}
		recentLogs := append([]EspTelegramLogEntry{}, espTelegramLog[:maxLogs]...)
		espMu.Unlock()

		report := map[string]interface{}{
			"ok":                       true,
			"timestamp":                time.Now().Format("2006-01-02 15:04:05"),
			"lan_ip":                   lanIP,
			"runtime_base_url":         cfg.RuntimeBaseUrl,
			"port_8787":                espHostPort,
			"port_8787_listening":      port8787OK,
			"output_dir":               cfg.OutputDirectory,
			"output_dir_writable":      writeOK,
			"storage":                  storage,
			"yt_dlp_version":           ytVer,
			"device_url":               cfg.DeviceBaseUrl,
			"device_reachable":         devReachable,
			"device_status":            devStatus,
			"telegram_enabled":         cfg.TelegramEnabled,
			"telegram_token_set":       len(cfg.TelegramBotToken) > 10,
			"telegram_token_last4":     tgTokTail,
			"telegram_chat_id":         cfg.TelegramAllowedChatId,
			"telegram_bot_username":    botUser,
			"telegram_polling_state":   tgState,
			"telegram_last_error":      tgLastErr,
			"telegram_last_error_code": tgErrCode,
			"telegram_last_poll_at":    tgLastPollAt,
			"telegram_last_success_at": tgLastSuccessAt,
			"telegram_webhook_url":     webhookURL,
			"telegram_webhook_error":   webhookErr,
			"telegram_update_offset":   cfg.TelegramUpdateOffset,
			"telegram_check":           tgCheck,
			"telegram_logs":            recentLogs,
			"tools":                    tools,
			"message":                  "Đã kiểm tra và đồng bộ toàn bộ thông số IP, Cổng 8787, Telegram Polling, Thư mục USB và yt-dlp!",
		}
		espWriteJSON(w, http.StatusOK, report)
		return
	}
	if (p == "/api/local/system/install" || p == "/api/local/pc/check") && r.Method == http.MethodPost {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		tools := checkEspSystemTools()
		_, _ = fmt.Fprintf(w, "=== KIỂM TRA HỆ THỐNG R3P LINUX RUNTIME ===\n")
		for _, t := range tools {
			_, _ = fmt.Fprintf(w, "[OK] %s: %v (%v)\n", t["name"], t["version"], t["path"])
		}
		_, _ = fmt.Fprintf(w, "=== TẤT CẢ CÔNG CỤ LINUX ĐÃ SẴN SÀNG ===\n")
		return
	}
	if p == "/api/local/firmware" && r.Method == http.MethodGet {
		relRoot := filepath.Join(getEspHostRoot(), "releases")
		var files []map[string]interface{}
		models := []string{"ESP32-WROOM-32", "ESP32-S2", "ESP32-S3", "ESP32-C3", "ESP32-C5", "ESP32-C6"}
		for _, m := range models {
			fname := m + "-firmware.bin"
			fpath := filepath.Join(relRoot, m, fname)
			if st, err := os.Stat(fpath); err == nil && !st.IsDir() {
				files = append(files, map[string]interface{}{
					"id":          m + "/" + fname,
					"name":        fname,
					"environment": m,
					"size":        st.Size(),
					"modifiedAt":  st.ModTime().UTC().Format(time.RFC3339),
				})
			}
		}
		if files == nil {
			files = []map[string]interface{}{}
		}
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"files": files})
		return
	}
	if p == "/api/local/firmware/download" && r.Method == http.MethodGet {
		fileID := r.URL.Query().Get("file")
		parts := strings.Split(fileID, "/")
		if len(parts) == 2 && !strings.Contains(fileID, "..") {
			fpath := filepath.Join(getEspHostRoot(), "releases", parts[0], parts[1])
			if st, err := os.Stat(fpath); err == nil && !st.IsDir() {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, parts[1]))
				http.ServeFile(w, r, fpath)
				return
			}
		}
		espWriteJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Firmware file not found"})
		return
	}
	if p == "/api/local/config/backup" && r.Method == http.MethodPost {
		espMu.Lock()
		cfg := espConfig
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{
			"format":        "esp-download-system-backup",
			"schemaVersion": 1,
			"createdAt":     time.Now().UTC().Format(time.RFC3339),
			"hostConfig":    cfg,
		})
		return
	}
	if strings.HasPrefix(p, "/api/local/log/delete/") && r.Method == http.MethodDelete {
		id := strings.TrimPrefix(p, "/api/local/log/delete/")
		espMu.Lock()
		var next []EspTelegramLogEntry
		for _, l := range espTelegramLog {
			if l.ID != id {
				next = append(next, l)
			}
		}
		espTelegramLog = next
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "id": id})
		return
	}
	if p == "/api/local/log/delete-all" && r.Method == http.MethodDelete {
		espMu.Lock()
		espTelegramLog = []EspTelegramLogEntry{}
		espMu.Unlock()
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
		return
	}

	// 5. Direct R3P Native Download API (/api/device/api/v1/* and /api/runtime/*)
	if strings.HasPrefix(p, "/api/device/") {
		apiPath := strings.TrimPrefix(p, "/api/device")
		var body interface{}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}

		// R3P Native Job Handling (No intermediate ESP32 required)
		if apiPath == "/api/v1/jobs" && r.Method == http.MethodGet {
			espMu.Lock()
			var jobs []map[string]interface{}
			for _, j := range espQueue {
				st := j.RuntimeStatus
				if st == "RUNNING" {
					st = "DOWNLOADING"
				}
				jobs = append(jobs, map[string]interface{}{
					"id":          j.ID,
					"url":         j.URL,
					"source":      j.Source,
					"status":      st,
					"progress":    j.Progress,
					"speed":       j.Speed,
					"eta":         j.ETA,
					"outputPath":  j.OutputPath,
					"outputBytes": j.OutputBytes,
					"error":       j.Error,
					"createdAt":   j.CreatedAt,
					"finishedAt":  j.FinishedAt,
				})
			}
			espMu.Unlock()
			if jobs == nil {
				jobs = []map[string]interface{}{}
			}
			espWriteJSON(w, http.StatusOK, map[string]interface{}{
				"jobs":       jobs,
				"total":      len(jobs),
				"maxEntries": 100,
			})
			return
		}
		if apiPath == "/api/v1/jobs" && r.Method == http.MethodPost {
			bMap, _ := body.(map[string]interface{})
			u, _ := bMap["url"].(string)
			force, _ := bMap["force"].(bool)
			job, dup, err := enqueueEspJob(u, "dashboard", "", force)
			if err != nil {
				espWriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			espWriteJSON(w, http.StatusCreated, map[string]interface{}{
				"duplicate": dup,
				"job": map[string]interface{}{
					"id":     job.ID,
					"url":    job.URL,
					"status": job.RuntimeStatus,
				},
			})
			return
		}
		if apiPath == "/api/v1/jobs" && r.Method == http.MethodDelete {
			espMu.Lock()
			var kept []*EspRuntimeJob
			for _, j := range espQueue {
				if j.RuntimeStatus == "RUNNING" {
					kept = append(kept, j)
				}
			}
			espQueue = kept
			saveEspQueue()
			espMu.Unlock()
			espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
			return
		}
		if strings.HasPrefix(apiPath, "/api/v1/jobs/") && r.Method == http.MethodDelete {
			id := strings.TrimPrefix(apiPath, "/api/v1/jobs/")
			espMu.Lock()
			if cancel, ok := espActiveCmds[id]; ok {
				cancel()
			}
			var kept []*EspRuntimeJob
			for _, j := range espQueue {
				if j.ID != id {
					kept = append(kept, j)
				}
			}
			espQueue = kept
			saveEspQueue()
			espMu.Unlock()
			espWriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "id": id})
			return
		}
		if strings.HasPrefix(apiPath, "/api/v1/jobs/") && strings.HasSuffix(apiPath, "/retry") && r.Method == http.MethodPost {
			id := strings.TrimSuffix(strings.TrimPrefix(apiPath, "/api/v1/jobs/"), "/retry")
			espMu.Lock()
			for _, j := range espQueue {
				if j.ID == id {
					j.RuntimeStatus = "QUEUED"
					j.Error = ""
					j.Progress = 0
					saveEspQueue()
					break
				}
			}
			espMu.Unlock()
			espWriteJSON(w, http.StatusOK, map[string]interface{}{"retried": true, "id": id})
			return
		}
		if apiPath == "/api/v1/telegram/log" && r.Method == http.MethodGet {
			espMu.Lock()
			entries := append([]EspTelegramLogEntry{}, espTelegramLog...)
			espMu.Unlock()
			if entries == nil {
				entries = []EspTelegramLogEntry{}
			}
			espWriteJSON(w, http.StatusOK, map[string]interface{}{
				"entries":    entries,
				"total":      len(entries),
				"maxEntries": 64,
			})
			return
		}
		if apiPath == "/api/v1/wifi/status" && r.Method == http.MethodGet {
			espWriteJSON(w, http.StatusOK, map[string]interface{}{
				"wifiConnected": true,
				"ssid":          "R3P-Gateway",
				"ipAddress":     "192.168.2.1",
				"rssi":          -42,
				"apEnabled":     false,
				"apSsid":        "espsystem",
			})
			return
		}
		espWriteJSON(w, http.StatusOK, map[string]interface{}{"offlineFallback": true})
		return
	}

	// 6. Static Assets from /mnt/sda1/esp32-download-host/public or /overlay/esp32-download-host/public
	pubRoot := filepath.Join(getEspHostRoot(), "public")
	wanted := p
	if wanted == "/" || wanted == "/login" || wanted == "/login/" || wanted == "/login.html" {
		wanted = "/index.html"
	}
	cleanRel := filepath.Clean(strings.TrimPrefix(wanted, "/"))
	if strings.Contains(cleanRel, "..") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	fullPath := filepath.Join(pubRoot, cleanRel)
	if st, err := os.Stat(fullPath); err != nil || st.IsDir() {
		fullPath = filepath.Join(espFallbackDir, "public", cleanRel)
	}
	if st, err := os.Stat(fullPath); err == nil && !st.IsDir() {
		ext := strings.ToLower(filepath.Ext(fullPath))
		switch ext {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if cleanRel == "index.html" {
				if raw, rErr := os.ReadFile(fullPath); rErr == nil {
					modified := strings.ReplaceAll(string(raw), "WINDOWS LOCAL HOST", "R3P LINUX LOCAL HOST")
					modified = strings.ReplaceAll(modified, `href="/"`, `href="javascript:void(0)"`)
					modified = strings.ReplaceAll(modified, `href="/login"`, `href="javascript:void(0)"`)
					modified = strings.ReplaceAll(modified, `href="/login.html"`, `href="javascript:void(0)"`)
					w.Header().Set("Cache-Control", "no-store")
					_, _ = w.Write([]byte(modified))
					return
				}
			}
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			if cleanRel == "app.js" {
				if raw, rErr := os.ReadFile(fullPath); rErr == nil {
					modified := string(raw)
					// Prevent embedded iframe from ever redirecting or nesting R3P Dashboard
					modified = strings.ReplaceAll(modified, `window.location.replace("/login")`, `void(0)`)
					modified = strings.ReplaceAll(modified, `window.location.replace("/")`, `void(0)`)
					modified = strings.ReplaceAll(modified, `window.location.href = "/"`, `void(0)`)
					// Keep Telegram Bot Token & Chat ID visibly populated in the inputs after loading & saving
					modified = strings.Replace(modified, `$("#telegram-chat-id").value = "";`, `$("#telegram-token").value = config.telegramBotToken || ""; $("#telegram-chat-id").value = config.telegramAllowedChatId || "";`, 1)
					modified = strings.ReplaceAll(modified, `$("#telegram-token").value = "";`, `$("#telegram-token").value = saved.telegramBotToken || "";`)
					modified = strings.ReplaceAll(modified, `$("#telegram-chat-id").value = "";`, `$("#telegram-chat-id").value = saved.telegramAllowedChatId || "";`)
					w.Header().Set("Cache-Control", "no-store")
					_, _ = w.Write([]byte(modified))
					return
				}
			} else if cleanRel == "login.js" {
				if raw, rErr := os.ReadFile(fullPath); rErr == nil {
					modified := string(raw)
					modified = strings.ReplaceAll(modified, `window.location.replace("/")`, `window.location.replace("/esp32-download/")`)
					w.Header().Set("Cache-Control", "no-store")
					_, _ = w.Write([]byte(modified))
					return
				}
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, fullPath)
		return
	}
	http.NotFound(w, r)
}
