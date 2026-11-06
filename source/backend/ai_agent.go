package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type AIConfig struct {
	Provider          string `json:"provider"`
	APIKey            string `json:"api_key"`
	APIKeyMasked      string `json:"api_key_masked,omitempty"`
	HasAPIKey         bool   `json:"has_api_key"`
	APIKeyConfigured  bool   `json:"api_key_configured"`
	BaseURL           string `json:"base_url"`
	Model             string `json:"model"`
	AutoExecute       bool   `json:"auto_execute"`
	AllowExec         bool   `json:"allow_exec"`
	SystemPromptExtra string `json:"system_prompt_extra"`
	SystemNote        string `json:"system_note"`
}

type AICmdLog struct {
	Command    string `json:"command"`
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

type AIChatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	Timestamp  string         `json:"timestamp"`
	ToolEvents []AIToolAction `json:"tool_events,omitempty"`
	Commands   []AICmdLog     `json:"commands,omitempty"`
}

type AIToolAction struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

const (
	aiConfigFile  = "/etc/r3p/ai_config.json"
	aiHistoryFile = "/etc/r3p/ai_history.json"
)

var (
	aiMu      sync.Mutex
	aiHistory []AIChatMessage
)

func loadAIConfig() AIConfig {
	def := AIConfig{
		Provider:    "gemini",
		APIKey:      "",
		BaseURL:     "https://generativelanguage.googleapis.com/v1beta/openai",
		Model:       "gemini-2.5-flash",
		AutoExecute: true,
		AllowExec:   true,
	}
	b, err := os.ReadFile(aiConfigFile)
	if err == nil {
		_ = json.Unmarshal(b, &def)
	}
	if def.BaseURL == "" {
		def.BaseURL = defaultBaseURLForProvider(def.Provider)
	}
	if def.Model == "" {
		def.Model = defaultModelForProvider(def.Provider)
	}
	if def.SystemNote != "" && def.SystemPromptExtra == "" {
		def.SystemPromptExtra = def.SystemNote
	} else if def.SystemPromptExtra != "" && def.SystemNote == "" {
		def.SystemNote = def.SystemPromptExtra
	}
	def.AllowExec = def.AutoExecute
	def.HasAPIKey = def.APIKey != ""
	def.APIKeyConfigured = def.APIKey != ""
	if len(def.APIKey) > 8 {
		def.APIKeyMasked = def.APIKey[:4] + "••••••••" + def.APIKey[len(def.APIKey)-4:]
	} else if def.APIKey != "" {
		def.APIKeyMasked = "••••••••"
	}
	return def
}

func saveAIConfig(cfg AIConfig) {
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.WriteFile(aiConfigFile, b, 0600)
	}
}

func loadAIHistory() []AIChatMessage {
	aiMu.Lock()
	defer aiMu.Unlock()
	if aiHistory != nil {
		return aiHistory
	}
	b, err := os.ReadFile(aiHistoryFile)
	if err == nil {
		_ = json.Unmarshal(b, &aiHistory)
	}
	if aiHistory == nil {
		aiHistory = []AIChatMessage{
			{
				Role:      "assistant",
				Content:   "Xin chào! Tôi là **R3P System AI Agent** có toàn quyền truy cập và điều khiển hệ thống **Xiaomi Mi Router 3 Pro (OpenWrt 25.12.5 Linux 6.12.94)**.\n\nTôi nắm rõ toàn bộ cấu trúc phần cứng (MT7621AT, 512MB RAM, NAND `/overlay`, ổ cứng USB 3.0 500GB `/mnt/sda1`, Wi-Fi MT7615N AP + Client WWAN, USB Tethering `usbwan`, AdBlock DNS, và dịch vụ `Esp32-Download` Linux Host trên cổng `8787`).\n\nBạn có thể:\n- Nhập **API Key** (Gemini / OpenAI / DeepSeek / Claude / Groq / OpenRouter) ở khung **Cấu Hình API** phía trên rồi trò chuyện bằng ngôn ngữ tự nhiên ở cửa sổ Chat bên dưới.\n- Hoặc gõ trực tiếp `!<lệnh shell>` (ví dụ: `!df -h`, `!uci show wireless`, `!yt-dlp --version`) vào khung Chat để thực thi lệnh trực tiếp với quyền `root` ngay lập tức!",
				Timestamp: time.Now().Format("15:04:05"),
			},
		}
	}
	return aiHistory
}

