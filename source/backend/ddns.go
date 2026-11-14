package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	publicIPMu         sync.Mutex
	cachedPublicIP     string
	cachedPublicIPAt   time.Time
	publicIPRefreshing bool
)

func getCachedPublicIPv4() string {
	publicIPMu.Lock()
	ip := cachedPublicIP
	stale := ip == "" || time.Since(cachedPublicIPAt) >= 60*time.Second
	shouldRefresh := stale && !publicIPRefreshing
	if shouldRefresh {
		publicIPRefreshing = true
	}
	publicIPMu.Unlock()

	if shouldRefresh {
		go func() {
			defer func() {
				publicIPMu.Lock()
				publicIPRefreshing = false
				publicIPMu.Unlock()
			}()
			_ = fetchPublicIPv4()
		}()
	}
	return ip
}

func maskDDNSToken(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return ""
	}
	if len(tok) <= 4 {
		return "••••"
	}
	return "••••" + tok[len(tok)-4:]
}

func sanitizePublicDDNS(cfg DDNSConfig) DDNSConfig {
	if cfg.Token != "" {
		cfg.Token = maskDDNSToken(cfg.Token)
	}
	return cfg
}

// ─── Dynamic DNS (DDNS) Manager (DuckDNS.org, Cloudflare, No-IP, Custom) ───

const ddnsConfigFile = "/etc/r3p/ddns.json"

type DDNSConfig struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"` // "duckdns", "cloudflare", "noip", "dyndns", "freedns", "custom"
	Domain        string `json:"domain"`   // e.g. "myhome" or "myhome.duckdns.org"
	Token         string `json:"token"`    // API token or secret key
	Username      string `json:"username"` // For No-IP / DynDNS / Cloudflare email
	CustomURL     string `json:"custom_url"`
	CheckInterval int    `json:"check_interval"` // Minutes (default 10)
	LastIP        string `json:"last_ip"`
	LastUpdate    string `json:"last_update"`
	LastStatus    string `json:"last_status"`
}

var (
	ddnsMu     sync.Mutex
	ddnsOpMu   sync.Mutex
	ddnsCached DDNSConfig
)

func defaultDDNSConfig() DDNSConfig {
	return DDNSConfig{
		Enabled:       false,
		Provider:      "duckdns",
		Domain:        "",
		Token:         "",
		Username:      "",
		CustomURL:     "",
		CheckInterval: 10,
		LastIP:        "",
		LastUpdate:    "Chưa cập nhật",
		LastStatus:    "Chưa kích hoạt",
	}
}

func loadDDNSConfig() DDNSConfig {
	ddnsMu.Lock()
	defer ddnsMu.Unlock()

	cfg := defaultDDNSConfig()
	settingsDBLoadJSON(bucketSettings, "ddns", ddnsConfigFile, &cfg)
	if cfg.CheckInterval < 5 || cfg.CheckInterval > 1440 {
		cfg.CheckInterval = 10
	}
	if cfg.Provider == "" {
		cfg.Provider = "duckdns"
	}
	ddnsCached = cfg
	return cfg
}

func saveDDNSConfig(cfg DDNSConfig) error {
	ddnsMu.Lock()
	defer ddnsMu.Unlock()

	err := settingsDBSaveJSON(bucketSettings, "ddns", ddnsConfigFile, 0600, cfg)
	if err == nil {
		ddnsCached = cfg
	}
	return err
}

