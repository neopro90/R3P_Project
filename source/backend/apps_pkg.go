package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ─── OpenWrt Package Manager & GitHub Repository Integration ───────────────

type AppRepoItem struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	URL         string          `json:"url"`
	Type        string          `json:"type"` // "github" or "apk_feed"
	Branch      string          `json:"branch"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	PkgCount    int             `json:"pkg_count"`
	LastSynced  string          `json:"last_synced"`
	Packages    []OpenWrtPkgApp `json:"packages,omitempty"`
}

type OpenWrtPkgApp struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Source      string `json:"source"`       // "OpenWrt Official", "GitHub: owner/repo", etc.
	DownloadURL string `json:"download_url"` // optional direct GitHub asset URL
	Installed   bool   `json:"installed"`
}

const appReposConfigFile = "/etc/r3p/app_repos.json"

var safePkgNamePattern = regexp.MustCompile(`^[a-zA-Z0-9+_.-]{1,80}$`)

var curatedOpenWrtCatalog = []OpenWrtPkgApp{
	{Name: "htop", Version: "3.3.0", Category: "System Tools", Description: "Interactive process viewer & CPU/RAM monitor for Linux terminal", Source: "OpenWrt Official"},
	{Name: "nano", Version: "8.2", Category: "System Tools", Description: "User-friendly terminal text editor for editing configuration files", Source: "OpenWrt Official"},
	{Name: "tmux", Version: "3.5a", Category: "System Tools", Description: "Terminal multiplexer for persistent background shell sessions", Source: "OpenWrt Official"},
	{Name: "jq", Version: "1.7.1", Category: "System Tools", Description: "Lightweight and flexible command-line JSON processor", Source: "OpenWrt Official"},
	{Name: "iperf3", Version: "3.17.1", Category: "Network & Benchmark", Description: "TCP/UDP network bandwidth measurement and throughput benchmark tool", Source: "OpenWrt Official"},
	{Name: "tcpdump", Version: "4.99.5", Category: "Network & Benchmark", Description: "Command-line packet analyzer and network traffic sniffer", Source: "OpenWrt Official"},
	{Name: "mtr", Version: "0.95", Category: "Network & Benchmark", Description: "Full network diagnostic tool combining traceroute and ping", Source: "OpenWrt Official"},
	{Name: "nmap", Version: "7.95", Category: "Network & Benchmark", Description: "Network exploration tool and security / port scanner", Source: "OpenWrt Official"},
	{Name: "socat", Version: "1.8.0", Category: "Network & Benchmark", Description: "Multipurpose relay for bidirectional data transfer between network channels", Source: "OpenWrt Official"},
	{Name: "bind-dig", Version: "9.20", Category: "Network & Benchmark", Description: "DNS lookup utility (dig) for testing DNS servers and records", Source: "OpenWrt Official"},
	{Name: "wireguard-tools", Version: "1.0.20210914", Category: "VPN & Tunnel", Description: "Next-generation high-speed kernel VPN tunnel utilities (wg, wg-quick)", Source: "OpenWrt Official"},
	{Name: "openvpn-openssl", Version: "2.6.12", Category: "VPN & Tunnel", Description: "Open source SSL/TLS VPN daemon for client and server connections", Source: "OpenWrt Official"},
	{Name: "tailscale", Version: "1.76", Category: "VPN & Tunnel", Description: "Zero-config mesh VPN built on WireGuard for remote LAN access", Source: "OpenWrt Official"},
	{Name: "zerotier", Version: "1.14", Category: "VPN & Tunnel", Description: "Global virtual Ethernet switch and SD-WAN mesh network client", Source: "OpenWrt Official"},
	{Name: "ksmbd-server", Version: "3.5.2", Category: "NAS & Storage", Description: "High-speed In-Kernel SMB3 / CIFS file sharing server for USB 3.0 NAS", Source: "OpenWrt Official"},
	{Name: "samba4-server", Version: "4.20", Category: "NAS & Storage", Description: "Full Samba 4 Windows SMB/CIFS network file sharing suite", Source: "OpenWrt Official"},
	{Name: "rsync", Version: "3.3.0", Category: "NAS & Storage", Description: "Fast incremental file transfer and backup synchronization utility", Source: "OpenWrt Official"},
	{Name: "hd-idle", Version: "1.05", Category: "NAS & Storage", Description: "Spins down external USB 3.0 hard disks after idle timeout to save power", Source: "OpenWrt Official"},
	{Name: "smartmontools", Version: "7.4", Category: "NAS & Storage", Description: "S.M.A.R.T. disk health monitoring and temperature inspection utility", Source: "OpenWrt Official"},
	{Name: "exfat-mkfs", Version: "1.2.5", Category: "NAS & Storage", Description: "Utilities to create and format exFAT filesystems on USB drives", Source: "OpenWrt Official"},
	{Name: "dosfstools", Version: "4.2", Category: "NAS & Storage", Description: "Utilities for making and checking MS-DOS FAT16/FAT32 filesystems (mkfs.vfat)", Source: "OpenWrt Official"},
	{Name: "yt-dlp", Version: "2026.10.21", Category: "Media & Download", Description: "Feature-rich audio/video downloader supporting YouTube, TikTok & 1000+ sites", Source: "OpenWrt Official"},
	{Name: "aria2", Version: "1.37.0", Category: "Media & Download", Description: "Lightweight multi-protocol & multi-source HTTP/FTP/BitTorrent download engine", Source: "OpenWrt Official"},
	{Name: "mosquitto-ssl", Version: "2.0.20", Category: "IoT & Smart Home", Description: "Lightweight open-source MQTT v3.1/v5.0 message broker for Smart Home & IoT", Source: "OpenWrt Official"},
	{Name: "mosquitto-client-ssl", Version: "2.0.20", Category: "IoT & Smart Home", Description: "MQTT command-line publish/subscribe clients (mosquitto_pub, mosquitto_sub)", Source: "OpenWrt Official"},
	{Name: "miniupnpd", Version: "2.3.7", Category: "Firewall & Routing", Description: "Lightweight UPnP IGD & NAT-PMP / PCP daemon for automatic port forwarding", Source: "OpenWrt Official"},
	{Name: "SQM-scripts", Version: "1.6.0", Category: "Firewall & Routing", Description: "Smart Queue Management (CAKE / FQ_CoDel) for anti-bufferbloat & low latency", Source: "OpenWrt Official"},
	{Name: "ddns-scripts", Version: "2.8.2", Category: "Firewall & Routing", Description: "Dynamic DNS client scripts for Cloudflare, DuckDNS, No-IP, etc.", Source: "OpenWrt Official"},
	{Name: "adblock-fast", Version: "1.1.2", Category: "Security & DNS", Description: "Fast DNS-based ad & malware blocker script for dnsmasq", Source: "OpenWrt Official"},
	{Name: "https-dns-proxy", Version: "2024.08", Category: "Security & DNS", Description: "DNS-over-HTTPS (DoH) proxy for encrypted DNS resolution", Source: "OpenWrt Official"},
}

func defaultAppRepos() []AppRepoItem {
	// Read existing /etc/apk/repositories.d/distfeeds.list count
	feedLines := readFileLines("/etc/apk/repositories.d/distfeeds.list", 20)
	feedURL := "https://downloads.openwrt.org/releases/25.12.5/packages/mipsel_24kc/packages/packages.adb"
	for _, l := range feedLines {
		if strings.Contains(l, "packages/packages.adb") {
			feedURL = strings.TrimSpace(l)
			break
		}
	}

	return []AppRepoItem{
		{
			ID:          "repo-openwrt-official",
			Name:        "OpenWrt 25.12 Official Package Feeds (mipsel_24kc)",
			URL:         feedURL,
			Type:        "apk_feed",
			Branch:      "25.12.5",
			Description: "Kho ứng dụng chính thức của OpenWrt (base, packages, luci, routing, telephony)",
			Enabled:     true,
			PkgCount:    6500,
			LastSynced:  time.Now().Format("2006-01-02 15:04"),
		},
		{
			ID:          "repo-github-openwrt-packages",
			Name:        "GitHub: openwrt/packages (Community OpenWrt Apps)",
			URL:         "https://github.com/openwrt/packages",
			Type:        "github",
			Branch:      "master",
			Description: "Kho mã nguồn và gói ứng dụng cộng đồng chính thức của OpenWrt trên GitHub",
			Enabled:     true,
			PkgCount:    len(curatedOpenWrtCatalog),
			LastSynced:  time.Now().Format("2006-01-02 15:04"),
		},
		{
			ID:          "repo-github-r3p-project",
			Name:        "GitHub: neopro90/R3P_Project (R3P Official OS & Apps)",
			URL:         "https://github.com/neopro90/R3P_Project.git",
			Type:        "github",
			Branch:      "main",
			Description: "Kho lưu trữ chính thức của dự án R3P Home Server OS (Firmware, Backend, Plugins, Local Downloader & AI)",
			Enabled:     true,
			PkgCount:    4,
			LastSynced:  time.Now().Format("2006-01-02 15:04"),
			Packages: []OpenWrtPkgApp{
				{
					Name:        "r3p-api",
					Version:     getCurrentSystemVersion().Version,
					Category:    "R3P Core",
					Description: "R3P Home Server OS Core Backend + Native Local Downloader + AI Root Agent",
					Source:      "GitHub: neopro90/R3P_Project",
					DownloadURL: "https://raw.githubusercontent.com/neopro90/R3P_Project/main/build/artifacts/r3p-api",
					Installed:   true,
				},
				{
					Name:        "yt-dlp",
					Version:     "2026.10.21",
					Category:    "Media & Download",
					Description: "Trình tải video YouTube/TikTok/Facebook chạy cục bộ trên R3P Home Server",
					Source:      "GitHub: neopro90/R3P_Project",
					Installed:   true,
				},
			},
		},
	}
}

func loadAppRepos() []AppRepoItem {
	var list []AppRepoItem
	if b, err := os.ReadFile(appReposConfigFile); err == nil {
		if json.Unmarshal(b, &list) == nil && len(list) > 0 {
			return list
		}
	}
	list = defaultAppRepos()
	saveAppRepos(list)
	return list
}

func saveAppRepos(list []AppRepoItem) {
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(appReposConfigFile, b, 0644)
	}
	// Sync custom apk_feed entries to /etc/apk/repositories.d/custom.list
	_ = os.MkdirAll("/etc/apk/repositories.d", 0755)
	var customFeeds []string
	for _, r := range list {
		if r.Enabled && r.Type == "apk_feed" && r.ID != "repo-openwrt-official" {
			customFeeds = append(customFeeds, r.URL)
		}
	}
	if len(customFeeds) > 0 {
		_ = os.WriteFile("/etc/apk/repositories.d/custom.list", []byte(strings.Join(customFeeds, "\n")+"\n"), 0644)
	} else {
		_ = os.Remove("/etc/apk/repositories.d/custom.list")
	}
}

func parseGitHubOwnerRepo(rawURL string) (string, string, bool) {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimPrefix(u, "https://github.com/")
	u = strings.TrimPrefix(u, "http://github.com/")
	u = strings.TrimPrefix(u, "github.com/")
	parts := strings.Split(u, "/")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func inspectGitHubRepoPackages(owner, repo, branch string) (string, int, []OpenWrtPkgApp) {
	client := &http.Client{Timeout: 10 * time.Second}
	desc := fmt.Sprintf("GitHub OpenWrt Repository (%s/%s)", owner, repo)
	var pkgs []OpenWrtPkgApp

	// 1. Fetch repository metadata
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo), nil)
	if err == nil {
		req.Header.Set("User-Agent", "R3P-Home-Server-OS")
		if resp, err := client.Do(req); err == nil {
			var meta struct {
				Description   string `json:"description"`
				DefaultBranch string `json:"default_branch"`
			}
			if json.NewDecoder(resp.Body).Decode(&meta) == nil {
				if meta.Description != "" {
					desc = meta.Description
				}
				if meta.DefaultBranch != "" && branch == "" {
					branch = meta.DefaultBranch
				}
			}
			resp.Body.Close()
		}
	}
	if branch == "" {
		branch = "main"
	}

	// 2. Check GitHub Releases for .apk / .ipk / .bin / .tar.gz assets
	relReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=5", owner, repo), nil)
	if err == nil {
		relReq.Header.Set("User-Agent", "R3P-Home-Server-OS")
		if resp, err := client.Do(relReq); err == nil {
			var releases []struct {
				TagName string `json:"tag_name"`
				Name    string `json:"name"`
				Assets  []struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
					Size               int64  `json:"size"`
				} `json:"assets"`
			}
			if json.NewDecoder(resp.Body).Decode(&releases) == nil {
				for _, rel := range releases {
					for _, a := range rel.Assets {
						lower := strings.ToLower(a.Name)
						if strings.HasSuffix(lower, ".apk") || strings.HasSuffix(lower, ".ipk") || strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".tar.gz") {
							pkgs = append(pkgs, OpenWrtPkgApp{
								Name:        a.Name,
								Version:     rel.TagName,
								Category:    "GitHub Release",
								Description: fmt.Sprintf("Release asset from %s/%s (%s)", owner, repo, rel.TagName),
								Source:      fmt.Sprintf("GitHub: %s/%s", owner, repo),
								DownloadURL: a.BrowserDownloadURL,
							})
						}
					}
				}
			}
			resp.Body.Close()
		}
	}

	// 3. Also check root directory contents for OpenWrt packages or scripts
	treeReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s/contents", owner, repo), nil)
	if err == nil {
		treeReq.Header.Set("User-Agent", "R3P-Home-Server-OS")
		if resp, err := client.Do(treeReq); err == nil {
			var items []struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				Type        string `json:"type"`
				DownloadURL string `json:"download_url"`
			}
			if json.NewDecoder(resp.Body).Decode(&items) == nil {
				for _, it := range items {
					lower := strings.ToLower(it.Name)
					if it.Type == "file" && (strings.HasSuffix(lower, ".apk") || strings.HasSuffix(lower, ".ipk") || strings.HasSuffix(lower, ".sh")) {
						pkgs = append(pkgs, OpenWrtPkgApp{
							Name:        it.Name,
							Version:     branch,
							Category:    "GitHub Package",
							Description: fmt.Sprintf("File %s in %s/%s", it.Path, owner, repo),
							Source:      fmt.Sprintf("GitHub: %s/%s", owner, repo),
							DownloadURL: it.DownloadURL,
						})
					} else if it.Type == "dir" && (strings.HasPrefix(lower, "luci-app-") || strings.HasPrefix(lower, "luci-theme-")) {
						pkgs = append(pkgs, OpenWrtPkgApp{
							Name:        it.Name,
							Version:     "latest",
							Category:    "OpenWrt LuCI App",
							Description: fmt.Sprintf("OpenWrt package %s from %s/%s", it.Name, owner, repo),
							Source:      fmt.Sprintf("GitHub: %s/%s", owner, repo),
						})
					}
				}
			}
			resp.Body.Close()
		}
	}

	// If repo itself is named like an OpenWrt package (e.g. luci-app-*, openwrt-*), add it as a searchable package entry
	if len(pkgs) == 0 {
		pkgs = append(pkgs, OpenWrtPkgApp{
			Name:        repo,
			Version:     branch,
			Category:    "GitHub Repo App",
			Description: desc,
			Source:      fmt.Sprintf("GitHub: %s/%s", owner, repo),
			DownloadURL: fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		})
	}

	return desc, len(pkgs), pkgs
}

func getInstalledPkgSet() (map[string]string, []string) {
	out, err := runCmd(6, "apk", "info", "-v")
	if err != nil || out == "" {
		out, _ = runCmd(6, "opkg", "list-installed")
	}
	installedMap := map[string]string{}
	var rawList []string
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "WARNING:") {
			continue
		}
		rawList = append(rawList, t)
		// Parse apk info -v format: e.g. "yt-dlp-2026.10.21-r1" -> name="yt-dlp", ver="2026.10.21-r1"
		parts := strings.Split(t, "-")
		if len(parts) >= 2 {
			// Find first segment starting with a digit
			splitIdx := -1
			for i := 1; i < len(parts); i++ {
				if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
					splitIdx = i
					break
				}
			}
			if splitIdx > 0 {
				name := strings.Join(parts[:splitIdx], "-")
				ver := strings.Join(parts[splitIdx:], "-")
				installedMap[name] = ver
			} else {
				installedMap[t] = "installed"
			}
		} else {
			installedMap[t] = "installed"
		}
	}
	sort.Strings(rawList)
	return installedMap, rawList
}

// GET / POST / DELETE /api/v1/packages/repos
func handlePackageRepos(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	repos := loadAppRepos()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"repos": repos,
		})

	case http.MethodPost:
		var body struct {
			Name   string `json:"name"`
			URL    string `json:"url"`
			Branch string `json:"branch"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		rawURL := strings.TrimSpace(body.URL)
		if rawURL == "" {
			writeJSON(w, http.StatusBadRequest, false, "Vui lòng nhập URL GitHub Repo hoặc OpenWrt Package Feed")
			return
		}

		repoType := "apk_feed"
		name := strings.TrimSpace(body.Name)
		branch := strings.TrimSpace(body.Branch)
		if branch == "" {
			branch = "main"
		}
		desc := "Custom OpenWrt Package Repository"
		pkgCount := 0
		var discoveredPkgs []OpenWrtPkgApp

		if owner, repo, ok := parseGitHubOwnerRepo(rawURL); ok {
			repoType = "github"
			if name == "" {
				name = fmt.Sprintf("GitHub: %s/%s", owner, repo)
			}
			desc, pkgCount, discoveredPkgs = inspectGitHubRepoPackages(owner, repo, branch)
		} else if name == "" {
			name = "Custom OpenWrt Feed"
		}

		newItem := AppRepoItem{
			ID:          "repo-" + generateGID()[:6],
			Name:        name,
			URL:         rawURL,
			Type:        repoType,
			Branch:      branch,
			Description: desc,
			Enabled:     true,
			PkgCount:    pkgCount,
			LastSynced:  time.Now().Format("2006-01-02 15:04"),
			Packages:    discoveredPkgs,
		}
		repos = append(repos, newItem)
		saveAppRepos(repos)

		writeJSON(w, http.StatusCreated, true, map[string]interface{}{
			"repos":   repos,
			"added":   newItem,
			"message": fmt.Sprintf("Đã thêm kho ứng dụng '%s' (%d gói) thành công!", name, pkgCount),
		})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "repo-openwrt-official" {
			writeJSON(w, http.StatusBadRequest, false, "Không thể xóa kho gói chính thức của OpenWrt")
			return
		}
		var updated []AppRepoItem
		for _, item := range repos {
			if item.ID != id {
				updated = append(updated, item)
			}
		}
		saveAppRepos(updated)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"repos":   updated,
			"message": "Đã xóa kho ứng dụng.",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// GET /api/v1/packages/search?q=...&category=...
