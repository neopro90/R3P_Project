package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	posixpath "path"
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
	".gif": true, ".bmp": true, ".ico": true,
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
	trimmed := strings.TrimSpace(strings.ReplaceAll(p, "\x00", ""))
	cleaned := posixpath.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if cleaned == "" || cleaned == "." {
		return "/overlay"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func isSmallTextFile(fullPath string, size int64) bool {
	if size <= 0 || size >= 512*1024 {
		return size == 0
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) < 0
}

func isSensitiveExplorerPath(p string) bool {
	c := filepath.ToSlash(filepath.Clean(p))
	sensitive := []string{
		"/etc/shadow",
		"/etc/dropbear",
		"/etc/uhttpd.key",
		"/etc/r3p/backups",
		"/etc/r3p/r3p.db",
		"/etc/r3p/r3p.db.lock",
		"/etc/r3p/r3p.db-wal",
		"/etc/r3p/r3p.db-shm",
		"/etc/r3p/api-token",
		"/etc/r3p/aria2-rpc-token",
		"/etc/r3p/ai.json",
		"/etc/r3p/ai_config.json",
		"/etc/r3p/ai_sessions.json",
		"/etc/r3p/ai_memory.json",
		"/etc/r3p/ddns.json",
		"/etc/r3p/downloader.json",
		"/etc/r3p/gdrive.json",
		"/etc/r3p/gdrive_sync.json",
		"/etc/r3p/saved-wifi.json",
		"/etc/r3p/telegram.json",
		"/etc/r3p/sessions.json",
		"/overlay/upper/etc/shadow",
		"/overlay/upper/etc/dropbear",
		"/overlay/upper/etc/uhttpd.key",
		"/overlay/upper/etc/r3p/backups",
		"/overlay/upper/etc/r3p/r3p.db",
		"/overlay/upper/etc/r3p/r3p.db.lock",
		"/overlay/upper/etc/r3p/r3p.db-wal",
		"/overlay/upper/etc/r3p/r3p.db-shm",
		"/overlay/upper/etc/r3p/api-token",
		"/overlay/upper/etc/r3p/aria2-rpc-token",
		"/overlay/upper/etc/r3p/ai.json",
		"/overlay/upper/etc/r3p/ai_config.json",
		"/overlay/upper/etc/r3p/ai_sessions.json",
		"/overlay/upper/etc/r3p/ai_memory.json",
		"/overlay/upper/etc/r3p/ddns.json",
		"/overlay/upper/etc/r3p/downloader.json",
		"/overlay/upper/etc/r3p/gdrive.json",
		"/overlay/upper/etc/r3p/gdrive_sync.json",
		"/overlay/upper/etc/r3p/saved-wifi.json",
		"/overlay/upper/etc/r3p/telegram.json",
		"/overlay/upper/etc/r3p/sessions.json",
	}
	for _, pref := range sensitive {
		if c == pref || strings.HasPrefix(c, pref+"/") {
			return true
		}
	}
	return false
}

func hasSymlinkAncestor(targetPath string) bool {
	cur := filepath.ToSlash(filepath.Clean(targetPath))
	for {
		if st, err := os.Lstat(cur); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return true
		}
		parent := filepath.ToSlash(filepath.Dir(cur))
		if parent == cur || cur == "/" || cur == "." {
			break
		}
		cur = parent
	}
	return false
}

func isProtectedExplorerPath(p string) bool {
	if isSensitiveExplorerPath(p) {
		return true
	}
	c := filepath.ToSlash(filepath.Clean(p))
	switch c {
	case "/", "/bin", "/sbin", "/lib", "/lib64", "/usr", "/rom",
		"/etc", "/etc/r3p", "/etc/config", "/etc/init.d", "/overlay", "/www", "/root",
		"/dev", "/proc", "/sys", "/boot":
		return true
	}
	for _, sysPref := range []string{
		"/proc/", "/sys/", "/dev/", "/rom/", "/bin/", "/sbin/", "/lib/", "/lib64/",
		"/usr/", "/www/", "/etc/config/", "/etc/init.d/", "/overlay/upper/",
	} {
		if strings.HasPrefix(c, sysPref) {
			return true
		}
	}
	return false
}

const maxExplorerEntries = 500

