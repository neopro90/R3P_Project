package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var aiConfigUpdateMu sync.Mutex

func isLocalAIBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func newAIHTTPClient(baseURL string, timeout time.Duration) *http.Client {
	explicitLocal := isLocalAIBaseURL(baseURL)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			checkIP := func(ip net.IP) error {
				if ip == nil {
					return errors.New("invalid IP address")
				}
				if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
					return fmt.Errorf("blocked unsafe AI destination IP %s", ip.String())
				}
				if !explicitLocal && (ip.IsLoopback() || ip.IsPrivate()) {
					return fmt.Errorf("blocked DNS rebinding to private or loopback IP %s", ip.String())
				}
				return nil
			}
			if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
				if err := checkIP(ip); err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, cand := range ips {
				if err := checkIP(cand.IP); err != nil {
					lastErr = err
					continue
				}
				conn, dErr := dialer.DialContext(ctx, network, net.JoinHostPort(cand.IP.String(), port))
				if dErr == nil {
					return conn, nil
				}
				lastErr = dErr
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no allowed IP address for %s", host)
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func validateAIBaseURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid AI BaseURL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("unsupported AI BaseURL scheme %q", u.Scheme)
	}
	if u.User != nil {
		return errors.New("AI BaseURL must not contain credentials")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return errors.New("AI BaseURL missing hostname")
	}
	// Allow localhost or private LAN LLM servers (e.g. Ollama), but strictly block metadata/link-local/unspecified/multicast IPs.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return fmt.Errorf("blocked unsafe AI BaseURL IP %s", ip.String())
		}
		if scheme == "http" && !ip.IsLoopback() && !ip.IsPrivate() {
			return errors.New("plaintext HTTP AI BaseURL is restricted to localhost or private LAN hosts")
		}
	}
	return nil
}

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
	TelegramAIEnabled bool   `json:"telegram_ai_enabled"`
	AutoRemember      bool   `json:"auto_remember"`
	ActiveSessionID   string `json:"active_session_id"`
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
	Source     string         `json:"source,omitempty"` // "web" or "telegram"
	ToolEvents []AIToolAction `json:"tool_events,omitempty"`
	Commands   []AICmdLog     `json:"commands,omitempty"`
}

type AIToolAction struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

type AIMemoryEntry struct {
	ID        string `json:"id"`
	Category  string `json:"category"` // "system_rule", "preference", "network_note", "fact"
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type AIChatSession struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Messages  []AIChatMessage `json:"messages"`
}

func cloneAISessions(src []AIChatSession) []AIChatSession {
	if src == nil {
		return nil
	}
	out := make([]AIChatSession, len(src))
	for i, s := range src {
		out[i] = s
		if s.Messages != nil {
			msgs := make([]AIChatMessage, len(s.Messages))
			for j, m := range s.Messages {
				msgs[j] = m
				if m.ToolEvents != nil {
					msgs[j].ToolEvents = append([]AIToolAction{}, m.ToolEvents...)
				}
				if m.Commands != nil {
					msgs[j].Commands = append([]AICmdLog{}, m.Commands...)
				}
			}
			out[i].Messages = msgs
		}
	}
	return out
}

const (
	aiConfigFile        = "/etc/r3p/ai_config.json"
	aiSessionsFile      = "/etc/r3p/ai_sessions.json"
	aiMemoryFile        = "/etc/r3p/ai_memory.json"
	aiQuickCommandsFile = "/etc/r3p/quick_commands.json"
)

var (
	aiBlueprintFile = "/etc/r3p/SYSTEM_BLUEPRINT.md"
	aiMu            sync.Mutex
	aiCfgMu         sync.Mutex
	aiSessions      []AIChatSession
	aiMemories      []AIMemoryEntry
)

func resetAIMemoryCaches() {
	aiMu.Lock()
	aiSessions = nil
	aiMemories = nil
	aiMu.Unlock()
}

const currentSystemBlueprintRev = "v1.1.26-r3p-os-v2"

const defaultSystemBlueprintMD = `# TÀI LIỆU KIẾN TRÚC HỆ THỐNG R3P HOME SERVER OS (SYSTEM BLUEPRINT)
> Phiên bản tài liệu: ` + currentSystemBlueprintRev + ` · Lưu đồng bộ trong /etc/r3p/r3p.db (system_meta/ai_blueprint) & /etc/r3p/SYSTEM_BLUEPRINT.md
> Đối tượng sử dụng: Root System AI Agent (Web UI + Telegram) & Quản trị viên

## 1. Phần Cứng & Hệ Điều Hành (Hardware & OS)
- **Thiết bị**: Xiaomi Mi Router 3 Pro (Mã board: mir3p / R3P)
- **SoC / CPU**: MediaTek MT7621AT Dual-Core 880MHz (4 hardware threads, kiến trúc mipsle_24kc)
- **Bộ nhớ RAM**: 512 MB DDR3 (đọc thực tế từ /proc/iomem; /tmp chạy RAM tmpfs tốc độ cao)
- **Bộ nhớ Flash NAND**: 256 MB SLC NAND Flash:
  - /rom: SquashFS read-only chứa hệ điều hành gốc OpenWrt 25.12 (Linux Kernel 6.12)
  - /overlay: Phân vùng UBIFS đọc-ghi bền vững (~206 MB). KHÔNG ghi log hay file tạm lớn/thường xuyên vào /overlay; đặt dữ liệu tạm trên /tmp hoặc USB /mnt/sda1.
- **Shell & Công cụ hệ thống**:
  - Shell mặc định: BusyBox ash (/bin/sh). Trình quản lý gói là **apk** (KHÔNG dùng opkg).
  - Tường lửa là **fw4 (nftables)** cấu hình qua /etc/config/firewall (KHÔNG tạo rule iptables/fw3 mới).
  - DNS & DHCP là **dnsmasq** (/etc/config/dhcp, lease động tại /tmp/dhcp.leases).
  - SSH là **dropbear** (/etc/config/dropbear, section @dropbear[0], port 22).
- **Cổng USB 3.0 & Lưu Trữ Ngoài**:
  - Ổ cứng / USB (/dev/sd*) mount vào /mnt/sda1 (symlink /mnt/usb nếu có)
  - Queue tuning: /sys/block/sda/queue/read_ahead_kb & max_sectors_kb

## 2. Cấu Trúc Wi-Fi (MediaTek MT7615N 4x4 Dual-Band) & Mạng
- **Radio & VAP mặc định trong /etc/config/wireless**:
  - **2.4GHz (` + "`radio0`" + ` / phy0)**: Device ` + "`wireless.radio0`" + ` (` + "`band='2g'`" + `, kênh 1–13, HT20/HT40); AP mặc định là section ` + "`wireless.default_radio0`" + ` (VAP ` + "`phy0-ap0`" + `).
  - **5GHz (` + "`radio1`" + ` / phy1)**: Device ` + "`wireless.radio1`" + ` (` + "`band='5g'`" + `, kênh 36–165, VHT80); AP mặc định là section ` + "`wireless.default_radio1`" + ` (VAP ` + "`phy1-ap0`" + `).
  - **Quy tắc BẮT BUỘC khi bật/tắt hoặc sửa Wi-Fi AP**:
    1. Điểm phát Wi-Fi CHỈ phát sóng khi **CẢ chip radio** (` + "`wireless.radio0.disabled=0`" + `) **VÀ giao diện AP** (` + "`wireless.default_radio0.disabled=0`" + `) đều ở trạng thái ` + "`0`" + `! Nếu ` + "`radio0.disabled='1'`" + ` mà chỉ bật ` + "`default_radio0.disabled=0`" + ` thì radio vẫn tắt (RADIO DISABLED).
    2. Sau khi ` + "`uci commit wireless`" + `, luôn cần reload bằng ` + "`wifi reload`" + `.
- **Mạng LAN & WAN (/etc/config/network)**:
  - Bridge nội bộ: ` + "`network.lan`" + ` trên thiết bị ` + "`br-lan`" + ` (kiểm tra IP thực tế bằng ` + "`uci get network.lan.ipaddr`" + `, tránh đoán cứng IP).
  - Thu sóng Wi-Fi Client (WISP / Repeater STA): interface ` + "`network.wwan`" + ` / section ` + "`wireless.wwan_sta`" + ` (hoặc ` + "`wclient`" + `).
  - USB Tethering / 4G WAN: ` + "`usb0`" + ` / ` + "`wwan0`" + ` / ` + "`eth1`" + ` gắn vào ` + "`network.usbwan`" + ` (zone ` + "`wan`" + `).

## 3. Dịch Vụ R3P Core & Cơ Sở Dữ Liệu Embedded ` + "`/etc/r3p/r3p.db`" + `
- **Daemon trung tâm**: ` + "`/usr/bin/r3p-api`" + ` (service ` + "`/etc/init.d/r3p-api`" + `, HTTP ` + "`:80`" + ` chuyển hướng sang HTTPS ` + "`:8443`" + `, SPA Vue 3 tại ` + "`/www/`" + `, health ` + "`/api/v1/health`" + `).
- **QUAN TRỌNG — Kho lưu trữ cấu hình thống nhất ` + "`/etc/r3p/r3p.db`" + ` (bbolt)**:
  - Toàn bộ cấu hình runtime R3P hiện lưu trong **bbolt DB ` + "`/etc/r3p/r3p.db`" + `** (mode ` + "`0600`" + `), KHÔNG dùng các file ` + "`/etc/r3p/*.json`" + ` cũ (những file ` + "`.json`" + ` nếu còn sót chỉ là dữ liệu di trú cũ).
  - **4 buckets trong ` + "`r3p.db`" + `**:
    - ` + "`settings`" + `: ` + "`adblock`" + `, ` + "`ai_config`" + `, ` + "`ddns`" + `, ` + "`downloader`" + `, ` + "`gdrive_sync`" + `, ` + "`iot_broker`" + `, ` + "`telegram`" + `, ` + "`ui_preferences`" + `, ` + "`version`" + `
    - ` + "`collections`" + `: ` + "`ai_memory`" + `, ` + "`ai_sessions`" + `, ` + "`ai_quick_commands`" + `, ` + "`app_repos`" + `, ` + "`downloader_queue`" + `, ` + "`downloader_schedules`" + `, ` + "`downloads_state`" + `, ` + "`iot_devices`" + `, ` + "`plugins`" + `, ` + "`saved_wifi`" + `, ` + "`shares`" + `
    - ` + "`system_meta`" + `: ` + "`ai_blueprint`" + `, ` + "`ai_blueprint_rev`" + `, ` + "`schema_version`" + `, ` + "`migrated_at`" + `, ` + "`last_migration_count`" + `
    - ` + "`audit_logs`" + `: Nhật ký migration & snapshot restore
  - KHÔNG mở hoặc chỉnh trực tiếp file nhị phân ` + "`/etc/r3p/r3p.db`" + ` bằng ` + "`sed/cat/write_file`" + `; hãy dùng thẻ công cụ AI hoặc REST API ` + "`/api/v1/*`" + ` / ` + "`uci`" + `.
- **Đường dẫn runtime quan trọng khác**:
  - DNS AdBlock: ` + "`/tmp/dnsmasq.d/r3p-adblock.conf`" + ` (tmpfs)
  - Cron Root: ` + "`/etc/crontabs/root`" + ` (service ` + "`/etc/init.d/cron`" + `)
  - R3P Downloader cục bộ: ` + "`/downloads`" + ` & ` + "`/api/v1/download/*`" + ` (` + "`/usr/bin/yt-dlp`" + `, ` + "`/usr/bin/aria2c`" + `), thư mục mặc định ` + "`/mnt/sda1/downloads`" + ` (hoặc ` + "`/overlay/downloads`" + `)

## 4. Bộ Công Cụ Quyền Root & Quy Tắc Cú Pháp (BẮT BUỘC TUÂN THỦ)
AI thực thi thao tác trên Router qua các thẻ XML sau:
1. **Sửa cấu hình OpenWrt UCI (` + "`<uci_set>`" + `)** — *tự động commit, verify bằng ` + "`uci get`" + ` và tự reload service (` + "`wifi reload`" + `, ` + "`network reload`" + `, ` + "`dnsmasq reload`" + `, ` + "`firewall reload`" + `...)*:
   **QUY TẮC QUOTING:** Viết giá trị **TRẦN, KHÔNG bọc thêm dấu nháy đơn ` + "`'`" + ` hay nháy kép ` + "`\"`" + `** quanh giá trị trong ` + "`<uci_set>`" + ` (trình thực thi gọi thẳng ` + "`uci set`" + ` không qua shell). Hỗ trợ ` + "`delete package.section.option`" + `.
   Ví dụ ĐÚNG:
   <uci_set>
   wireless.default_radio0.ssid=Hello
   wireless.default_radio0.disabled=0
   wireless.radio0.disabled=0
   </uci_set>
2. **Thực thi lệnh Shell BusyBox ash (` + "`<exec_cmd>`" + `)**:
   <exec_cmd>lệnh shell tại đây</exec_cmd>
   *Lưu ý: Nếu chạy ` + "`uci set`" + ` trực tiếp trong ` + "`<exec_cmd>`" + ` thì mới cần shell-quote kiểu ` + "`uci set wireless.default_radio0.ssid='Ten Moi' && uci commit wireless && wifi reload && uci get wireless.default_radio0.ssid`" + `.*
3. **Ghi file cấu hình / script (` + "`<write_file>`" + `)**:
   <write_file path="/đường/dẫn/file">nội dung đầy đủ</write_file>
4. **Lưu ghi nhớ dài hạn vào r3p.db (` + "`collections/ai_memory`" + `)**:
   <memory_save>Quy tắc hoặc thông số cần ghi nhớ</memory_save>
5. **Tải video/file qua R3P Downloader**:
   <download_url>https://...</download_url>

## 5. Quy Tắc Kiểm Chứng Trung Thực & Lệnh OpenWrt/BusyBox
- **KHÔNG BAO GIỜ bịa hoặc khẳng định đã xong** nếu kết quả thực thi trả về có lỗi hoặc ` + "`verified:`" + ` chưa khớp giá trị mong muốn. Luôn đối chiếu đầu ra thực tế từ công cụ trước khi trả lời người dùng.
- **Đo tốc độ USB (` + "`/mnt/sda1`" + `)**: BusyBox ` + "`dd`" + ` KHÔNG hỗ trợ ` + "`conv=fdatasync`" + `. Dùng ` + "`dd if=/dev/zero of=/mnt/sda1/test_speed.bin bs=1M count=30 2>&1; sync; dd if=/mnt/sda1/test_speed.bin of=/dev/null bs=1M 2>&1; rm -f /mnt/sda1/test_speed.bin`" + `.
- **Kiểm tra nhanh tài nguyên**: ` + "`df -h /mnt/sda1 /overlay 2>/dev/null; free -m; uptime`" + `
`