// Fetch current external public IPv4 address (60s TTL cache avoids blocking UI page loads)
func fetchPublicIPv4() string {
	publicIPMu.Lock()
	if cachedPublicIP != "" && time.Since(cachedPublicIPAt) < 60*time.Second {
		ip := cachedPublicIP
		publicIPMu.Unlock()
		return ip
	}
	publicIPMu.Unlock()

	endpoints := []string{
		"https://api.ipify.org",
		"https://checkip.amazonaws.com",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}
	client := newPublicHTTPClient(publicClientOptions{
		Timeout:     3 * time.Second,
		DialTimeout: 3 * time.Second,
	})
	for _, ep := range endpoints {
		resp, err := client.Get(ep)
		if err == nil && resp.StatusCode == http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
			resp.Body.Close()
			ip := strings.TrimSpace(string(b))
			if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
				v4 := parsed.To4().String()
				publicIPMu.Lock()
				cachedPublicIP = v4
				cachedPublicIPAt = time.Now()
				publicIPMu.Unlock()
				return v4
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	return ""
}

// Perform DDNS update for DuckDNS or other providers
func performDDNSUpdate(cfg *DDNSConfig) (bool, string) {
	ddnsOpMu.Lock()
	defer ddnsOpMu.Unlock()
	return performDDNSUpdateLocked(cfg)
}

func performDDNSUpdateLocked(cfg *DDNSConfig) (bool, string) {
	cfg.Domain = strings.TrimSpace(cfg.Domain)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.CustomURL = strings.TrimSpace(cfg.CustomURL)

	recordFail := func(msg string) (bool, string) {
		cfg.LastStatus = msg
		cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
		_ = saveDDNSConfig(*cfg)
		return false, msg
	}

	if cfg.Domain == "" {
		return recordFail("Vui lòng nhập tên miền (Domain / Subdomain)")
	}
	if cfg.Token == "" && cfg.Provider != "custom" {
		return recordFail("Vui lòng nhập Token / API Key")
	}

	pubIP := fetchPublicIPv4()
	if pubIP == "" {
		return recordFail("Không thể xác định IP công cộng của Router (Kiểm tra kết nối Internet)")
	}

	redactToken := func(s string) string {
		if cfg.Token == "" {
			return s
		}
		s = strings.ReplaceAll(s, cfg.Token, "[REDACTED]")
		s = strings.ReplaceAll(s, url.QueryEscape(cfg.Token), "[REDACTED]")
		return s
	}

	client := newPublicHTTPClient(publicClientOptions{
		Timeout:   10 * time.Second,
		AllowHTTP: true,
	})
	provider := strings.ToLower(cfg.Provider)

	switch provider {
	case "duckdns":
		// DuckDNS requires subdomain only (e.g. "myhome" for "myhome.duckdns.org")
		subdomain := cfg.Domain
		subdomain = strings.TrimSuffix(subdomain, ".duckdns.org")
		subdomain = strings.TrimPrefix(subdomain, "http://")
		subdomain = strings.TrimPrefix(subdomain, "https://")
		subdomain = strings.TrimSpace(subdomain)
		token := strings.TrimSpace(cfg.Token)

		updateURL := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s&ip=%s",
			url.QueryEscape(subdomain), url.QueryEscape(token), url.QueryEscape(pubIP))

		req, err := http.NewRequest(http.MethodGet, updateURL, nil)
		if err != nil {
			return recordFail("Lỗi tạo request DuckDNS: " + redactToken(err.Error()))
		}
		req.Header.Set("User-Agent", "R3P-DDNS/1.2")

		resp, err := client.Do(req)
		if err != nil {
			return recordFail("Lỗi kết nối DuckDNS: " + redactToken(err.Error()))
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		reply := strings.TrimSpace(string(body))
		if strings.EqualFold(reply, "OK") {
			msg := fmt.Sprintf("Cập nhật DuckDNS thành công! IP: %s (Tên miền: %s.duckdns.org)", pubIP, subdomain)
			cfg.LastIP = pubIP
			cfg.LastStatus = "Thành công (OK)"
			cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			_ = saveDDNSConfig(*cfg)
			return true, msg
		}
		return recordFail(fmt.Sprintf("DuckDNS phản hồi thất bại (KO) - Kiểm tra Token và Domain: %s", redactToken(reply)))

	case "noip", "dyndns":
		if cfg.Username == "" {
			return recordFail(fmt.Sprintf("Vui lòng nhập Username tài khoản %s", strings.ToUpper(provider)))
		}
		epHost := "dynupdate.no-ip.com"
		label := "No-IP"
		if provider == "dyndns" {
			epHost = "members.dyndns.org"
			label = "DynDNS"
		}
		updateURL := fmt.Sprintf("https://%s/nic/update?hostname=%s&myip=%s",
			epHost, url.QueryEscape(cfg.Domain), url.QueryEscape(pubIP))
		req, err := http.NewRequest(http.MethodGet, updateURL, nil)
		if err != nil {
			return recordFail(fmt.Sprintf("Lỗi tạo request %s: %s", label, redactToken(err.Error())))
		}
		req.SetBasicAuth(cfg.Username, cfg.Token)
		req.Header.Set("User-Agent", "R3P-Home-Server/1.1")
		resp, err := client.Do(req)
		if err != nil {
			return recordFail(fmt.Sprintf("Lỗi kết nối %s: %s", label, redactToken(err.Error())))
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		reply := strings.TrimSpace(string(body))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && (strings.HasPrefix(reply, "good") || strings.HasPrefix(reply, "nochg")) {
			msg := fmt.Sprintf("Cập nhật %s thành công! IP: %s", label, pubIP)
			cfg.LastIP = pubIP
			cfg.LastStatus = fmt.Sprintf("Thành công (%s)", label)
			cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			_ = saveDDNSConfig(*cfg)
			return true, msg
		}
		return recordFail(fmt.Sprintf("%s phản hồi lỗi (HTTP %d): %s", label, resp.StatusCode, redactToken(reply)))

	case "custom":
		rawURL := cfg.CustomURL
		if rawURL == "" {
			return recordFail("Vui lòng nhập URL cập nhật tùy chỉnh")
		}
		rawURL = strings.ReplaceAll(rawURL, "[DOMAIN]", url.QueryEscape(cfg.Domain))
		rawURL = strings.ReplaceAll(rawURL, "[TOKEN]", url.QueryEscape(cfg.Token))
		rawURL = strings.ReplaceAll(rawURL, "[IP]", url.QueryEscape(pubIP))
		rawURL = strings.ReplaceAll(rawURL, "[USERNAME]", url.QueryEscape(cfg.Username))
		if err := validatePublicHTTPURL(context.Background(), rawURL); err != nil {
			return recordFail("Custom DDNS URL không hợp lệ hoặc trỏ tới IP nội bộ: " + redactToken(err.Error()))
		}

		resp, err := client.Get(rawURL)
		if err != nil {
			return recordFail("Lỗi gọi Custom DDNS URL: " + redactToken(err.Error()))
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		cfg.LastStatus = fmt.Sprintf("HTTP %d (Custom)", resp.StatusCode)
		cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = saveDDNSConfig(*cfg)
			return false, fmt.Sprintf("Custom DDNS trả về lỗi HTTP %d", resp.StatusCode)
		}
		cfg.LastIP = pubIP
		_ = saveDDNSConfig(*cfg)
		return true, fmt.Sprintf("Cập nhật Custom DDNS thành công (HTTP %d, IP: %s)", resp.StatusCode, pubIP)

	default:
		return recordFail(fmt.Sprintf("Nhà cung cấp '%s' chưa được hỗ trợ — vui lòng dùng duckdns, noip, dyndns hoặc custom", cfg.Provider))
	}
}

// Background DDNS periodic runner
func initDDNSBackgroundRunner() {
	go func() {
		time.Sleep(90 * time.Second)
		for {
			ddnsOpMu.Lock()
			cfg := loadDDNSConfig()
			if cfg.Enabled && cfg.Domain != "" && (cfg.Token != "" || cfg.Provider == "custom") {
				currentIP := fetchPublicIPv4()
				if currentIP != "" && currentIP != cfg.LastIP {
					_, _ = performDDNSUpdateLocked(&cfg)
				}
			}
			ddnsOpMu.Unlock()
			interval := cfg.CheckInterval
			if interval < 5 {
				interval = 10
			}
			time.Sleep(time.Duration(interval) * time.Minute)
		}
	}()
}

// GET /api/v1/access/ddns & PUT /api/v1/access/ddns
func handleAccessDDNS(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg := sanitizePublicDDNS(loadDDNSConfig())
		extIP := getCachedPublicIPv4()
		if r.URL.Query().Get("refresh") == "1" {
			extIP = fetchPublicIPv4()
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"ddns":        cfg,
			"external_ip": extIP,
			"providers": []map[string]string{
				{"id": "duckdns", "name": "DuckDNS.org (Ưu tiên / Miễn phí trọn đời)"},
				{"id": "noip", "name": "No-IP (noip.com)"},
				{"id": "dyndns", "name": "DynDNS (Dyn.com)"},
				{"id": "custom", "name": "Custom URL (Tùy chỉnh máy chủ)"},
			},
		})
	case http.MethodPut:
		var req struct {
			Enabled       *bool   `json:"enabled"`
			Provider      *string `json:"provider"`
			Domain        *string `json:"domain"`
			Token         *string `json:"token"`
			Username      *string `json:"username"`
			CustomURL     *string `json:"custom_url"`
			CheckInterval *int    `json:"check_interval"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Dữ liệu JSON không hợp lệ: "+err.Error())
			return
		}

		ddnsOpMu.Lock()
		defer ddnsOpMu.Unlock()

		body := loadDDNSConfig()
		if req.Enabled != nil {
			body.Enabled = *req.Enabled
		}
		if req.Provider != nil && strings.TrimSpace(*req.Provider) != "" {
			body.Provider = strings.ToLower(strings.TrimSpace(*req.Provider))
		}
		if req.Domain != nil {
			body.Domain = strings.TrimSpace(*req.Domain)
		}
		if req.Token != nil {
			tok := strings.TrimSpace(*req.Token)
			if !strings.HasPrefix(tok, "••••") {
				body.Token = tok
			}
		}
		if req.Username != nil {
			body.Username = strings.TrimSpace(*req.Username)
		}
		if req.CustomURL != nil {
			body.CustomURL = strings.TrimSpace(*req.CustomURL)
		}
		if req.CheckInterval != nil {
			ci := *req.CheckInterval
			if ci < 5 {
				ci = 5
			} else if ci > 1440 {
				ci = 1440
			}
			body.CheckInterval = ci
		} else if body.CheckInterval < 5 || body.CheckInterval > 1440 {
			body.CheckInterval = 10
		}

		if err := saveDDNSConfig(body); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi lưu cấu hình DDNS: "+err.Error())
			return
		}
		// If user enabled it and provided domain/token, run an immediate update
		msg := "Đã lưu cài đặt DDNS thành công!"
		if body.Enabled && body.Domain != "" && (body.Token != "" || body.Provider == "custom") {
			ok, updateMsg := performDDNSUpdateLocked(&body)
			if ok {
				msg = updateMsg
			} else {
				msg = "Đã lưu cài đặt, nhưng kiểm tra IP: " + updateMsg
			}
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": msg,
			"ddns":    sanitizePublicDDNS(loadDDNSConfig()),
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "Method not allowed")
	}
}

// POST /api/v1/access/ddns/update (Force immediate sync)
func handleAccessDDNSUpdate(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "Method not allowed")
		return
	}
	ddnsOpMu.Lock()
	defer ddnsOpMu.Unlock()
	cfg := loadDDNSConfig()
	ok, msg := performDDNSUpdateLocked(&cfg)
	if ok {
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": msg,
			"ddns":    sanitizePublicDDNS(cfg),
		})
	} else {
		writeJSON(w, http.StatusBadRequest, false, msg)
	}
}