func saveAIHistory() {
	_ = os.MkdirAll("/etc/r3p", 0755)
	if len(aiHistory) > 60 {
		aiHistory = aiHistory[len(aiHistory)-60:]
	}
	if b, err := json.MarshalIndent(aiHistory, "", "  "); err == nil {
		_ = os.WriteFile(aiHistoryFile, b, 0600)
	}
}

func defaultBaseURLForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	case "openai":
		return "https://api.openai.com/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

func defaultModelForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "gemini":
		return "gemini-2.5-flash"
	case "openai":
		return "gpt-4o-mini"
	case "deepseek":
		return "deepseek-chat"
	case "groq":
		return "llama-3.3-70b-versatile"
	case "openrouter":
		return "google/gemini-2.5-flash"
	case "anthropic":
		return "claude-3-7-sonnet-latest"
	default:
		return "gpt-4o-mini"
	}
}

func buildSystemArchitecturePrompt(extra string) string {
	dfOut, _ := runCmd(2, "df", "-h")
	ipOut, _ := runCmd(2, "ip", "-br", "addr")
	routeOut, _ := runCmd(2, "ip", "route")
	wifiOut, _ := runCmd(2, "uci", "show", "wireless")
	netStatus := getInternetStatusSnapshot()

	return fmt.Sprintf(`You are the embedded Root System AI Administrator for R3P Home Server OS running on a Xiaomi Mi Router 3 Pro (R3P / mir3p).
You have FULL ROOT ACCESS to inspect, configure, and modify the entire Linux / OpenWrt system.

=== COMPLETE SYSTEM ARCHITECTURE ===
1. Hardware & Kernel:
   - Board: Xiaomi Mi Router 3 Pro (MediaTek MT7621AT Dual-Core 4-Threads @ 880MHz, mipsle)
   - Memory: 512MB DDR3 RAM (/tmp tmpfs)
   - Internal Flash: 256MB SLC NAND (UBI /overlay ~206MB)
   - External USB 3.0 Storage: 500GB HGST G-DRIVE HDD formatted as high-speed ext4 (no_journal, read_ahead_kb=4096, max_sectors_kb=1024), mounted at /mnt/sda1 (symlinked at /mnt/usb).
   - Wireless: MediaTek MT7615N 4x4 MU-MIMO Dual-Band:
     * radio0 (2.4GHz) & radio1 (5GHz)
     * Supports simultaneous AP broadcasting (default_radio0, default_radio1) AND Wireless Client / WISP STA mode (wireless.wwan_sta on network.wwan).
   - Dual-Role USB 3.0 Port:
     * Hotplug Storage (/etc/hotplug.d/block/20-r3p-usb-storage): Auto-mounts /dev/sd* to /mnt/sd* and tunes block queue.
     * Hotplug USB Tethering / 4G WAN (/etc/hotplug.d/net/20-r3p-usb-tether): Auto-binds usb0/wwan0/eth1 to UCI network.usbwan (DHCP) in firewall wan zone.

2. Services & Paths:
   - Main Backend Binary: /usr/bin/r3p-api (serves Web UI & REST API v1 on HTTPS port 8443, init script /etc/init.d/r3p-api)
   - Web Frontend Assets: /www/ (Vue 3 SPA with 15 tabs including Dashboard, Network LAN/WAN Config, Wireless AP+Client, AdBlock, Firewall, NAS & USB Format, Download Bot + ESP32 Host, AI Agent, OpenWrt + GitHub Apps, Firmware Update, etc.)
   - Embedded ESP32-Download Linux Runtime: Built into r3p-api listening on HTTP port 8787 (0.0.0.0:8787) and /esp32-download/.
     * Data & Config: /overlay/esp32-download-host/data/local-config.json
     * Media Output Directory: /mnt/sda1/esp32-downloads
     * Installed CLI Tools: /usr/bin/yt-dlp, /usr/bin/aria2c, /usr/sbin/iwinfo, /usr/sbin/mkfs.ext4
   - DNS AdBlock Engine: /etc/r3p/adblock.json, /tmp/r3p-adblock.hosts, /tmp/dnsmasq.d/r3p-adblock.conf

3. Live System Telemetry Right Now:
   - Internet Status: Online=%v, Mode=%s, Interface=%s, Latency=%.1fms, External IP=%s (%s)
   - Filesystems (df -h):
%s
   - Network Interfaces (ip -br addr):
%s
   - Routing Table (ip route):
%s
   - Wireless UCI:
%s

=== HOW TO EXECUTE COMMANDS ON THE SYSTEM ===
Whenever the user asks you to check, change, fix, configure, or run anything on the router (or when you need live info), emit one or more tool execution blocks in this exact XML format:
<exec_cmd>sh command here</exec_cmd>

The backend will automatically execute every <exec_cmd>...</exec_cmd> block as root (/bin/sh -c) on the router, capture stdout/stderr, and feed the real result back to you so you can verify and report the outcome clearly in Vietnamese.
%s`, netStatus.Online, netStatus.WanMode, netStatus.WanIface, netStatus.LatencyMs, netStatus.ExternalIP, netStatus.ISP, dfOut, ipOut, routeOut, wifiOut, extra)
}

