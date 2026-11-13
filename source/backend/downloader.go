package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type NativeDownloadTask struct {
	GID             string    `json:"gid"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	Dir             string    `json:"dir"`
	FilePath        string    `json:"file_path"`
	Status          string    `json:"status"` // active, waiting, paused, complete, error
	TotalLength     int64     `json:"totalLength"`
	CompletedLength int64     `json:"completedLength"`
	DownloadSpeed   int64     `json:"downloadSpeed"`
	ErrorMsg        string    `json:"error_msg,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type DownloadSettings struct {
	DefaultDir    string `json:"default_dir"`
	MaxConcurrent int    `json:"max_concurrent"`
	SpeedLimitKBs int    `json:"speed_limit_kbs"`
}

type NativeDownloadManager struct {
	mu         sync.Mutex
	tasks      map[string]*NativeDownloadTask
	cancels    map[string]context.CancelFunc
	settings   DownloadSettings
	configFile string
}

var dlManager = newNativeDownloadManager()

func newNativeDownloadManager() *NativeDownloadManager {
	defDir := "/mnt/sda1/downloads"
	if st, err := os.Stat("/mnt/sda1"); err != nil || !st.IsDir() {
		defDir = "/tmp/downloads"
	}
	m := &NativeDownloadManager{
		tasks:      make(map[string]*NativeDownloadTask),
		cancels:    make(map[string]context.CancelFunc),
		configFile: "/etc/r3p/downloads_state.json",
		settings: DownloadSettings{
			DefaultDir:    defDir,
			MaxConcurrent: 3,
			SpeedLimitKBs: 0,
		},
	}
	return m
}

func (m *NativeDownloadManager) init() {
	_ = os.MkdirAll("/etc/r3p", 0755)
	_ = os.MkdirAll("/tmp/downloads", 0755)
	if st, err := os.Stat("/mnt/sda1"); err == nil && st.IsDir() {
		_ = os.MkdirAll("/mnt/sda1/downloads", 0755)
		_ = os.MkdirAll("/mnt/sda1/share", 0755)
		m.settings.DefaultDir = "/mnt/sda1/downloads"
	} else if m.settings.DefaultDir == "" || strings.HasPrefix(m.settings.DefaultDir, "/overlay") {
		m.settings.DefaultDir = "/tmp/downloads"
	}

	data, err := os.ReadFile(m.configFile)
	if err == nil {
		var saved struct {
			Settings DownloadSettings      `json:"settings"`
			Tasks    []*NativeDownloadTask `json:"tasks"`
		}
		if json.Unmarshal(data, &saved) == nil {
			if saved.Settings.DefaultDir != "" {
				if strings.HasPrefix(saved.Settings.DefaultDir, "/overlay") {
					if st, err := os.Stat("/mnt/sda1/downloads"); err == nil && st.IsDir() {
						saved.Settings.DefaultDir = "/mnt/sda1/downloads"
					} else {
						saved.Settings.DefaultDir = "/tmp/downloads"
					}
				}
				m.settings = saved.Settings
			}
			for _, t := range saved.Tasks {
				if t.Status == "active" || t.Status == "waiting" {
					t.Status = "paused"
					t.DownloadSpeed = 0
				}
				m.tasks[t.GID] = t
			}
		}
	}
}

func (m *NativeDownloadManager) saveLocked() {
	list := make([]*NativeDownloadTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		list = append(list, t)
	}
	payload := map[string]interface{}{
		"settings": m.settings,
		"tasks":    list,
	}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(m.configFile, b, 0600)
	}
}

func generateGID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sanitizeDownloadDir(dir string) string {
	dir = strings.TrimSpace(dir)
	defaultFallback := "/mnt/sda1/downloads"
	if st, err := os.Stat("/mnt/sda1"); err != nil || !st.IsDir() {
		defaultFallback = "/tmp/downloads"
	}
	if dir == "" {
		_ = os.MkdirAll(defaultFallback, 0755)
		return defaultFallback
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	if (isUnderBaseDir(clean, "/mnt") && clean != "/mnt") ||
		(isUnderBaseDir(clean, "/media") && clean != "/media") ||
		isUnderBaseDir(clean, "/tmp/downloads") ||
		isUnderBaseDir(clean, "/overlay/downloads") {
		_ = os.MkdirAll(clean, 0755)
		return clean
	}
	_ = os.MkdirAll(defaultFallback, 0755)
	return defaultFallback
}

func isUnderBaseDir(clean, base string) bool {
	c := filepath.ToSlash(clean)
	b := filepath.ToSlash(base)
	return c == b || strings.HasPrefix(c, b+"/")
}

func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil && u.Path != "" {
		base := path.Base(u.Path)
		if base != "" && base != "/" && base != "." {
			return sanitizeFilename(base)
		}
	}
	return "download_" + time.Now().Format("20060102_150405") + ".bin"
}

