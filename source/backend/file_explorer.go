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
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	ModTime   string `json:"mod_time"`
	Ext       string `json:"ext"`
	IsEditable bool  `json:"is_editable"`
}

type ExplorerQuickFolder struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Icon  string `json:"icon"`
	Desc  string `json:"desc"`
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
		result = append(result, ExplorerEntry{
			Name:       e.Name(),
			Path:       full,
			IsDir:      e.IsDir(),
			Size:       info.Size(),
			Mode:       info.Mode().String(),
			ModTime:    info.ModTime().Format("2006-01-02 15:04:05"),
			Ext:        ext,
			IsEditable: !e.IsDir() && (isFileEditable(e.Name()) || info.Size() < 512*1024),
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
		{Label: "Tải Về /downloads", Path: "/mnt/sda1/esp32-downloads", Icon: "📥", Desc: "Thư mục tải video yt-dlp và media"},
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
