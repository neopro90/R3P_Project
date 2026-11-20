package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type AutoBackupConfig struct {
	Enabled       bool   `json:"enabled"`
	Schedule      string `json:"schedule"`       // e.g. "0 2 * * *" (5-field cron)
	BackupType    string `json:"backup_type"`    // "full", "config", "db"
	TargetDir     string `json:"target_dir"`     // e.g. "/mnt/sda1/backups"
	RetentionDays int    `json:"retention_days"` // e.g. 14 (0 = do not prune)
	LastRun       string `json:"last_run"`
	LastStatus    string `json:"last_status"`
	LastFile      string `json:"last_file"`
	LastOutput    string `json:"last_output"`
}

type BackupFileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	ModTime   string `json:"mod_time"`
	Type      string `json:"type"`
}

var (
	autoBackupMu     sync.Mutex
	autoBackupFile   = "/etc/r3p/autobackup.json"
	defaultBackupDir = "/mnt/sda1/backups"
)

func getDefaultAutoBackupConfig() AutoBackupConfig {
	return AutoBackupConfig{
		Enabled:       false,
		Schedule:      "0 2 * * *",
		BackupType:    "full",
		TargetDir:     defaultBackupDir,
		RetentionDays: 14,
		LastRun:       "",
		LastStatus:    "ready",
		LastFile:      "",
		LastOutput:    "",
	}
}

func loadAutoBackupConfig() AutoBackupConfig {
	cfg := getDefaultAutoBackupConfig()
	settingsDBLoadJSON(bucketSettings, "autobackup", autoBackupFile, &cfg)
	if cfg.Schedule == "" {
		cfg.Schedule = "0 2 * * *"
	}
	if cfg.BackupType == "" {
		cfg.BackupType = "full"
	}
	if cfg.TargetDir == "" {
		cfg.TargetDir = defaultBackupDir
	}
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 14
	}
	return cfg
}

func saveAutoBackupConfig(cfg AutoBackupConfig) error {
	_ = os.MkdirAll("/etc/r3p", 0755)
	return settingsDBSaveJSON(bucketSettings, "autobackup", autoBackupFile, 0644, cfg)
}

// syncAutoBackupToCron updates /etc/crontabs/root with the configured auto-backup job.
func syncAutoBackupToCron(cfg AutoBackupConfig) error {
	cronMu.Lock()
	defer cronMu.Unlock()

	entries := loadRootCrontab()
	var updated []CronEntry
	found := false

	cronCmd := fmt.Sprintf("/usr/bin/r3p-api backup %s %s %d", cfg.BackupType, cfg.TargetDir, cfg.RetentionDays)
	cronDesc := fmt.Sprintf("R3P_AUTO_BACKUP: Auto backup %s to %s (retention %dd)", cfg.BackupType, cfg.TargetDir, cfg.RetentionDays)

	for _, e := range entries {
		// Identify auto-backup job by r3p-api backup command or description tag
		if strings.Contains(e.Command, "r3p-api backup") || strings.Contains(e.Description, "R3P_AUTO_BACKUP") {
			found = true
			if cfg.Enabled {
				e.Schedule = cfg.Schedule
				e.Command = cronCmd
				e.Description = cronDesc
				e.Enabled = true
				updated = append(updated, e)
			}
			// If not enabled, we omit it to keep crontabs clean, or set e.Enabled = false
			continue
		}
		updated = append(updated, e)
	}

	if cfg.Enabled && !found {
		updated = append(updated, CronEntry{
			Index:       len(updated),
			Schedule:    cfg.Schedule,
			Command:     cronCmd,
			Description: cronDesc,
			Enabled:     true,
		})
	}

	saveRootCrontab(updated)
	return nil
}