// GET /api/v1/explorer/list?path=/mnt/sda1
func handleExplorerList(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	targetPath := sanitizeExplorerPath(r.URL.Query().Get("path"))
	if hasSymlinkAncestor(targetPath) {
		writeJSON(w, http.StatusBadRequest, false, "Không hỗ trợ duyệt thư mục qua symbolic link")
		return
	}

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

	truncated := false
	if len(entries) > maxExplorerEntries {
		entries = entries[:maxExplorerEntries]
		truncated = true
	}

	result := make([]ExplorerEntry, 0, len(entries))
	for _, e := range entries {
		info, iErr := e.Info()
		if iErr != nil {
			continue
		}
		isSym := info.Mode()&os.ModeSymlink != 0
		full := filepath.Join(targetPath, e.Name())
		ext := strings.ToLower(filepath.Ext(e.Name()))
		isImg := !e.IsDir() && !isSym && isImageFile(ext)
		isVid := !e.IsDir() && !isSym && isVideoFile(ext)
		isAud := !e.IsDir() && !isSym && isAudioFile(ext)
		isEdit := !e.IsDir() && !isSym && !isSensitiveExplorerPath(full) && !isImg && !isVid && !isAud &&
			(isFileEditable(e.Name()) || isSmallTextFile(full, info.Size()))
		result = append(result, ExplorerEntry{
			Name:       e.Name(),
			Path:       full,
			IsDir:      e.IsDir(),
			Size:       info.Size(),
			Mode:       info.Mode().String(),
			ModTime:    info.ModTime().Format("2006-01-02 15:04:05"),
			Ext:        ext,
			IsEditable: isEdit,
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
		"truncated":     truncated,
		"quick_folders": quickFolders,
	})
}

// GET /api/v1/explorer/read?path=/etc/config/wireless
func handleExplorerRead(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	targetPath := sanitizeExplorerPath(r.URL.Query().Get("path"))
	if isSensitiveExplorerPath(targetPath) {
		writeJSON(w, http.StatusForbidden, false, "Không được phép đọc tệp tin nhạy cảm hoặc thông tin xác thực hệ thống")
		return
	}
	if hasSymlinkAncestor(targetPath) {
		writeJSON(w, http.StatusBadRequest, false, "Không hỗ trợ đọc qua symbolic link")
		return
	}

	root, rootErr := os.OpenRoot(filepath.Dir(targetPath))
	if rootErr != nil {
		writeJSON(w, http.StatusNotFound, false, "Tệp không tồn tại: "+rootErr.Error())
		return
	}
	defer root.Close()
	f, openErr := root.Open(filepath.Base(targetPath))
	if openErr != nil {
		writeJSON(w, http.StatusNotFound, false, "Tệp không tồn tại: "+openErr.Error())
		return
	}
	defer f.Close()

	st, err := f.Stat()
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

	data, err := io.ReadAll(io.LimitReader(f, 2*1024*1024+1))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi đọc tệp: "+err.Error())
		return
	}
	if int64(len(data)) > 2*1024*1024 {
		writeJSON(w, http.StatusBadRequest, false, "Tệp quá lớn (> 2MB), vui lòng tải về máy để chỉnh sửa")
		return
	}
	if bytes.IndexByte(data, 0) >= 0 {
		writeJSON(w, http.StatusBadRequest, false, "Tệp nhị phân không hỗ trợ mở bằng trình soạn thảo văn bản")
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
	if isSensitiveExplorerPath(targetPath) {
		writeJSON(w, http.StatusForbidden, false, "Không được phép ghi đè tệp tin nhạy cảm của hệ thống")
		return
	}
	if hasSymlinkAncestor(targetPath) {
		writeJSON(w, http.StatusBadRequest, false, "Không được ghi đè qua symbolic link")
		return
	}

	parentDir := filepath.Dir(targetPath)
	baseName := filepath.Base(targetPath)
	_ = os.MkdirAll(parentDir, 0755)
	root, rootErr := os.OpenRoot(parentDir)
	if rootErr != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi mở thư mục đích: "+rootErr.Error())
		return
	}
	defer root.Close()

	perm := os.FileMode(0644)
	if st, err := root.Lstat(baseName); err == nil {
		perm = st.Mode().Perm()
	} else if strings.HasSuffix(targetPath, ".sh") || strings.HasPrefix(targetPath, "/etc/init.d/") || strings.HasPrefix(targetPath, "/etc/hotplug.d/") {
		perm = 0755
	}

	// Create backup if requested, preserving original file permissions (e.g. 0600 for sensitive configs)
	if body.Backup {
		if orig, err := root.ReadFile(baseName); err == nil {
			bakPerm := perm
			if bakPerm == 0 {
				bakPerm = 0600
			}
			_ = root.WriteFile(baseName+".bak", orig, bakPerm)
		}
	}

	wf, werr := os.CreateTemp(parentDir, "."+baseName+".r3ptmp-*")
	if werr != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi tạo tệp tạm: "+werr.Error())
		return
	}
	tmpBase := filepath.Base(wf.Name())
	_, writeErr := wf.WriteString(body.Content)
	closeErr := wf.Close()
	if writeErr != nil || closeErr != nil {
		_ = root.Remove(tmpBase)
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi lưu tệp")
		return
	}
	_ = root.Chmod(tmpBase, perm)
	if err := root.Rename(tmpBase, baseName); err != nil {
		_ = root.Remove(tmpBase)
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi lưu tệp: "+err.Error())
		return
	}
	_, _ = runCmd(2, "sync")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"saved":   true,
		"path":    targetPath,
		"bytes":   len(body.Content),
		"mode":    fmt.Sprintf("%04o", perm),
		"message": fmt.Sprintf("Đã lưu thành công %s (%d bytes)!", baseName, len(body.Content)),
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
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		writeJSON(w, http.StatusBadRequest, false, "Tên không hợp lệ")
		return
	}
	targetPath := filepath.Join(parent, name)
	if isSensitiveExplorerPath(targetPath) {
		writeJSON(w, http.StatusForbidden, false, "Không được phép tạo tệp tin vào vùng bảo vệ của hệ thống")
		return
	}
	if hasSymlinkAncestor(targetPath) {
		writeJSON(w, http.StatusBadRequest, false, "Thư mục cha không hợp lệ (symlink)")
		return
	}

	_ = os.MkdirAll(parent, 0755)
	root, rootErr := os.OpenRoot(parent)
	if rootErr != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Không thể mở thư mục cha: "+rootErr.Error())
		return
	}
	defer root.Close()

	if body.Type == "dir" {
		if err := root.MkdirAll(name, 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi tạo thư mục: "+err.Error())
			return
		}
	} else {
		perm := os.FileMode(0644)
		if strings.HasSuffix(name, ".sh") {
			perm = 0755
		}
		f, cerr := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
		if cerr != nil {
			writeJSON(w, http.StatusConflict, false, "Lỗi tạo tệp (tệp đã tồn tại hoặc lỗi quyền): "+cerr.Error())
			return
		}
		_, werr := f.WriteString(body.Content)
		closeErr := f.Close()
		if werr != nil || closeErr != nil {
			_ = root.Remove(name)
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi ghi nội dung tệp")
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
	if isProtectedExplorerPath(targetPath) {
		writeJSON(w, http.StatusForbidden, false, "Không được phép xóa các thư mục cốt lõi của hệ thống!")
		return
	}
	if hasSymlinkAncestor(filepath.Dir(targetPath)) {
		writeJSON(w, http.StatusBadRequest, false, "Đường dẫn chứa symbolic link không hợp lệ!")
		return
	}

	root, rootErr := os.OpenRoot(filepath.Dir(targetPath))
	if rootErr != nil {
		writeJSON(w, http.StatusNotFound, false, "Đường dẫn không tồn tại: "+rootErr.Error())
		return
	}
	defer root.Close()

	baseName := filepath.Base(targetPath)
	st, err := root.Lstat(baseName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, false, "Đường dẫn không tồn tại: "+err.Error())
		return
	}

	if st.IsDir() {
		if err := root.RemoveAll(baseName); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi xóa thư mục: "+err.Error())
			return
		}
	} else {
		if err := root.Remove(baseName); err != nil {
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
	serveExplorerRawFile(w, r, targetPath)
}

func serveExplorerRawFile(w http.ResponseWriter, r *http.Request, targetPath string) {
	if isSensitiveExplorerPath(targetPath) {
		http.Error(w, "Không được phép truy cập tệp tin nhạy cảm", http.StatusForbidden)
		return
	}
	if hasSymlinkAncestor(targetPath) {
		http.Error(w, "Không hỗ trợ phục vụ symbolic link", http.StatusBadRequest)
		return
	}
	root, rootErr := os.OpenRoot(filepath.Dir(targetPath))
	if rootErr != nil {
		http.Error(w, "Tệp không tồn tại: "+rootErr.Error(), http.StatusNotFound)
		return
	}
	defer root.Close()
	f, openErr := root.Open(filepath.Base(targetPath))
	if openErr != nil {
		http.Error(w, "Tệp không tồn tại: "+openErr.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "Tệp không tồn tại: "+err.Error(), http.StatusNotFound)
		return
	}
	if st.IsDir() || !st.Mode().IsRegular() {
		http.Error(w, "Đường dẫn là thư mục hoặc không hợp lệ", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(targetPath))
	safeInline := true
	switch ext {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
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
	default:
		// Prevent stored XSS via .svg, .html, .htm, .xml, .js or arbitrary active files
		safeInline = false
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; media-src 'self'; img-src 'self'")

	isDownload := r.URL.Query().Get("download") == "1" || !safeInline
	if isDownload {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", st.Name()))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", st.Name()))
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
