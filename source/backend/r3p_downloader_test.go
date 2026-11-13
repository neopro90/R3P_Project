package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupDownloaderTestEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	dlMu.Lock()
	origCfgFile := downloaderConfigFile
	origQueueFile := downloaderQueueFile
	origSchedFile := downloaderSchedulesFile
	origCfg := dlConfig
	origQueue := dlQueue
	origSched := dlSchedules
	origLogs := dlTelegramLog

	downloaderConfigFile = filepath.Join(dir, "downloader.json")
	downloaderQueueFile = filepath.Join(dir, "downloader-queue.json")
	downloaderSchedulesFile = filepath.Join(dir, "downloader-schedules.json")
	dlConfig = defaultDownloaderConfig()
	dlConfig.OutputDirectory = filepath.Join(dir, "downloads")
	dlQueue = []*DownloadJob{}
	dlSchedules = []*ScheduledDownload{}
	dlTelegramLog = []TelegramRouteLogEntry{}
	dlMu.Unlock()

	t.Setenv("R3P_TELEGRAM_CONFIG", filepath.Join(dir, "telegram.json"))

	t.Cleanup(func() {
		dlMu.Lock()
		downloaderConfigFile = origCfgFile
		downloaderQueueFile = origQueueFile
		downloaderSchedulesFile = origSchedFile
		dlConfig = origCfg
		dlQueue = origQueue
		dlSchedules = origSched
		dlTelegramLog = origLogs
		dlMu.Unlock()
	})
	return dir
}

func TestLoadOrMigrateDownloaderConfigPreservesUserSettings(t *testing.T) {
	dir := setupDownloaderTestEnv(t)

	legacyPath := filepath.Join(dir, "esp32-local-config.json")
	targetPath := filepath.Join(dir, "downloader.json")
	legacyJSON := `{
  "schemaVersion": 2,
  "systemId": "esp-host-r3p-linux",
  "deviceBaseUrl": "http://192.168.4.1",
  "deviceToken": "legacy-esp-secret",
  "telegramBotToken": "123456789:TEST_SYNTHETIC_BOT_TOKEN_VALUE",
  "telegramEnabled": true,
  "telegramAllowedChatId": "99887766",
  "outputDirectory": "/mnt/sda1/esp32-downloads",
  "workerName": "R3P-Linux-Runtime",
  "downloadSettings": {
    "videoQuality": "1080p",
    "maxConcurrentJobs": 2
  }
}`
	if err := os.WriteFile(legacyPath, []byte(legacyJSON), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg := loadOrMigrateDownloaderConfig(targetPath, []string{legacyPath})
	if cfg.SystemID != "r3p-local-downloader" {
		t.Fatalf("SystemID = %q, want r3p-local-downloader", cfg.SystemID)
	}
	if cfg.WorkerName != "R3P-Local-Downloader" {
		t.Fatalf("WorkerName = %q, want R3P-Local-Downloader", cfg.WorkerName)
	}
	if cfg.OutputDirectory != "/mnt/sda1/esp32-downloads" {
		t.Fatalf("OutputDirectory = %q, want preserved /mnt/sda1/esp32-downloads", cfg.OutputDirectory)
	}
	if cfg.TelegramBotToken != "123456789:TEST_SYNTHETIC_BOT_TOKEN_VALUE" || !cfg.TelegramEnabled || cfg.TelegramAllowedChatId != "99887766" {
		t.Fatalf("Telegram settings lost during migration: %+v", cfg)
	}
	if cfg.DownloadSettings.VideoQuality != "1080p" || cfg.DownloadSettings.MaxConcurrentJobs != 2 {
		t.Fatalf("DownloadSettings lost during migration: %+v", cfg.DownloadSettings)
	}
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("migrated config was not written to %s: %v", targetPath, err)
	}
}