var execCmdRegex = regexp.MustCompile(`(?s)<exec_cmd>(.*?)</exec_cmd>`)

func callLLMAPI(cfg AIConfig, messages []map[string]string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURLForProvider(cfg.Provider)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModelForProvider(cfg.Provider)
	}

	client := &http.Client{Timeout: 45 * time.Second}

	if strings.ToLower(cfg.Provider) == "anthropic" && !strings.Contains(baseURL, "/openai") {
		sysPrompt := ""
		var anthMsgs []map[string]string
		for _, m := range messages {
			if m["role"] == "system" {
				sysPrompt = m["content"]
			} else {
				anthMsgs = append(anthMsgs, m)
			}
		}
		payload := map[string]interface{}{
			"model":      model,
			"max_tokens": 2048,
			"system":     sysPrompt,
			"messages":   anthMsgs,
		}
		b, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, baseURL+"/messages", bytes.NewReader(b))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("Anthropic HTTP %d: %s", resp.StatusCode, string(respBytes))
		}
		var parsed struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(respBytes, &parsed); err != nil || len(parsed.Content) == 0 {
			return "", fmt.Errorf("invalid Anthropic response")
		}
		return parsed.Content[0].Text, nil
	}

	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": 0.2,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("API HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", fmt.Errorf("invalid API response: %s", string(respBytes))
	}
	return parsed.Choices[0].Message.Content, nil
}