func handlePackagesSearch(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	catFilter := strings.TrimSpace(r.URL.Query().Get("category"))

	installedMap, _ := getInstalledPkgSet()
	repos := loadAppRepos()

	seen := map[string]bool{}
	var results []OpenWrtPkgApp

	// 1. Include packages from added GitHub repos first
	for _, repo := range repos {
		if !repo.Enabled {
			continue
		}
		for _, p := range repo.Packages {
			if query != "" && !strings.Contains(strings.ToLower(p.Name), query) && !strings.Contains(strings.ToLower(p.Description), query) {
				continue
			}
			if catFilter != "" && catFilter != "all" && p.Category != catFilter {
				continue
			}
			_, inst := installedMap[p.Name]
			p.Installed = inst || p.Installed
			seen[p.Name] = true
			results = append(results, p)
		}
	}

	// 2. Include Curated OpenWrt Catalog matches
	for _, item := range curatedOpenWrtCatalog {
		if seen[item.Name] {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name), query) && !strings.Contains(strings.ToLower(item.Description), query) && !strings.Contains(strings.ToLower(item.Category), query) {
			continue
		}
		if catFilter != "" && catFilter != "all" && item.Category != catFilter {
			continue
		}
		c := item
		if ver, ok := installedMap[c.Name]; ok {
			c.Installed = true
			if ver != "installed" {
				c.Version = ver
			}
		}
		seen[c.Name] = true
		results = append(results, c)
	}

	// 3. If user typed a search query, also query live `apk search` on the router!
	if query != "" && safePkgNamePattern.MatchString(query) {
		apkOut, _ := runCmd(8, "apk", "search", "-v", query)
		if apkOut == "" {
			apkOut, _ = runCmd(8, "apk", "info", "-a", "*"+query+"*")
		}
		for _, line := range strings.Split(apkOut, "\n") {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "WARNING:") || strings.HasPrefix(t, "fetch ") {
				continue
			}
			// Parse "pkgname-1.2.3-r1 - description" or "pkgname-1.2.3-r1"
			desc := "OpenWrt Official Package (mipsel_24kc)"
			pkgToken := t
			if idx := strings.Index(t, " - "); idx > 0 {
				pkgToken = strings.TrimSpace(t[:idx])
				desc = strings.TrimSpace(t[idx+3:])
			}
			parts := strings.Split(pkgToken, "-")
			name := pkgToken
			ver := "latest"
			for i := 1; i < len(parts); i++ {
				if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
					name = strings.Join(parts[:i], "-")
					ver = strings.Join(parts[i:], "-")
					break
				}
			}
			if seen[name] || name == "kmod-sound-core" || name == "ffmpeg" {
				continue
			}
			seen[name] = true
			_, inst := installedMap[name]
			results = append(results, OpenWrtPkgApp{
				Name:        name,
				Version:     ver,
				Category:    "OpenWrt Feed",
				Description: desc,
				Source:      "OpenWrt Official",
				Installed:   inst,
			})
			if len(results) >= 120 {
				break
			}
		}
	}

	if results == nil {
		results = []OpenWrtPkgApp{}
	}

	categories := []string{
		"System Tools",
		"Network & Benchmark",
		"VPN & Tunnel",
		"NAS & Storage",
		"Media & Download",
		"IoT & Smart Home",
		"Firewall & Routing",
		"Security & DNS",
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"packages":   results,
		"results":    results,
		"categories": categories,
		"count":      len(results),
		"total":      len(results),
	})
}

