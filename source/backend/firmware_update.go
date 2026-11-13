package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var safeGitBranchPattern = regexp.MustCompile(`^[a-zA-Z0-9._/-]{1,80}$`)

func isSafeGitBranch(branch string) bool {
	if !safeGitBranchPattern.MatchString(branch) {
		return false
	}
	if strings.Contains(branch, "..") || strings.Contains(branch, "//") {
		return false
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, ".") {
		return false
	}
	return true
}

func isELFBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [4]byte
	n, err := io.ReadFull(f, hdr[:])
	return err == nil && n == 4 && hdr[0] == 0x7f && hdr[1] == 'E' && hdr[2] == 'L' && hdr[3] == 'F'
}

func shortSHA(sha string) string {
	if len(sha) > 16 {
		return sha[:16]
	}
	return sha
}

// inspectTarGzSafety validates gzip and tar structure and rejects path traversal,
// special device nodes, hardlinks, and unsafe symlinks before extracting onto '/'.
func inspectTarGzSafety(tgzPath string, allowSafeSymlinks bool, allowedPrefixes []string) (int, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("file gzip không hợp lệ: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	const maxEntries = 10000
	const maxTotalBytes = int64(128 << 20)
	var count int
	var totalBytes int64
	symlinkTargets := make(map[string]struct{})

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("lỗi đọc cấu trúc tar: %w", err)
		}
		count++
		if count > maxEntries {
			return 0, fmt.Errorf("gói nén vượt quá giới hạn %d tệp", maxEntries)
		}
		if hdr.Size < 0 || totalBytes+hdr.Size > maxTotalBytes {
			return 0, fmt.Errorf("dung lượng giải nén vượt quá giới hạn an toàn")
		}
		totalBytes += hdr.Size

		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeDir, tar.TypeGNULongName, tar.TypeGNULongLink, tar.TypeXHeader, tar.TypeXGlobalHeader:
		case tar.TypeSymlink:
			if !allowSafeSymlinks {
				return 0, fmt.Errorf("không cho phép symlink trong gói cập nhật: %s", hdr.Name)
			}
			link := filepath.ToSlash(hdr.Linkname)
			for _, seg := range strings.Split(link, "/") {
				if seg == ".." {
					return 0, fmt.Errorf("symlink không an toàn trong gói nén: %s -> %s", hdr.Name, hdr.Linkname)
				}
			}
		default:
			return 0, fmt.Errorf("loại tệp không an toàn (%c) trong gói nén: %s", hdr.Typeflag, hdr.Name)
		}

		rawName := strings.TrimSpace(filepath.ToSlash(hdr.Name))
		for _, seg := range strings.Split(rawName, "/") {
			if seg == ".." {
				return 0, fmt.Errorf("phát hiện path traversal trong gói nén: %s", hdr.Name)
			}
		}
		clean := strings.TrimPrefix(rawName, "./")
		clean = strings.TrimPrefix(clean, "/")
		clean = strings.TrimSuffix(clean, "/")
		if clean == "" || clean == "." {
			continue
		}
		if len(allowedPrefixes) > 0 {
			matched := false
			for _, p := range allowedPrefixes {
				pClean := strings.TrimSuffix(p, "/")
				if clean == pClean || strings.HasPrefix(clean, p) {
					matched = true
					break
				}
			}
			if !matched {
				return 0, fmt.Errorf("đường dẫn không hợp lệ trong gói cập nhật: %s", hdr.Name)
			}
		}
		parts := strings.Split(clean, "/")
		prefix := ""
		for i := 0; i < len(parts)-1; i++ {
			if i > 0 {
				prefix += "/"
			}
			prefix += parts[i]
			if _, ok := symlinkTargets[prefix]; ok {
				return 0, fmt.Errorf("tệp nằm dưới thư mục symlink không an toàn: %s", hdr.Name)
			}
		}
		if hdr.Typeflag == tar.TypeSymlink {
			symlinkTargets[clean] = struct{}{}
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("gói nén rỗng")
	}
	return count, nil
}

func verifyOptionalRemoteSHA256(client *http.Client, shaURL, localPath string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, shaURL, nil)
	if err != nil {
		return false, nil
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return false, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil {
		return false, nil
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return false, nil
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return false, nil
	}
	actual := fileSHA256(localPath)
	if !strings.EqualFold(actual, fields[0]) {
		return false, fmt.Errorf("SHA256 mismatch: expected %s, got %s", fields[0], actual)
	}
	return true, nil
}

