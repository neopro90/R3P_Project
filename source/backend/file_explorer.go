package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ExplorerEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	ModTime    string `json:"mod_time"`
	Ext        string `json:"ext"`
	IsEditable bool   `json:"is_editable"`
	IsImage    bool   `json:"is_image"`
	IsVideo    bool   `json:"is_video"`
	IsAudio    bool   `json:"is_audio"`
}

type ExplorerQuickFolder struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Icon  string `json:"icon"`
	Desc  string `json:"desc"`
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	".gif": true, ".svg": true, ".bmp": true, ".ico": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".mov": true,
	".avi": true, ".m4v": true, ".ts": true, ".flv": true,
}

var audioExts = map[string]bool{
	".mp3": true, ".m4a": true, ".ogg": true, ".flac": true,
	".wav": true, ".aac": true, ".wma": true,
}

func isImageFile(ext string) bool {
	return imageExts[strings.ToLower(ext)]
}

func isVideoFile(ext string) bool {
	return videoExts[strings.ToLower(ext)]
}

func isAudioFile(ext string) bool {
	return audioExts[strings.ToLower(ext)]
}

var editableExts = map[string]bool{
	".sh": true, ".bash": true, ".conf": true, ".config": true,
	".json": true, ".yaml": true, ".yml": true, ".txt": true,
	".md": true, ".hosts": true, ".lua": true, ".py": true,
	".js": true, ".ts": true, ".vue": true, ".css": true,
	".html": true, ".xml": true, ".log": true, ".ini": true,
	".service": true, ".repo": true,
}

func isFileEditable(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if isImageFile(ext) || isVideoFile(ext) || isAudioFile(ext) {
		return false
	}
	if editableExts[ext] {
		return true
	}
	base := strings.ToLower(name)
	if base == "hosts" || base == "version" || base == "profile" || base == "crontabs" || base == "inittab" {
		return true
	}
	return false
}

func sanitizeExplorerPath(p string) string {
	cleaned := filepath.Clean(strings.TrimSpace(p))
	if cleaned == "" || cleaned == "." {
		return "/overlay"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

// GET /api/v1/explorer/list?path=/mnt/sda1
func handleExplorerList(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	targetPath := sanitizeExplorerPath(r.URL.Query().Get("path"))

	st, err := os.Stat(targetPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, false, "Đường dẫn không tồn tại: "+err.Error())
		return
	}
	if !st.IsDir() {
		targetPath = filepath.Dir(targetPath)
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		writeJSON(w, http.StatusForbidden, false, "Không thể đọc thư mục: "+err.Error())
		return
	}

	var result []ExplorerEntry
	for _, e := range entries {
		info, iErr := e.Info()
		if iErr != nil {
			continue
		}
		full := filepath.Join(targetPath, e.Name())
		ext := strings.ToLower(filepath.Ext(e.Name()))
		isImg := !e.IsDir() && isImageFile(ext)
		isVid := !e.IsDir() && isVideoFile(ext)
		isAud := !e.IsDir() && isAudioFile(ext)
		result = append(result, ExplorerEntry{
			Name:       e.Name(),
			Path:       full,
			IsDir:      e.IsDir(),
			Size:       info.Size(),
			Mode:       info.Mode().String(),
			ModTime:    info.ModTime().Format("2006-01-02 15:04:05"),
			Ext:        ext,
			IsEditable: !e.IsDir() && !isImg && !isVid && !isAud && (isFileEditable(e.Name()) || info.Size() < 512*1024),
			IsImage:    isImg,
			IsVideo:    isVid,
			IsAudio:    isAud,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir // Folders first
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	quickFolders := []ExplorerQuickFolder{
		{Label: "USB HDD 500GB", Path: "/mnt/sda1", Icon: "💾", Desc: "Ổ lưu trữ ngoài USB 3.0 tốc độ cao"},
		{Label: "NAND /overlay", Path: "/overlay", Icon: "⚡", Desc: "Bộ nhớ bền vững SLC Flash của hệ thống"},
		{Label: "Cấu Hình /etc", Path: "/etc", Icon: "⚙️", Desc: "Cấu hình OpenWrt UCI, init scripts và daemon"},
		{Label: "Dữ Liệu R3P", Path: "/etc/r3p", Icon: "🛡️", Desc: "Blueprint, AI Memory, Sessions và AdBlock"},
		{Label: "Bộ Nhớ RAM /tmp", Path: "/tmp", Icon: "🚀", Desc: "RAM tmpfs tạm thời của Router"},
		{Label: "Tải Về /downloads", Path: "/mnt/sda1/downloads", Icon: "📥", Desc: "Thư mục tải video yt-dlp và media"},
		{Label: "Giao Diện Web /www", Path: "/www", Icon: "🌐", Desc: "Mã nguồn frontend Vue 3 Dashboard"},
		{Label: "Thư Mục /root", Path: "/root", Icon: "👤", Desc: "Thư mục cá nhân của tài khoản root"},
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"current_path":  targetPath,
		"parent_path":   filepath.Dir(targetPath),
		"entries":       result,
		"total":         len(result),
		"quick_folders": quickFolders,
	})
}

// GET /api/v1/explorer/read?path=/etc/config/wireless
func handleExplorerRead(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	targetPath := sanitizeExplorerPath(r.URL.Query().Get("path"))

	st, err := os.Stat(targetPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, false, "Tệp không tồn tại: "+err.Error())
		return
	}
	if st.IsDir() {
		writeJSON(w, http.StatusBadRequest, false, "Đường dẫn là thư mục, không thể xem nội dung tệp")
		return
	}
	if st.Size() > 2*1024*1024 {
		writeJSON(w, http.StatusBadRequest, false, "Tệp quá lớn (> 2MB), vui lòng tải về máy để chỉnh sửa")
		return
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi đọc tệp: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"path":     targetPath,
		"name":     filepath.Base(targetPath),
		"size":     st.Size(),
		"mode":     st.Mode().String(),
		"mod_time": st.ModTime().Format("2006-01-02 15:04:05"),
		"ext":      filepath.Ext(targetPath),
		"content":  string(data),
	})
}

// POST /api/v1/explorer/write
func handleExplorerWrite(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Backup  bool   `json:"backup"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4*1024*1024)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error())
		return
	}
	targetPath := sanitizeExplorerPath(body.Path)
	if targetPath == "" || targetPath == "/" {
		writeJSON(w, http.StatusBadRequest, false, "Đường dẫn không hợp lệ")
		return
	}

	// Create backup if requested or file exists
	if body.Backup {
		if orig, err := os.ReadFile(targetPath); err == nil {
			_ = os.WriteFile(targetPath+".bak", orig, 0644)
		}
	}

	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)

	perm := os.FileMode(0644)
	if st, err := os.Stat(targetPath); err == nil {
		perm = st.Mode().Perm()
	} else if strings.HasSuffix(targetPath, ".sh") || strings.HasPrefix(targetPath, "/etc/init.d/") || strings.HasPrefix(targetPath, "/etc/hotplug.d/") {
		perm = 0755
	}

	if err := os.WriteFile(targetPath, []byte(body.Content), perm); err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi lưu tệp: "+err.Error())
		return
	}
	_, _ = runCmd(2, "sync")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"saved":   true,
		"path":    targetPath,
		"bytes":   len(body.Content),
		"mode":    fmt.Sprintf("%04o", perm),
		"message": fmt.Sprintf("Đã lưu thành công %s (%d bytes)!", filepath.Base(targetPath), len(body.Content)),
	})
}

// POST /api/v1/explorer/create
func handleExplorerCreate(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		ParentPath string `json:"parent_path"`
		Name       string `json:"name"`
		Type       string `json:"type"` // "file" or "dir"
		Content    string `json:"content"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	parent := sanitizeExplorerPath(body.ParentPath)
	name := strings.TrimSpace(body.Name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		writeJSON(w, http.StatusBadRequest, false, "Tên không hợp lệ")
		return
	}
	targetPath := filepath.Join(parent, name)

	if body.Type == "dir" {
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi tạo thư mục: "+err.Error())
			return
		}
	} else {
		_ = os.MkdirAll(parent, 0755)
		perm := os.FileMode(0644)
		if strings.HasSuffix(name, ".sh") {
			perm = 0755
		}
		if err := os.WriteFile(targetPath, []byte(body.Content), perm); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi tạo tệp: "+err.Error())
			return
		}
	}
	_, _ = runCmd(2, "sync")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"created": true,
		"path":    targetPath,
		"name":    name,
		"type":    body.Type,
	})
}