const defaultQuickCommandsJSON = `{
  "quick": [
    { "label": "🩺 Kiểm tra tổng quan Router", "cmd": "!uname -a && uptime && df -h /overlay /mnt/sda1 2>/dev/null && free -m" },
    { "label": "📊 Tải CPU & Tiến trình top", "cmd": "!top -b -n 1 | head -n 25" },
    { "label": "🌐 IP các cổng mạng", "cmd": "!ip -br addr && echo \"---\" && ip route" },
    { "label": "📜 30 dòng log hệ thống gần nhất", "cmd": "!logread | tail -n 30" }
  ],
  "network": [
    { "label": "📡 Quét Wi-Fi 2.4G & 5G xung quanh", "cmd": "!iwinfo phy1 scan 2>/dev/null | head -n 35; iwinfo phy0 scan 2>/dev/null | head -n 35" },
    { "label": "📶 Cấu hình Wi-Fi hiện tại (UCI)", "cmd": "!uci show wireless" },
    { "label": "👥 Danh sách thiết bị kết nối DHCP", "cmd": "!cat /tmp/dhcp.leases" },
    { "label": "⚡ Ping kiểm tra Internet (Google)", "cmd": "!ping -c 4 8.8.8.8" },
    { "label": "🔎 Tra cứu DNS qua nslookup", "cmd": "!nslookup google.com 127.0.0.1" }
  ],
  "storage": [
    { "label": "💾 Chi tiết ổ đĩa USB 3.0 & Phân vùng", "cmd": "!lsblk -o NAME,FSTYPE,SIZE,MOUNTPOINT,LABEL 2>/dev/null || df -h" },
    { "label": "🚀 Tốc độ đọc tuần tự ổ đĩa USB", "cmd": "!dd if=/dev/sda1 of=/dev/null bs=1M count=50 2>&1" },
    { "label": "📁 Xem file trong ổ cứng USB", "cmd": "!ls -lah /mnt/sda1" },
    { "label": "⚡ Thông số tinh chỉnh USB Queue", "cmd": "!cat /sys/block/sda/queue/read_ahead_kb /sys/block/sda/queue/max_sectors_kb 2>/dev/null" }
  ],
  "adblock": [
    { "label": "🛡️ Trạng thái AdBlock & Số domain", "cmd": "!wc -l /tmp/dnsmasq.d/r3p-adblock.conf 2>/dev/null || echo 'Chưa có file AdBlock runtime'; head -n 5 /tmp/dnsmasq.d/r3p-adblock.conf 2>/dev/null" },
    { "label": "🔍 Kiểm tra cổng DNS 53 listening", "cmd": "!netstat -tlpn | grep :53" },
    { "label": "🚫 Thử phân giải tên miền quảng cáo", "cmd": "!nslookup doubleclick.net 127.0.0.1" }
  ],
  "download": [
    { "label": "⬇️ Phiên bản yt-dlp & aria2c", "cmd": "!yt-dlp --version 2>/dev/null; aria2c --version 2>/dev/null | head -n 1" },
    { "label": "📂 File trong thư mục tải xuống & r3p.db", "cmd": "!ls -lh /etc/r3p/r3p.db; ls -lah /mnt/sda1/downloads 2>/dev/null || ls -lah /overlay/downloads 2>/dev/null" },
    { "label": "🤖 Kiểm tra dịch vụ r3p-api (80/8443)", "cmd": "!netstat -tlpn | grep r3p-api" }
  ],
  "maintenance": [
    { "label": "🧹 Giải phóng RAM (Drop Caches)", "cmd": "!sync && echo 3 > /proc/sys/vm/drop_caches && free -m" },
    { "label": "🔥 Xem ruleset tường lửa fw4 (nftables)", "cmd": "!nft list ruleset 2>/dev/null | head -n 30" },
    { "label": "📶 Khởi động lại Wi-Fi subsystem", "cmd": "!wifi reload && wifi status | head -n 30" },
    { "label": "📦 Kiểm tra kho ứng dụng apk", "cmd": "!apk info 2>/dev/null | wc -l" }
  ]
}`

func writeSystemBlueprintToStorage(content string, rev string, isCustom bool) {
	_ = settingsDBPutRaw(bucketSystemMeta, "ai_blueprint", []byte(content))
	if rev != "" {
		_ = settingsDBPutRaw(bucketSystemMeta, "ai_blueprint_rev", []byte(rev))
	}
	if isCustom {
		_ = settingsDBPutRaw(bucketSystemMeta, "ai_blueprint_custom", []byte("1"))
	} else {
		_ = settingsDBPutRaw(bucketSystemMeta, "ai_blueprint_custom", []byte("0"))
	}
	_ = os.MkdirAll(filepath.Dir(aiBlueprintFile), 0755)
	_ = os.WriteFile(aiBlueprintFile, []byte(content), 0644)
}

func isOutdatedStockBlueprint(content string) bool {
	if strings.TrimSpace(content) == "" {
		return true
	}
	// Legacy built-in markers shipped in v1.1.24-v1.1.26 prior to R3P OS Blueprint v2.
	return strings.Contains(content, "RAM盘") ||
		strings.Contains(content, "/etc/r3p/ai_config.json") ||
		strings.Contains(content, "wireless.default_radio0.ssid='TenWiFiMoi'")
}

