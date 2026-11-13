package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

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
	if b, err := os.ReadFile(ddnsConfigFile); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg.CheckInterval < 2 {
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

	_ = os.MkdirAll("/etc/r3p", 0755)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(ddnsConfigFile, b, 0600)
	if err == nil {
		ddnsCached = cfg
	}
	return err
}

// Fetch current external public IPv4 address
func fetchPublicIPv4() string {
	endpoints := []string{
		"https://api.ipify.org",
		"https://checkip.amazonaws.com",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, ep := range endpoints {
		resp, err := client.Get(ep)
		if err == nil && resp.StatusCode == http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
			resp.Body.Close()
			ip := strings.TrimSpace(string(b))
			if net.ParseIP(ip) != nil {
				return ip
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
	cfg.Domain = strings.TrimSpace(cfg.Domain)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.CustomURL = strings.TrimSpace(cfg.CustomURL)

	if cfg.Domain == "" {
		return false, "Vui lòng nhập tên miền (Domain / Subdomain)"
	}
	if cfg.Token == "" && cfg.Provider != "custom" {
		return false, "Vui lòng nhập Token / API Key"
	}

	pubIP := fetchPublicIPv4()
	if pubIP == "" {
		return false, "Không thể xác định IP công cộng của Router (Kiểm tra kết nối Internet)"
	}

	client := &http.Client{Timeout: 10 * time.Second}
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
			msg := "Lỗi tạo request DuckDNS: " + err.Error()
			cfg.LastStatus = msg
			cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			return false, msg
		}
		req.Header.Set("User-Agent", "curl/7.88.1")

		resp, err := client.Do(req)
		if err != nil {
			msg := "Lỗi kết nối DuckDNS: " + err.Error()
			cfg.LastStatus = msg
			cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			return false, msg
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
		msg := fmt.Sprintf("DuckDNS phản hồi thất bại (KO) - Kiểm tra Token và Domain: %s", reply)
		cfg.LastStatus = msg
		cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
		_ = saveDDNSConfig(*cfg)
		return false, msg

	case "noip", "dyndns":
		if cfg.Username == "" {
			return false, fmt.Sprintf("Vui lòng nhập Username tài khoản %s", strings.ToUpper(provider))
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
			return false, err.Error()
		}
		req.SetBasicAuth(cfg.Username, cfg.Token)
		req.Header.Set("User-Agent", "R3P-Home-Server/1.1")
		resp, err := client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("Lỗi kết nối %s: %s", label, err.Error())
			cfg.LastStatus = msg
			cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			_ = saveDDNSConfig(*cfg)
			return false, msg
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
		msg := fmt.Sprintf("%s phản hồi lỗi (HTTP %d): %s", label, resp.StatusCode, reply)
		cfg.LastStatus = msg
		cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
		_ = saveDDNSConfig(*cfg)
		return false, msg

	case "custom":
		rawURL := cfg.CustomURL
		if rawURL == "" {
			return false, "Vui lòng nhập URL cập nhật tùy chỉnh"
		}
		if err := validateDownloadURL(rawURL); err != nil {
			return false, "Custom DDNS URL không hợp lệ hoặc không an toàn: " + err.Error()
		}
		rawURL = strings.ReplaceAll(rawURL, "[DOMAIN]", url.QueryEscape(cfg.Domain))
		rawURL = strings.ReplaceAll(rawURL, "[TOKEN]", url.QueryEscape(cfg.Token))
		rawURL = strings.ReplaceAll(rawURL, "[IP]", url.QueryEscape(pubIP))
		rawURL = strings.ReplaceAll(rawURL, "[USERNAME]", url.QueryEscape(cfg.Username))

		resp, err := client.Get(rawURL)
		if err != nil {
			msg := "Lỗi gọi Custom DDNS URL: " + err.Error()
			cfg.LastStatus = msg
			cfg.LastUpdate = time.Now().Format("2006-01-02 15:04:05")
			_ = saveDDNSConfig(*cfg)
			return false, msg
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
		msg := fmt.Sprintf("Nhà cung cấp '%s' chưa được hỗ trợ — vui lòng dùng duckdns, noip, dyndns hoặc custom", cfg.Provider)
		cfg.LastStatus = msg
		return false, msg
	}
}

// Background DDNS periodic runner
func initDDNSBackgroundRunner() {
	go func() {
		for {
			time.Sleep(3 * time.Minute)
			cfg := loadDDNSConfig()
			if cfg.Enabled && cfg.Domain != "" && (cfg.Token != "" || cfg.Provider == "custom") {
				currentIP := fetchPublicIPv4()
				if currentIP != "" && currentIP != cfg.LastIP {
					_, _ = performDDNSUpdate(&cfg)
				}
			}
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
		extIP := fetchPublicIPv4()
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
		var body DDNSConfig
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Dữ liệu JSON không hợp lệ: "+err.Error())
			return
		}
		body.Domain = strings.TrimSpace(body.Domain)
		body.Token = strings.TrimSpace(body.Token)
		body.Username = strings.TrimSpace(body.Username)
		body.CustomURL = strings.TrimSpace(body.CustomURL)
		cur := loadDDNSConfig()
		if strings.HasPrefix(body.Token, "••••") {
			body.Token = cur.Token
		}
		if body.CheckInterval < 5 || body.CheckInterval > 1440 {
			body.CheckInterval = 10
		}
		body.LastIP = cur.LastIP
		body.LastUpdate = cur.LastUpdate
		body.LastStatus = cur.LastStatus
		if err := saveDDNSConfig(body); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi lưu cấu hình DDNS: "+err.Error())
			return
		}
		// If user enabled it and provided domain/token, run an immediate update
		msg := "Đã lưu cài đặt DDNS thành công!"
		if body.Enabled && body.Domain != "" && (body.Token != "" || body.Provider == "custom") {
			ok, updateMsg := performDDNSUpdate(&body)
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
	cfg := loadDDNSConfig()
	ok, msg := performDDNSUpdate(&cfg)
	if ok {
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": msg,
			"ddns":    sanitizePublicDDNS(cfg),
		})
	} else {
		writeJSON(w, http.StatusBadRequest, false, msg)
	}
}