// DELETE /api/v1/explorer/delete?path=/path/to/file
func handleExplorerDelete(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	targetPath := sanitizeExplorerPath(r.URL.Query().Get("path"))
	if targetPath == "/" || targetPath == "/bin" || targetPath == "/sbin" || targetPath == "/lib" || targetPath == "/usr" || targetPath == "/rom" {
		writeJSON(w, http.StatusForbidden, false, "Không được phép xóa các thư mục cốt lõi của hệ thống!")
		return
	}

	st, err := os.Stat(targetPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, false, "Đường dẫn không tồn tại: "+err.Error())
		return
	}

	if st.IsDir() {
		if err := os.RemoveAll(targetPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi xóa thư mục: "+err.Error())
			return
		}
	} else {
		if err := os.Remove(targetPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi xóa tệp: "+err.Error())
			return
		}
	}
	_, _ = runCmd(2, "sync")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"deleted": true,
		"path":    targetPath,
	})
}

// GET /api/v1/explorer/raw?path=/mnt/sda1/...&download=0
func handleExplorerRaw(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	targetPath := sanitizeExplorerPath(r.URL.Query().Get("path"))
	st, err := os.Stat(targetPath)
	if err != nil {
		http.Error(w, "Tệp không tồn tại: "+err.Error(), http.StatusNotFound)
		return
	}
	if st.IsDir() {
		http.Error(w, "Đường dẫn là thư mục, không thể xem thô", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(targetPath))
	switch ext {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".bmp":
		w.Header().Set("Content-Type", "image/bmp")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".webm":
		w.Header().Set("Content-Type", "video/webm")
	case ".mkv":
		w.Header().Set("Content-Type", "video/x-matroska")
	case ".mov":
		w.Header().Set("Content-Type", "video/quicktime")
	case ".avi":
		w.Header().Set("Content-Type", "video/x-msvideo")
	case ".m4v":
		w.Header().Set("Content-Type", "video/x-m4v")
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
	case ".mp3":
		w.Header().Set("Content-Type", "audio/mpeg")
	case ".m4a":
		w.Header().Set("Content-Type", "audio/mp4")
	case ".ogg":
		w.Header().Set("Content-Type", "audio/ogg")
	case ".flac":
		w.Header().Set("Content-Type", "audio/flac")
	case ".wav":
		w.Header().Set("Content-Type", "audio/wav")
	}

	isDownload := r.URL.Query().Get("download") == "1"
	if isDownload {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", st.Name()))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", st.Name()))
	}
	w.Header().Set("Accept-Ranges", "bytes")

	// http.ServeFile natively supports HTTP 206 Partial Content (Range requests) for video seeking & streaming!
	http.ServeFile(w, r, targetPath)
}