func ensureSystemBlueprintFile() string {
	customFlag, _ := settingsDBGetRaw(bucketSystemMeta, "ai_blueprint_custom")
	revRaw, _ := settingsDBGetRaw(bucketSystemMeta, "ai_blueprint_rev")
	storedRev := strings.TrimSpace(string(revRaw))
	isCustom := strings.TrimSpace(string(customFlag)) == "1"

	if raw, ok := settingsDBGetRaw(bucketSystemMeta, "ai_blueprint"); ok && len(strings.TrimSpace(string(raw))) > 0 {
		s := string(raw)
		if isCustom {
			return s
		}
		if storedRev == currentSystemBlueprintRev && !isOutdatedStockBlueprint(s) {
			return s
		}
		// Auto-upgrade built-in blueprint when firmware upgrades or legacy markers remain.
		writeSystemBlueprintToStorage(defaultSystemBlueprintMD, currentSystemBlueprintRev, false)
		return defaultSystemBlueprintMD
	}

	if b, err := os.ReadFile(aiBlueprintFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		s := string(b)
		if !isOutdatedStockBlueprint(s) {
			writeSystemBlueprintToStorage(s, currentSystemBlueprintRev, isCustom)
			return s
		}
	}

	writeSystemBlueprintToStorage(defaultSystemBlueprintMD, currentSystemBlueprintRev, false)
	return defaultSystemBlueprintMD
}

func isBareURLMessage(text string) bool {
	return analyzeTelegramMessage(text).Route == TelegramRouteDownloader
}

func filterOutBareURLMessages(msgs []AIChatMessage) []AIChatMessage {
	var out []AIChatMessage
	for _, m := range msgs {
		if m.Role == "user" && isBareURLMessage(m.Content) {
			continue
		}
		out = append(out, m)
	}
	if out == nil {
		out = []AIChatMessage{}
	}
	return out
}

func loadAIConfig() AIConfig {
	aiCfgMu.Lock()
	defer aiCfgMu.Unlock()
	def := AIConfig{
		Provider:          "gemini",
		APIKey:            "",
		BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai",
		Model:             "gemini-3.1-flash-lite",
		AutoExecute:       true,
		AllowExec:         true,
		TelegramAIEnabled: true,
		AutoRemember:      true,
		ActiveSessionID:   "default",
	}
	settingsDBLoadJSON(bucketSettings, "ai_config", aiConfigFile, &def)
	if def.BaseURL == "" {
		def.BaseURL = defaultBaseURLForProvider(def.Provider)
	}
	if strings.TrimSpace(def.Model) == "" {
		def.Model = defaultModelForProvider(def.Provider)
	}
	if def.SystemNote != "" && def.SystemPromptExtra == "" {
		def.SystemPromptExtra = def.SystemNote
	} else if def.SystemPromptExtra != "" && def.SystemNote == "" {
		def.SystemNote = def.SystemPromptExtra
	}
	if def.ActiveSessionID == "" {
		def.ActiveSessionID = "default"
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
	aiCfgMu.Lock()
	defer aiCfgMu.Unlock()
	cfg.HasAPIKey = cfg.APIKey != ""
	cfg.APIKeyConfigured = cfg.APIKey != ""
	_ = settingsDBSaveJSON(bucketSettings, "ai_config", aiConfigFile, 0600, cfg)
}

func loadAIMemories() []AIMemoryEntry {
	aiMu.Lock()
	defer aiMu.Unlock()
	return loadAIMemoriesLocked()
}

func loadAIMemoriesLocked() []AIMemoryEntry {
	if aiMemories != nil {
		return aiMemories
	}
	settingsDBLoadJSON(bucketCollections, "ai_memory", aiMemoryFile, &aiMemories)
	if len(aiMemories) == 0 {
		aiMemories = []AIMemoryEntry{
			{
				ID:        "mem-init-1",
				Category:  "system_rule",
				Content:   "Router R3P chạy tại IP LAN 192.168.2.1, kết nối Wi-Fi Client tới Gateway chính 192.168.1.1 (IP Client 192.168.1.6).",
				CreatedAt: time.Now().Format("2006-01-02 15:04"),
			},
			{
				ID:        "mem-init-2",
				Category:  "preference",
				Content:   "Luôn trả lời bằng Tiếng Việt ngắn gọn, rõ ràng và tự động thực thi lệnh root khi người dùng yêu cầu kiểm tra hoặc cấu hình hệ thống.",
				CreatedAt: time.Now().Format("2006-01-02 15:04"),
			},
		}
		saveAIMemoriesLocked()
	}
	return aiMemories
}

func saveAIMemoriesLocked() {
	if len(aiMemories) > 100 {
		aiMemories = aiMemories[len(aiMemories)-100:]
	}
	_ = settingsDBSaveJSON(bucketCollections, "ai_memory", aiMemoryFile, 0600, aiMemories)
}

func addAIMemoryEntry(category, content string) AIMemoryEntry {
	content = strings.TrimSpace(content)
	if category == "" {
		category = "fact"
	}
	aiMu.Lock()
	defer aiMu.Unlock()
	_ = loadAIMemoriesLocked()
	// Avoid exact duplicate
	for _, m := range aiMemories {
		if strings.EqualFold(strings.TrimSpace(m.Content), content) {
			return m
		}
	}
	entry := AIMemoryEntry{
		ID:        fmt.Sprintf("mem-%d", time.Now().UnixNano()/1e6),
		Category:  category,
		Content:   content,
		CreatedAt: time.Now().Format("2006-01-02 15:04"),
	}
	aiMemories = append([]AIMemoryEntry{entry}, aiMemories...)
	saveAIMemoriesLocked()
	return entry
}

func defaultWelcomeMessages() []AIChatMessage {
	return []AIChatMessage{
		{
			Role:      "assistant",
			Content:   "Xin chào! Tôi là **R3P System AI Agent** có **toàn quyền `root`** điều khiển hệ thống **Xiaomi Mi Router 3 Pro (OpenWrt 25.12.5 · Kernel 6.12.94)**.\n\n- **Bộ nhớ AI & Phiên làm việc (Sessions)**: Mọi phiên chat và ghi nhớ hệ thống được lưu trữ bền vững tại `/etc/r3p/ai_sessions.json` và `/etc/r3p/ai_memory.json`.\n- **Bản thiết kế hệ thống (Blueprint)**: Tôi luôn tham chiếu `/etc/r3p/SYSTEM_BLUEPRINT.md` để nắm rõ sơ đồ phần cứng, mạng, lưu trữ và dịch vụ.\n- **Đồng bộ Telegram Bot**: Nhắn URL đơn vào Telegram sẽ tự chuyển sang Host Download; nhắn văn bản/lệnh vào Telegram sẽ được tôi xử lý và phản hồi trực tiếp!",
			Timestamp: time.Now().Format("15:04:05"),
			Source:    "web",
		},
	}
}

func loadAISessionsLocked() []AIChatSession {
	if aiSessions != nil {
		return aiSessions
	}
	settingsDBLoadJSON(bucketCollections, "ai_sessions", aiSessionsFile, &aiSessions)
	if len(aiSessions) == 0 {
		legacy := defaultWelcomeMessages()
		nowStr := time.Now().Format("2006-01-02 15:04")
		aiSessions = []AIChatSession{
			{
				ID:        "default",
				Title:     "Phiên Điều Khiển Chính (Root Session)",
				CreatedAt: nowStr,
				UpdatedAt: nowStr,
				Messages:  filterOutBareURLMessages(legacy),
			},
		}
		saveAISessionsLocked()
	}
	return aiSessions
}

func saveAISessionsLocked() {
	for i := range aiSessions {
		aiSessions[i].Messages = filterOutBareURLMessages(aiSessions[i].Messages)
		if len(aiSessions[i].Messages) > 80 {
			aiSessions[i].Messages = aiSessions[i].Messages[len(aiSessions[i].Messages)-80:]
		}
	}
	_ = settingsDBSaveJSON(bucketCollections, "ai_sessions", aiSessionsFile, 0600, aiSessions)
}

func getActiveSessionMessages(sessionID string) (string, []AIChatMessage) {
	aiMu.Lock()
	defer aiMu.Unlock()
	_ = loadAISessionsLocked()
	if sessionID == "" {
		cfg := loadAIConfig()
		sessionID = cfg.ActiveSessionID
	}
	for _, s := range aiSessions {
		if s.ID == sessionID {
			return s.ID, filterOutBareURLMessages(s.Messages)
		}
	}
	return aiSessions[0].ID, filterOutBareURLMessages(aiSessions[0].Messages)
}

func appendMessageToSession(sessionID string, msg AIChatMessage) []AIChatMessage {
	if msg.Role == "user" && isBareURLMessage(msg.Content) {
		_, msgs := getActiveSessionMessages(sessionID)
		return msgs
	}
	aiMu.Lock()
	defer aiMu.Unlock()
	_ = loadAISessionsLocked()
	if sessionID == "" {
		cfg := loadAIConfig()
		sessionID = cfg.ActiveSessionID
	}
	idx := -1
	for i := range aiSessions {
		if aiSessions[i].ID == sessionID {
			idx = i
			break
		}
	}
	if idx == -1 {
		idx = 0
	}
	aiSessions[idx].Messages = append(aiSessions[idx].Messages, msg)
	aiSessions[idx].UpdatedAt = time.Now().Format("2006-01-02 15:04")
	// Auto-title new session from first user message
	if msg.Role == "user" && (strings.HasPrefix(aiSessions[idx].Title, "Phiên Mới") || aiSessions[idx].Title == "New Session") {
		titleRunes := []rune(strings.TrimSpace(msg.Content))
		if len(titleRunes) > 36 {
			aiSessions[idx].Title = string(titleRunes[:36]) + "…"
		} else if len(titleRunes) > 0 {
			aiSessions[idx].Title = string(titleRunes)
		}
	}
	saveAISessionsLocked()
	return append([]AIChatMessage{}, aiSessions[idx].Messages...)
}

func loadAIHistory() []AIChatMessage {
	_, msgs := getActiveSessionMessages("")
	return msgs
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
		return "gemini-3.1-flash-lite"
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

func buildLightweightSystemContext(extra string) string {
	blueprint := ensureSystemBlueprintFile()
	netStatus := getInternetStatusSnapshot()
	mems := loadAIMemories()
	var memLines []string
	for _, m := range mems {
		memLines = append(memLines, fmt.Sprintf("- [%s] (%s): %s", m.Category, m.CreatedAt, m.Content))
	}
	memBlock := "Chưa có ghi nhớ nào."
	if len(memLines) > 0 {
		memBlock = strings.Join(memLines, "\n")
	}
	return fmt.Sprintf("Blueprint: %d bytes | Internet Online=%v (%s) | Memories=%d\n%s\n%s",
		len(blueprint), netStatus.Online, netStatus.WanIface, len(mems), memBlock, extra)
}

func formatWirelessSummaryForPrompt(wifiRaw string) string {
	uciMap := map[string]string{}
	for _, line := range strings.Split(wifiRaw, "\n") {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			uciMap[strings.TrimSpace(parts[0])] = unquoteUCIValue(parts[1])
		}
	}
	if len(uciMap) == 0 {
		return "(không có dữ liệu wireless UCI)"
	}
	lineFor := func(radioSec, apSec, label string) string {
		rDis := uciMap["wireless."+radioSec+".disabled"]
		if rDis == "" {
			rDis = "0"
		}
		aDis := uciMap["wireless."+apSec+".disabled"]
		if aDis == "" {
			aDis = "0"
		}
		ssid := uciMap["wireless."+apSec+".ssid"]
		ch := uciMap["wireless."+radioSec+".channel"]
		enc := uciMap["wireless."+apSec+".encryption"]
		effective := "UP"
		if rDis == "1" || aDis == "1" {
			effective = "OFF"
			if rDis == "1" {
				effective += " (radio chip disabled=1)"
			} else {
				effective += " (AP disabled=1)"
			}
		}
		return fmt.Sprintf("- %s [%s/%s]: %s | ssid=%q | channel=%s | enc=%s | %s.disabled=%s | %s.disabled=%s",
			label, radioSec, apSec, effective, ssid, ch, enc, radioSec, rDis, apSec, aDis)
	}
	lines := []string{
		lineFor("radio0", "default_radio0", "2.4GHz AP"),
		lineFor("radio1", "default_radio1", "5GHz AP"),
	}
	if wSsid := uciMap["wireless.wwan_sta.ssid"]; wSsid != "" {
		wDis := uciMap["wireless.wwan_sta.disabled"]
		lines = append(lines, fmt.Sprintf("- WISP Client [wwan_sta]: ssid=%q | disabled=%s | device=%s",
			wSsid, wDis, uciMap["wireless.wwan_sta.device"]))
	}
	return strings.Join(lines, "\n")
}

func readOSReleaseSummary() string {
	if raw, err := os.ReadFile("/etc/openwrt_release"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "DISTRIB_DESCRIPTION=") {
				return unquoteUCIValue(strings.TrimPrefix(line, "DISTRIB_DESCRIPTION="))
			}
		}
	}
	unameOut, _ := runCmd(2, "uname", "-srm")
	if unameOut != "" {
		return unameOut
	}
	return "OpenWrt R3P OS"
}

