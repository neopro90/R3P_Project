package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// ─── R3P Native Local Downloader & Unified Telegram Bot Poller ───────────────

var (
	downloaderConfigFile    = "/etc/r3p/downloader.json"
	downloaderQueueFile     = "/etc/r3p/downloader-queue.json"
	downloaderSchedulesFile = "/etc/r3p/downloader-schedules.json"

	legacyLocalConfigFile   = "/etc/r3p/local-downloader.json"
	legacySdaDataDir        = "/mnt/sda1/download-host/data"
)

func isSda1Mounted() bool {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "/mnt/sda1")
}

func getDefaultDownloadOutputDir() string {
	if isSda1Mounted() {
		d := "/mnt/sda1/downloads"
		_ = os.MkdirAll(d, 0755)
		return d
	}
	d := "/overlay/downloads"
	_ = os.MkdirAll(d, 0755)
	return d
}

// DownloadPolicySettings controls yt-dlp download behavior on R3P.
type DownloadPolicySettings struct {
	VideoQuality         string  `json:"videoQuality"`
	ThumbnailFormat      string  `json:"thumbnailFormat"`
	OutputStructure      string  `json:"outputStructure"`
	FilenameTemplate     string  `json:"filenameTemplate"`
	MergeAudioVideo      bool    `json:"mergeAudioVideo"`
	MergeOutputFormat    string  `json:"mergeOutputFormat"`
	AllowImageDownloads  bool    `json:"allowImageDownloads"`
	MaxConcurrentJobs    int     `json:"maxConcurrentJobs"`
	FragmentConcurrency  int     `json:"fragmentConcurrency"`
	BandwidthLimit       string  `json:"bandwidthLimit"`
	RetryLimit           int     `json:"retryLimit"`
	RetryBackoffSeconds  int     `json:"retryBackoffSeconds"`
	SocketTimeoutSeconds int     `json:"socketTimeoutSeconds"`
	DuplicateUrlPolicy   string  `json:"duplicateUrlPolicy"`
	ExistingFilePolicy   string  `json:"existingFilePolicy"`
	JobTimeoutMinutes    int     `json:"jobTimeoutMinutes"`
	VerifyOutput         bool    `json:"verifyOutput"`
	AdaptiveMode         bool    `json:"adaptiveMode"`
	MaxOutputDirectoryGb float64 `json:"maxOutputDirectoryGb"`
	MinimumFreeDiskGb    float64 `json:"minimumFreeDiskGb"`
	RuntimeHistoryLimit  int     `json:"runtimeHistoryLimit"`
}

// DownloaderConfig holds persistent configuration for the local R3P downloader and Telegram bot.
type DownloaderConfig struct {
	SchemaVersion         int                    `json:"schemaVersion"`
	SystemID              string                 `json:"systemId"`
	TelegramBotToken      string                 `json:"telegramBotToken"`
	TelegramEnabled       bool                   `json:"telegramEnabled"`
	TelegramAllowedChatId string                 `json:"telegramAllowedChatId"`
	TelegramUpdateOffset  int64                  `json:"telegramUpdateOffset"`
	YtDlpPath             string                 `json:"ytDlpPath"`
	FfmpegPath            string                 `json:"ffmpegPath"`
	DenoPath              string                 `json:"denoPath"`
	OutputDirectory       string                 `json:"outputDirectory"`
	UrlRetentionDays      int                    `json:"urlRetentionDays"`
	DdnsEnabled           bool                   `json:"ddnsEnabled"`
	DdnsDomain            string                 `json:"ddnsDomain"`
	DdnsToken             string                 `json:"ddnsToken"`
	WorkerName            string                 `json:"workerName"`
	DownloadSettings      DownloadPolicySettings `json:"downloadSettings"`
	SettingsRevision      int                    `json:"settingsRevision"`
}

// DownloadJob represents a single queued, active, or completed yt-dlp download task.
type DownloadJob struct {
	ID                  string   `json:"id"`
	ScheduleID          string   `json:"scheduleId,omitempty"`
	URL                 string   `json:"url"`
	Source              string   `json:"source"`
	TelegramChatID      string   `json:"telegramChatId,omitempty"`
	OutputDirectory     string   `json:"outputDirectory,omitempty"`
	Quality             string   `json:"quality,omitempty"`
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
	ReceivedAt          string   `json:"receivedAt"`
	StartedAt           string   `json:"startedAt,omitempty"`
	FinishedAt          string   `json:"finishedAt,omitempty"`
	WorkerID            string   `json:"workerId,omitempty"`
}