// POST /api/v1/packages/update-feeds
func handlePackagesUpdateFeeds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}

	out, err := runCmd(35, "apk", "update")
	repos := loadAppRepos()
	for i := range repos {
		if repos[i].Enabled && repos[i].Type == "github" {
			if owner, repo, ok := parseGitHubOwnerRepo(repos[i].URL); ok {
				desc, cnt, pkgs := inspectGitHubRepoPackages(owner, repo, repos[i].Branch)
				repos[i].Description = desc
				if cnt > 0 {
					repos[i].PkgCount = cnt
					repos[i].Packages = pkgs
				}
				repos[i].LastSynced = time.Now().Format("2006-01-02 15:04")
			}
		} else if repos[i].Enabled {
			repos[i].LastSynced = time.Now().Format("2006-01-02 15:04")
		}
	}
	saveAppRepos(repos)

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"updated": err == nil,
		"output":  out,
		"repos":   repos,
		"message": "Đã đồng bộ danh sách gói từ OpenWrt Feeds và các kho GitHub!",
	})
}

var packageAliases = map[string]string{
	"samba":          "samba4-server",
	"samba-server":   "samba4-server",
	"samba-sever":    "samba4-server",
	"smb":            "samba4-server",
	"cifs":           "samba4-server",
	"ftp":            "vsftpd",
	"ftp-server":     "vsftpd",
	"wireguard":      "wireguard-tools",
	"wg":             "wireguard-tools",
	"shadowsocks":    "shadowsocks-libev-ss-server",
	"v2ray":          "v2ray-core",
	"xray":           "xray-core",
	"aria2":          "aria2",
	"iperf":          "iperf3",
	"wget":           "wget-ssl",
}