func buildSystemArchitecturePrompt(extra string) string {
	blueprint := ensureSystemBlueprintFile()
	osDesc := readOSReleaseSummary()
	dfOut, _ := runCmd(2, "df", "-h")
	freeOut, _ := runCmd(2, "free", "-m")
	ipOut, _ := runCmd(2, "ip", "-br", "addr")
	routeOut, _ := runCmd(2, "ip", "route")
	sysRaw, _ := runCmd(2, "uci", "show", "system")
	netRaw, _ := runCmd(2, "uci", "show", "network")
	wifiRaw, _ := runCmd(2, "uci", "show", "wireless")
	sysOut := redactSensitiveLines(sysRaw)
	netOut := redactSensitiveLines(netRaw)
	wifiOut := redactSensitiveLines(wifiRaw)
	wifiSummary := formatWirelessSummaryForPrompt(wifiRaw)
	netStatus := getInternetStatusSnapshot()
	dbSummary := getSettingsDBSummary()

	adblockInfo := "không có /tmp/dnsmasq.d/r3p-adblock.conf"
	if st, err := os.Stat("/tmp/dnsmasq.d/r3p-adblock.conf"); err == nil {
		adblockInfo = fmt.Sprintf("/tmp/dnsmasq.d/r3p-adblock.conf (%d bytes)", st.Size())
	}

	mems := loadAIMemories()
	var memLines []string
	for _, m := range mems {
		memLines = append(memLines, fmt.Sprintf("- [%s] (%s): %s", m.Category, m.CreatedAt, m.Content))
	}
	memBlock := "Chưa có ghi nhớ nào."
	if len(memLines) > 0 {
		memBlock = strings.Join(memLines, "\n")
	}

	return fmt.Sprintf(`Bạn là Root System AI Administrator nhúng trực tiếp trong R3P Home Server OS (Xiaomi Mi Router 3 Pro - mir3p).
Bạn có TOÀN QUYỀN ROOT để kiểm tra, cấu hình, sửa file, chỉnh UCI, quản lý mạng/Wi-Fi/AdBlock/Download và ghi nhớ thông tin hệ thống.

=== TÀI LIỆU KIẾN TRÚC HỆ THỐNG (SYSTEM BLUEPRINT) ===
%s

=== BỘ NHỚ DÀI HẠN CỦA AI (r3p.db -> collections/ai_memory) ===
%s

=== TRẠNG THÁI HỆ THỐNG THỜI GIAN THỰC (LIVE TELEMETRY) ===
- Firmware / OS: %s (Rev %s) | %s
- Embedded DB: %s (initialized=%v, keys=%d, size=%d bytes)
- AdBlock Runtime: %s
- Internet: Online=%v | Mode=%s | Interface=%s | Latency=%.1fms | External IP=%s (%s)
- RAM (free -m):
%s
- Dung lượng ổ đĩa (df -h):
%s
- Địa chỉ IP các cổng mạng (ip -br addr):
%s
- Bảng định tuyến (ip route):
%s
- Tóm tắt Wi-Fi đã giải mã:
%s
- Cấu hình Hệ thống (uci show system):
%s
- Cấu hình Mạng (uci show network, đã che secret):
%s
- Cấu hình Wi-Fi chi tiết (uci show wireless, đã che key):
%s

=== HƯỚNG DẪN SỬ DỤNG CÔNG CỤ ROOT (TOOL DIRECTIVES) ===
Khi người dùng yêu cầu kiểm tra, cài đặt, chỉnh sửa hệ thống, lưu ghi nhớ, hoặc tải video:
1. Thực thi lệnh Shell BusyBox ash với quyền root:
   <exec_cmd>lệnh shell tại đây</exec_cmd>
2. Ghi hoặc cập nhật file cấu hình / script trên hệ thống:
   <write_file path="/đường/dẫn/file">nội dung file</write_file>
3. Thay đổi cấu hình OpenWrt UCI (KHÔNG bọc thêm nháy đơn '...' quanh giá trị; hệ thống tự commit, xác minh lại bằng uci get và tự reload service):
   <uci_set>
   wireless.default_radio0.ssid=TenMoi
   </uci_set>
4. Ghi nhớ thông tin/quy tắc/cài đặt mới vào bộ nhớ dài hạn (r3p.db -> collections/ai_memory):
   <memory_save>nội dung cần ghi nhớ lâu dài</memory_save>
5. Đẩy URL vào hàng đợi tải xuống R3P Downloader:
   <download_url>https://...</download_url>

QUY TẮC BẮT BUỘC: Không bao giờ khẳng định "Đã cập nhật/sửa xong" nếu đầu ra thực thi công cụ có Errors hoặc giá trị verified chưa khớp đúng yêu cầu.

%s`, blueprint, memBlock, Version, RevisionStr, osDesc, dbSummary.Path, dbSummary.Initialized, dbSummary.TotalKeys, dbSummary.SizeBytes, adblockInfo, netStatus.Online, netStatus.WanMode, netStatus.WanIface, netStatus.LatencyMs, netStatus.ExternalIP, netStatus.ISP, freeOut, dfOut, ipOut, routeOut, wifiSummary, sysOut, netOut, wifiOut, extra)
}

type uciSetDirective struct {
	action string // "set", "add_list", "del_list", "delete"
	pkg    string
	key    string
	val    string
	hasVal bool
}

var safeUCIPathPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_@\[\]-]+)+$`)

func isSecretUCIOption(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.HasSuffix(lower, ".key") ||
		strings.HasSuffix(lower, ".password") ||
		strings.HasSuffix(lower, ".secret") ||
		strings.HasSuffix(lower, ".token") ||
		strings.HasSuffix(lower, ".api_key") ||
		strings.HasSuffix(lower, ".private_key") ||
		strings.HasSuffix(lower, ".preshared_key") ||
		strings.HasSuffix(lower, ".pincode")
}

func parseUCISetDirective(rawLine string) (uciSetDirective, bool, error) {
	l := strings.TrimSpace(rawLine)
	if l == "" || strings.HasPrefix(l, "#") {
		return uciSetDirective{}, false, nil
	}
	if strings.HasPrefix(l, "uci ") {
		l = strings.TrimSpace(strings.TrimPrefix(l, "uci "))
		if strings.HasPrefix(l, "-q ") {
			l = strings.TrimSpace(strings.TrimPrefix(l, "-q "))
		}
	}
	if l == "commit" || strings.HasPrefix(l, "commit ") {
		return uciSetDirective{}, false, nil
	}

	action := "set"
	switch {
	case strings.HasPrefix(l, "set "):
		action = "set"
		l = strings.TrimSpace(strings.TrimPrefix(l, "set "))
	case strings.HasPrefix(l, "add_list "):
		action = "add_list"
		l = strings.TrimSpace(strings.TrimPrefix(l, "add_list "))
	case strings.HasPrefix(l, "del_list "):
		action = "del_list"
		l = strings.TrimSpace(strings.TrimPrefix(l, "del_list "))
	case strings.HasPrefix(l, "delete "):
		action = "delete"
		l = strings.TrimSpace(strings.TrimPrefix(l, "delete "))
	}

	if action == "delete" {
		key := unquoteUCIValue(l)
		if !safeUCIPathPattern.MatchString(key) {
			return uciSetDirective{}, false, fmt.Errorf("đường dẫn UCI delete không hợp lệ: %s", key)
		}
		pkg := strings.SplitN(key, ".", 2)[0]
		return uciSetDirective{action: "delete", pkg: pkg, key: key, hasVal: false}, true, nil
	}

	eq := strings.Index(l, "=")
	if eq <= 0 {
		return uciSetDirective{}, false, fmt.Errorf("thiếu dấu '=' trong dòng UCI: %s", rawLine)
	}
	key := strings.TrimSpace(l[:eq])
	if !safeUCIPathPattern.MatchString(key) {
		return uciSetDirective{}, false, fmt.Errorf("đường dẫn UCI không hợp lệ: %s", key)
	}
	rawVal := strings.TrimSpace(l[eq+1:])
	val := unquoteUCIValue(rawVal)
	if strings.ContainsAny(val, "\r\n\x00") {
		return uciSetDirective{}, false, fmt.Errorf("giá trị UCI cho %s chứa ký tự điều khiển không hợp lệ", key)
	}
	pkg := strings.SplitN(key, ".", 2)[0]
	return uciSetDirective{action: action, pkg: pkg, key: key, val: val, hasVal: true}, true, nil
}

var (
	execCmdRegex     = regexp.MustCompile(`(?s)<exec_cmd>(.*?)</exec_cmd>`)
	writeFileRegex   = regexp.MustCompile(`(?s)<write_file\s+path="([^"]+)"\s*>(.*?)</write_file>`)
	uciSetRegex      = regexp.MustCompile(`(?s)<uci_set>(.*?)</uci_set>`)
	memorySaveRegex  = regexp.MustCompile(`(?s)<memory_save>(.*?)</memory_save>`)
	downloadUrlRegex = regexp.MustCompile(`(?s)<download_url>(.*?)</download_url>`)
)