// executeAutoBackup performs the actual backup, saves it into targetDir, and prunes old backups.
func executeAutoBackup(backupType, targetDir string, retentionDays int) (string, error) {
	autoBackupMu.Lock()
	defer autoBackupMu.Unlock()

	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		targetDir = defaultBackupDir
	}
	cleanDir := filepath.Clean(targetDir)
	if !filepath.IsAbs(cleanDir) || cleanDir == "/" || cleanDir == "." {
		cleanDir = defaultBackupDir
	}

	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return "", fmt.Errorf("không thể tạo thư mục lưu bản sao lưu %s: %w", cleanDir, err)
	}

	ts := time.Now().Format("20060102-150405")
	bType := strings.ToLower(strings.TrimSpace(backupType))
	if bType == "" {
		bType = "full"
	}

	fileName := fmt.Sprintf("r3p-backup-%s-%s.tar.gz", bType, ts)
	outPath := filepath.Join(cleanDir, fileName)

	var execOutput string
	var backupErr error

	switch bType {
	case "config":
		backupErr = buildSanitizedConfigTarGz(outPath)
		if backupErr != nil {
			execOutput = "buildSanitizedConfigTarGz error: " + backupErr.Error()
		} else {
			execOutput = "Sanitized config backup created successfully."
		}

	case "db":
		_ = syncSettingsDB()
		backupErr = buildSettingsDBSnapshotTarGz(outPath)
		if backupErr != nil {
			execOutput = "buildSettingsDBSnapshotTarGz error: " + backupErr.Error()
		} else {
			execOutput = "Settings database snapshot created successfully."
		}

	default: // "full"
		_ = syncSettingsDB()
		// Try sysupgrade -b first
		out, err := runCmd(30, "sysupgrade", "-b", outPath)
		if err != nil {
			// Fallback to tar
			tarOut, tarErr := runCmd(20, "tar", "-czf", outPath, "/etc/config", "/etc/r3p", "/etc/crontabs", "/etc/passwd", "/etc/shadow", "/etc/dropbear")
			if tarErr != nil {
				backupErr = fmt.Errorf("tar fallback failed: %s (%w)", tarOut, tarErr)
				execOutput = "Full backup error: " + backupErr.Error()
			} else {
				execOutput = "Full system backup created via tar fallback."
			}
		} else {
			execOutput = "Full system backup created via sysupgrade: " + strings.TrimSpace(out)
		}
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	cfg := loadAutoBackupConfig()
	cfg.LastRun = nowStr
	if backupErr != nil {
		cfg.LastStatus = "failed: " + backupErr.Error()
		cfg.LastOutput = execOutput
		_ = saveAutoBackupConfig(cfg)
		return "", backupErr
	}

	cfg.LastStatus = "success"
	cfg.LastFile = fileName
	cfg.LastOutput = execOutput
	_ = saveAutoBackupConfig(cfg)

	// Prune backups older than retentionDays
	if retentionDays > 0 {
		pruneOldBackups(cleanDir, retentionDays)
	}

	log.Printf("[autobackup] %s backup completed: %s", bType, outPath)
	return outPath, nil
}

// buildSettingsDBSnapshotTarGz compresses /etc/r3p/settings.db and release metadata.
func buildSettingsDBSnapshotTarGz(destPath string) error {
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	items := []string{"/etc/r3p/settings.db", "/etc/r3p/version.json", "/etc/r3p/blueprint.json", "/etc/r3p/autobackup.json"}
	for _, item := range items {
		st, statErr := os.Stat(item)
		if statErr != nil {
			continue
		}
		data, readErr := os.ReadFile(item)
		if readErr != nil {
			continue
		}
		hdr := &tar.Header{
			Name:    strings.TrimPrefix(filepath.ToSlash(item), "/"),
			Mode:    int64(st.Mode().Perm()),
			Size:    int64(len(data)),
			ModTime: st.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// pruneOldBackups deletes backups older than retentionDays in targetDir.
func pruneOldBackups(targetDir string, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "r3p-backup-") && strings.HasSuffix(name, ".tar.gz") {
			info, iErr := e.Info()
			if iErr == nil && info.ModTime().Before(cutoff) {
				full := filepath.Join(targetDir, name)
				_ = os.Remove(full)
				log.Printf("[autobackup] pruned expired backup: %s (older than %d days)", name, retentionDays)
			}
		}
	}
}

// listExistingBackups scans targetDir for r3p-backup-*.tar.gz files.
func listExistingBackups(targetDir string) []BackupFileInfo {
	cleanDir := filepath.Clean(targetDir)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return []BackupFileInfo{}
	}
	var res []BackupFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "r3p-backup-") && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tar")) {
			info, iErr := e.Info()
			if iErr != nil {
				continue
			}
			bType := "full"
			if strings.Contains(name, "-config-") {
				bType = "config"
			} else if strings.Contains(name, "-db-") {
				bType = "db"
			}
			res = append(res, BackupFileInfo{
				Name:      name,
				Path:      filepath.Join(cleanDir, name),
				SizeBytes: info.Size(),
				ModTime:   info.ModTime().Format("2006-01-02 15:04:05"),
				Type:      bType,
			})
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ModTime > res[j].ModTime
	})
	return res
}