// POST /api/v1/packages/install
func handlePackagesInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Package     string `json:"package"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}

	pkgName := strings.TrimSpace(body.Package)
	dlURL := strings.TrimSpace(body.DownloadURL)

	// Resolve package aliases (e.g. samba-server -> samba4-server, ftp -> vsftpd)
	lowerPkg := strings.ToLower(pkgName)
	if alias, exists := packageAliases[lowerPkg]; exists {
		pkgName = alias
	}

	// Guard against kmod-sound-core / ffmpeg which causes MT7621 kernel panic
	if strings.Contains(strings.ToLower(pkgName), "kmod-sound") || strings.ToLower(pkgName) == "ffmpeg" || strings.ToLower(pkgName) == "alsa-lib" {
		writeJSON(w, http.StatusBadRequest, false, "Gói này phụ thuộc vào kmod-sound-core gây xung đột phần cứng trên chip MT7621. Hệ thống đã chặn để bảo vệ Router!")
		return
	}

	isDirectBinary := (strings.HasSuffix(strings.ToLower(dlURL), ".apk") ||
		strings.HasSuffix(strings.ToLower(dlURL), ".ipk") ||
		strings.HasSuffix(strings.ToLower(dlURL), ".sh") ||
		strings.Contains(dlURL, "/releases/download/"))

	// Case 1: Direct URL installation from GitHub (.apk, .ipk, .sh)
	if dlURL != "" && isDirectBinary && (strings.HasPrefix(dlURL, "https://") || strings.HasPrefix(dlURL, "http://")) {
		fileName := filepath.Base(dlURL)
		if !safePkgNamePattern.MatchString(fileName) {
			fileName = "github-pkg.apk"
		}
		tmpPath := "/tmp/" + fileName
		_ = os.Remove(tmpPath)
		dlOut, dlErr := runCmd(60, "curl", "-L", "-s", "-o", tmpPath, dlURL)
		if dlErr != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Tải gói từ GitHub thất bại: "+dlErr.Error()+" "+dlOut)
			return
		}
		var instOut string
		var instErr error
		if strings.HasSuffix(strings.ToLower(fileName), ".apk") {
			instOut, instErr = runCmd(60, "apk", "add", "--allow-untrusted", tmpPath)
		} else if strings.HasSuffix(strings.ToLower(fileName), ".sh") {
			_ = os.Chmod(tmpPath, 0755)
			instOut, instErr = runCmd(60, "sh", tmpPath)
		} else {
			instOut = fmt.Sprintf("Đã tải gói %s về %s thành công.", fileName, tmpPath)
		}
		_, _ = runCmd(3, "sync")
		if instErr != nil {
			writeJSON(w, http.StatusInternalServerError, false, fmt.Sprintf("Lỗi khi cài đặt %s:\n%s\n%v", fileName, instOut, instErr))
			return
		}
		_, allPkgs := getInstalledPkgSet()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"installed": true,
			"package":   pkgName,
			"output":    instOut,
			"packages":  allPkgs,
			"message":   fmt.Sprintf("Đã tải và cài đặt '%s' từ GitHub thành công!", pkgName),
		})
		return
	}

	// Case 2: Standard OpenWrt APK package install
	if !safePkgNamePattern.MatchString(pkgName) {
		writeJSON(w, http.StatusBadRequest, false, "Tên gói ứng dụng không hợp lệ")
		return
	}

	out, err := runCmd(60, "apk", "add", pkgName)
	if err != nil && (strings.Contains(out, "UNTRUSTED") || strings.Contains(out, "not found") || strings.Contains(out, "unable to select") || strings.Contains(out, "Unable to select")) {
		_, _ = runCmd(40, "apk", "update")
		out, err = runCmd(60, "apk", "add", "--allow-untrusted", pkgName)
	}
	_, _ = runCmd(3, "sync")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, fmt.Sprintf("Cài đặt gói '%s' thất bại:\n%s\n%v", pkgName, out, err))
		return
	}

	_, allPkgs := getInstalledPkgSet()
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"installed": true,
		"package":   pkgName,
		"output":    out,
		"packages":  allPkgs,
		"message":   fmt.Sprintf("Đã cài đặt ứng dụng '%s' thành công!", pkgName),
	})
}

// POST /api/v1/packages/remove
func handlePackagesRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Package string `json:"package"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	pkgName := strings.TrimSpace(body.Package)
	// Strip version suffix if user passed full "pkgname-1.0-r1"
	installedMap, _ := getInstalledPkgSet()
	if _, ok := installedMap[pkgName]; !ok {
		parts := strings.Split(pkgName, "-")
		for i := 1; i < len(parts); i++ {
			if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
				cand := strings.Join(parts[:i], "-")
				if _, ok2 := installedMap[cand]; ok2 {
					pkgName = cand
					break
				}
			}
		}
	}

	if !safePkgNamePattern.MatchString(pkgName) {
		writeJSON(w, http.StatusBadRequest, false, "Tên gói không hợp lệ")
		return
	}

	protected := map[string]bool{
		"base-files": true, "busybox": true, "dropbear": true, "dnsmasq": true,
		"firewall4": true, "netifd": true, "procd": true, "ubus": true, "uci": true,
		"kmod-mt7615e": true, "wpad-basic-mbedtls": true, "apk-tools": true,
	}
	if protected[pkgName] {
		writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Gói '%s' là thành phần cốt lõi của hệ điều hành, không được phép gỡ bỏ!", pkgName))
		return
	}

	out, err := runCmd(45, "apk", "del", pkgName)
	_, _ = runCmd(3, "sync")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, fmt.Sprintf("Gỡ cài đặt '%s' thất bại:\n%s\n%v", pkgName, out, err))
		return
	}

	_, allPkgs := getInstalledPkgSet()
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"removed":  true,
		"package":  pkgName,
		"output":   out,
		"packages": allPkgs,
		"message":  fmt.Sprintf("Đã gỡ cài đặt '%s' thành công!", pkgName),
	})
}