// Executes all root tool tags (<exec_cmd>, <write_file>, <uci_set>, <memory_save>, <download_url>)
func executeAIToolBlocks(respText string, allowExec bool) ([]AIToolAction, []AICmdLog, []string, string) {
	return executeAIToolBlocksForChat(respText, allowExec, "")
}

func executeAIToolBlocksForChat(respText string, allowExec bool, telegramChatID string) ([]AIToolAction, []AICmdLog, []string, string) {
	var allToolEvents []AIToolAction
	var allCmdLogs []AICmdLog
	var cmdOutputs []string

	// 1. Always process <memory_save> blocks
	memMatches := memorySaveRegex.FindAllStringSubmatch(respText, -1)
	for _, m := range memMatches {
		fact := strings.TrimSpace(m[1])
		if fact != "" {
			entry := addAIMemoryEntry("fact", fact)
			allToolEvents = append(allToolEvents, AIToolAction{
				Tool:    "memory_save",
				Command: "Lưu vào bộ nhớ AI (r3p.db -> collections/ai_memory)",
				Output:  entry.Content,
				Success: true,
			})
		}
	}

	// 2. Process <download_url> blocks
	dlMatches := downloadUrlRegex.FindAllStringSubmatch(respText, -1)
	for _, m := range dlMatches {
		u := strings.TrimSpace(m[1])
		if u != "" {
			job, dup, err := enqueueR3pJob(u, "ai_agent", telegramChatID, "", false)
			outMsg := ""
			if err != nil {
				outMsg = "Lỗi thêm URL tải xuống: " + err.Error()
			} else if dup {
				outMsg = "URL đã có trong hàng đợi tải xuống: #" + job.ID
			} else {
				outMsg = "Đã thêm vào hàng đợi R3P Downloader: #" + job.ID + " (" + u + ")"
			}
			allToolEvents = append(allToolEvents, AIToolAction{
				Tool:    "download_url",
				Command: u,
				Output:  outMsg,
				Success: err == nil,
			})
			cmdOutputs = append(cmdOutputs, fmt.Sprintf("[DOWNLOAD_URL %s] -> %s", u, outMsg))
		}
	}

	if allowExec {
		// 3. Process <write_file path="...">...</write_file>
		wfMatches := writeFileRegex.FindAllStringSubmatch(respText, -1)
		for _, m := range wfMatches {
			targetPath := strings.TrimSpace(m[1])
			fileContent := strings.TrimPrefix(m[2], "\n")
			if targetPath == "" {
				continue
			}
			t0 := time.Now()
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			perm := os.FileMode(0644)
			if strings.HasSuffix(targetPath, ".sh") || strings.HasPrefix(targetPath, "/etc/init.d/") || strings.HasPrefix(targetPath, "/etc/hotplug.d/") {
				perm = 0755
			}
			wErr := os.WriteFile(targetPath, []byte(fileContent), perm)
			durMs := time.Since(t0).Milliseconds()
			outMsg := fmt.Sprintf("Đã ghi %d bytes vào %s (mode %04o)", len(fileContent), targetPath, perm)
			exitCode := 0
			if wErr != nil {
				exitCode = 1
				outMsg = "Lỗi ghi file " + targetPath + ": " + wErr.Error()
			}
			allToolEvents = append(allToolEvents, AIToolAction{
				Tool:    "write_file",
				Command: "write " + targetPath,
				Output:  outMsg,
				Success: wErr == nil,
			})
			allCmdLogs = append(allCmdLogs, AICmdLog{
				Command:    "write_file " + targetPath,
				Output:     outMsg,
				ExitCode:   exitCode,
				DurationMs: durMs,
			})
			cmdOutputs = append(cmdOutputs, fmt.Sprintf("[WRITE_FILE %s]\n%s", targetPath, outMsg))
		}

		// 4. Process <uci_set>...</uci_set>
		uciMatches := uciSetRegex.FindAllStringSubmatch(respText, -1)
		for _, m := range uciMatches {
			lines := strings.Split(strings.TrimSpace(m[1]), "\n")
			t0 := time.Now()
			pkgs := map[string]bool{}
			var applied []string
			var cmdParts []string
			var uciErrs []string
			var changedDirs []uciSetDirective

			for _, line := range lines {
				dir, ok, parseErr := parseUCISetDirective(line)
				if parseErr != nil {
					uciErrs = append(uciErrs, parseErr.Error())
					continue
				}
				if !ok {
					continue
				}
				if dir.action == "delete" {
					if out, err := runCmd(5, "uci", "delete", dir.key); err != nil {
						uciErrs = append(uciErrs, "delete "+dir.key+": "+strings.TrimSpace(out+" "+err.Error()))
					} else {
						cmdLine := "uci delete " + dir.key
						applied = append(applied, cmdLine)
						cmdParts = append(cmdParts, cmdLine)
						pkgs[dir.pkg] = true
						changedDirs = append(changedDirs, dir)
					}
					continue
				}
				arg := dir.key + "=" + dir.val
				if out, err := runCmd(5, "uci", dir.action, arg); err != nil {
					uciErrs = append(uciErrs, dir.action+" "+dir.key+": "+strings.TrimSpace(out+" "+err.Error()))
				} else {
					dispVal := dir.val
					if isSecretUCIOption(dir.key) {
						dispVal = "***REDACTED***"
					}
					applied = append(applied, fmt.Sprintf("uci %s %s=%q", dir.action, dir.key, dispVal))
					cmdParts = append(cmdParts, "uci "+dir.action+" "+dir.key)
					pkgs[dir.pkg] = true
					changedDirs = append(changedDirs, dir)
				}
			}

			var pkgNames []string
			for pkg := range pkgs {
				pkgNames = append(pkgNames, pkg)
			}
			sort.Strings(pkgNames)

			for _, pkg := range pkgNames {
				if out, err := runCmd(5, "uci", "commit", pkg); err != nil {
					uciErrs = append(uciErrs, "uci commit "+pkg+": "+strings.TrimSpace(out+" "+err.Error()))
					continue
				}
				applied = append(applied, "uci commit "+pkg)
				cmdParts = append(cmdParts, "uci commit "+pkg)
			}

			// Verify committed state directly from UCI before reporting back to the LLM.
			for _, dir := range changedDirs {
				gotRaw, getErr := runCmd(4, "uci", "-q", "get", dir.key)
				gotVal := strings.TrimRight(gotRaw, "\r\n")
				if dir.action == "delete" {
					if getErr == nil && gotVal != "" {
						uciErrs = append(uciErrs, fmt.Sprintf("verify %s: vẫn còn giá trị %q sau khi xóa", dir.key, gotVal))
					} else {
						applied = append(applied, "verified deleted: "+dir.key)
					}
					continue
				}
				if dir.action == "set" {
					if getErr != nil {
						uciErrs = append(uciErrs, fmt.Sprintf("verify %s: uci get lỗi (%v)", dir.key, getErr))
						continue
					}
					if isSecretUCIOption(dir.key) {
						applied = append(applied, fmt.Sprintf("verified: %s=***REDACTED*** (len=%d)", dir.key, len(gotVal)))
					} else if gotVal != dir.val {
						uciErrs = append(uciErrs, fmt.Sprintf("verify %s: mong đợi %q nhưng thực tế là %q", dir.key, dir.val, gotVal))
					} else {
						applied = append(applied, fmt.Sprintf("verified: %s=%q", dir.key, gotVal))
					}
				}
			}

			// Reload affected services and surface parent radio lock diagnostics for wireless.
			for _, pkg := range pkgNames {
				reloaded := reloadUCIService(pkg)
				if reloaded {
					applied = append(applied, "reload "+pkg+": OK")
				} else {
					applied = append(applied, "reload "+pkg+": skipped_or_failed")
				}
				if pkg == "wireless" {
					r0, _ := runCmd(2, "uci", "-q", "get", "wireless.radio0.disabled")
					d0, _ := runCmd(2, "uci", "-q", "get", "wireless.default_radio0.disabled")
					r1, _ := runCmd(2, "uci", "-q", "get", "wireless.radio1.disabled")
					d1, _ := runCmd(2, "uci", "-q", "get", "wireless.default_radio1.disabled")
					applied = append(applied, fmt.Sprintf("wireless radio status: radio0.disabled=%q default_radio0.disabled=%q | radio1.disabled=%q default_radio1.disabled=%q",
						strings.TrimSpace(r0), strings.TrimSpace(d0), strings.TrimSpace(r1), strings.TrimSpace(d1)))
				}
			}

			durMs := time.Since(t0).Milliseconds()
			outMsg := strings.Join(applied, "\n")
			if len(uciErrs) > 0 {
				if outMsg != "" {
					outMsg += "\n"
				}
				outMsg += "Errors:\n" + strings.Join(uciErrs, "\n")
			}
			allToolEvents = append(allToolEvents, AIToolAction{
				Tool:    "uci_set",
				Command: strings.Join(cmdParts, " && "),
				Output:  outMsg,
				Success: len(uciErrs) == 0,
			})
			allCmdLogs = append(allCmdLogs, AICmdLog{
				Command:    "uci_set (" + fmt.Sprintf("%d ops", len(changedDirs)) + ")",
				Output:     outMsg,
				ExitCode:   map[bool]int{true: 0, false: 1}[len(uciErrs) == 0],
				DurationMs: durMs,
			})
			cmdOutputs = append(cmdOutputs, fmt.Sprintf("[UCI_SET]\n%s", outMsg))
		}

		// 5. Process <exec_cmd>...</exec_cmd>
		matches := execCmdRegex.FindAllStringSubmatch(respText, -1)
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
	}

	cleaned := execCmdRegex.ReplaceAllString(respText, "")
	cleaned = writeFileRegex.ReplaceAllString(cleaned, "")
	cleaned = uciSetRegex.ReplaceAllString(cleaned, "")
	cleaned = memorySaveRegex.ReplaceAllString(cleaned, "")
	cleaned = downloadUrlRegex.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return allToolEvents, allCmdLogs, cmdOutputs, cleaned
}