// ─── Firmware & GitHub Online System Updater ───────────────────────────────

type SystemVersionInfo struct {
	Version    string `json:"version"`
	Revision   int    `json:"revision"`
	ReleasedAt string `json:"released_at"`
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	CommitSHA  string `json:"commit_sha"`
	Changelog  string `json:"changelog"`
}

type StagedFirmwareInfo struct {
	Present    bool   `json:"present"`
	Filename   string `json:"filename"`
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
	FormatType string `json:"format_type"` // "sysupgrade_bin", "os_bundle_targz", "r3p_api_binary"
	Valid      bool   `json:"valid"`
	CheckMsg   string `json:"check_msg"`
	UploadedAt string `json:"uploaded_at"`
}

const (
	versionConfigFile = "/etc/r3p/version.json"
	stagedFwPath      = "/tmp/r3p-staged-firmware.bin"
	stagedMetaPath    = "/tmp/r3p-staged-meta.json"
)

var (
	fwUpdateMu  sync.Mutex
	fwUpdateLog []string
)

func appendFwLog(msg string) {
	fwUpdateMu.Lock()
	defer fwUpdateMu.Unlock()
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	fwUpdateLog = append(fwUpdateLog, line)
	if len(fwUpdateLog) > 100 {
		fwUpdateLog = fwUpdateLog[len(fwUpdateLog)-100:]
	}
}

func getFwLogs() []string {
	fwUpdateMu.Lock()
	defer fwUpdateMu.Unlock()
	if fwUpdateLog == nil {
		return []string{}
	}
	cp := make([]string, len(fwUpdateLog))
	copy(cp, fwUpdateLog)
	return cp
}

func getCurrentSystemVersion() SystemVersionInfo {
	def := SystemVersionInfo{
		Version:    "v1.0.0",
		Revision:   1,
		ReleasedAt: BuildDate,
		RepoURL:    "https://github.com/neopro90/R3P_Project.git",
		Branch:     "main",
		CommitSHA:  "v1.0.0-initial",
		Changelog:  "R3P Home Server OS v1.0.0 — Full LAN/WAN Config, AdBlock DNS Engine, USB 3.0 Format & Benchmark, Root AI Agent, OpenWrt + GitHub App Manager & Online GitHub Updater",
	}
	if def.ReleasedAt == "" || def.ReleasedAt == "unknown" {
		def.ReleasedAt = time.Now().Format("2006-01-02")
	}
	if b, err := os.ReadFile(versionConfigFile); err == nil {
		_ = json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &def)
	}
	if def.RepoURL == "" {
		def.RepoURL = "https://github.com/neopro90/R3P_Project.git"
	}
	if def.Branch == "" {
		def.Branch = "main"
	}
	if def.Version == "" {
		def.Version = "v1.0.0"
	}
	if def.Revision < 1 {
		def.Revision = 1
	}
	return def
}

func saveCurrentSystemVersion(v SystemVersionInfo) {
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(v, "", "  "); err == nil {
		_ = os.WriteFile(versionConfigFile, b, 0644)
	}
}

func loadStagedFirmwareInfo() StagedFirmwareInfo {
	var info StagedFirmwareInfo
	if b, err := os.ReadFile(stagedMetaPath); err == nil {
		if json.Unmarshal(b, &info) == nil {
			if st, err := os.Stat(info.Path); err == nil && st.Size() > 0 {
				info.Present = true
				return info
			}
		}
	}
	return StagedFirmwareInfo{Present: false}
}

// GET /api/v1/system/firmware/status
func handleFirmwareStatus(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	ver := getCurrentSystemVersion()
	staged := loadStagedFirmwareInfo()
	openwrt := parseOpenwrtRelease()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"current":         ver,
		"openwrt_release": openwrt["DISTRIB_RELEASE"],
		"openwrt_target":  openwrt["DISTRIB_TARGET"],
		"kernel":          readFile("/proc/sys/kernel/osrelease"),
		"staged":          staged,
		"logs":            getFwLogs(),
	})
}