// ScheduledDownload represents a recurring automated download job.
type ScheduledDownload struct {
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

// TelegramRouteLogEntry records how an incoming Telegram message was analyzed and routed.
type TelegramRouteLogEntry struct {
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
	dlMu             sync.Mutex
	dlConfig         DownloaderConfig
	dlQueue          []*DownloadJob
	dlSchedules      []*ScheduledDownload
	dlTelegramLog    []TelegramRouteLogEntry
	dlActiveCmds     = make(map[string]context.CancelFunc)
	dlTgState        = "initializing" // unconfigured | disabled | initializing | polling | conflict | auth_error | error
	dlTgLastErr      = ""
	dlTgLastErrCode  = 0
	dlTgLastPollAt   = ""
	dlTgLastSuccessAt = ""
	dlTgWebhookURL   = ""
	dlBotUsername    = ""
	dlTgPollCancel   context.CancelFunc
	dlTgWakeCh       = make(chan struct{}, 1)
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultDownloaderConfig() DownloaderConfig {
	return DownloaderConfig{
		SchemaVersion:         3,
		SystemID:              "r3p-local-downloader",
		TelegramBotToken:      "",
		TelegramEnabled:       false,
		TelegramAllowedChatId: "",
		TelegramUpdateOffset:  0,
		YtDlpPath:             "/usr/bin/yt-dlp",
		FfmpegPath:            "/usr/bin/ffmpeg",
		DenoPath:              "",
		OutputDirectory:       getDefaultDownloadOutputDir(),
		UrlRetentionDays:      30,
		DdnsEnabled:           false,
		DdnsDomain:            "",
		DdnsToken:             "",
		WorkerName:            "R3P-Local-Downloader",
		DownloadSettings: DownloadPolicySettings{
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
		SettingsRevision: 29,
	}
}

// loadOrMigrateDownloaderConfig loads /etc/r3p/downloader.json or migrates legacy config once without data loss.
func loadOrMigrateDownloaderConfig(cfgPath string, legacyPaths []string) DownloaderConfig {
	cfg := defaultDownloaderConfig()
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	} else {
		migrated := false
		for _, legacy := range legacyPaths {
			if lb, lErr := os.ReadFile(legacy); lErr == nil {
				if json.Unmarshal(lb, &cfg) == nil {
					migrated = true
					break
				}
			}
		}
		if migrated {
			cfg.SchemaVersion = 3
			cfg.SystemID = "r3p-local-downloader"
			cfg.WorkerName = "R3P-Local-Downloader"
			_ = os.MkdirAll(filepath.Dir(cfgPath), 0755)
			if out, mErr := json.MarshalIndent(cfg, "", "  "); mErr == nil {
				_ = os.WriteFile(cfgPath, out, 0600)
			}
		}
	}

	if strings.Contains(cfg.YtDlpPath, "\\") || cfg.YtDlpPath == "" {
		cfg.YtDlpPath = "/usr/bin/yt-dlp"
	}
	if strings.Contains(cfg.FfmpegPath, "\\") || cfg.FfmpegPath == "" {
		cfg.FfmpegPath = "/usr/bin/ffmpeg"
	}
	if strings.Contains(cfg.DenoPath, "\\") {
		cfg.DenoPath = ""
	}
	if strings.Contains(cfg.OutputDirectory, ":") || strings.Contains(cfg.OutputDirectory, "\\") || strings.TrimSpace(cfg.OutputDirectory) == "" {
		cfg.OutputDirectory = getDefaultDownloadOutputDir()
	}
	if strings.Contains(cfg.WorkerName, "DESKTOP-") || strings.TrimSpace(cfg.WorkerName) == "" {
		cfg.WorkerName = "R3P-Local-Downloader"
	}
	if strings.TrimSpace(cfg.SystemID) == "" || cfg.SystemID == "local-host-r3p-linux" {
		cfg.SystemID = "r3p-local-downloader"
	}
	return cfg
}

func loadJSONWithMigration(primaryPath string, legacyPaths []string, dest interface{}) bool {
	if b, err := os.ReadFile(primaryPath); err == nil {
		return json.Unmarshal(b, dest) == nil
	}
	for _, legacy := range legacyPaths {
		if lb, err := os.ReadFile(legacy); err == nil {
			if json.Unmarshal(lb, dest) == nil {
				return true
			}
		}
	}
	return false
}

func initR3pDownloadRuntime() {
	_ = os.MkdirAll("/etc/r3p", 0755)

	dlConfig = loadOrMigrateDownloaderConfig(downloaderConfigFile, []string{
		legacyLocalConfigFile,
		filepath.Join(legacySdaDataDir, "local-config.json"),
	})
	syncR3pDownloaderSettings()

	var loadedQueue []*DownloadJob
	migratedQueue := false
	if _, err := os.Stat(downloaderQueueFile); os.IsNotExist(err) {
		migratedQueue = true
	}
	loadJSONWithMigration(downloaderQueueFile, []string{
		filepath.Join(legacySdaDataDir, "runtime-queue.json"),
	}, &loadedQueue)
	if loadedQueue == nil {
		loadedQueue = []*DownloadJob{}
	}
	for _, j := range loadedQueue {
		if j.RuntimeStatus == "RUNNING" {
			j.RuntimeStatus = "QUEUED"
		}
	}
	dlMu.Lock()
	dlQueue = loadedQueue
	if migratedQueue || len(dlQueue) > 0 {
		saveDownloaderQueue()
	}
	dlMu.Unlock()

	var loadedSchedules []*ScheduledDownload
	migratedSched := false
	if _, err := os.Stat(downloaderSchedulesFile); os.IsNotExist(err) {
		migratedSched = true
	}
	loadJSONWithMigration(downloaderSchedulesFile, []string{
		filepath.Join(legacySdaDataDir, "scheduled-downloads.json"),
	}, &loadedSchedules)
	if loadedSchedules == nil {
		loadedSchedules = []*ScheduledDownload{}
	}
	dlMu.Lock()
	dlSchedules = loadedSchedules
	if migratedSched && len(dlSchedules) > 0 {
		saveDownloaderSchedules()
	}
	dlMu.Unlock()

	log.Printf("R3P Native Local Downloader initialized (output=%s)", dlConfig.OutputDirectory)

	go downloadWorkerLoop()
	go r3pTelegramPollerLoop()
}

// getRouterLanIP retrieves the primary LAN IP dynamically configured on the router.
func getRouterLanIP() string {
	if out, err := runCmd(2, "uci", "-q", "get", "network.lan.ipaddr"); err == nil {
		ip := strings.TrimSpace(out)
		if ip != "" && strings.Contains(ip, ".") {
			return ip
		}
	}
	if out, err := runCmd(2, "ip", "-4", "addr", "show", "br-lan"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "inet ") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					ip := strings.Split(fields[1], "/")[0]
					if ip != "" {
						return ip
					}
				}
			}
		}
	}
	return "192.168.2.1"
}

func resolveMountAndSubdir(dir string) (string, string) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(dir)))
	if strings.HasPrefix(cleaned, "/mnt/sda1/") {
		return "/mnt/sda1", strings.TrimPrefix(cleaned, "/mnt/sda1/")
	}
	if cleaned == "/mnt/sda1" {
		return "/mnt/sda1", ""
	}
	if strings.HasPrefix(cleaned, "/overlay/") {
		return "/overlay", strings.TrimPrefix(cleaned, "/overlay/")
	}
	return "/overlay", "downloads"
}

func syncR3pDownloaderSettings() {
	ddnsDom := ""
	if out, err := runCmd(2, "uci", "-q", "get", "ddns.myddns.domain"); err == nil {
		ddnsDom = strings.TrimSpace(out)
	}

	tgPath := strings.TrimSpace(os.Getenv("R3P_TELEGRAM_CONFIG"))
	if tgPath == "" {
		tgPath = "/etc/r3p/telegram.json"
	}
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

	dlMu.Lock()
	// 1. Preserve valid Linux OutputDirectory instead of overwriting user choices.
	outDir := strings.TrimSpace(dlConfig.OutputDirectory)
	changedCfg := false
	if outDir == "" || strings.Contains(outDir, ":") || strings.Contains(outDir, "\\") {
		outDir = getDefaultDownloadOutputDir()
		dlConfig.OutputDirectory = outDir
		changedCfg = true
	}

	// 2. Telegram API & Chat ID synchronization (never overwrite configured tokens with empty defaults).
	if tg.BotToken != "" && dlConfig.TelegramBotToken == "" {
		dlConfig.TelegramBotToken = tg.BotToken
		dlConfig.TelegramEnabled = tg.Enabled
		changedCfg = true
	}
	if tg.ChatID != "" && dlConfig.TelegramAllowedChatId == "" {
		dlConfig.TelegramAllowedChatId = tg.ChatID
		changedCfg = true
	}

	mountTarget, pathTarget := resolveMountAndSubdir(outDir)

	changedTgFile := false
	if dlConfig.TelegramBotToken != "" {
		if tg.BotToken != dlConfig.TelegramBotToken {
			tg.BotToken = dlConfig.TelegramBotToken
			changedTgFile = true
		}
		if tg.ChatID != dlConfig.TelegramAllowedChatId {
			tg.ChatID = dlConfig.TelegramAllowedChatId
			changedTgFile = true
		}
		if tg.Enabled != dlConfig.TelegramEnabled {
			tg.Enabled = dlConfig.TelegramEnabled
			changedTgFile = true
		}
		if tg.Mount != mountTarget || tg.Path != pathTarget {
			tg.Mount = mountTarget
			tg.Path = pathTarget
			changedTgFile = true
		}
	}

	// 3. DDNS domain synchronization.
	if ddnsDom != "" && dlConfig.DdnsDomain != ddnsDom {
		dlConfig.DdnsDomain = ddnsDom
		changedCfg = true
	}
	ddnsActive := dlConfig.DdnsDomain != "" || dlConfig.DdnsToken != ""
	if dlConfig.DdnsEnabled != ddnsActive {
		dlConfig.DdnsEnabled = ddnsActive
		changedCfg = true
	}

	if changedCfg {
		saveDownloaderConfig()
	}
	syncTok := dlConfig.TelegramBotToken
	syncChat := dlConfig.TelegramAllowedChatId
	syncEn := dlConfig.TelegramEnabled
	dlMu.Unlock()

	_ = os.MkdirAll(outDir, 0755)

	if shouldWriteTelegramFile(tgFileExists, changedTgFile, syncTok) {
		_ = writePrivateJSON(tgPath, tg)
	}
	telegramDownloader.syncFromDownloader(syncTok, syncChat, syncEn, mountTarget, pathTarget)
}