func callLLMAPI(cfg AIConfig, messages []map[string]string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURLForProvider(cfg.Provider)
	}
	if err := validateAIBaseURL(baseURL); err != nil {
		return "", err
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModelForProvider(cfg.Provider)
	}

	const maxLLMRespBytes = 2 << 20
	client := newAIHTTPClient(baseURL, 45*time.Second)

	redactAPIKey := func(s string) string {
		if strings.TrimSpace(cfg.APIKey) != "" {
			return strings.ReplaceAll(s, strings.TrimSpace(cfg.APIKey), "[REDACTED]")
		}
		return s
	}

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
			return "", errors.New(redactAPIKey(err.Error()))
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := client.Do(req)
		if err != nil {
			return "", errors.New(redactAPIKey(err.Error()))
		}
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxLLMRespBytes))
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("Anthropic HTTP %d: %s", resp.StatusCode, redactAPIKey(string(respBytes)))
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

	modelsToTry := []string{model}
	if strings.ToLower(cfg.Provider) == "gemini" {
		geminiCandidates := []string{
			"gemini-3.1-flash-lite",
			"gemini-3.1-flash-lite-preview",
			"gemini-flash-latest",
			"gemini-3-flash-preview",
		}
		for _, fb := range geminiCandidates {
			if fb != model {
				modelsToTry = append(modelsToTry, fb)
			}
		}
	}

	var lastErr error
	for _, tryModel := range modelsToTry {
		payload := map[string]interface{}{
			"model":       tryModel,
			"messages":    messages,
			"temperature": 0.2,
		}
		b, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
		if err != nil {
			return "", errors.New(redactAPIKey(err.Error()))
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", errors.New(redactAPIKey(err.Error()))
		}
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxLLMRespBytes))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("API HTTP %d (%s): %s", resp.StatusCode, tryModel, redactAPIKey(string(respBytes)))
			if resp.StatusCode == 404 || resp.StatusCode == 503 || resp.StatusCode == 429 {
				// 429: Free Tier quota exceeded for model; continue to next fallback candidate
				continue
			}
			return "", lastErr
		}
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(respBytes, &parsed); err != nil || len(parsed.Choices) == 0 {
			return "", fmt.Errorf("invalid API response: %s", redactAPIKey(string(respBytes)))
		}
		// If a fallback candidate succeeded, update system config so next calls use working model
		if tryModel != model && strings.ToLower(cfg.Provider) == "gemini" {
			aiConfigUpdateMu.Lock()
			latest := loadAIConfig()
			latest.Model = tryModel
			saveAIConfig(latest)
			aiConfigUpdateMu.Unlock()
		}
		return parsed.Choices[0].Message.Content, nil
	}
	return "", lastErr
}

// Process a text message coming from Telegram Bot (when not a bare URL)
func processTelegramAIChat(text, chatID string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || isBareURLMessage(trimmed) {
		return ""
	}
	cfg := loadAIConfig()
	if !cfg.TelegramAIEnabled {
		return "ℹ️ Tính năng AI qua Telegram đang tạm tắt trong tab Trợ Lý AI."
	}
	activeSessID, _ := getActiveSessionMessages(cfg.ActiveSessionID)

	// Direct root shell command via Telegram (!cmd, /exec cmd, /sh cmd)
	if strings.HasPrefix(trimmed, "!") || strings.HasPrefix(trimmed, "/exec ") || strings.HasPrefix(trimmed, "/sh ") {
		if !cfg.AllowExec {
			return "⛔ Chế độ thực thi lệnh tự động (AllowExec) đang tắt trong cấu hình Trợ Lý AI."
		}
		dlMu.Lock()
		allowedChat := strings.TrimSpace(dlConfig.TelegramAllowedChatId)
		dlMu.Unlock()
		if allowedChat == "" || !isAllowedTelegramChat(allowedChat, chatID) {
			return "⛔ Lệnh shell từ Telegram chỉ cho phép khi đã khóa Allowed Chat ID hợp lệ."
		}
		cmdStr := trimmed
		switch {
		case strings.HasPrefix(cmdStr, "!"):
			cmdStr = strings.TrimPrefix(cmdStr, "!")
		case strings.HasPrefix(cmdStr, "/exec "):
			cmdStr = strings.TrimPrefix(cmdStr, "/exec ")
		case strings.HasPrefix(cmdStr, "/sh "):
			cmdStr = strings.TrimPrefix(cmdStr, "/sh ")
		}
		cmdStr = strings.TrimSpace(cmdStr)
		if cmdStr == "" {
			return "ℹ️ Nhập lệnh sau `!` hoặc `/exec `, ví dụ: `!uptime`"
		}
		appendMessageToSession(activeSessID, AIChatMessage{
			Role:      "user",
			Content:   "📱 [Telegram] " + trimmed,
			Timestamp: time.Now().Format("15:04:05"),
			Source:    "telegram",
		})
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
		if len(out) > 3000 {
			out = out[:3000] + "\n...(truncated)"
		}
		replyMsg := AIChatMessage{
			Role:       "assistant",
			Content:    fmt.Sprintf("Đã thực thi lệnh từ Telegram (`root@R3P`):\n```\n$ %s\n%s\n```", cmdStr, out),
			Timestamp:  time.Now().Format("15:04:05"),
			Source:     "telegram",
			ToolEvents: []AIToolAction{{Tool: "root_shell", Command: cmdStr, Output: out, Success: err == nil}},
			Commands:   []AICmdLog{{Command: cmdStr, Output: out, ExitCode: exitCode, DurationMs: durMs}},
		}
		appendMessageToSession(activeSessID, replyMsg)
		return fmt.Sprintf("⚡ [R3P Root Shell]\n$ %s\n\n%s", cmdStr, out)
	}

	appendMessageToSession(activeSessID, AIChatMessage{
		Role:      "user",
		Content:   "📱 [Telegram] " + trimmed,
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "telegram",
	})

	if cfg.APIKey == "" {
		reply := "🤖 R3P AI Agent: Chưa cấu hình API Key trong tab Trợ Lý AI trên Router. Bạn vẫn có thể gửi `!<lệnh>` (ví dụ: `!df -h`, `!uptime`, `!ip -br addr`) qua Telegram để chạy lệnh root trực tiếp, hoặc gửi URL để tải video!"
		appendMessageToSession(activeSessID, AIChatMessage{
			Role:      "assistant",
			Content:   reply,
			Timestamp: time.Now().Format("15:04:05"),
			Source:    "telegram",
		})
		return reply
	}

	sysPrompt := buildSystemArchitecturePrompt(cfg.SystemPromptExtra)
	llmMsgs := []map[string]string{
		{"role": "system", "content": sysPrompt},
	}
	_, sessMsgs := getActiveSessionMessages(activeSessID)
	startIdx := 0
	if len(sessMsgs) > 10 {
		startIdx = len(sessMsgs) - 10
	}
	for _, m := range sessMsgs[startIdx:] {
		if m.Role == "user" || m.Role == "assistant" {
			llmMsgs = append(llmMsgs, map[string]string{"role": m.Role, "content": m.Content})
		}
	}

	var allToolEvents []AIToolAction
	var allCmdLogs []AICmdLog
	var finalReply string

	effectiveExec := cfg.AutoExecute && cfg.AllowExec
	for turn := 0; turn < 3; turn++ {
		respText, err := callLLMAPI(cfg, llmMsgs)
		if err != nil {
			finalReply = "❌ Lỗi kết nối AI API: " + err.Error()
			break
		}
		evts, logs, outputs, cleaned := executeAIToolBlocksForChat(respText, effectiveExec, chatID)
		allToolEvents = append(allToolEvents, evts...)
		allCmdLogs = append(allCmdLogs, logs...)
		finalReply = cleaned
		if len(outputs) == 0 {
			break
		}
		llmMsgs = append(llmMsgs,
			map[string]string{"role": "assistant", "content": respText},
			map[string]string{"role": "user", "content": "Kết quả thực thi công cụ trên Router:\n```\n" + strings.Join(outputs, "\n\n") + "\n```\nChỉ báo thành công nếu không có Errors và giá trị verified/đầu ra khớp đúng yêu cầu; nêu rõ nếu còn cảnh báo (vd radio bị disabled). Trả lời ngắn gọn, rõ ràng bằng tiếng Việt (không lặp lại thẻ công cụ)."},
		)
	}
	if strings.TrimSpace(finalReply) == "" {
		finalReply = "✅ Đã thực thi yêu cầu trên Router R3P."
	}
	appendMessageToSession(activeSessID, AIChatMessage{
		Role:       "assistant",
		Content:    finalReply,
		Timestamp:  time.Now().Format("15:04:05"),
		Source:     "telegram",
		ToolEvents: allToolEvents,
		Commands:   allCmdLogs,
	})
	return "🤖 [R3P Root AI]\n" + finalReply
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
		safeCfg.APIKey = ""
		aiMu.Lock()
		sessions := cloneAISessions(loadAISessionsLocked())
		aiMu.Unlock()
		activeID, hist := getActiveSessionMessages(cfg.ActiveSessionID)
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
			"telegram_ai_enabled": cfg.TelegramAIEnabled,
			"auto_remember":       cfg.AutoRemember,
			"active_session_id":   activeID,
			"system_prompt_extra": cfg.SystemPromptExtra,
			"system_note":         cfg.SystemNote,
			"system_context":      buildLightweightSystemContext(cfg.SystemPromptExtra),
			"blueprint_path":      aiBlueprintFile,
			"memories":            loadAIMemories(),
			"sessions":            sessions,
			"history":             hist,
		})

	case http.MethodPut:
		aiConfigUpdateMu.Lock()
		defer aiConfigUpdateMu.Unlock()

		var body struct {
			Provider          string `json:"provider"`
			APIKey            string `json:"api_key"`
			ClearKey          bool   `json:"clear_key"`
			BaseURL           string `json:"base_url"`
			Model             string `json:"model"`
			AutoExecute       *bool  `json:"auto_execute"`
			AllowExec         *bool  `json:"allow_exec"`
			TelegramAIEnabled *bool  `json:"telegram_ai_enabled"`
			AutoRemember      *bool  `json:"auto_remember"`
			ActiveSessionID   string `json:"active_session_id"`
			SystemPromptExtra string `json:"system_prompt_extra"`
			SystemNote        string `json:"system_note"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		if strings.TrimSpace(body.BaseURL) != "" {
			if err := validateAIBaseURL(body.BaseURL); err != nil {
				writeJSON(w, http.StatusBadRequest, false, err.Error())
				return
			}
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
		if body.TelegramAIEnabled != nil {
			cfg.TelegramAIEnabled = *body.TelegramAIEnabled
		}
		if body.AutoRemember != nil {
			cfg.AutoRemember = *body.AutoRemember
		}
		if strings.TrimSpace(body.ActiveSessionID) != "" {
			cfg.ActiveSessionID = strings.TrimSpace(body.ActiveSessionID)
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
			"saved":               true,
			"config":              updated,
			"provider":            updated.Provider,
			"api_key_configured":  updated.HasAPIKey,
			"has_api_key":         updated.HasAPIKey,
			"api_key_masked":      updated.APIKeyMasked,
			"base_url":            updated.BaseURL,
			"model":               updated.Model,
			"auto_execute":        updated.AutoExecute,
			"allow_exec":          updated.AllowExec,
			"telegram_ai_enabled": updated.TelegramAIEnabled,
			"auto_remember":       updated.AutoRemember,
			"active_session_id":   updated.ActiveSessionID,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// GET / POST / DELETE /api/v1/ai/memory
func handleAIMemory(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"memories": loadAIMemories(),
		})
	case http.MethodPost:
		var body struct {
			Category string `json:"category"`
			Content  string `json:"content"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil || strings.TrimSpace(body.Content) == "" {
			writeJSON(w, http.StatusBadRequest, false, "Nội dung ghi nhớ không được để trống")
			return
		}
		entry := addAIMemoryEntry(body.Category, body.Content)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"entry":    entry,
			"memories": loadAIMemories(),
		})
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		aiMu.Lock()
		_ = loadAIMemoriesLocked()
		if id == "" || id == "all" {
			aiMemories = []AIMemoryEntry{}
		} else {
			var kept []AIMemoryEntry
			for _, m := range aiMemories {
				if m.ID != id {
					kept = append(kept, m)
				}
			}
			aiMemories = kept
		}
		saveAIMemoriesLocked()
		res := append([]AIMemoryEntry{}, aiMemories...)
		aiMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"memories": res,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// GET / POST / PUT / DELETE /api/v1/ai/sessions