func TestResolveJobVideoQualityAndMountSubdir(t *testing.T) {
	if got := resolveJobVideoQuality("720p", "best"); got != "720p" {
		t.Fatalf("resolveJobVideoQuality(720p, best) = %q, want 720p", got)
	}
	if got := resolveJobVideoQuality("", "audio"); got != "audio" {
		t.Fatalf("resolveJobVideoQuality('', audio) = %q, want audio", got)
	}
	if got := resolveJobVideoQuality("invalid", ""); got != "best" {
		t.Fatalf("resolveJobVideoQuality(invalid, '') = %q, want best", got)
	}

	m, p := resolveMountAndSubdir("/mnt/sda1/videos")
	if m != "/mnt/sda1" || p != "videos" {
		t.Fatalf("resolveMountAndSubdir(/mnt/sda1/videos) = (%q, %q)", m, p)
	}
	m2, p2 := resolveMountAndSubdir("/mnt/sda1/esp32-downloads")
	if m2 != "/mnt/sda1" || p2 != "esp32-downloads" {
		t.Fatalf("resolveMountAndSubdir(/mnt/sda1/esp32-downloads) = (%q, %q)", m2, p2)
	}
}

func TestDownloadJobsAndAnalyzeHandlers(t *testing.T) {
	_ = setupDownloaderTestEnv(t)

	// 1. Enqueue a job with explicit quality
	postBody := []byte(`{"url":"https://youtu.be/dQw4w9WgXcQ","quality":"1080p","force":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/download/jobs", bytes.NewReader(postBody))
	rec := httptest.NewRecorder()
	handleDownloadJobs(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /jobs status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var postResp struct {
		OK   bool `json:"ok"`
		Data struct {
			Job struct {
				ID      string `json:"id"`
				URL     string `json:"url"`
				Quality string `json:"quality"`
				Status  string `json:"status"`
			} `json:"job"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &postResp); err != nil {
		t.Fatalf("decode POST /jobs: %v", err)
	}
	if !postResp.OK || postResp.Data.Job.Quality != "1080p" || postResp.Data.Job.Status != "QUEUED" {
		t.Fatalf("unexpected POST /jobs response: %+v", postResp)
	}

	// 2. Verify GET /jobs lists the queued job
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/download/jobs", nil)
	getRec := httptest.NewRecorder()
	handleDownloadJobs(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /jobs status = %d", getRec.Code)
	}
	var getResp struct {
		OK   bool `json:"ok"`
		Data struct {
			Total int `json:"total"`
			Jobs  []struct {
				ID      string `json:"id"`
				Quality string `json:"quality"`
			} `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode GET /jobs: %v", err)
	}
	if !getResp.OK || getResp.Data.Total != 1 || getResp.Data.Jobs[0].Quality != "1080p" {
		t.Fatalf("unexpected GET /jobs response: %+v", getResp)
	}

	// 3. Delete individual job
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/download/jobs/"+postResp.Data.Job.ID, nil)
	delRec := httptest.NewRecorder()
	handleDownloadJobItem(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE /jobs/{id} status = %d", delRec.Code)
	}

	// 4. Analyze endpoint routes URL -> downloader, plain question -> ai
	anaReq := httptest.NewRequest(http.MethodPost, "/api/v1/download/telegram/analyze", bytes.NewReader([]byte(`{"text":"https://www.tiktok.com/@user/video/123"}`)))
	anaRec := httptest.NewRecorder()
	handleDownloadTelegramAnalyze(anaRec, anaReq)
	if anaRec.Code != http.StatusOK {
		t.Fatalf("POST /telegram/analyze status = %d", anaRec.Code)
	}
	if !bytes.Contains(anaRec.Body.Bytes(), []byte(`"route":"downloader"`)) {
		t.Fatalf("expected downloader route, got: %s", anaRec.Body.String())
	}

	// 5. Debug endpoint reports local R3P runtime without port 8787 / ESP32 fields
	dbgReq := httptest.NewRequest(http.MethodGet, "/api/v1/download/debug", nil)
	dbgRec := httptest.NewRecorder()
	handleDownloadDebug(dbgRec, dbgReq)
	if dbgRec.Code != http.StatusOK {
		t.Fatalf("GET /debug status = %d, body = %s", dbgRec.Code, dbgRec.Body.String())
	}
	var dbgResp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(dbgRec.Body.Bytes(), &dbgResp); err != nil {
		t.Fatalf("decode GET /debug: %v", err)
	}
	if !dbgResp.OK || dbgResp.Data["service_online"] != true {
		t.Fatalf("unexpected /debug response: %+v", dbgResp)
	}
	if _, hasOldPort := dbgResp.Data["port_8787"]; hasOldPort {
		t.Fatalf("/debug should not expose legacy port_8787 field")
	}
	if _, hasOldDevice := dbgResp.Data["device_url"]; hasOldDevice {
		t.Fatalf("/debug should not expose legacy device_url field")
	}
}