// ─── HTTP Endpoints ──────────────────────────────────────────────────────────

// handleSystemAutoBackup handles GET (status/list) and POST (save config & sync to cron)
func handleSystemAutoBackup(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}

	if r.Method == http.MethodGet {
		cfg := loadAutoBackupConfig()
		backups := listExistingBackups(cfg.TargetDir)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"config":   cfg,
			"backups":  backups,
			"total":    len(backups),
		})
		return
	}

	if r.Method == http.MethodPost {
		var req AutoBackupConfig
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Dữ liệu JSON không hợp lệ: "+err.Error())
			return
		}

		req.Schedule = strings.TrimSpace(req.Schedule)
		if req.Schedule == "" {
			req.Schedule = "0 2 * * *"
		}
		if !isValidCronSchedule(req.Schedule) {
			writeJSON(w, http.StatusBadRequest, false, "Biểu thức lịch Cron không hợp lệ (cần đúng chuẩn 5 trường ví dụ: '0 2 * * *')")
			return
		}

		req.BackupType = strings.ToLower(strings.TrimSpace(req.BackupType))
		switch req.BackupType {
		case "full", "config", "db":
		default:
			req.BackupType = "full"
		}

		req.TargetDir = filepath.Clean(strings.TrimSpace(req.TargetDir))
		if !filepath.IsAbs(req.TargetDir) || req.TargetDir == "/" || req.TargetDir == "." {
			req.TargetDir = defaultBackupDir
		}
		if err := os.MkdirAll(req.TargetDir, 0755); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Không thể tạo thư mục lưu trữ: "+err.Error())
			return
		}

		if req.RetentionDays < 0 {
			req.RetentionDays = 14
		}

		current := loadAutoBackupConfig()
		current.Enabled = req.Enabled
		current.Schedule = req.Schedule
		current.BackupType = req.BackupType
		current.TargetDir = req.TargetDir
		current.RetentionDays = req.RetentionDays

		if err := saveAutoBackupConfig(current); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi khi lưu cài đặt: "+err.Error())
			return
		}

		if err := syncAutoBackupToCron(current); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi khi đồng bộ sang /etc/crontabs/root: "+err.Error())
			return
		}

		backups := listExistingBackups(current.TargetDir)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": "Đã lưu cài đặt và đồng bộ lịch sao lưu thành công sang /etc/crontabs/root!",
			"config":  current,
			"backups": backups,
		})
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, false, "Phương thức không được hỗ trợ")
}

// handleSystemAutoBackupRun triggers an immediate backup using current settings
func handleSystemAutoBackupRun(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "Phương thức không được hỗ trợ")
		return
	}

	cfg := loadAutoBackupConfig()
	createdPath, err := executeAutoBackup(cfg.BackupType, cfg.TargetDir, cfg.RetentionDays)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Chạy sao lưu thất bại: "+err.Error())
		return
	}

	backups := listExistingBackups(cfg.TargetDir)
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"message":      "Đã tạo bản sao lưu thành công tại " + filepath.Base(createdPath),
		"created_file": filepath.Base(createdPath),
		"config":       loadAutoBackupConfig(),
		"backups":      backups,
	})
}

// handleSystemAutoBackupFile handles downloading or deleting a specific backup file
func handleSystemAutoBackupFile(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}

	fileName := filepath.Base(r.URL.Query().Get("file"))
	if fileName == "" || fileName == "." || fileName == ".." || !strings.HasPrefix(fileName, "r3p-backup-") || !strings.HasSuffix(fileName, ".tar.gz") {
		writeJSON(w, http.StatusBadRequest, false, "Tên file sao lưu không hợp lệ")
		return
	}

	cfg := loadAutoBackupConfig()
	fullPath := filepath.Join(cfg.TargetDir, fileName)

	if r.Method == http.MethodGet {
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			writeJSON(w, http.StatusNotFound, false, "File sao lưu không tồn tại")
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		http.ServeFile(w, r, fullPath)
		return
	}

	if r.Method == http.MethodDelete {
		if err := os.Remove(fullPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Không thể xóa file sao lưu: "+err.Error())
			return
		}
		backups := listExistingBackups(cfg.TargetDir)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": "Đã xóa bản sao lưu " + fileName + " thành công!",
			"backups": backups,
		})
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, false, "Phương thức không được hỗ trợ")
}