func handleAISessions(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg := loadAIConfig()
		aiMu.Lock()
		sessions := cloneAISessions(loadAISessionsLocked())
		aiMu.Unlock()
		activeID, msgs := getActiveSessionMessages(cfg.ActiveSessionID)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"active_session_id": activeID,
			"sessions":          sessions,
			"history":           msgs,
		})

	case http.MethodPost:
		var body struct {
			Title string `json:"title"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
		title := strings.TrimSpace(body.Title)
		if title == "" {
			title = "Phiên Mới (" + time.Now().Format("15:04") + ")"
		}
		newID := fmt.Sprintf("sess-%d", time.Now().UnixNano()/1e6)
		nowStr := time.Now().Format("2006-01-02 15:04")
		newSess := AIChatSession{
			ID:        newID,
			Title:     title,
			CreatedAt: nowStr,
			UpdatedAt: nowStr,
			Messages:  defaultWelcomeMessages(),
		}
		aiMu.Lock()
		_ = loadAISessionsLocked()
		aiSessions = append([]AIChatSession{newSess}, aiSessions...)
		saveAISessionsLocked()
		sessionsCopy := cloneAISessions(aiSessions)
		aiMu.Unlock()

		cfg := loadAIConfig()
		cfg.ActiveSessionID = newID
		saveAIConfig(cfg)

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"active_session_id": newID,
			"session":           newSess,
			"sessions":          sessionsCopy,
			"history":           newSess.Messages,
		})

	case http.MethodPut:
		var body struct {
			SessionID string `json:"session_id"`
			Title     string `json:"title"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil || strings.TrimSpace(body.SessionID) == "" {
			writeJSON(w, http.StatusBadRequest, false, "session_id is required")
			return
		}
		sid := strings.TrimSpace(body.SessionID)
		aiMu.Lock()
		_ = loadAISessionsLocked()
		if strings.TrimSpace(body.Title) != "" {
			for i := range aiSessions {
				if aiSessions[i].ID == sid {
					aiSessions[i].Title = strings.TrimSpace(body.Title)
					break
				}
			}
			saveAISessionsLocked()
		}
		sessionsCopy := cloneAISessions(aiSessions)
		aiMu.Unlock()

		cfg := loadAIConfig()
		cfg.ActiveSessionID = sid
		saveAIConfig(cfg)
		activeID, msgs := getActiveSessionMessages(sid)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"active_session_id": activeID,
			"sessions":          sessionsCopy,
			"history":           msgs,
		})

	case http.MethodDelete:
		sid := strings.TrimSpace(r.URL.Query().Get("id"))
		aiMu.Lock()
		_ = loadAISessionsLocked()
		var kept []AIChatSession
		for _, s := range aiSessions {
			if s.ID != sid {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			nowStr := time.Now().Format("2006-01-02 15:04")
			kept = []AIChatSession{{
				ID:        "default",
				Title:     "Phiên Điều Khiển Chính",
				CreatedAt: nowStr,
				UpdatedAt: nowStr,
				Messages:  defaultWelcomeMessages(),
			}}
		}
		aiSessions = kept
		saveAISessionsLocked()
		nextActive := aiSessions[0].ID
		sessionsCopy := cloneAISessions(aiSessions)
		msgsCopy := append([]AIChatMessage{}, aiSessions[0].Messages...)
		aiMu.Unlock()

		cfg := loadAIConfig()
		cfg.ActiveSessionID = nextActive
		saveAIConfig(cfg)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"active_session_id": nextActive,
			"sessions":          sessionsCopy,
			"history":           msgsCopy,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// GET / PUT /api/v1/ai/blueprint
func handleAIBlueprint(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		content := ensureSystemBlueprintFile()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"path":    aiBlueprintFile,
			"content": content,
		})
	case http.MethodPut:
		var body struct {
			Content string `json:"content"`
			Reset   bool   `json:"reset"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 131072)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		content := body.Content
		if body.Reset || strings.TrimSpace(content) == "" {
			content = defaultSystemBlueprintMD
			writeSystemBlueprintToStorage(content, currentSystemBlueprintRev, false)
		} else {
			writeSystemBlueprintToStorage(content, currentSystemBlueprintRev, true)
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"saved":   true,
			"path":    aiBlueprintFile,
			"content": content,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// GET / PUT / POST / DELETE /api/v1/ai/quick-commands
func handleAIQuickCommands(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var parsed map[string]interface{}
		if !settingsDBLoadJSON(bucketCollections, "ai_quick_commands", aiQuickCommandsFile, &parsed) || len(parsed) == 0 {
			var defMap map[string]interface{}
			_ = json.Unmarshal([]byte(defaultQuickCommandsJSON), &defMap)
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"commands": defMap,
				"custom":   false,
			})
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"commands": parsed,
			"custom":   true,
		})
	case http.MethodPut, http.MethodPost:
		var body struct {
			Commands map[string]interface{} `json:"commands"`
			Reset    bool                   `json:"reset"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 262144)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		if body.Reset {
			_ = settingsDBDelete(bucketCollections, "ai_quick_commands")
			_ = os.Remove(aiQuickCommandsFile)
			var defMap map[string]interface{}
			_ = json.Unmarshal([]byte(defaultQuickCommandsJSON), &defMap)
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"saved":    true,
				"reset":    true,
				"commands": defMap,
			})
			return
		}
		if body.Commands == nil {
			writeJSON(w, http.StatusBadRequest, false, "missing commands")
			return
		}
		if err := settingsDBSaveJSON(bucketCollections, "ai_quick_commands", aiQuickCommandsFile, 0644, body.Commands); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "failed to save commands: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"saved":    true,
			"commands": body.Commands,
		})
	case http.MethodDelete:
		_ = settingsDBDelete(bucketCollections, "ai_quick_commands")
		_ = os.Remove(aiQuickCommandsFile)
		var defMap map[string]interface{}
		_ = json.Unmarshal([]byte(defaultQuickCommandsJSON), &defMap)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"deleted":  true,
			"commands": defMap,
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
	cfg := loadAIConfig()
	activeSessID, _ := getActiveSessionMessages(cfg.ActiveSessionID)

	if r.Method == http.MethodDelete {
		aiMu.Lock()
		_ = loadAISessionsLocked()
		for i := range aiSessions {
			if aiSessions[i].ID == activeSessID {
				aiSessions[i].Messages = defaultWelcomeMessages()
				aiSessions[i].UpdatedAt = time.Now().Format("2006-01-02 15:04")
				break
			}
		}
		saveAISessionsLocked()
		sessionsCopy := cloneAISessions(aiSessions)
		aiMu.Unlock()
		_, hist := getActiveSessionMessages(activeSessID)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"cleared":  true,
			"history":  hist,
			"sessions": sessionsCopy,
		})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}

	var body struct {
		SessionID    string     `json:"session_id"`
		Message      string     `json:"message"`
		Execute      *bool      `json:"execute"`
		BrowserReply string     `json:"browser_reply"`
		PrevCommands []AICmdLog `json:"prev_commands"`
		BridgeTurn   int        `json:"bridge_turn"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	if strings.TrimSpace(body.SessionID) != "" {
		activeSessID = strings.TrimSpace(body.SessionID)
	}

	// Server policy is upper bound: request can only restrict execution, never enable it above cfg.AllowExec.
	effectiveExec := cfg.AutoExecute && cfg.AllowExec
	if body.Execute != nil && !*body.Execute {
		effectiveExec = false
	}

	// Browser-Bridged AI Continuation
	if strings.TrimSpace(body.BrowserReply) != "" {
		respText := strings.TrimSpace(body.BrowserReply)
		allToolEvents, newCmdLogs, cmdOutputs, cleanReply := executeAIToolBlocks(respText, effectiveExec)
		var allCmdLogs []AICmdLog
		allCmdLogs = append(allCmdLogs, body.PrevCommands...)
		allCmdLogs = append(allCmdLogs, newCmdLogs...)

		if len(cmdOutputs) > 0 && body.BridgeTurn < 3 && (len(body.PrevCommands) == 0 || body.BridgeTurn > 0) {
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"needs_browser_followup": true,
				"bridge_turn":            body.BridgeTurn + 1,
				"commands":               allCmdLogs,
				"tool_events":            allToolEvents,
				"followup_user_prompt":   "Kết quả thực thi công cụ trên Router:\n```\n" + strings.Join(cmdOutputs, "\n\n") + "\n```\nChỉ báo thành công nếu không có Errors và giá trị verified/đầu ra khớp đúng yêu cầu; nêu rõ nếu còn cảnh báo (vd radio bị disabled). Trả lời ngắn gọn, rõ ràng bằng tiếng Việt cho người dùng (không lặp lại thẻ công cụ).",
			})
			return
		}

		if cleanReply == "" {
			cleanReply = "Đã thực thi lệnh hệ thống trên Router (`root@R3P`)."
		}
		replyMsg := AIChatMessage{
			Role:       "assistant",
			Content:    cleanReply,
			Timestamp:  time.Now().Format("15:04:05"),
			Source:     "web",
			ToolEvents: allToolEvents,
			Commands:   allCmdLogs,
		}
		histCopy := appendMessageToSession(activeSessID, replyMsg)
		aiMu.Lock()
		sessionsCopy := cloneAISessions(loadAISessionsLocked())
		aiMu.Unlock()

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"reply":       replyMsg,
			"tool_events": allToolEvents,
			"commands":    allCmdLogs,
			"history":     histCopy,
			"sessions":    sessionsCopy,
			"memories":    loadAIMemories(),
		})
		return
	}

	userMsg := strings.TrimSpace(body.Message)
	if userMsg == "" {
		writeJSON(w, http.StatusBadRequest, false, "empty message")
		return
	}

	// Filter bare URL messages: route directly to R3P Downloader without cluttering AI chat
	if isBareURLMessage(userMsg) {
		urls := urlExtractRegex.FindAllString(userMsg, 5)
		var queuedIDs []string
		for _, u := range urls {
			if job, _, err := enqueueR3pJob(u, "ai_url_filter", "", "", false); err == nil && job != nil {
				queuedIDs = append(queuedIDs, "#"+job.ID)
			}
		}
		_, histCopy := getActiveSessionMessages(activeSessID)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"filtered_url": true,
			"message":      "Đã chuyển thẳng URL sang R3P Downloader (" + strings.Join(queuedIDs, ", ") + ") và lọc khỏi cửa sổ giao tiếp AI.",
			"history":      histCopy,
		})
		return
	}

	appendMessageToSession(activeSessID, AIChatMessage{
		Role:      "user",
		Content:   userMsg,
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "web",
	})

	// Direct root shell command mode (!cmd, /exec cmd, /sh cmd)
	if strings.HasPrefix(userMsg, "!") || strings.HasPrefix(userMsg, "/exec ") || strings.HasPrefix(userMsg, "/sh ") {
		if !effectiveExec {
			replyMsg := AIChatMessage{
				Role:      "assistant",
				Content:   "⛔ Chế độ tự động thực thi lệnh đang tắt. Bật AllowExec trong cấu hình Trợ Lý AI để chạy lệnh trực tiếp.",
				Timestamp: time.Now().Format("15:04:05"),
				Source:    "web",
			}
			histCopy := appendMessageToSession(activeSessID, replyMsg)
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"reply":   replyMsg,
				"history": histCopy,
			})
			return
		}
		cmdStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(userMsg, "!"), "/exec "), "/sh "))
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
		reply := "Đã thực thi lệnh trực tiếp trên Router (`root@R3P`):"
		action := AIToolAction{Tool: "root_shell", Command: cmdStr, Output: out, Success: err == nil}
		cmdLog := AICmdLog{Command: cmdStr, Output: out, ExitCode: exitCode, DurationMs: durMs}
		replyMsg := AIChatMessage{
			Role:       "assistant",
			Content:    reply,
			Timestamp:  time.Now().Format("15:04:05"),
			Source:     "web",
			ToolEvents: []AIToolAction{action},
			Commands:   []AICmdLog{cmdLog},
		}
		histCopy := appendMessageToSession(activeSessID, replyMsg)
		aiMu.Lock()
		sessionsCopy := cloneAISessions(loadAISessionsLocked())
		aiMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"reply":       replyMsg,
			"tool_events": []AIToolAction{action},
			"commands":    []AICmdLog{cmdLog},
			"history":     histCopy,
			"sessions":    sessionsCopy,
		})
		return
	}
	if cfg.APIKey == "" && !isLocalAIBaseURL(cfg.BaseURL) {
		t0 := time.Now()
		out, _ := runCmd(5, "sh", "-c", "uptime; df -h /overlay /mnt/sda1 2>/dev/null; ip -br addr")
		durMs := time.Since(t0).Milliseconds()
		replyText := fmt.Sprintf("Chưa cấu hình **API Key** cho nhà cung cấp AI (**%s**). Hãy nhập API Key ở khung **Cấu Hình Nhúng API Cho AI Agent** phía trên rồi bấm **Lưu Cấu Hình API**.\n\n💡 *Mẹo:* Ngay cả khi chưa nhập API Key, bạn có thể bấm vào **Bảng Lệnh Kiểm Tra & Cài Đặt Có Sẵn** ở cột bên trái hoặc gõ `!<lệnh>` để chạy lệnh trực tiếp với quyền `root`!", strings.ToUpper(cfg.Provider))
		cmdLog := AICmdLog{Command: "uptime && df -h && ip -br addr", Output: out, ExitCode: 0, DurationMs: durMs}
		replyMsg := AIChatMessage{
			Role:      "assistant",
			Content:   replyText,
			Timestamp: time.Now().Format("15:04:05"),
			Source:    "web",
			Commands:  []AICmdLog{cmdLog},
		}
		histCopy := appendMessageToSession(activeSessID, replyMsg)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"reply":   replyMsg,
			"history": histCopy,
		})
		return
	}

	sysPrompt := buildSystemArchitecturePrompt(cfg.SystemPromptExtra)
	llmMsgs := []map[string]string{
		{"role": "system", "content": sysPrompt},
	}
	_, sessMsgs := getActiveSessionMessages(activeSessID)
	startIdx := 0
	if len(sessMsgs) > 12 {
		startIdx = len(sessMsgs) - 12
	}
	for _, m := range sessMsgs[startIdx:] {
		if m.Role == "user" || m.Role == "assistant" {
			llmMsgs = append(llmMsgs, map[string]string{"role": m.Role, "content": m.Content})
		}
	}

	var allToolEvents []AIToolAction
	var allCmdLogs []AICmdLog
	var finalReply string

	for turn := 0; turn < 3; turn++ {
		respText, err := callLLMAPI(cfg, llmMsgs)
		if err != nil {
			errStr := err.Error()
			isNativeAnthropic := strings.ToLower(cfg.Provider) == "anthropic" && !strings.Contains(cfg.BaseURL, "/openai")
			if turn == 0 && !isNativeAnthropic && (strings.Contains(errStr, "dial tcp") || strings.Contains(errStr, "lookup ") || strings.Contains(errStr, "server misbehaving") || strings.Contains(errStr, "network is unreachable") || strings.Contains(errStr, "i/o timeout")) {
				baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
				if baseURL == "" {
					baseURL = defaultBaseURLForProvider(cfg.Provider)
				}
				endpoint := baseURL
				if !strings.HasSuffix(endpoint, "/chat/completions") {
					endpoint += "/chat/completions"
				}
				writeJSON(w, http.StatusOK, true, map[string]interface{}{
					"needs_browser_bridge": true,
					"bridge_turn":          body.BridgeTurn + 1,
					"bridge": map[string]interface{}{
						"provider": cfg.Provider,
						"endpoint": endpoint,
						"model":    cfg.Model,
						"messages": llmMsgs,
					},
				})
				return
			}
			finalReply = "❌ Lỗi kết nối AI API: `" + errStr + "`\nHãy kiểm tra lại kết nối Internet của Router và API Key ở khung cài đặt phía trên."
			break
		}
		evts, logs, outputs, cleaned := executeAIToolBlocks(respText, effectiveExec)
		allToolEvents = append(allToolEvents, evts...)
		allCmdLogs = append(allCmdLogs, logs...)
		finalReply = cleaned
		if len(outputs) == 0 {
			break
		}
		llmMsgs = append(llmMsgs,
			map[string]string{"role": "assistant", "content": respText},
			map[string]string{"role": "user", "content": "Kết quả thực thi công cụ trên Router:\n```\n" + strings.Join(outputs, "\n\n") + "\n```\nChỉ báo thành công nếu không có Errors và giá trị verified/đầu ra khớp đúng yêu cầu; nêu rõ nếu còn cảnh báo (vd radio bị disabled). Trả lời ngắn gọn, rõ ràng cho người dùng (không lặp lại thẻ công cụ nếu đã xong)."},
		)
	}

	if strings.TrimSpace(finalReply) == "" {
		finalReply = "Đã thực thi yêu cầu hệ thống trên Router (`root@R3P`)."
	}

	replyMsg := AIChatMessage{
		Role:       "assistant",
		Content:    finalReply,
		Timestamp:  time.Now().Format("15:04:05"),
		Source:     "web",
		ToolEvents: allToolEvents,
		Commands:   allCmdLogs,
	}

	histCopy := appendMessageToSession(activeSessID, replyMsg)
	aiMu.Lock()
	sessionsCopy := cloneAISessions(loadAISessionsLocked())
	aiMu.Unlock()

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"reply":       replyMsg,
		"tool_events": allToolEvents,
		"commands":    allCmdLogs,
		"history":     histCopy,
		"sessions":    sessionsCopy,
		"memories":    loadAIMemories(),
	})
}