func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := b.String()
	if res == "" || res == "." || res == ".." {
		return "file.bin"
	}
	return res
}

func (m *NativeDownloadManager) addTask(rawURL, dir string) (*NativeDownloadTask, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "magnet") {
		return nil, fmt.Errorf("only HTTP, HTTPS, and Magnet links are supported")
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		if err := validateDownloadURL(rawURL); err != nil {
			return nil, err
		}
	}

	m.mu.Lock()
	if dir == "" {
		dir = m.settings.DefaultDir
	}
	targetDir := sanitizeDownloadDir(dir)
	gid := generateGID()
	name := filenameFromURL(rawURL)
	if parsed.Scheme == "magnet" {
		name = "Magnet_" + gid
	}
	filePath := filepath.Join(targetDir, name)

	task := &NativeDownloadTask{
		GID:       gid,
		Name:      name,
		URL:       rawURL,
		Dir:       targetDir,
		FilePath:  filePath,
		Status:    "waiting",
		CreatedAt: time.Now(),
	}
	m.tasks[gid] = task
	m.saveLocked()
	m.mu.Unlock()

	if parsed.Scheme == "magnet" {
		// Try aria2 if running
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if ariaGID, err := queueAria2URL(ctx, rawURL, targetDir); err == nil {
			m.mu.Lock()
			task.GID = ariaGID
			task.Status = "active"
			m.saveLocked()
			m.mu.Unlock()
			return task, nil
		}
		m.mu.Lock()
		task.Status = "error"
		task.ErrorMsg = "Magnet links require aria2c daemon to be installed and running"
		m.saveLocked()
		m.mu.Unlock()
		return task, nil
	}

	go m.startWorker(gid)
	return task, nil
}

func (m *NativeDownloadManager) startWorker(gid string) {
	m.mu.Lock()
	task, ok := m.tasks[gid]
	if !ok {
		m.mu.Unlock()
		return
	}
	if cancel, running := m.cancels[gid]; running {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[gid] = cancel
	task.Status = "active"
	task.ErrorMsg = ""
	rawURL := task.URL
	filePath := task.FilePath
	m.saveLocked()
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.cancels, gid)
		if t, exists := m.tasks[gid]; exists {
			t.DownloadSpeed = 0
		}
		m.saveLocked()
		m.mu.Unlock()
	}()

	var existingSize int64
	if info, err := os.Stat(filePath); err == nil {
		existingSize = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		m.failTask(gid, err.Error())
		return
	}
	req.Header.Set("User-Agent", "R3P-DownloadBot/1.0")
	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		m.failTask(gid, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		m.failTask(gid, fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status))
		return
	}

	openFlags := os.O_CREATE | os.O_WRONLY
	if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
		openFlags |= os.O_APPEND
	} else {
		openFlags |= os.O_TRUNC
		existingSize = 0
	}

	if linfo, lerr := os.Lstat(filePath); lerr == nil && linfo.Mode()&os.ModeSymlink != 0 {
		m.failTask(gid, "refusing to overwrite symlink target")
		return
	}
	_ = os.MkdirAll(filepath.Dir(filePath), 0755)
	f, err := os.OpenFile(filePath, openFlags, 0644)
	if err != nil {
		m.failTask(gid, "cannot open target file: "+err.Error())
		return
	}
	defer f.Close()

	total := resp.ContentLength
	if total > 0 {
		total += existingSize
	}

	m.mu.Lock()
	if t, exists := m.tasks[gid]; exists {
		t.TotalLength = total
		t.CompletedLength = existingSize
	}
	m.mu.Unlock()

	buf := make([]byte, 32*1024)
	lastTick := time.Now()
	var bytesSinceTick int64
	completed := existingSize

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				m.failTask(gid, "disk write error: "+wErr.Error())
				return
			}
			completed += int64(n)
			bytesSinceTick += int64(n)

			now := time.Now()
			elapsed := now.Sub(lastTick).Seconds()
			if elapsed >= 0.5 {
				speed := int64(float64(bytesSinceTick) / elapsed)
				m.mu.Lock()
				if t, exists := m.tasks[gid]; exists {
					t.CompletedLength = completed
					if total > 0 {
						t.TotalLength = total
					} else {
						t.TotalLength = completed
					}
					t.DownloadSpeed = speed
				}
				m.mu.Unlock()
				lastTick = now
				bytesSinceTick = 0
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				m.mu.Lock()
				if t, exists := m.tasks[gid]; exists {
					t.CompletedLength = completed
					if t.TotalLength == 0 {
						t.TotalLength = completed
					}
					t.Status = "complete"
					t.DownloadSpeed = 0
				}
				m.saveLocked()
				m.mu.Unlock()
				return
			}
			if ctx.Err() != nil {
				return
			}
			m.failTask(gid, readErr.Error())
			return
		}
	}
}