// PUT /api/v1/system/firmware/config
func handleFirmwareConfigPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		RepoURL string `json:"repo_url"`
		Branch  string `json:"branch"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	ver := getCurrentSystemVersion()
	if strings.TrimSpace(body.RepoURL) != "" {
		ver.RepoURL = strings.TrimSpace(body.RepoURL)
	}
	if strings.TrimSpace(body.Branch) != "" {
		branch := strings.TrimSpace(body.Branch)
		if !isSafeGitBranch(branch) {
			writeJSON(w, http.StatusBadRequest, false, "Tên nhánh Git (branch) không hợp lệ")
			return
		}
		ver.Branch = branch
	}
	saveCurrentSystemVersion(ver)
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"saved":   true,
		"current": ver,
		"message": "Đã lưu cấu hình kho cập nhật GitHub!",
	})
}

// GET /api/v1/system/firmware/github-check?repo_url=...&branch=...
func handleFirmwareGitHubCheck(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	cur := getCurrentSystemVersion()
	repoURL := strings.TrimSpace(r.URL.Query().Get("repo_url"))
	if repoURL == "" {
		repoURL = cur.RepoURL
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		branch = cur.Branch
	}
	if !isSafeGitBranch(branch) {
		writeJSON(w, http.StatusBadRequest, false, "Tên nhánh Git (branch) không hợp lệ")
		return
	}

	owner, repo, ok := parseGitHubOwnerRepo(repoURL)
	if !ok {
		writeJSON(w, http.StatusBadRequest, false, "URL GitHub không hợp lệ (ví dụ: https://github.com/neopro90/R3P_Project.git)")
		return
	}

	appendFwLog(fmt.Sprintf("Đang kiểm tra bản cập nhật từ GitHub %s/%s (nhánh %s)...", owner, repo, branch))
	client := &http.Client{Timeout: 10 * time.Second}

	remoteVer := cur.Version
	remoteRev := cur.Revision
	remoteChangelog := ""
	remoteReleasedAt := ""
	commitSHA := ""
	commitMsg := ""
	commitDate := ""
	reachable := false

	// 1. Try reading release.json from main or master
	for _, br := range []string{branch, "main", "master"} {
		relURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/release.json", owner, repo, br)
		req, err := http.NewRequest(http.MethodGet, relURL, nil)
		if err == nil {
			req.Header.Set("Cache-Control", "no-cache")
			if resp, err := client.Do(req); err == nil {
				if resp.StatusCode == http.StatusOK {
					var relData struct {
						Version    string `json:"version"`
						Revision   int    `json:"revision"`
						ReleasedAt string `json:"released_at"`
						Changelog  string `json:"changelog"`
					}
					rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
					rawBody = bytes.TrimPrefix(rawBody, []byte("\xef\xbb\xbf"))
					if json.Unmarshal(rawBody, &relData) == nil && relData.Version != "" {
						remoteVer = relData.Version
						if relData.Revision > 0 {
							remoteRev = relData.Revision
						}
						remoteChangelog = relData.Changelog
						remoteReleasedAt = relData.ReleasedAt
						branch = br
						reachable = true
						resp.Body.Close()
						break
					}
				}
				resp.Body.Close()
			}
		}
	}

	// 2. Also check VERSION file if release.json wasn't found
	if !reachable {
		for _, br := range []string{branch, "main", "master"} {
			vURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/VERSION", owner, repo, br)
			if resp, err := client.Get(vURL); err == nil {
				if resp.StatusCode == http.StatusOK {
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
					vStr := strings.TrimSpace(string(b))
					if vStr != "" {
						remoteVer = vStr
						branch = br
						reachable = true
						resp.Body.Close()
						break
					}
				}
				resp.Body.Close()
			}
		}
	}

	// 3. Query latest commit info via GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?per_page=1", owner, repo)
	if req, err := http.NewRequest(http.MethodGet, apiURL, nil); err == nil {
		req.Header.Set("User-Agent", "R3P-Home-Server-OS")
		if resp, err := client.Do(req); err == nil {
			if resp.StatusCode == http.StatusOK {
				var commits []struct {
					SHA    string `json:"sha"`
					Commit struct {
						Message string `json:"message"`
						Author  struct {
							Date string `json:"date"`
						} `json:"author"`
					} `json:"commit"`
				}
				if json.NewDecoder(resp.Body).Decode(&commits) == nil && len(commits) > 0 {
					reachable = true
					commitSHA = commits[0].SHA
					if len(commitSHA) > 8 {
						commitSHA = commitSHA[:8]
					}
					commitMsg = strings.Split(commits[0].Commit.Message, "\n")[0]
					commitDate = commits[0].Commit.Author.Date
					if remoteChangelog == "" {
						remoteChangelog = commitMsg
					}
				}
			}
			resp.Body.Close()
		}
	}

	updateAvailable := (remoteVer != cur.Version) || (remoteRev > cur.Revision)
	statusMsg := fmt.Sprintf("Đã kết nối GitHub %s/%s: Phiên bản trên kho là %s (Rev %d).", owner, repo, remoteVer, remoteRev)
	if !reachable {
		statusMsg = fmt.Sprintf("Không thể kết nối trực tiếp tới GitHub %s/%s (Kiểm tra kết nối Internet của Router hoặc kho lưu trữ đang trống).", owner, repo)
	}
	appendFwLog(statusMsg)

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"reachable":         reachable,
		"repo_url":          repoURL,
		"owner":             owner,
		"repo":              repo,
		"branch":            branch,
		"current_version":   cur.Version,
		"current_revision":  cur.Revision,
		"remote_version":    remoteVer,
		"remote_revision":   remoteRev,
		"update_available":  updateAvailable,
		"released_at":       remoteReleasedAt,
		"commit_sha":        commitSHA,
		"commit_message":    commitMsg,
		"commit_date":       commitDate,
		"changelog":         remoteChangelog,
		"bundle_url":        fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/build/artifacts/r3p-update-bundle.tar.gz", owner, repo, branch),
		"binary_url":        fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/build/artifacts/r3p-api", owner, repo, branch),
		"sysupgrade_url":    fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/R3P-Firmware-Sysupgrade.bin", owner, repo, branch),
		"message":           statusMsg,
		"logs":              getFwLogs(),
	})
}

// POST /api/v1/system/firmware/github-update
func handleFirmwareGitHubUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		RepoURL      string `json:"repo_url"`
		Branch       string `json:"branch"`
		Mode         string `json:"mode"`          // "os_bundle" (default) or "sysupgrade"
		KeepSettings bool   `json:"keep_settings"` // for sysupgrade
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)

	cur := getCurrentSystemVersion()
	repoURL := strings.TrimSpace(body.RepoURL)
	if repoURL == "" {
		repoURL = cur.RepoURL
	}
	branch := strings.TrimSpace(body.Branch)
	if branch == "" {
		branch = cur.Branch
	}
	if !isSafeGitBranch(branch) {
		writeJSON(w, http.StatusBadRequest, false, "Tên nhánh Git (branch) không hợp lệ")
		return
	}
	owner, repo, ok := parseGitHubOwnerRepo(repoURL)
	if !ok {
		writeJSON(w, http.StatusBadRequest, false, "URL GitHub không hợp lệ")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	if mode == "" {
		mode = "os_bundle"
	}

	appendFwLog(fmt.Sprintf("Bắt đầu cập nhật hệ thống từ GitHub %s/%s (nhánh %s, chế độ: %s)...", owner, repo, branch, mode))
	dlClient := &http.Client{Timeout: 15 * time.Second}

	if mode == "sysupgrade" {
		fwURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/R3P-Firmware-Sysupgrade.bin", owner, repo, branch)
		appendFwLog("Đang tải R3P-Firmware-Sysupgrade.bin từ GitHub: " + fwURL)
		_ = os.Remove(stagedFwPath)
		_, dlErr := runCmd(90, "curl", "-L", "-f", "-s", "-o", stagedFwPath, fwURL)
		if dlErr != nil {
			appendFwLog("Lỗi tải R3P-Firmware-Sysupgrade.bin từ GitHub: " + dlErr.Error())
			writeJSON(w, http.StatusBadGateway, false, "Không thể tải R3P-Firmware-Sysupgrade.bin từ GitHub (Kiểm tra kết nối Internet hoặc file trên nhánh "+branch+"): "+dlErr.Error())
			return
		}
		if shaOK, shaErr := verifyOptionalRemoteSHA256(dlClient, fwURL+".sha256", stagedFwPath); shaErr != nil {
			_ = os.Remove(stagedFwPath)
			appendFwLog("Lỗi kiểm tra checksum SHA256: " + shaErr.Error())
			writeJSON(w, http.StatusBadGateway, false, "Kiểm tra checksum SHA256 thất bại: "+shaErr.Error())
			return
		} else if shaOK {
			appendFwLog("Đã xác thực chữ ký SHA256 sidecar khớp hoàn toàn.")
		}
		st, err := os.Stat(stagedFwPath)
		if err != nil || st.Size() < 1024*1024 {
			_ = os.Remove(stagedFwPath)
			writeJSON(w, http.StatusBadGateway, false, "File firmware tải về không hợp lệ (< 1MB)")
			return
		}
		chkOut, chkErr := runCmd(10, "sysupgrade", "-T", stagedFwPath)
		valid := chkErr == nil
		sha := fileSHA256(stagedFwPath)
		meta := StagedFirmwareInfo{
			Present:    true,
			Filename:   "R3P-Firmware-Sysupgrade.bin (GitHub)",
			Path:       stagedFwPath,
			SizeBytes:  st.Size(),
			SHA256:     sha,
			FormatType: "sysupgrade_bin",
			Valid:      valid,
			CheckMsg:   chkOut,
			UploadedAt: time.Now().Format("2006-01-02 15:04:05"),
		}
		if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
			_ = os.WriteFile(stagedMetaPath, b, 0644)
		}
		appendFwLog(fmt.Sprintf("Đã tải xong R3P-Firmware-Sysupgrade.bin (%.2f MB, SHA256: %s). Sẵn sàng Flash!", float64(st.Size())/(1024*1024), shortSHA(sha)))
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"staged":  meta,
			"logs":    getFwLogs(),
			"message": "Đã tải Firmware Sysupgrade từ GitHub và kiểm tra hợp lệ! Bấm 'Xác Nhận Flash Firmware' để nạp vào NAND.",
		})
		return
	}

	// Default Mode: "os_bundle" (Download r3p-update-bundle.tar.gz or r3p-api binary + release.json from GitHub and hot-upgrade!)
	bundleURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/build/artifacts/r3p-update-bundle.tar.gz", owner, repo, branch)
	tmpBundle := "/tmp/r3p-github-update.tar.gz"
	_ = os.Remove(tmpBundle)
	defer func() { _ = os.Remove(tmpBundle) }()
	appendFwLog("Đang tải gói cập nhật R3P OS Bundle: " + bundleURL)
	_, err := runCmd(60, "curl", "-L", "-f", "-s", "-o", tmpBundle, bundleURL)
	if err == nil {
		if st, sErr := os.Stat(tmpBundle); sErr == nil && st.Size() > 100*1024 {
			if shaOK, shaErr := verifyOptionalRemoteSHA256(dlClient, bundleURL+".sha256", tmpBundle); shaErr != nil {
				appendFwLog("Từ chối gói cập nhật do sai lệch SHA256: " + shaErr.Error())
				writeJSON(w, http.StatusBadGateway, false, "Gói cập nhật từ GitHub sai lệch SHA256: "+shaErr.Error())
				return
			} else if shaOK {
				appendFwLog("Đã xác thực SHA256 sidecar của OS Bundle.")
			}
			if _, safetyErr := inspectTarGzSafety(tmpBundle, false, []string{"usr/", "www/", "etc/", "sbin/", "lib/"}); safetyErr != nil {
				appendFwLog("Gói cập nhật không hợp lệ hoặc chứa đường dẫn không an toàn: " + safetyErr.Error())
				writeJSON(w, http.StatusBadGateway, false, "Gói cập nhật tải về không hợp lệ: "+safetyErr.Error())
				return
			}
			appendFwLog(fmt.Sprintf("Đã tải gói r3p-update-bundle.tar.gz (%.2f MB). Đang giải nén vào hệ thống...", float64(st.Size())/(1024*1024)))
			_ = os.Remove("/usr/bin/r3p-api.old")
			_ = os.Rename("/usr/bin/r3p-api", "/usr/bin/r3p-api.old")
			out, tarErr := runCmd(30, "tar", "-xzf", tmpBundle, "-C", "/")
			_ = os.Remove(tmpBundle)
			if tarErr != nil {
				_ = os.Rename("/usr/bin/r3p-api.old", "/usr/bin/r3p-api")
				appendFwLog("Lỗi giải nén gói cập nhật: " + tarErr.Error() + " " + out)
				writeJSON(w, http.StatusInternalServerError, false, "Giải nén gói cập nhật thất bại: "+tarErr.Error())
				return
			}
			_ = os.Remove("/usr/bin/r3p-api.old")
			_ = os.Chmod("/usr/bin/r3p-api", 0755)
		} else {
			err = fmt.Errorf("bundle too small")
		}
	}

	if err != nil {
		// Fallback: download build/artifacts/r3p-api directly
		binURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/build/artifacts/r3p-api", owner, repo, branch)
		tmpBin := "/tmp/r3p-api.new"
		_ = os.Remove(tmpBin)
		defer func() { _ = os.Remove(tmpBin) }()
		appendFwLog("Đang tải trực tiếp binary r3p-api từ GitHub: " + binURL)
		_, binErr := runCmd(60, "curl", "-L", "-f", "-s", "-o", tmpBin, binURL)
		if binErr != nil {
			appendFwLog("Không thể tải bản cập nhật từ GitHub: " + binErr.Error())
			writeJSON(w, http.StatusBadGateway, false, "Không thể tải bản cập nhật từ GitHub (Kiểm tra kết nối Internet của Router): "+binErr.Error())
			return
		}
		if shaOK, shaErr := verifyOptionalRemoteSHA256(dlClient, binURL+".sha256", tmpBin); shaErr != nil {
			appendFwLog("Từ chối binary r3p-api do sai lệch SHA256: " + shaErr.Error())
			writeJSON(w, http.StatusBadGateway, false, "Binary r3p-api sai lệch SHA256: "+shaErr.Error())
			return
		} else if shaOK {
			appendFwLog("Đã xác thực SHA256 sidecar của binary r3p-api.")
		}
		if st, sErr := os.Stat(tmpBin); sErr == nil && st.Size() > 1024*1024 && isELFBinary(tmpBin) {
			_ = os.Chmod(tmpBin, 0755)
			// Use cp -f because /tmp and /usr/bin are on different filesystems (cross-device link)
			if _, cpErr := runCmd(10, "cp", "-f", tmpBin, "/usr/bin/r3p-api"); cpErr != nil {
				writeJSON(w, http.StatusInternalServerError, false, "Không thể ghi đè /usr/bin/r3p-api: "+cpErr.Error())
				return
			}
			_ = os.Remove(tmpBin)
			appendFwLog(fmt.Sprintf("Đã cập nhật /usr/bin/r3p-api (%.2f MB) thành công!", float64(st.Size())/(1024*1024)))
		} else {
			writeJSON(w, http.StatusBadGateway, false, "File r3p-api tải từ GitHub không hợp lệ (không phải ELF binary > 1MB)")
			return
		}
	}

	// Update /etc/r3p/version.json from remote release.json if available
	relURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/release.json", owner, repo, branch)
	client := &http.Client{Timeout: 8 * time.Second}
	if resp, err := client.Get(relURL); err == nil {
		if resp.StatusCode == http.StatusOK {
			rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
			rawBody = bytes.TrimPrefix(rawBody, []byte("\xef\xbb\xbf"))
			var rel SystemVersionInfo
			if json.Unmarshal(rawBody, &rel) == nil && rel.Version != "" {
				rel.RepoURL = repoURL
				rel.Branch = branch
				saveCurrentSystemVersion(rel)
			}
		}
		resp.Body.Close()
	}

	_, _ = runCmd(4, "sync")
	newVer := getCurrentSystemVersion()
	appendFwLog(fmt.Sprintf("Hoàn tất cập nhật lên %s (Rev %d)! Đang khởi động lại dịch vụ r3p-api...", newVer.Version, newVer.Revision))

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"updated": true,
		"current": newVer,
		"logs":    getFwLogs(),
		"message": fmt.Sprintf("Đã cập nhật hệ thống từ GitHub lên phiên bản %s (Rev %d) thành công! Dịch vụ đang tự động khởi động lại...", newVer.Version, newVer.Revision),
	})

	go func() {
		time.Sleep(1500 * time.Millisecond)
		_, _ = runCmd(10, "/etc/init.d/r3p-api", "restart")
	}()
}

func fileSHA256(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

// POST /api/v1/system/firmware/upload
func handleFirmwareManualUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 48<<20) // up to 48MB
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "Không thể đọc file tải lên (tối đa 48MB): "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, "Vui lòng chọn file Firmware (.bin, .tar.gz hoặc r3p-api)")
		return
	}
	defer file.Close()

	_ = os.Remove(stagedFwPath)
	out, err := os.OpenFile(stagedFwPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Không thể tạo file tạm trong /tmp: "+err.Error())
		return
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hashherWriter{hasher}), file)
	out.Close()
	if copyErr != nil {
		_ = os.Remove(stagedFwPath)
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi khi ghi file firmware: "+copyErr.Error())
		return
	}

	sha := hex.EncodeToString(hasher.Sum(nil))
	fname := filepath.Base(header.Filename)
	lower := strings.ToLower(fname)

	formatType := "sysupgrade_bin"
	valid := true
	checkMsg := "File đã sẵn sàng để cập nhật."

	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		formatType = "os_bundle_targz"
		entryCount, sErr := inspectTarGzSafety(stagedFwPath, false, []string{"usr/", "www/", "etc/", "sbin/", "lib/"})
		if sErr != nil {
			valid = false
			checkMsg = "Gói .tar.gz không hợp lệ hoặc không an toàn: " + sErr.Error()
		} else {
			valid = true
			checkMsg = fmt.Sprintf("Gói cập nhật R3P OS Bundle hợp lệ (%d entries đã kiểm tra an toàn).", entryCount)
		}
	} else if lower == "r3p-api" || strings.HasPrefix(lower, "r3p-api") {
		formatType = "r3p_api_binary"
		valid = written > 1024*1024 && isELFBinary(stagedFwPath)
		if valid {
			checkMsg = "Binary ELF r3p-api (MIPS Little-Endian) sẵn sàng cập nhật nóng."
		} else {
			checkMsg = "File binary không hợp lệ (yêu cầu ELF executable > 1MB)."
		}
	} else {
		formatType = "sysupgrade_bin"
		chkOut, chkErr := runCmd(10, "sysupgrade", "-T", stagedFwPath)
		valid = chkErr == nil
		if valid {
			checkMsg = "Kiểm tra sysupgrade -T: Hợp lệ cho Xiaomi Mi Router 3 Pro (mir3p)."
		} else {
			checkMsg = "Cảnh báo sysupgrade -T: " + chkOut
		}
	}

	meta := StagedFirmwareInfo{
		Present:    true,
		Filename:   fname,
		Path:       stagedFwPath,
		SizeBytes:  written,
		SHA256:     sha,
		FormatType: formatType,
		Valid:      valid,
		CheckMsg:   checkMsg,
		UploadedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(stagedMetaPath, b, 0644)
	}

	appendFwLog(fmt.Sprintf("Đã tải lên thủ công '%s' (%.2f MB, Loại: %s, SHA256: %s)", fname, float64(written)/(1024*1024), formatType, shortSHA(sha)))

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"staged":  meta,
		"logs":    getFwLogs(),
		"message": fmt.Sprintf("Đã tải lên '%s' thành công! Kiểm tra thông tin bên dưới và bấm 'Áp Dụng / Flash Ngay'.", fname),
	})
}

type hashherWriter struct {
	w io.Writer
}

func (h hashherWriter) Write(p []byte) (int, error) {
	return h.w.Write(p)
}

// POST /api/v1/system/firmware/apply
func handleFirmwareApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		KeepSettings bool `json:"keep_settings"`
		Force        bool `json:"force"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body)

	staged := loadStagedFirmwareInfo()
	if !staged.Present {
		writeJSON(w, http.StatusBadRequest, false, "Chưa có file Firmware/Bundle nào được tải lên")
		return
	}
	if staged.SHA256 != "" && !strings.EqualFold(fileSHA256(staged.Path), staged.SHA256) {
		writeJSON(w, http.StatusBadRequest, false, "File tạm trên RAM không khớp mã SHA256 ban đầu — vui lòng tải lại file.")
		return
	}
	if !staged.Valid && !body.Force {
		writeJSON(w, http.StatusBadRequest, false, "File chưa vượt qua kiểm tra hợp lệ ("+staged.CheckMsg+"). Bật tùy chọn Force Flash nếu bạn chắc chắn.")
		return
	}

	switch staged.FormatType {
	case "os_bundle_targz":
		if _, safetyErr := inspectTarGzSafety(staged.Path, false, []string{"usr/", "www/", "etc/", "sbin/", "lib/"}); safetyErr != nil && !body.Force {
			writeJSON(w, http.StatusBadRequest, false, "Gói OS Bundle không an toàn để giải nén: "+safetyErr.Error())
			return
		}
		appendFwLog("Đang giải nén gói R3P OS Bundle thủ công (" + staged.Filename + ") vào hệ thống...")
		out, err := runCmd(35, "tar", "-xzf", staged.Path, "-C", "/")
		if err != nil {
			appendFwLog("Lỗi giải nén: " + err.Error() + " " + out)
			writeJSON(w, http.StatusInternalServerError, false, "Giải nén gói cập nhật thất bại: "+err.Error())
			return
		}
		_ = os.Chmod("/usr/bin/r3p-api", 0755)
		_ = os.Remove(staged.Path)
		_ = os.Remove(stagedMetaPath)
		_, _ = runCmd(4, "sync")
		appendFwLog("Đã cập nhật R3P OS Bundle thành công! Đang khởi động lại r3p-api...")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"applied": true,
			"mode":    "os_bundle_targz",
			"logs":    getFwLogs(),
			"message": "Đã áp dụng gói cập nhật R3P OS Bundle thành công! Dịch vụ đang tự động khởi động lại...",
		})
		go func() {
			time.Sleep(1500 * time.Millisecond)
			_, _ = runCmd(10, "/etc/init.d/r3p-api", "restart")
		}()

	case "r3p_api_binary":
		if !isELFBinary(staged.Path) && !body.Force {
			writeJSON(w, http.StatusBadRequest, false, "File tải lên không phải binary ELF hợp lệ")
			return
		}
		appendFwLog("Đang cập nhật binary /usr/bin/r3p-api từ file tải lên...")
		_ = os.Chmod(staged.Path, 0755)
		if err := os.Rename(staged.Path, "/usr/bin/r3p-api"); err != nil {
			_, cpErr := runCmd(10, "cp", "-f", staged.Path, "/usr/bin/r3p-api")
			if cpErr != nil {
				writeJSON(w, http.StatusInternalServerError, false, "Không thể ghi đè /usr/bin/r3p-api: "+cpErr.Error())
				return
			}
			_ = os.Chmod("/usr/bin/r3p-api", 0755)
		}
		_ = os.Remove(staged.Path)
		_ = os.Remove(stagedMetaPath)
		_, _ = runCmd(4, "sync")
		appendFwLog("Đã cập nhật /usr/bin/r3p-api thành công! Đang khởi động lại dịch vụ...")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"applied": true,
			"mode":    "r3p_api_binary",
			"logs":    getFwLogs(),
			"message": "Đã cập nhật /usr/bin/r3p-api thành công! Dịch vụ đang tự động khởi động lại...",
		})
		go func() {
			time.Sleep(1500 * time.Millisecond)
			_, _ = runCmd(10, "/etc/init.d/r3p-api", "restart")
		}()

	case "sysupgrade_bin":
		if !staged.Valid && !body.Force {
			writeJSON(w, http.StatusBadRequest, false, "File Firmware chưa vượt qua kiểm tra sysupgrade -T. Tích chọn 'Force Flash' nếu bạn chắc chắn đây là bản build cho R3P.")
			return
		}
		var args []string
		if !body.KeepSettings {
			args = append(args, "-n")
		}
		if body.Force {
			args = append(args, "-F")
		}
		args = append(args, staged.Path)
		appendFwLog(fmt.Sprintf("Đang tiến hành Flash NAND Firmware: sysupgrade %s...", strings.Join(args, " ")))
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"applied": true,
			"mode":    "sysupgrade_bin",
			"logs":    getFwLogs(),
			"message": "Router đang tiến hành nạp Firmware mới vào NAND Flash và sẽ tự khởi động lại sau ~60-90 giây!",
		})
		go func(sysArgs []string) {
			time.Sleep(1500 * time.Millisecond)
			_, _ = runCmd(4, "sync")
			_, _ = runCmd(120, "sysupgrade", sysArgs...)
		}(args)
	}
}