func shouldWriteTelegramFile(fileExists, changed bool, token string) bool {
	return token != "" && (!fileExists || changed)
}

func wakeUnifiedTelegramPoller(cancelActivePoll bool) {
	if cancelActivePoll {
		dlMu.Lock()
		cancelFn := dlTgPollCancel
		dlMu.Unlock()
		if cancelFn != nil {
			cancelFn()
		}
	}
	select {
	case dlTgWakeCh <- struct{}{}:
	default:
	}
}

func syncTelegramConfigToDownloader(botToken, chatID string, enabled bool) {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	dlMu.Lock()
	tokenChanged := dlConfig.TelegramBotToken != botToken
	changed := tokenChanged || dlConfig.TelegramAllowedChatId != chatID || dlConfig.TelegramEnabled != enabled
	if tokenChanged {
		dlConfig.TelegramBotToken = botToken
		dlConfig.TelegramUpdateOffset = 0
		dlTgWebhookURL = ""
		dlBotUsername = ""
	}
	dlConfig.TelegramAllowedChatId = chatID
	dlConfig.TelegramEnabled = enabled
	if changed {
		saveDownloaderConfig()
	}
	dlMu.Unlock()
	if changed {
		wakeUnifiedTelegramPoller(tokenChanged || !enabled)
	}
}