// GET / PUT /api/v1/ai/config
func handleAIConfig(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg := loadAIConfig()
		safeCfg := cfg
		safeCfg.APIKey = "" // Do not expose raw key in GET response
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"config":              safeCfg,
			"provider":            cfg.Provider,
			"api_key_configured":  cfg.HasAPIKey,
			"has_api_key":         cfg.HasAPIKey,
			"api_key_masked":      cfg.APIKeyMasked,
			"base_url":            cfg.BaseURL,
			"model":               cfg.Model,
			"auto_execute":        cfg.AutoExecute,
			"allow_exec":          cfg.AllowExec,
			"system_prompt_extra": cfg.SystemPromptExtra,
			"system_note":         cfg.SystemNote,
			"system_context":      buildSystemArchitecturePrompt(cfg.SystemPromptExtra),
			"history":             loadAIHistory(),
		})

	case http.MethodPut:
		var body struct {
			Provider          string `json:"provider"`
			APIKey            string `json:"api_key"`
			ClearKey          bool   `json:"clear_key"`
			BaseURL           string `json:"base_url"`
			Model             string `json:"model"`
			AutoExecute       *bool  `json:"auto_execute"`
			AllowExec         *bool  `json:"allow_exec"`
			SystemPromptExtra string `json:"system_prompt_extra"`
			SystemNote        string `json:"system_note"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		cfg := loadAIConfig()
		prevProvider := cfg.Provider
		if body.Provider != "" {
			cfg.Provider = body.Provider
		}
		if body.ClearKey {
			cfg.APIKey = ""
		} else if strings.TrimSpace(body.APIKey) != "" && !strings.Contains(body.APIKey, "••••") {
			cfg.APIKey = strings.TrimSpace(body.APIKey)
		}
		if strings.TrimSpace(body.BaseURL) != "" && body.Provider == prevProvider {
			cfg.BaseURL = strings.TrimSpace(body.BaseURL)
		} else if strings.TrimSpace(body.BaseURL) != "" && body.BaseURL != defaultBaseURLForProvider(prevProvider) {
			cfg.BaseURL = strings.TrimSpace(body.BaseURL)
		} else {
			cfg.BaseURL = defaultBaseURLForProvider(cfg.Provider)
		}
		if strings.TrimSpace(body.Model) != "" {
			cfg.Model = strings.TrimSpace(body.Model)
		} else {
			cfg.Model = defaultModelForProvider(cfg.Provider)
		}
		if body.AllowExec != nil {
			cfg.AutoExecute = *body.AllowExec
			cfg.AllowExec = *body.AllowExec
		} else if body.AutoExecute != nil {
			cfg.AutoExecute = *body.AutoExecute
			cfg.AllowExec = *body.AutoExecute
		}
		note := strings.TrimSpace(body.SystemNote)
		if note == "" {
			note = strings.TrimSpace(body.SystemPromptExtra)
		}
		cfg.SystemNote = note
		cfg.SystemPromptExtra = note
		saveAIConfig(cfg)
		updated := loadAIConfig()
		updated.APIKey = ""
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"saved":              true,
			"config":             updated,
			"provider":           updated.Provider,
			"api_key_configured": updated.HasAPIKey,
			"has_api_key":        updated.HasAPIKey,
			"api_key_masked":     updated.APIKeyMasked,
			"base_url":           updated.BaseURL,
			"model":              updated.Model,
			"auto_execute":       updated.AutoExecute,
			"allow_exec":         updated.AllowExec,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST / DELETE /api/v1/ai/chat
func handleAIChat(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	if r.Method == http.MethodDelete {
		aiMu.Lock()
		aiHistory = []AIChatMessage{}
		saveAIHistory()
		aiMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"cleared": true, "history": loadAIHistory()})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}

	var body struct {
		Message string `json:"message"`
		Execute *bool  `json:"execute"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32768)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	userMsg := strings.TrimSpace(body.Message)
	if userMsg == "" {
		writeJSON(w, http.StatusBadRequest, false, "empty message")
		return
	}

	_ = loadAIHistory()
	aiMu.Lock()
	aiHistory = append(aiHistory, AIChatMessage{
		Role:      "user",
		Content:   userMsg,
		Timestamp: time.Now().Format("15:04:05"),
	})
	saveAIHistory()
	aiMu.Unlock()

	// Direct root shell command mode (!cmd or /exec cmd) — works anytime even without API key!
	if strings.HasPrefix(userMsg, "!") || strings.HasPrefix(userMsg, "/exec ") {
		cmdStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(userMsg, "!"), "/exec "))
		t0 := time.Now()
		out, err := runCmd(25, "sh", "-c", cmdStr)
		durMs := time.Since(t0).Milliseconds()
		exitCode := 0
		if err != nil {
			exitCode = 1
			if out == "" {
				out = err.Error()
			}
		}
		reply := fmt.Sprintf("Đã thực thi lệnh trực tiếp trên Router (`root@R3P`):")
		action := AIToolAction{Tool: "root_shell", Command: cmdStr, Output: out, Success: err == nil}
		cmdLog := AICmdLog{Command: cmdStr, Output: out, ExitCode: exitCode, DurationMs: durMs}
		replyMsg := AIChatMessage{
			Role:       "assistant",
			Content:    reply,
			Timestamp:  time.Now().Format("15:04:05"),
			ToolEvents: []AIToolAction{action},
			Commands:   []AICmdLog{cmdLog},
		}
		aiMu.Lock()
		aiHistory = append(aiHistory, replyMsg)
		saveAIHistory()
		histCopy := append([]AIChatMessage{}, aiHistory...)
		aiMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"reply":       replyMsg,
			"tool_events": []AIToolAction{action},
			"commands":    []AICmdLog{cmdLog},
			"history":     histCopy,
		})
		return
	}

	cfg := loadAIConfig()
	if body.Execute != nil {
		cfg.AutoExecute = *body.Execute
	}
	if cfg.APIKey == "" && !strings.Contains(cfg.BaseURL, "127.0.0.1") && !strings.Contains(cfg.BaseURL, "localhost") && !strings.Contains(cfg.BaseURL, "192.168.") {
		t0 := time.Now()
		out, _ := runCmd(5, "sh", "-c", "uptime; df -h /overlay /mnt/sda1 2>/dev/null; ip -br addr")
		durMs := time.Since(t0).Milliseconds()
		replyText := fmt.Sprintf("Chưa cấu hình **API Key** cho nhà cung cấp AI (**%s**). Hãy nhập API Key ở khung **Cấu Hình Nhúng API Cho AI Agent** ngay phía trên rồi bấm **Lưu Cấu Hình API**.\n\n💡 *Mẹo:* Ngay cả khi chưa có Internet hoặc chưa nhập API Key, bạn vẫn có thể điều khiển toàn bộ hệ thống trong cửa sổ Chat này bằng cách gõ `!<lệnh>` (ví dụ: `!df -h`, `!uci show wireless`, `!ls -la /mnt/sda1`).", strings.ToUpper(cfg.Provider))
		cmdLog := AICmdLog{Command: "uptime && df -h && ip -br addr", Output: out, ExitCode: 0, DurationMs: durMs}
		replyMsg := AIChatMessage{
			Role:      "assistant",
			Content:   replyText,
			Timestamp: time.Now().Format("15:04:05"),
			Commands:  []AICmdLog{cmdLog},
		}
		aiMu.Lock()
		aiHistory = append(aiHistory, replyMsg)
		saveAIHistory()
		histCopy := append([]AIChatMessage{}, aiHistory...)
		aiMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"reply":   replyMsg,
			"history": histCopy,
		})
		return
	}

	// Build conversation messages with full system context
	sysPrompt := buildSystemArchitecturePrompt(cfg.SystemPromptExtra)
	llmMsgs := []map[string]string{
		{"role": "system", "content": sysPrompt},
	}
	aiMu.Lock()
	startIdx := 0
	if len(aiHistory) > 12 {
		startIdx = len(aiHistory) - 12
	}
	for _, m := range aiHistory[startIdx:] {
		if m.Role == "user" || m.Role == "assistant" {
			llmMsgs = append(llmMsgs, map[string]string{"role": m.Role, "content": m.Content})
		}
	}
	aiMu.Unlock()

	var allToolEvents []AIToolAction
	var allCmdLogs []AICmdLog
	var finalReply string

	for turn := 0; turn < 3; turn++ {
		respText, err := callLLMAPI(cfg, llmMsgs)
		if err != nil {
			finalReply = "❌ Lỗi kết nối AI API: `" + err.Error() + "`\nHãy kiểm tra lại kết nối Internet của Router (WAN / Wi-Fi Client / USB Tether) và API Key ở khung cài đặt phía trên."
			break
		}
		finalReply = respText
		if !cfg.AutoExecute {
			break
		}
		matches := execCmdRegex.FindAllStringSubmatch(respText, -1)
		if len(matches) == 0 {
			break
		}
		var cmdOutputs []string
		for _, m := range matches {
			cmdStr := strings.TrimSpace(m[1])
			if cmdStr == "" {
				continue
			}
			t0 := time.Now()
			out, cmdErr := runCmd(25, "sh", "-c", cmdStr)
			durMs := time.Since(t0).Milliseconds()
			exitCode := 0
			if cmdErr != nil {
				exitCode = 1
				if out == "" {
					out = cmdErr.Error()
				}
			}
			if len(out) > 4000 {
				out = out[:4000] + "\n...(truncated)"
			}
			allToolEvents = append(allToolEvents, AIToolAction{
				Tool:    "exec_cmd",
				Command: cmdStr,
				Output:  out,
				Success: cmdErr == nil,
			})
			allCmdLogs = append(allCmdLogs, AICmdLog{
				Command:    cmdStr,
				Output:     out,
				ExitCode:   exitCode,
				DurationMs: durMs,
			})
			cmdOutputs = append(cmdOutputs, fmt.Sprintf("$ %s\n%s", cmdStr, out))
		}
		if len(cmdOutputs) == 0 {
			break
		}
		llmMsgs = append(llmMsgs,
			map[string]string{"role": "assistant", "content": respText},
			map[string]string{"role": "user", "content": "Kết quả thực thi lệnh trên Router:\n```\n" + strings.Join(cmdOutputs, "\n\n") + "\n```\nHãy tổng hợp kết quả và trả lời ngắn gọn, rõ ràng cho người dùng (không lặp lại thẻ <exec_cmd> nếu đã xong)."},
		)
	}

	cleanReply := strings.TrimSpace(execCmdRegex.ReplaceAllString(finalReply, ""))
	if cleanReply == "" {
		cleanReply = finalReply
	}

	replyMsg := AIChatMessage{
		Role:       "assistant",
		Content:    cleanReply,
		Timestamp:  time.Now().Format("15:04:05"),
		ToolEvents: allToolEvents,
		Commands:   allCmdLogs,
	}

	aiMu.Lock()
	aiHistory = append(aiHistory, replyMsg)
	saveAIHistory()
	histCopy := append([]AIChatMessage{}, aiHistory...)
	aiMu.Unlock()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"reply":       replyMsg,
		"tool_events": allToolEvents,
		"commands":    allCmdLogs,
		"history":     histCopy,
	})
}