func (m *NativeDownloadManager) failTask(gid, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, exists := m.tasks[gid]; exists {
		t.Status = "error"
		t.ErrorMsg = msg
		t.DownloadSpeed = 0
		m.saveLocked()
	}
}

func (m *NativeDownloadManager) controlTask(gid, action string) error {
	m.mu.Lock()
	task, exists := m.tasks[gid]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("download task not found")
	}

	switch action {
	case "pause":
		if cancel, ok := m.cancels[gid]; ok {
			cancel()
			delete(m.cancels, gid)
		}
		task.Status = "paused"
		task.DownloadSpeed = 0
		m.saveLocked()
		m.mu.Unlock()
		return nil

	case "resume", "retry":
		if action == "retry" {
			_ = os.Remove(task.FilePath)
			task.CompletedLength = 0
		}
		m.mu.Unlock()
		go m.startWorker(gid)
		return nil

	case "cancel", "remove":
		if cancel, ok := m.cancels[gid]; ok {
			cancel()
			delete(m.cancels, gid)
		}
		fp := task.FilePath
		delete(m.tasks, gid)
		m.saveLocked()
		m.mu.Unlock()
		if action == "remove" || task.Status != "complete" {
			_ = os.Remove(fp)
		}
		return nil

	default:
		m.mu.Unlock()
		return fmt.Errorf("unsupported action: %s", action)
	}
}

func (m *NativeDownloadManager) snapshot() ([]*NativeDownloadTask, int64, DownloadSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]*NativeDownloadTask, 0, len(m.tasks))
	var totalSpeed int64
	for _, t := range m.tasks {
		cp := *t
		list = append(list, &cp)
		if t.Status == "active" {
			totalSpeed += t.DownloadSpeed
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, totalSpeed, m.settings
}

// HTTP Handler for /api/v1/downloads
func handleDownloadsUnified(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		tasks, totalSpeed, settings := dlManager.snapshot()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"tasks":         tasks,
			"downloads":     tasks,
			"downloadSpeed": totalSpeed,
			"uploadSpeed":   0,
			"settings":      settings,
		})

	case http.MethodPost:
		var body struct {
			URL   string `json:"url"`
			Dir   string `json:"dir"`
			Mount string `json:"mount"`
			Path  string `json:"path"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid request body")
			return
		}
		dir := body.Dir
		if dir == "" && body.Mount != "" {
			dir = filepath.Join(body.Mount, body.Path)
		}
		task, err := dlManager.addTask(body.URL, dir)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, true, task)

	case http.MethodPut:
		var body DownloadSettings
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid settings JSON")
			return
		}
		dlManager.mu.Lock()
		if body.DefaultDir != "" {
			dlManager.settings.DefaultDir = sanitizeDownloadDir(body.DefaultDir)
		}
		if body.MaxConcurrent >= 1 && body.MaxConcurrent <= 10 {
			dlManager.settings.MaxConcurrent = body.MaxConcurrent
		}
		if body.SpeedLimitKBs >= 0 {
			dlManager.settings.SpeedLimitKBs = body.SpeedLimitKBs
		}
		dlManager.saveLocked()
		settings := dlManager.settings
		dlManager.mu.Unlock()
		writeJSON(w, http.StatusOK, true, settings)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/downloads/action
func handleDownloadsAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		GID    string `json:"gid"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	if err := dlManager.controlTask(body.GID, body.Action); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	tasks, totalSpeed, settings := dlManager.snapshot()
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"tasks":         tasks,
		"downloadSpeed": totalSpeed,
		"settings":      settings,
	})
}

// GET /api/v1/downloads/file?gid=...
func handleDownloadsFile(w http.ResponseWriter, r *http.Request) {
	gid := r.URL.Query().Get("gid")
	dlManager.mu.Lock()
	task, exists := dlManager.tasks[gid]
	dlManager.mu.Unlock()
	if !exists || task.FilePath == "" {
		writeJSON(w, http.StatusNotFound, false, "file not found")
		return
	}
	linfo, lerr := os.Lstat(task.FilePath)
	if lerr != nil || linfo.Mode()&os.ModeSymlink != 0 || linfo.IsDir() {
		writeJSON(w, http.StatusNotFound, false, "file not found")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(task.FilePath)))
	http.ServeFile(w, r, task.FilePath)
}
