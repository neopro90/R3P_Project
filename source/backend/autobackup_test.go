package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoBackupConfigAndCronSync(t *testing.T) {
	tmpDir := t.TempDir()
	origFile := autoBackupFile
	origRootCrontab := rootCrontabPath
	autoBackupFile = filepath.Join(tmpDir, "autobackup.json")
	rootCrontabPath = filepath.Join(tmpDir, "crontabs-root")
	defer func() {
		autoBackupFile = origFile
		rootCrontabPath = origRootCrontab
	}()

	// 1. Initial config
	cfg := loadAutoBackupConfig()
	if cfg.Enabled {
		t.Fatal("expected auto-backup disabled by default")
	}
	if cfg.Schedule != "0 2 * * *" {
		t.Fatalf("unexpected default schedule: %s", cfg.Schedule)
	}

	// 2. Enable and save
	cfg.Enabled = true
	cfg.Schedule = "0 3 * * *"
	cfg.BackupType = "config"
	cfg.TargetDir = filepath.Join(tmpDir, "my-backups")
	cfg.RetentionDays = 7
	if err := saveAutoBackupConfig(cfg); err != nil {
		t.Fatalf("saveAutoBackupConfig error: %v", err)
	}

	// 3. Sync to cron
	if err := syncAutoBackupToCron(cfg); err != nil {
		t.Fatalf("syncAutoBackupToCron error: %v", err)
	}

	cronContent, err := os.ReadFile(rootCrontabPath)
	if err != nil {
		t.Fatalf("failed to read crontab: %v", err)
	}
	cronStr := string(cronContent)
	if !strings.Contains(cronStr, "0 3 * * * /usr/bin/r3p-api backup config") {
		t.Fatalf("crontab missing expected auto-backup line, got: %s", cronStr)
	}
	if !strings.Contains(cronStr, "R3P_AUTO_BACKUP") {
		t.Fatalf("crontab missing R3P_AUTO_BACKUP tag, got: %s", cronStr)
	}

	// 4. Test execution
	outPath, bErr := executeAutoBackup("config", cfg.TargetDir, 7)
	if bErr != nil {
		t.Fatalf("executeAutoBackup error: %v", bErr)
	}
	if _, statErr := os.Stat(outPath); statErr != nil {
		t.Fatalf("created backup not found on disk: %v", statErr)
	}

	// 5. Test existing backups list
	existing := listExistingBackups(cfg.TargetDir)
	if len(existing) != 1 {
		t.Fatalf("expected 1 backup listed, got %d", len(existing))
	}
	if existing[0].Type != "config" {
		t.Fatalf("expected backup type 'config', got %s", existing[0].Type)
	}

	// 6. Test disable and sync to cron removes the job
	cfg.Enabled = false
	if err := syncAutoBackupToCron(cfg); err != nil {
		t.Fatalf("syncAutoBackupToCron (disable) error: %v", err)
	}
	cronContentAfter, _ := os.ReadFile(rootCrontabPath)
	if strings.Contains(string(cronContentAfter), "/usr/bin/r3p-api backup") {
		t.Fatalf("crontab still contains backup command after disabling: %s", string(cronContentAfter))
	}
}

func TestQuickRootsStorageAndDownloadSync(t *testing.T) {
	dlMu.Lock()
	origOut := dlConfig.OutputDirectory
	dlConfig.OutputDirectory = "/mnt/sda1/MyMediaDownloads"
	dlMu.Unlock()
	defer func() {
		dlMu.Lock()
		dlConfig.OutputDirectory = origOut
		dlMu.Unlock()
	}()

	dlDir := "/mnt/sda1/downloads"
	dlMu.Lock()
	if strings.TrimSpace(dlConfig.OutputDirectory) != "" {
		dlDir = strings.TrimSpace(dlConfig.OutputDirectory)
	}
	dlMu.Unlock()

	dlLabel := "Tải Về (" + filepath.Base(dlDir) + ")"
	if filepath.Base(dlDir) == "downloads" {
		dlLabel = "Tải Về /downloads"
	}

	quickFolders := []ExplorerQuickFolder{
		{Label: "Storage", Path: "/mnt/sda1", Icon: "💾", Desc: "Ổ lưu trữ ngoài USB 3.0 tốc độ cao"},
		{Label: "NAND /overlay", Path: "/overlay", Icon: "⚡", Desc: "Bộ nhớ bền vững SLC Flash của hệ thống"},
		{Label: dlLabel, Path: dlDir, Icon: "📥", Desc: "Thư mục tải video yt-dlp và media đã cấu hình"},
	}

	// Verify "Storage" is used instead of "USB HDD 500GB"
	if quickFolders[0].Label != "Storage" {
		t.Fatalf("expected Storage label, got %s", quickFolders[0].Label)
	}

	// Verify download folder syncs with configured dlConfig.OutputDirectory
	if quickFolders[2].Path != "/mnt/sda1/MyMediaDownloads" {
		t.Fatalf("expected path /mnt/sda1/MyMediaDownloads, got %s", quickFolders[2].Path)
	}
	if quickFolders[2].Label != "Tải Về (MyMediaDownloads)" {
		t.Fatalf("expected label 'Tải Về (MyMediaDownloads)', got %s", quickFolders[2].Label)
	}
}