func appendTelegramRouteLog(entry TelegramRouteLogEntry) {
	if strings.TrimSpace(entry.ReceivedAt) == "" {
		entry.ReceivedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	dlMu.Lock()
	dlTelegramLog = append([]TelegramRouteLogEntry{entry}, dlTelegramLog...)
	if len(dlTelegramLog) > 64 {
		dlTelegramLog = dlTelegramLog[:64]
	}
	dlMu.Unlock()
}

func updateTelegramRouteLogEntry(id, jobStatus, reason string, updateID int64) {
	if id == "" {
		return
	}
	dlMu.Lock()
	for i := range dlTelegramLog {
		if dlTelegramLog[i].ID == id {
			if jobStatus != "" {
				dlTelegramLog[i].JobStatus = jobStatus
			}
			if reason != "" {
				dlTelegramLog[i].Reason = reason
			}
			if updateID > 0 {
				dlTelegramLog[i].UpdateID = updateID
			}
			break
		}
	}
	dlMu.Unlock()
}

func truncateLogText(s string, maxRunes int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func saveDownloaderConfig() {
	_ = os.MkdirAll(filepath.Dir(downloaderConfigFile), 0755)
	if b, err := json.MarshalIndent(dlConfig, "", "  "); err == nil {
		_ = os.WriteFile(downloaderConfigFile, b, 0600)
	}
}

func saveDownloaderQueue() {
	_ = os.MkdirAll(filepath.Dir(downloaderQueueFile), 0755)
	if b, err := json.MarshalIndent(dlQueue, "", "  "); err == nil {
		_ = os.WriteFile(downloaderQueueFile, b, 0600)
	}
}

func saveDownloaderSchedules() {
	_ = os.MkdirAll(filepath.Dir(downloaderSchedulesFile), 0755)
	if b, err := json.MarshalIndent(dlSchedules, "", "  "); err == nil {
		_ = os.WriteFile(downloaderSchedulesFile, b, 0600)
	}
}

func isTerminalDownloadJob(status string) bool {
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

	dlMu.Lock()
	bin := dlConfig.YtDlpPath
	dlMu.Unlock()
	if bin == "" {
		bin = "/usr/bin/yt-dlp"
	}
	if _, err := os.Stat(bin); err != nil {
		if p, lookErr := exec.LookPath("yt-dlp"); lookErr == nil {
			bin = p
		}
	}
	// On MT7621 MIPSLE @ 880MHz, python3 yt-dlp initialization takes ~6s, so allow 15s.
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

func checkTelegramToken(token string) map[string]interface{} {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	token = strings.TrimSpace(token)
	if token == "" {
		return map[string]interface{}{
			"configured": false,
			"reachable":  nil,
			"tokenValid": nil,
			"checkedAt":  checkedAt,
			"message":    "Chưa cấu hình Telegram bot token.",
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
		dlMu.Lock()
		dlBotUsername = data.Result.Username
		dlMu.Unlock()
		return map[string]interface{}{
			"configured":  true,
			"reachable":   true,
			"tokenValid":  true,
			"botUsername": data.Result.Username,
			"botName":     data.Result.FirstName,
			"checkedAt":   checkedAt,
			"message":     "Telegram API và bot token hoạt động cục bộ trên R3P.",
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
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
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
	dlMu.Lock()
	dlTgWebhookURL = data.Result.URL
	dlMu.Unlock()
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
	dlMu.Lock()
	dlTgWebhookURL = ""
	dlMu.Unlock()
	return nil
}

func sendR3pTelegramChatAction(chatID, action string) {
	dlMu.Lock()
	tok := dlConfig.TelegramBotToken
	dlMu.Unlock()
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

func sendR3pTelegramMsg(chatID, text string) error {
	dlMu.Lock()
	tok := dlConfig.TelegramBotToken
	dlMu.Unlock()
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

func downloaderOutputStorageStatus(dir string) map[string]interface{} {
	if dir == "" {
		dlMu.Lock()
		dir = dlConfig.OutputDirectory
		dlMu.Unlock()
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
	dlMu.Lock()
	minDiskGb := dlConfig.DownloadSettings.MinimumFreeDiskGb
	dlMu.Unlock()
	return map[string]interface{}{
		"writable":     true,
		"usedBytes":    usedBytes,
		"freeBytes":    freeBytes,
		"freeGb":       fmt.Sprintf("%.2f", float64(freeBytes)/(1024*1024*1024)),
		"totalBytes":   totalBytes,
		"limitReached": false,
		"lowDisk":      freeBytes > 0 && freeBytes < int64(minDiskGb*1024*1024*1024),
		"error":        "",
	}
}

func publicDownloaderConfig() map[string]interface{} {
	dlMu.Lock()
	cfg := dlConfig
	dlMu.Unlock()
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
		"workerName":                 cfg.WorkerName,
		"telegramTokenConfigured":    cfg.TelegramBotToken != "",
		"telegramTokenLast4":         tgTail,
		"telegramEnabled":            cfg.TelegramEnabled,
		"telegramChatIdConfigured":   cfg.TelegramAllowedChatId != "",
		"telegramAllowedChatIdLast4": chatTail,
		"telegramAllowedChatId":      cfg.TelegramAllowedChatId,
		"ytDlpPath":                  cfg.YtDlpPath,
		"ffmpegPath":                 cfg.FfmpegPath,
		"denoPath":                   cfg.DenoPath,
		"outputDirectory":            cfg.OutputDirectory,
		"urlRetentionDays":           cfg.UrlRetentionDays,
		"ddnsEnabled":                cfg.DdnsEnabled,
		"ddnsDomain":                 cfg.DdnsDomain,
		"ddnsTokenConfigured":        cfg.DdnsToken != "",
		"downloadSettings":           cfg.DownloadSettings,
	}
}

func checkDownloaderSystemTools() []map[string]interface{} {
	defs := []struct {
		ID       string
		Name     string
		Bin      string
		Args     []string
		Required bool
		Desc     string
	}{
		{"r3p-api", "R3P Core Service", "/usr/bin/r3p-api", nil, true, "Dịch vụ trung tâm R3P Home Server OS (Web UI, REST API v1, Downloader & AI)"},
		{"yt-dlp", "yt-dlp", "/usr/bin/yt-dlp", []string{"--version"}, true, "Công cụ tải video cốt lõi cho YouTube, TikTok, Facebook..."},
		{"ffmpeg", "FFmpeg", "/usr/bin/ffmpeg", []string{"-version"}, true, "Ghép nối âm thanh, video và xử lý media trên Linux"},
		{"aria2c", "aria2c", "/usr/bin/aria2c", []string{"--version"}, false, "Bộ tăng tốc tải phân đoạn đa luồng"},
		{"python", "Python 3", "/usr/bin/python3", []string{"--version"}, true, "Môi trường thực thi yt-dlp trên OpenWrt Linux"},
		{"curl", "cURL", "/usr/bin/curl", []string{"--version"}, true, "Truyền tải HTTP/HTTPS & Telegram API"},
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
		if d.ID == "r3p-api" {
			installed = true
			ver = getCurrentSystemVersion().Version
		} else if d.ID == "yt-dlp" {
			v, _ := getYtDlpVer()
			if v != "" {
				installed = true
				ver = v
			}
		} else if _, err := os.Stat(binPath); err == nil {
			installed = true
			if out, cmdErr := runCmd(4, binPath, d.Args...); cmdErr == nil {
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) > 0 {
					ver = strings.TrimSpace(lines[0])
					if len(ver) > 48 {
						ver = ver[:48]
					}
				}
			}
		}
		results = append(results, map[string]interface{}{
			"id":        d.ID,
			"name":      d.Name,
			"installed": installed,
			"version":   ver,
			"path":      binPath,
			"required":  d.Required,
			"desc":      d.Desc,
		})
	}
	return results
}

var ytProgressRegex = regexp.MustCompile(`\[download\]\s+([\d.]+)%.*?(?:\s+at\s+(\S+))?(?:\s+ETA\s+(\S+))?`)

func resolveJobVideoQuality(jobQuality, settingsQuality string) string {
	q := strings.ToLower(strings.TrimSpace(jobQuality))
	switch q {
	case "best", "1080p", "720p", "audio":
		return q
	}
	sq := strings.ToLower(strings.TrimSpace(settingsQuality))
	if sq == "" {
		return "best"
	}
	return sq
}

func startDownloadJob(job *DownloadJob) {
	dlMu.Lock()
	cfg := dlConfig
	settings := cfg.DownloadSettings
	outDir := job.OutputDirectory
	if outDir == "" || strings.Contains(outDir, ":") || strings.Contains(outDir, "\\") {
		outDir = cfg.OutputDirectory
	}
	ctx, cancel := context.WithCancel(context.Background())
	dlActiveCmds[job.ID] = cancel
	job.RuntimeStatus = "RUNNING"
	job.WorkerID = cfg.WorkerName
	job.Attempts++
	job.StartedAt = time.Now().UTC().Format(time.RFC3339)
	vq := resolveJobVideoQuality(job.Quality, settings.VideoQuality)
	saveDownloaderQueue()
	dlMu.Unlock()

	go func() {
		defer func() {
			dlMu.Lock()
			delete(dlActiveCmds, job.ID)
			dlMu.Unlock()
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
			switch vq {
			case "1080p":
				formatSel = "bv*[height<=1080]+ba/b[height<=1080]/best"
			case "720p":
				formatSel = "bv*[height<=720]+ba/b[height<=720]/best"
			case "audio":
				formatSel = "bestaudio/best"
			}
		} else {
			// Without ffmpeg, pick progressive streams (single file containing video & audio).
			switch vq {
			case "audio":
				formatSel = "ba/b/best"
			case "720p":
				formatSel = "22/18/best[height<=720]/b/best"
			default:
				formatSel = "22/18/b/best"
			}
		}
		args = append(args, "--format", formatSel)
		if vq == "audio" && hasFfmpeg {
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
			finishDownloadJob(job, false, false, "", nil, "Failed to start yt-dlp: "+err.Error())
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
					dlMu.Lock()
					job.Progress = pct
					job.Speed = spd
					job.ETA = eta
					dlMu.Unlock()
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

		// Clean up partial fragments or .part temporary files left behind.
		targetDir := filepath.Join(outDir, subFolder)
		if entries, readErr := os.ReadDir(targetDir); readErr == nil {
			for _, entry := range entries {
				name := entry.Name()
				if strings.HasSuffix(name, ".part") {
					_ = os.Remove(filepath.Join(targetDir, name))
					continue
				}
				if strings.Contains(name, ".f") && (strings.HasSuffix(name, ".webm") || strings.HasSuffix(name, ".mp4")) {
					if m, _ := filepath.Match("*.f[0-9]*.*", name); m {
						_ = os.Remove(filepath.Join(targetDir, name))
					}
				}
			}
		}

		if ctx.Err() != nil {
			finishDownloadJob(job, false, true, "", nil, "Cancelled by user")
			return
		}
		if err != nil {
			errMsg := strings.Join(lastErrLines, " | ")
			if errMsg == "" {
				errMsg = err.Error()
			}
			finishDownloadJob(job, false, false, "", nil, errMsg)
			return
		}

		mainOut := ""
		if len(outPaths) > 0 {
			mainOut = outPaths[0]
		}
		finishDownloadJob(job, true, false, mainOut, outPaths, "")
	}()
}

func finishDownloadJob(job *DownloadJob, success, cancelled bool, outPath string, outPaths []string, errMsg string) {
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

	dlMu.Lock()
	job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	job.OutputPath = outPath
	job.OutputPaths = outPaths
	job.OutputBytes = totalBytes
	job.OutputBytesMeasured = totalBytes > 0
	if cancelled {
		job.RuntimeStatus = "CANCELLED"
		job.Error = "Cancelled"
	} else if success {
		job.RuntimeStatus = "COMPLETED"
		job.Progress = 100
		job.Speed = ""
		job.ETA = "00:00"
		job.Error = ""
	} else {
		job.RuntimeStatus = "FAILED"
		job.Error = errMsg
	}

	for i := range dlTelegramLog {
		if dlTelegramLog[i].JobID == job.ID {
			dlTelegramLog[i].JobStatus = job.RuntimeStatus
			if errMsg != "" {
				dlTelegramLog[i].Reason = errMsg
			}
		}
	}
	saveDownloaderQueue()
	dlMu.Unlock()

	if job.TelegramChatID != "" && !cancelled {
		var msg string
		if success {
			name := filepath.Base(outPath)
			if name == "" || name == "." {
				name = "media file"
			}
			msg = fmt.Sprintf("✅ Tải hoàn tất (#%s)!\nFile: %s (%s)\nLưu tại: %s", job.ID, name, fmtBytesHuman(totalBytes), filepath.Dir(outPath))
		} else {
			cleanErr := errMsg
			if len(cleanErr) > 350 {
				cleanErr = cleanErr[:350] + "..."
			}
			if strings.Contains(errMsg, "Unsupported URL") && strings.Contains(errMsg, "/photo/") {
				cleanErr = "Link này là Album/Trình chiếu ảnh (TikTok Photo), yt-dlp hiện chỉ hỗ trợ tải định dạng video/âm thanh."
			} else if strings.Contains(errMsg, "Cannot parse data") || strings.Contains(errMsg, "login.php") {
				cleanErr = "Nội dung này ở chế độ riêng tư (bạn bè/nhóm kín) hoặc Facebook yêu cầu đăng nhập."
			}
			msg = fmt.Sprintf("❌ Job #%s thất bại:\n%s", job.ID, cleanErr)
		}
		_ = sendR3pTelegramMsg(job.TelegramChatID, msg)
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

func enqueueR3pJob(rawURL, source, chatID, quality string, force bool) (*DownloadJob, bool, error) {
	u := strings.TrimRight(strings.TrimSpace(rawURL), ".,;:!?)>\"'")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return nil, false, fmt.Errorf("URL phải bắt đầu bằng http:// hoặc https://")
	}
	dlMu.Lock()
	defer dlMu.Unlock()

	if dlConfig.DownloadSettings.DuplicateUrlPolicy == "skip_pending" && !force {
		for _, existing := range dlQueue {
			if existing.URL == u && (existing.RuntimeStatus == "QUEUED" || existing.RuntimeStatus == "RUNNING") {
				return existing, true, nil
			}
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	job := &DownloadJob{
		ID:              randHex(6),
		URL:             u,
		Source:          source,
		TelegramChatID:  chatID,
		OutputDirectory: dlConfig.OutputDirectory,
		Quality:         strings.TrimSpace(quality),
		CreatedAt:       nowStr,
		ReceivedAt:      nowStr,
		RuntimeStatus:   "QUEUED",
		Force:           force,
	}
	dlQueue = append([]*DownloadJob{job}, dlQueue...)
	if len(dlQueue) > 100 {
		dlQueue = dlQueue[:100]
	}
	saveDownloaderQueue()

	dlTelegramLog = append([]TelegramRouteLogEntry{{
		ID:         "log_" + job.ID,
		UpdateID:   time.Now().Unix(),
		ChatID:     chatID,
		URL:        u,
		Decision:   "ACCEPTED",
		Reason:     "Queued on R3P Local Downloader",
		JobID:      job.ID,
		JobStatus:  "QUEUED",
		ReceivedAt: nowStr,
	}}, dlTelegramLog...)
	if len(dlTelegramLog) > 64 {
		dlTelegramLog = dlTelegramLog[:64]
	}

	return job, false, nil
}

func downloadWorkerLoop() {
	for {
		time.Sleep(3 * time.Second)
		dlMu.Lock()
		maxJobs := dlConfig.DownloadSettings.MaxConcurrentJobs
		if maxJobs < 1 {
			maxJobs = 1
		}
		activeCount := len(dlActiveCmds)
		var nextJob *DownloadJob
		if activeCount < maxJobs {
			// Pick oldest QUEUED job (FIFO).
			for i := len(dlQueue) - 1; i >= 0; i-- {
				if dlQueue[i].RuntimeStatus == "QUEUED" {
					nextJob = dlQueue[i]
					break
				}
			}
		}
		nowMs := time.Now().UnixMilli()
		for _, s := range dlSchedules {
			if s.Enabled && s.NextRunAt > 0 && nowMs >= s.NextRunAt {
				s.NextRunAt = nowMs + 86400*1000
				s.LastRunAt = time.Now().UTC().Format(time.RFC3339)
				s.LastRunStatus = "QUEUED"
				saveDownloaderSchedules()
				go func(schedURL string) {
					_, _, _ = enqueueR3pJob(schedURL, "schedule", "", "", false)
				}(s.URL)
			}
		}
		dlMu.Unlock()

		if nextJob != nil {
			startDownloadJob(nextJob)
		}
	}
}

var urlExtractRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

func waitTelegramPollBackoff(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-dlTgWakeCh:
	case <-timer.C:
	}
}

func setTgPollState(state, lastErr string, errCode int, markPoll, markSuccess bool) {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	dlMu.Lock()
	dlTgState = state
	dlTgLastErr = lastErr
	dlTgLastErrCode = errCode
	if markPoll {
		dlTgLastPollAt = nowStr
	}
	if markSuccess {
		dlTgLastSuccessAt = nowStr
	}
	dlMu.Unlock()
}

func r3pTelegramPollerLoop() {
	client := newTelegramHTTPClient(32 * time.Second)
	webhookClearedToken := ""

	for {
		dlMu.Lock()
		enabled := dlConfig.TelegramEnabled
		tok := strings.TrimSpace(dlConfig.TelegramBotToken)
		offset := dlConfig.TelegramUpdateOffset
		dlMu.Unlock()

		if tok == "" {
			webhookClearedToken = ""
			setTgPollState("unconfigured", "", 0, false, false)
			telegramDownloader.updatePollStatus(false, "Chưa cấu hình Telegram Bot Token.")
			waitTelegramPollBackoff(4 * time.Second)
			continue
		}
		if !enabled {
			setTgPollState("disabled", "", 0, false, false)
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
			dlMu.Lock()
			needBotUser := dlBotUsername == ""
			dlMu.Unlock()
			if needBotUser {
				_ = checkTelegramToken(tok)
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		dlMu.Lock()
		dlTgPollCancel = cancel
		dlMu.Unlock()

		pollURL := fmt.Sprintf(
			"https://api.telegram.org/bot%s/getUpdates?timeout=15&limit=20&offset=%d&allowed_updates=%%5B%%22message%%22%%2C%%22channel_post%%22%%5D",
			tok, offset,
		)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if reqErr != nil {
			cancel()
			dlMu.Lock()
			dlTgPollCancel = nil
			dlMu.Unlock()
			setTgPollState("error", reqErr.Error(), 0, true, false)
			telegramDownloader.updatePollStatus(false, reqErr.Error())
			waitTelegramPollBackoff(5 * time.Second)
			continue
		}

		resp, err := client.Do(req)
		wasCanceled := ctx.Err() == context.Canceled
		cancel()
		dlMu.Lock()
		dlTgPollCancel = nil
		dlMu.Unlock()

		if err != nil {
			client.CloseIdleConnections()
			if wasCanceled {
				continue
			}
			errMsg := err.Error()
			setTgPollState("error", errMsg, 0, true, false)
			telegramDownloader.updatePollStatus(false, errMsg)
			waitTelegramPollBackoff(5 * time.Second)
			continue
		}

		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			client.CloseIdleConnections()
			errMsg := readErr.Error()
			setTgPollState("error", errMsg, 0, true, false)
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
			setTgPollState("error", errMsg, resp.StatusCode, true, false)
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
				errMsg = fmt.Sprintf("HTTP 409 Conflict: %s — Có tiến trình hoặc instance bot khác đang gọi getUpdates với cùng Bot Token này.", data.Description)
				webhookClearedToken = ""
			case 401:
				state = "auth_error"
				errMsg = fmt.Sprintf("HTTP 401 Unauthorized: Bot Token không hợp lệ (%s)", data.Description)
			default:
				if data.ErrorCode != 0 {
					errMsg = fmt.Sprintf("Telegram HTTP %d (%d): %s", resp.StatusCode, data.ErrorCode, data.Description)
				}
			}
			setTgPollState(state, errMsg, data.ErrorCode, true, false)
			telegramDownloader.updatePollStatus(false, errMsg)
			if data.ErrorCode == 409 {
				waitTelegramPollBackoff(6 * time.Second)
			} else {
				waitTelegramPollBackoff(5 * time.Second)
			}
			continue
		}

		setTgPollState("polling", "", 0, true, true)
		telegramDownloader.updatePollStatus(true, "")

		for _, upd := range data.Result {
			dlMu.Lock()
			if dlConfig.TelegramBotToken != tok {
				dlMu.Unlock()
				break
			}
			if upd.UpdateID >= dlConfig.TelegramUpdateOffset {
				dlConfig.TelegramUpdateOffset = upd.UpdateID + 1
				saveDownloaderConfig()
			}
			allowedChat := strings.TrimSpace(dlConfig.TelegramAllowedChatId)
			dlMu.Unlock()

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
				appendTelegramRouteLog(TelegramRouteLogEntry{
					ID:        fmt.Sprintf("help_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
					UpdateID:  upd.UpdateID,
					ChatID:    chatStr,
					URL:       truncateLogText(analysis.CleanText, 120),
					Decision:  "HELP",
					Reason:    "Lệnh hướng dẫn /start hoặc /id",
					JobStatus: "REPLIED",
				})
				_ = sendR3pTelegramMsg(chatStr, fmt.Sprintf(
					"🤖 R3P Local Downloader & Root AI Bot sẵn sàng!\nChat ID của bạn: %s\n• Gửi CHỈ URL (TikTok, YouTube, Facebook...) -> Tự động tải cục bộ trên R3P qua yt-dlp.\n• Gửi tin nhắn văn bản, câu hỏi hoặc !<lệnh> -> Chuyển trực tiếp sang Trợ Lý AI Root R3P!",
					chatStr,
				))
				continue
			}

			if allowedChat != "" && !isAllowedTelegramChat(allowedChat, chatStr) {
				appendTelegramRouteLog(TelegramRouteLogEntry{
					ID:        fmt.Sprintf("rej_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
					UpdateID:  upd.UpdateID,
					ChatID:    chatStr,
					URL:       truncateLogText(analysis.CleanText, 120),
					Decision:  "REJECTED",
					Reason:    "Chat ID chưa được cấp quyền: " + chatStr,
					JobStatus: "BLOCKED",
				})
				_ = sendR3pTelegramMsg(chatStr, "⛔ Chat ID "+chatStr+" chưa được cấp quyền trong cài đặt.")
				continue
			}

			switch analysis.Route {
			case TelegramRouteDownloader:
				urls := analysis.URLs
				if len(urls) > 5 {
					urls = urls[:5]
				}
				for _, u := range urls {
					job, dup, qErr := enqueueR3pJob(u, "telegram", chatStr, "", false)
					if qErr != nil {
						appendTelegramRouteLog(TelegramRouteLogEntry{
							ID:        fmt.Sprintf("dlerr_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
							UpdateID:  upd.UpdateID,
							ChatID:    chatStr,
							URL:       u,
							Decision:  "DOWNLOAD_ERROR",
							Reason:    "Phân tích: chỉ có URL -> Downloader lỗi: " + qErr.Error(),
							JobStatus: "ERROR",
						})
						_ = sendR3pTelegramMsg(chatStr, "❌ Lỗi: "+qErr.Error())
					} else if dup {
						appendTelegramRouteLog(TelegramRouteLogEntry{
							ID:        fmt.Sprintf("dup_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000),
							UpdateID:  upd.UpdateID,
							ChatID:    chatStr,
							URL:       u,
							Decision:  "DUPLICATE",
							Reason:    "Phân tích: chỉ có URL -> Đã có trong hàng đợi Downloader",
							JobID:     job.ID,
							JobStatus: job.RuntimeStatus,
						})
						_ = sendR3pTelegramMsg(chatStr, fmt.Sprintf("ℹ️ [R3P] URL này đã có trong hàng đợi (#%s):\n%s", job.ID, u))
					} else {
						updateTelegramRouteLogEntry("log_"+job.ID, "", "Phân tích: chỉ có URL -> Downloader (yt-dlp)", upd.UpdateID)
						_ = sendR3pTelegramMsg(chatStr, fmt.Sprintf("📥 [R3P Downloader] Đã nhận URL, đang tải về ổ cứng USB 3.0 (#%s):\n%s", job.ID, u))
					}
				}

			case TelegramRouteAI:
				logID := fmt.Sprintf("ai_%d_%d", upd.UpdateID, time.Now().UnixNano()%10000)
				appendTelegramRouteLog(TelegramRouteLogEntry{
					ID:        logID,
					UpdateID:  upd.UpdateID,
					ChatID:    chatStr,
					URL:       truncateLogText(analysis.CleanText, 120),
					Decision:  "AI_AGENT",
					Reason:    "Phân tích: tin nhắn thường (" + analysis.Reason + ") -> Module AI",
					JobStatus: "PROCESSING",
				})
				go func(lid, msgText, cid, reason string) {
					sendR3pTelegramChatAction(cid, "typing")
					aiReply := processTelegramAIChat(msgText, cid)
					if strings.TrimSpace(aiReply) == "" {
						updateTelegramRouteLogEntry(lid, "DONE", "AI hoàn tất (không có nội dung trả lời)", 0)
						return
					}
					if sErr := sendR3pTelegramMsg(cid, aiReply); sErr != nil {
						errText := "Gửi phản hồi AI thất bại: " + sErr.Error()
						updateTelegramRouteLogEntry(lid, "SEND_ERROR", errText, 0)
						dlMu.Lock()
						dlTgLastErr = errText
						dlMu.Unlock()
					} else {
						updateTelegramRouteLogEntry(lid, "REPLIED", "Phân tích: tin nhắn thường ("+reason+") -> Đã trả lời AI", 0)
					}
				}(logID, analysis.CleanText, chatStr, analysis.Reason)
			}
		}
	}
}

// ─── Authenticated REST API v1 Handlers (/api/v1/download/*) ─────────────────

// handleDownloadJobs handles GET (list), POST (enqueue), and DELETE (clear finished) at /api/v1/download/jobs.
func handleDownloadJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dlMu.Lock()
		jobs := make([]map[string]interface{}, 0, len(dlQueue))
		for _, j := range dlQueue {
			st := j.RuntimeStatus
			if st == "" {
				st = "QUEUED"
			}
			jobs = append(jobs, map[string]interface{}{
				"id":          j.ID,
				"url":         j.URL,
				"source":      j.Source,
				"quality":     j.Quality,
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
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"jobs":        jobs,
			"total":       len(jobs),
			"max_entries": 100,
		})

	case http.MethodPost:
		var body struct {
			URL     string `json:"url"`
			Quality string `json:"quality"`
			Force   bool   `json:"force"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Dữ liệu yêu cầu không hợp lệ")
			return
		}
		job, dup, err := enqueueR3pJob(body.URL, "dashboard", "", body.Quality, body.Force)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, true, map[string]interface{}{
			"duplicate": dup,
			"job": map[string]interface{}{
				"id":      job.ID,
				"url":     job.URL,
				"quality": job.Quality,
				"status":  job.RuntimeStatus,
			},
		})

	case http.MethodDelete:
		dlMu.Lock()
		var kept []*DownloadJob
		removed := 0
		for _, j := range dlQueue {
			if !isTerminalDownloadJob(j.RuntimeStatus) {
				kept = append(kept, j)
			} else {
				removed++
			}
		}
		dlQueue = kept
		saveDownloaderQueue()
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"deleted": true, "count": removed})

	default:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// handleDownloadJobItem handles DELETE /api/v1/download/jobs/{id} and POST /api/v1/download/jobs/{id}/retry.
func handleDownloadJobItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/download/jobs/")
	if rest == "" {
		writeJSON(w, http.StatusNotFound, false, "job id required")
		return
	}

	if strings.HasSuffix(rest, "/retry") && r.Method == http.MethodPost {
		id := strings.TrimSuffix(rest, "/retry")
		dlMu.Lock()
		found := false
		for _, j := range dlQueue {
			if j.ID == id {
				j.RuntimeStatus = "QUEUED"
				j.Error = ""
				j.Progress = 0
				j.Speed = ""
				j.ETA = ""
				saveDownloaderQueue()
				found = true
				break
			}
		}
		dlMu.Unlock()
		if !found {
			writeJSON(w, http.StatusNotFound, false, "Job không tồn tại")
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"retried": true, "id": id})
		return
	}

	if r.Method == http.MethodDelete && !strings.Contains(rest, "/") {
		id := rest
		targetURL := r.URL.Query().Get("url")
		dlMu.Lock()
		var kept []*DownloadJob
		removed := 0
		for _, j := range dlQueue {
			if j.ID == id || (targetURL != "" && j.URL == targetURL) {
				if cancel, ok := dlActiveCmds[j.ID]; ok {
					cancel()
				}
				removed++
				continue
			}
			kept = append(kept, j)
		}
		dlQueue = kept
		saveDownloaderQueue()
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"deleted": true, "id": id, "count": removed})
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
}

// handleDownloadDebug handles GET (inspect) and POST (auto-fix & resync) at /api/v1/download/debug.
func handleDownloadDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}

	autoSync := r.Method == http.MethodPost
	syncR3pDownloaderSettings()

	if autoSync {
		dlMu.Lock()
		tok := strings.TrimSpace(dlConfig.TelegramBotToken)
		dlMu.Unlock()
		if tok != "" {
			_ = ensureTelegramWebhookDeleted(tok)
		}
		wakeUnifiedTelegramPoller(true)
	}

	ytVer, _ := getYtDlpVer()
	lanIP := getRouterLanIP()
	dlMu.Lock()
	cfg := dlConfig
	dlMu.Unlock()

	storage := downloaderOutputStorageStatus(cfg.OutputDirectory)
	tools := checkDownloaderSystemTools()
	hasFfmpeg := false
	for _, t := range tools {
		if t["id"] == "ffmpeg" && t["installed"] == true {
			hasFfmpeg = true
			break
		}
	}

	var tgCheck map[string]interface{}
	webhookURL := ""
	webhookErr := ""
	if strings.TrimSpace(cfg.TelegramBotToken) != "" {
		tgCheck = checkTelegramToken(cfg.TelegramBotToken)
		if hURL, hErr := getTelegramWebhookInfo(cfg.TelegramBotToken); hErr != nil {
			webhookErr = hErr.Error()
		} else {
			webhookURL = hURL
		}
	}

	_ = os.MkdirAll(cfg.OutputDirectory, 0755)
	testFile := filepath.Join(cfg.OutputDirectory, ".write_test")
	writeOK := false
	if err := os.WriteFile(testFile, []byte("ok"), 0644); err == nil {
		_ = os.Remove(testFile)
		writeOK = true
	}

	dlMu.Lock()
	tgTokTail := ""
	if len(dlConfig.TelegramBotToken) >= 4 {
		tgTokTail = dlConfig.TelegramBotToken[len(dlConfig.TelegramBotToken)-4:]
	}
	tgState := dlTgState
	tgLastErr := dlTgLastErr
	tgErrCode := dlTgLastErrCode
	tgLastPollAt := dlTgLastPollAt
	tgLastSuccessAt := dlTgLastSuccessAt
	if webhookURL == "" {
		webhookURL = dlTgWebhookURL
	}
	botUser := dlBotUsername
	maxLogs := len(dlTelegramLog)
	if maxLogs > 20 {
		maxLogs = 20
	}
	recentLogs := append([]TelegramRouteLogEntry{}, dlTelegramLog[:maxLogs]...)
	dlMu.Unlock()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"timestamp":                time.Now().Format("2006-01-02 15:04:05"),
		"lan_ip":                   lanIP,
		"http_port":                80,
		"https_port":               8443,
		"service_online":           true,
		"service_url":              fmt.Sprintf("https://%s:8443/downloads", lanIP),
		"output_dir":               cfg.OutputDirectory,
		"output_dir_writable":      writeOK,
		"storage":                  storage,
		"yt_dlp_version":           ytVer,
		"ffmpeg_installed":         hasFfmpeg,
		"tools":                    tools,
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
		"message":                  "Đã kiểm tra và đồng bộ thông số R3P Local Downloader (HTTPS/HTTP, Telegram Polling, Thư mục USB và yt-dlp)!",
	})
}

// handleDownloadConfig handles GET and PUT at /api/v1/download/config.
func handleDownloadConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, publicDownloaderConfig())
	case http.MethodPut:
		var body struct {
			OutputDirectory  *string                 `json:"outputDirectory"`
			DownloadSettings *DownloadPolicySettings `json:"downloadSettings"`
			UrlRetentionDays *int                    `json:"urlRetentionDays"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Dữ liệu cấu hình không hợp lệ")
			return
		}
		dlMu.Lock()
		if body.OutputDirectory != nil {
			dir := strings.TrimSpace(*body.OutputDirectory)
			if dir != "" && !strings.Contains(dir, ":") && !strings.Contains(dir, "\\") {
				cleanDir := filepath.Clean(dir)
				_ = os.MkdirAll(cleanDir, 0755)
				dlConfig.OutputDirectory = cleanDir
			}
		}
		if body.DownloadSettings != nil {
			dlConfig.DownloadSettings = *body.DownloadSettings
		}
		if body.UrlRetentionDays != nil && *body.UrlRetentionDays > 0 {
			dlConfig.UrlRetentionDays = *body.UrlRetentionDays
		}
		dlConfig.SettingsRevision++
		saveDownloaderConfig()
		dlMu.Unlock()
		syncR3pDownloaderSettings()
		writeJSON(w, http.StatusOK, true, publicDownloaderConfig())
	default:
		w.Header().Set("Allow", "GET, PUT, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// handleDownloadTelegramLog handles GET and DELETE at /api/v1/download/telegram/log.
func handleDownloadTelegramLog(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dlMu.Lock()
		entries := append([]TelegramRouteLogEntry{}, dlTelegramLog...)
		dlMu.Unlock()
		if entries == nil {
			entries = []TelegramRouteLogEntry{}
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"entries":     entries,
			"total":       len(entries),
			"max_entries": 64,
		})
	case http.MethodDelete:
		dlMu.Lock()
		dlTelegramLog = nil
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"deleted": true})
	default:
		w.Header().Set("Allow", "GET, DELETE, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// handleDownloadTelegramTest sends a test message through the configured local Telegram bot.
func handleDownloadTelegramTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		ChatID string `json:"chatId"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	chatID := strings.TrimSpace(body.ChatID)
	if chatID == "" {
		dlMu.Lock()
		fields := strings.FieldsFunc(dlConfig.TelegramAllowedChatId, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '|' })
		if len(fields) > 0 {
			chatID = fields[0]
		}
		dlMu.Unlock()
	}
	if chatID == "" {
		writeJSON(w, http.StatusBadRequest, false, "Chưa cấu hình Chat ID Telegram hợp lệ.")
		return
	}
	if err := sendR3pTelegramMsg(chatID, "✅ Tin nhắn thử từ R3P Home Server OS! Kênh gửi Telegram hoạt động bình thường."); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{"sent": true, "chat_id": chatID})
}

// handleDownloadTelegramCheck checks bot token reachability via Telegram getMe.
func handleDownloadTelegramCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	tok := strings.TrimSpace(body.Token)
	if tok == "" {
		dlMu.Lock()
		tok = dlConfig.TelegramBotToken
		dlMu.Unlock()
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{"telegram": checkTelegramToken(tok)})
}

// handleDownloadTelegramAnalyze previews how the Telegram message analyzer classifies input text.
func handleDownloadTelegramAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"analysis": analyzeTelegramMessage(body.Text),
	})
}

// handleDownloadSchedules handles GET and POST /api/v1/download/schedules.
func handleDownloadSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dlMu.Lock()
		list := append([]*ScheduledDownload{}, dlSchedules...)
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"schedules": list})
	case http.MethodPost:
		var body ScheduledDownload
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		if strings.TrimSpace(body.URL) == "" {
			writeJSON(w, http.StatusBadRequest, false, "URL lịch tải không được để trống")
			return
		}
		dlMu.Lock()
		if body.ID == "" {
			body.ID = randHex(8)
		}
		body.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		if body.OutputDirectory == "" {
			body.OutputDirectory = dlConfig.OutputDirectory
		}
		if body.NextRunAt == 0 {
			body.NextRunAt = time.Now().Add(24 * time.Hour).UnixMilli()
		}
		dlSchedules = append([]*ScheduledDownload{&body}, dlSchedules...)
		saveDownloaderSchedules()
		dlMu.Unlock()
		writeJSON(w, http.StatusCreated, true, map[string]interface{}{"id": body.ID, "schedule": body})
	default:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// handleDownloadScheduleItem handles POST /api/v1/download/schedules/{id}/toggle and DELETE /api/v1/download/schedules/{id}.
func handleDownloadScheduleItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/download/schedules/")
	if rest == "" {
		writeJSON(w, http.StatusNotFound, false, "schedule id required")
		return
	}
	if strings.HasSuffix(rest, "/toggle") && r.Method == http.MethodPost {
		id := strings.TrimSuffix(rest, "/toggle")
		dlMu.Lock()
		defer dlMu.Unlock()
		for _, s := range dlSchedules {
			if s.ID == id {
				s.Enabled = !s.Enabled
				saveDownloaderSchedules()
				writeJSON(w, http.StatusOK, true, map[string]interface{}{"enabled": s.Enabled, "schedule": s})
				return
			}
		}
		writeJSON(w, http.StatusNotFound, false, "Schedule not found")
		return
	}
	if r.Method == http.MethodDelete && !strings.Contains(rest, "/") {
		id := rest
		dlMu.Lock()
		var next []*ScheduledDownload
		for _, s := range dlSchedules {
			if s.ID != id {
				next = append(next, s)
			}
		}
		dlSchedules = next
		saveDownloaderSchedules()
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"deleted": true, "id": id})
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
}
