// R3P Firmware — Full-Featured System Info API (Phase 1)
// Cross-compiled for MIPS little-endian (MT7621, Xiaomi Mi Router 3 Pro)
// Build: GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0
//
//	go build -ldflags="-s -w" -trimpath -o r3p-api main.go
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var Version = "v1.1.16"

// BuildDate is injected at compile time via -ldflags="-X main.BuildDate=..."
var BuildDate = "2026-11-06"

var startTime = time.Now()

// ─── Known safe services for toggle endpoint ───────────────────────────────

var allowedServices = map[string]struct{}{
	"dropbear": {}, "dnsmasq": {}, "rpcd": {},
	"uhttpd": {}, "ntpd": {}, "odhcp6c": {},
	"adblock-fast": {}, "r3p-aria2": {}, "ksmbd": {},
}

// ─── Embedded top-100 OUI vendor prefix map ────────────────────────────────

var ouiMap = map[string]string{
	"00:00:0C": "Cisco",
	"00:0C:29": "VMware",
	"00:1A:11": "Google",
	"00:50:56": "VMware",
	"00:1B:63": "Apple",
	"00:1C:B3": "Apple",
	"00:23:12": "Apple",
	"00:26:BB": "Apple",
	"3C:15:C2": "Apple",
	"A4:5E:60": "Apple",
	"DC:A9:04": "Apple",
	"F0:18:98": "Apple",
	"00:17:F2": "Apple",
	"00:16:CB": "Apple",
	"00:19:E3": "Apple",
	"B8:E8:56": "Apple",
	"AC:87:A3": "Apple",
	"00:1E:C2": "Apple",
	"00:25:00": "Apple",
	"18:AF:61": "Xiaomi",
	"28:6C:07": "Xiaomi",
	"34:CE:00": "Xiaomi",
	"64:09:80": "Xiaomi",
	"8C:BE:BE": "Xiaomi",
	"AC:F7:F3": "Xiaomi",
	"F0:B4:29": "Xiaomi",
	"FC:64:BA": "Xiaomi",
	"00:00:AA": "Xerox",
	"00:10:18": "Broadcom",
	"00:17:88": "Philips Hue",
	"EC:FA:BC": "Philips Hue",
	"00:1D:7E": "Cisco-Linksys",
	"20:AA:4B": "Cisco",
	"00:1A:A0": "Dell",
	"00:14:22": "Dell",
	"14:18:77": "Dell",
	"18:03:73": "Dell",
	"00:26:B9": "Dell",
	"D4:BE:D9": "Dell",
	"F8:DB:88": "Dell",
	"54:9F:35": "Dell",
	"00:1A:4B": "HP",
	"00:1C:C4": "HP",
	"00:21:5A": "HP",
	"00:24:81": "HP",
	"3C:D9:2B": "HP",
	"94:57:A5": "HP",
	"FC:15:B4": "HP",
	"10:60:4B": "Intel",
	"00:02:B3": "Intel",
	"00:03:47": "Intel",
	"8C:70:5A": "Intel",
	"A4:C3:F0": "Intel",
	"00:1B:21": "Intel",
	"84:3A:4B": "Intel",
	"C8:D9:D2": "Intel",
	"00:1F:3B": "Intel",
	"60:F6:77": "Intel",
	"00:15:17": "Intel",
	"B4:B6:76": "Intel",
	"00:0A:F7": "Motorola",
	"00:04:0F": "Motorola",
	"00:23:68": "Motorola",
	"9C:4E:36": "Samsung",
	"00:15:99": "Samsung",
	"00:17:C9": "Samsung",
	"00:1A:8A": "Samsung",
	"00:21:D1": "Samsung",
	"00:23:D6": "Samsung",
	"00:26:5D": "Samsung",
	"5C:F6:DC": "Samsung",
	"A0:07:98": "Samsung",
	"F4:09:D8": "Samsung",
	"CC:07:AB": "Samsung",
	"50:CC:F8": "Samsung",
	"00:1F:5B": "Apple",
	"00:21:E9": "Apple",
	"00:22:41": "Apple",
	"00:23:32": "Apple",
	"00:23:DF": "Apple",
	"00:24:36": "Apple",
	"00:25:4B": "Apple",
	"00:25:BC": "Apple",
	"00:26:08": "Apple",
	"00:26:4A": "Apple",
	"00:50:F2": "Microsoft",
	"28:18:78": "Microsoft",
	"3C:83:75": "Microsoft",
	"48:44:F7": "Microsoft",
	"50:1A:C5": "Microsoft",
	"7C:1E:52": "Microsoft",
	"00:15:5D": "Microsoft (Hyper-V)",
	"00:03:FF": "Microsoft (Xbox)",
	"00:22:48": "Microsoft (Xbox)",
	"00:25:AE": "Microsoft (Xbox)",
	"00:1D:D8": "Microsoft (Xbox)",
	"E8:D0:55": "Asus",
	"AC:22:0B": "Asus",
	"00:1A:92": "Asus",
	"00:26:18": "Asus",
}

func ouiLookup(mac string) string {
	if len(mac) < 8 {
		return "Unknown"
	}
	prefix := strings.ToUpper(mac[:8])
	if v, ok := ouiMap[prefix]; ok {
		return v
	}
	return "Unknown"
}

// ─── Generic file helpers ──────────────────────────────────────────────────

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// readFileLines returns lines of a file, limited to last `limit` lines (0 = all).
func readFileLines(path string, limit int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

// ─── /proc/stat per-core CPU parsing ──────────────────────────────────────

type cpuStat struct {
	idle  uint64
	total uint64
}

func parseProcStatAll() map[string]cpuStat {
	result := map[string]cpuStat{}
	data := readFile("/proc/stat")
	for _, line := range strings.Split(data, "\n") {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		name := fields[0]
		var vals []uint64
		for _, f := range fields[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			vals = append(vals, v)
		}
		var total uint64
		for _, v := range vals {
			total += v
		}
		idle := uint64(0)
		if len(vals) > 3 {
			idle = vals[3]
		}
		if len(vals) > 4 {
			idle += vals[4]
		}
		result[name] = cpuStat{idle: idle, total: total}
	}
	return result
}

func cpuPercentAll(sampleMs int) map[string]float64 {
	snap1 := parseProcStatAll()
	time.Sleep(time.Duration(sampleMs) * time.Millisecond)
	snap2 := parseProcStatAll()

	result := map[string]float64{}
	for name, s2 := range snap2 {
		s1, ok := snap1[name]
		if !ok {
			continue
		}
		dTotal := counterDelta(s2.total, s1.total)
		dIdle := counterDelta(s2.idle, s1.idle)
		if dTotal == 0 {
			result[name] = 0
		} else {
			busy := counterDelta(dTotal, dIdle)
			result[name] = float64(busy) / float64(dTotal) * 100
		}
	}
	return result
}

// ─── Memory ───────────────────────────────────────────────────────────────

func memInfo() map[string]uint64 {
	return parseMemInfoData(readFile("/proc/meminfo"))
}

func availableMemory(info map[string]uint64) uint64 {
	available := info["MemAvailable"]
	if available == 0 {
		available = info["MemFree"] + info["Buffers"] + info["Cached"]
	}
	if total := info["MemTotal"]; available > total {
		return total
	}
	return available
}

func parseMemInfoData(data string) map[string]uint64 {
	info := map[string]uint64{}
	for _, line := range strings.Split(data, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, _ := strconv.ParseUint(parts[1], 10, 64)
		info[key] = val // kB
	}
	return info
}

// ─── Network rate helpers ──────────────────────────────────────────────────

type ifaceSnapshot struct {
	rxBytes uint64
	txBytes uint64
	rxPkts  uint64
	txPkts  uint64
}

func parseNetDev() map[string]ifaceSnapshot {
	return parseNetDevData(readFile("/proc/net/dev"))
}

func parseNetDevData(data string) map[string]ifaceSnapshot {
	result := map[string]ifaceSnapshot{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.Fields(strings.Replace(line, ":", " ", 1))
		if len(parts) < 11 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(parts[1], 10, 64)
		rxPkts, _ := strconv.ParseUint(parts[2], 10, 64)
		txBytes, _ := strconv.ParseUint(parts[9], 10, 64)
		txPkts, _ := strconv.ParseUint(parts[10], 10, 64)
		result[parts[0]] = ifaceSnapshot{rxBytes, txBytes, rxPkts, txPkts}
	}
	return result
}

func counterDelta(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func interfaceType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case lower == "lo":
		return "loopback"
	case strings.Contains(lower, "wan") || strings.HasPrefix(lower, "pppoe"):
		return "wan"
	case strings.Contains(lower, "wlan") || strings.Contains(lower, "phy") || strings.Contains(lower, "ap"):
		return "wlan"
	case strings.Contains(lower, "lan") || strings.HasPrefix(lower, "br-"):
		return "lan"
	default:
		return "other"
	}
}

func interfaceMetadata(name string) map[string]interface{} {
	metadata := map[string]interface{}{"type": interfaceType(name)}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return metadata
	}
	metadata["up"] = iface.Flags&net.FlagUp != 0
	if mac := iface.HardwareAddr.String(); mac != "" {
		metadata["mac"] = mac
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return metadata
	}
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil && metadata["ipv4"] == nil {
			metadata["ipv4"] = ip4.String()
		} else if ip.To16() != nil && ip4 == nil && metadata["ipv6"] == nil {
			metadata["ipv6"] = ip.String()
		}
	}
	return metadata
}

// netRates polls /proc/net/dev twice sampleMs apart and returns per-iface rates.
func netRates(sampleMs int) []map[string]interface{} {
	snap1 := parseNetDev()
	time.Sleep(time.Duration(sampleMs) * time.Millisecond)
	snap2 := parseNetDev()

	elapsed := float64(sampleMs) / 1000.0
	var ifaces []map[string]interface{}
	for name, s2 := range snap2 {
		s1, ok := snap1[name]
		if !ok {
			continue
		}
		rxRate := float64(counterDelta(s2.rxBytes, s1.rxBytes)) / elapsed
		txRate := float64(counterDelta(s2.txBytes, s1.txBytes)) / elapsed
		item := map[string]interface{}{
			"iface":               name,
			"rx_bytes":            s2.rxBytes,
			"tx_bytes":            s2.txBytes,
			"rx_pkts":             s2.rxPkts,
			"tx_pkts":             s2.txPkts,
			"rx_bytes_per_second": rxRate,
			"tx_bytes_per_second": txRate,
		}
		for key, value := range interfaceMetadata(name) {
			item[key] = value
		}
		ifaces = append(ifaces, item)
	}
	if ifaces == nil {
		ifaces = []map[string]interface{}{}
	}
	return ifaces
}

// ─── Subprocess helper ────────────────────────────────────────────────────

func runCmd(timeoutSec int, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ─── Thermal temperature & Disk Power Management ─────────────────────────

var (
	cachedDiskTemp float64
	lastDiskCheck  time.Time
	diskTempMu     sync.Mutex
)

func getDiskTempAndState() (float64, string) {
	diskTempMu.Lock()
	defer diskTempMu.Unlock()

	state := "active"
	if out, err := runCmd(2, "hdparm", "-C", "/dev/sda"); err == nil {
		if strings.Contains(out, "standby") {
			state = "standby"
			return cachedDiskTemp, state
		}
	}

	if time.Since(lastDiskCheck) > 15*time.Second || cachedDiskTemp == 0 {
		lastDiskCheck = time.Now()
		out, err := runCmd(3, "smartctl", "-a", "-d", "sntjmicron", "/dev/sda")
		if err != nil || !strings.Contains(out, "Temperature") {
			out, _ = runCmd(3, "smartctl", "-a", "/dev/sda")
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Temperature:") {
				f := strings.Fields(line)
				if len(f) >= 2 {
					if t, pErr := strconv.ParseFloat(f[1], 64); pErr == nil && t > 0 {
						cachedDiskTemp = t
						break
					}
				}
			}
		}
	}
	return cachedDiskTemp, state
}

func readTemperatures() []map[string]interface{} {
	var temps []map[string]interface{}

	// 1. Read SoC / MT7615 Wi-Fi thermal sensors
	hwmonFiles, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp*_input")
	var socTemps []float64
	for _, f := range hwmonFiles {
		raw := strings.TrimSpace(readFile(f))
		if millic, err := strconv.ParseInt(raw, 10, 64); err == nil && millic > 0 {
			socTemps = append(socTemps, float64(millic)/1000.0)
		}
	}
	if len(socTemps) > 0 {
		var sum float64
		for _, t := range socTemps {
			sum += t
		}
		avg := sum / float64(len(socTemps))
		temps = append(temps, map[string]interface{}{
			"zone":         "soc",
			"type":         "CPU / Bo Mạch (MT7621/MT7615)",
			"temp_celsius": avg,
			"temp_milli":   int64(avg * 1000),
			"status":       "Bình Thường",
		})
	}

	// 2. Read USB 3.0 NVMe SSD Temperature (JMicron JMS583 / WD SN530)
	diskTemp, diskState := getDiskTempAndState()
	if diskTemp > 0 || diskState != "" {
		st := "Bình Thường"
		if diskTemp >= 70 {
			st = "Quá Nóng"
		} else if diskTemp >= 60 {
			st = "Cảnh Báo Nóng"
		} else if diskState == "standby" {
			st = "Đang Nghỉ (Sleep)"
		}
		temps = append(temps, map[string]interface{}{
			"zone":         "usb_disk",
			"type":         "Ổ Cứng NVMe USB 3.0 (WDC SN530)",
			"temp_celsius": diskTemp,
			"temp_milli":   int64(diskTemp * 1000),
			"disk_state":   diskState,
			"status":       st,
		})
	}

	// 3. Fallback to standard thermal zones if present
	if len(temps) == 0 {
		zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
		for _, z := range zones {
			raw := strings.TrimSpace(readFile(z))
			if millic, err := strconv.ParseInt(raw, 10, 64); err == nil {
				temps = append(temps, map[string]interface{}{
					"zone":         filepath.Base(filepath.Dir(z)),
					"type":         "Thermal Zone",
					"temp_celsius": float64(millic) / 1000.0,
					"temp_milli":   millic,
				})
			}
		}
	}

	return temps
}

// ─── Uptime from /proc/uptime ─────────────────────────────────────────────

func procUptime() (float64, float64) {
	raw := readFile("/proc/uptime")
	if raw == "" {
		return 0, 0
	}
	parts := strings.Fields(raw)
	up, _ := strconv.ParseFloat(parts[0], 64)
	idle := 0.0
	if len(parts) > 1 {
		idle, _ = strconv.ParseFloat(parts[1], 64)
	}
	return up, idle
}

// ─── Load average ─────────────────────────────────────────────────────────

type loadAvg struct {
	Load1   float64 `json:"load1"`
	Load5   float64 `json:"load5"`
	Load15  float64 `json:"load15"`
	RunQ    string  `json:"running_threads"`
	LastPID string  `json:"last_pid"`
}

func parseLoadAvg() loadAvg {
	raw := readFile("/proc/loadavg")
	parts := strings.Fields(raw)
	var la loadAvg
	if len(parts) >= 3 {
		la.Load1, _ = strconv.ParseFloat(parts[0], 64)
		la.Load5, _ = strconv.ParseFloat(parts[1], 64)
		la.Load15, _ = strconv.ParseFloat(parts[2], 64)
	}
	if len(parts) >= 4 {
		la.RunQ = parts[3]
	}
	if len(parts) >= 5 {
		la.LastPID = parts[4]
	}
	return la
}

// ─── OpenWrt release ──────────────────────────────────────────────────────

func parseOpenwrtRelease() map[string]string {
	info := map[string]string{}
	for _, line := range strings.Split(readFile("/etc/openwrt_release"), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		info[k] = v
	}
	return info
}

// ─── Service scan via /proc ───────────────────────────────────────────────

func runningProcessNames() map[string]bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only numeric directories (PIDs)
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		statusPath := "/proc/" + e.Name() + "/status"
		for _, line := range strings.Split(readFile(statusPath), "\n") {
			if strings.HasPrefix(line, "Name:") {
				name := strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
				names[name] = true
				break
			}
		}
	}
	return names
}

// ─── DHCP lease parser ────────────────────────────────────────────────────

type dhcpLease struct {
	Expiry      int64  `json:"expiry"`
	MAC         string `json:"mac"`
	IP          string `json:"ip"`
	Hostname    string `json:"hostname"`
	Vendor      string `json:"vendor"`
	OUI         string `json:"oui_vendor"`
	LeaseActive bool   `json:"lease_active"`
}

func parseDHCPLeases(path string) []dhcpLease {
	leases := parseDHCPLeaseData(readFile(path), time.Now().Unix())
	seenIP := make(map[string]bool)
	seenMAC := make(map[string]bool)
	for _, l := range leases {
		seenIP[l.IP] = true
		seenMAC[strings.ToLower(l.MAC)] = true
	}

	// Read /proc/net/arp to discover all LAN devices (e.g. static IPs 1-99, gateway, etc.)
	if arpData := readFile("/proc/net/arp"); arpData != "" {
		for _, line := range strings.Split(arpData, "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 6 && parts[0] != "IP" {
				ip := parts[0]
				mac := strings.ToLower(parts[3])
				flags := parts[2]
				// 0x2 is valid complete ARP resolution, mac != 00:00:00:00:00:00
				if flags != "0x0" && mac != "00:00:00:00:00:00" && !seenIP[ip] && net.ParseIP(ip) != nil {
					seenIP[ip] = true
					seenMAC[mac] = true
					hostname := "Static / LAN"
					if strings.HasSuffix(ip, ".1") {
						hostname = "Gateway / Modem"
					}
					leases = append(leases, dhcpLease{
						Expiry:      0,
						MAC:         mac,
						IP:          ip,
						Hostname:    hostname,
						Vendor:      "ARP",
						OUI:         ouiLookup(mac),
						LeaseActive: true,
					})
				}
			}
		}
	}

	// Sort leases numerically by IPv4 address
	sort.Slice(leases, func(i, j int) bool {
		ipA := net.ParseIP(leases[i].IP)
		ipB := net.ParseIP(leases[j].IP)
		if ipA != nil && ipB != nil {
			v4A := ipA.To4()
			v4B := ipB.To4()
			if v4A != nil && v4B != nil {
				return bytes.Compare(v4A, v4B) < 0
			}
		}
		return leases[i].IP < leases[j].IP
	})

	return leases
}

func parseDHCPLeaseData(data string, now int64) []dhcpLease {
	var leases []dhcpLease
	for _, line := range strings.Split(data, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		exp, err := strconv.ParseInt(parts[0], 10, 64)
		_, macErr := net.ParseMAC(parts[1])
		if err != nil || macErr != nil || exp < 0 || net.ParseIP(parts[2]) == nil {
			continue
		}
		mac := parts[1]
		ip := parts[2]
		hostname := parts[3]
		vendor := ""
		if len(parts) >= 5 {
			vendor = parts[4]
		}
		leases = append(leases, dhcpLease{
			Expiry:      exp,
			MAC:         mac,
			IP:          ip,
			Hostname:    hostname,
			Vendor:      vendor,
			OUI:         ouiLookup(mac),
			LeaseActive: exp == 0 || exp > now,
		})
	}
	return leases
}

// ─── /proc/net/wireless ───────────────────────────────────────────────────

type wirelessIface struct {
	Name   string  `json:"iface"`
	Status int64   `json:"status"`
	Link   float64 `json:"link_quality"`
	Level  float64 `json:"signal_level_dbm"`
	Noise  float64 `json:"noise_dbm"`
}

func parseProcNetWireless() []wirelessIface {
	return parseWirelessData(readFile("/proc/net/wireless"))
}

func parseWirelessData(data string) []wirelessIface {
	lines := strings.Split(data, "\n")
	// First two lines are header
	var result []wirelessIface
	if len(lines) <= 2 {
		return result
	}
	for _, line := range lines[2:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: iface: status  link  level  noise  nwid  crypt  frag  retry  misc  beacon
		parts := strings.Fields(strings.Replace(line, ":", " ", 1))
		if len(parts) < 5 {
			continue
		}
		name := strings.TrimSuffix(parts[0], ":")
		status, _ := strconv.ParseInt(parts[1], 10, 64)
		link, _ := strconv.ParseFloat(parts[2], 64)
		level, _ := strconv.ParseFloat(parts[3], 64)
		noise, _ := strconv.ParseFloat(parts[4], 64)
		// Adjust if values are > 0 (some drivers report unsigned)
		if level > 0 {
			level -= 256
		}
		if noise > 0 {
			noise -= 256
		}
		result = append(result, wirelessIface{
			Name:   name,
			Status: status,
			Link:   link,
			Level:  level,
			Noise:  noise,
		})
	}
	return result
}

// ─── JSON response helpers ────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, ok bool, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	var resp interface{}
	if ok {
		resp = map[string]interface{}{"ok": true, "data": payload}
	} else {
		resp = map[string]interface{}{"ok": false, "error": payload}
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func requireSystemFile(w http.ResponseWriter, path string) bool {
	if _, err := os.Stat(path); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, false, fmt.Sprintf("required system data unavailable: %s", path))
		return false
	}
	return true
}

func requireOpenWrt(w http.ResponseWriter) bool {
	if runtime.GOOS != "linux" {
		writeJSON(w, http.StatusServiceUnavailable, false, "router data is available only on OpenWrt")
		return false
	}
	return requireSystemFile(w, "/etc/openwrt_release")
}

// ─── CORS + method check middleware ──────────────────────────────────────

func cors(next http.HandlerFunc, methods ...string) http.HandlerFunc {
	allowed := "GET, OPTIONS"
	if len(methods) > 0 {
		allowed = strings.Join(methods, ", ") + ", OPTIONS"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Methods", allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if origin := r.Header.Get("Origin"); origin != "" {
				if !sameHostOrigin(origin, r.Host) {
					writeJSON(w, http.StatusForbidden, false, "origin not allowed")
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Handlers ─────────────────────────────────────────────────────────────

// GET /api/v1/health
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, true, map[string]interface{}{
		"status":  "ok",
		"version": Version,
		"arch":    runtime.GOARCH,
		"os":      runtime.GOOS,
		"uptime_seconds": func() float64 {
			up, _ := procUptime()
			if up == 0 {
				return time.Since(startTime).Seconds()
			}
			return up
		}(),
	})
}

// GET /api/v1/system
func handleSystem(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	for _, path := range []string{"/proc/stat", "/proc/uptime", "/proc/loadavg", "/proc/meminfo"} {
		if !requireSystemFile(w, path) {
			return
		}
	}
	cpuAll := cpuPercentAll(300)
	up, idleTime := procUptime()
	la := parseLoadAvg()
	hostname, _ := os.Hostname()
	kernel := readFile("/proc/sys/kernel/osrelease")
	if kernel == "" {
		kernel = readFile("/proc/version")
	}
	openwrt := parseOpenwrtRelease()
	temps := readTemperatures()

	// Build per-core slice (cpu0, cpu1, ...)
	type coreUsage struct {
		Core    string  `json:"core"`
		Percent float64 `json:"percent"`
	}
	var cores []coreUsage
	// aggregate
	agg := cpuAll["cpu"]
	for i := 0; ; i++ {
		key := fmt.Sprintf("cpu%d", i)
		pct, ok := cpuAll[key]
		if !ok {
			break
		}
		cores = append(cores, coreUsage{Core: key, Percent: pct})
	}

	writeJSON(w, 200, true, map[string]interface{}{
		"hostname":          hostname,
		"kernel":            kernel,
		"arch":              runtime.GOARCH,
		"firmware_version":  openwrt["DISTRIB_RELEASE"],
		"openwrt":           openwrt,
		"cpu_aggregate_pct": agg,
		"cpu_cores":         cores,
		"temperatures":      temps,
		"uptime_seconds":    up,
		"idle_seconds":      idleTime,
		"load_avg":          la,
		"internet":          getInternetStatusSnapshot(),
	})
}

// GET /api/v1/memory
func handleMemory(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) || !requireSystemFile(w, "/proc/meminfo") {
		return
	}
	m := memInfo()
	total := m["MemTotal"]
	free := m["MemFree"]
	avail := availableMemory(m)
	buffers := m["Buffers"]
	cached := m["Cached"]
	swapTotal := m["SwapTotal"]
	swapFree := m["SwapFree"]

	used := counterDelta(total, avail)
	usedPct := 0.0
	if total > 0 {
		usedPct = float64(used) / float64(total) * 100
	}
	swapUsed := counterDelta(swapTotal, swapFree)
	swapPct := 0.0
	if swapTotal > 0 {
		swapPct = float64(swapUsed) / float64(swapTotal) * 100
	}

	writeJSON(w, 200, true, map[string]interface{}{
		"total_kb":      total,
		"used_kb":       used,
		"free_kb":       free,
		"available_kb":  avail,
		"buffers_kb":    buffers,
		"cached_kb":     cached,
		"used_percent":  usedPct,
		"swap_total_kb": swapTotal,
		"swap_free_kb":  swapFree,
		"swap_used_kb":  swapUsed,
		"swap_used_pct": swapPct,
	})
}

// GET /api/v1/network
func handleNetwork(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) || !requireSystemFile(w, "/proc/net/dev") {
		return
	}
	ifaces := netRates(1000)
	writeJSON(w, 200, true, map[string]interface{}{
		"interfaces": ifaces,
		"sample_ms":  1000,
	})
}

// GET /api/v1/wireless
func handleWireless(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	procWifi := parseProcNetWireless()

	uciOut, err := runCmd(5, "uci", "show", "wireless")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, false, "could not read wireless configuration")
		return
	}
	uciLines := strings.Split(uciOut, "\n")
	uciConf := map[string]string{}
	for _, line := range uciLines {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), "'")
			uciConf[k] = v
		}
	}

	// Station count via iw
	stationOut, _ := runCmd(5, "iw", "dev", "wlan0", "station", "dump")
	stationCount := 0
	for _, line := range strings.Split(stationOut, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Station ") {
			stationCount++
		}
	}

	writeJSON(w, 200, true, map[string]interface{}{
		"interfaces":    procWifi,
		"uci_config":    uciConf,
		"station_count": stationCount,
	})
}

// GET /api/v1/clients
func handleClients(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	leases := parseDHCPLeases("/tmp/dhcp.leases")
	if leases == nil {
		leases = []dhcpLease{}
	}
	writeJSON(w, 200, true, map[string]interface{}{
		"count":  len(leases),
		"leases": leases,
	})
}

// GET /api/v1/storage
func handleStorage(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) || !requireSystemFile(w, "/proc/mounts") {
		return
	}
	// Parse /proc/mounts for USB
	type mountEntry struct {
		Device     string `json:"device"`
		MountPoint string `json:"mount_point"`
		FSType     string `json:"fstype"`
		Options    string `json:"options"`
	}
	var mounts []mountEntry
	for _, line := range readFileLines("/proc/mounts", 0) {
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		dev := unescapeMountField(parts[0])
		mp := unescapeMountField(parts[1])
		// Include USB-like mounts (sd*, /dev/sd*, /mnt/*)
		if strings.HasPrefix(dev, "/dev/sd") ||
			strings.HasPrefix(dev, "/dev/mmcblk") ||
			strings.HasPrefix(mp, "/mnt/") ||
			strings.HasPrefix(mp, "/media/") {
			mounts = append(mounts, mountEntry{
				Device:     dev,
				MountPoint: mp,
				FSType:     parts[2],
				Options:    parts[3],
			})
		}
	}
	if mounts == nil {
		mounts = []mountEntry{}
	}

	// df -h output
	dfOut, err := runCmd(5, "df", "-h")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, false, "could not read filesystem usage")
		return
	}
	dfLines := strings.Split(dfOut, "\n")

	writeJSON(w, 200, true, map[string]interface{}{
		"usb_mounts": mounts,
		"df_output":  dfLines,
	})
}

type storageFileEntry struct {
	Name      string    `json:"name"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size"`
	Modified  time.Time `json:"modified"`
}

const maxStorageDirectoryEntries = 500

func cleanStorageRelativePath(raw string) (string, error) {
	if raw == "" || raw == "." {
		return ".", nil
	}
	if strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("invalid relative path")
	}
	for _, part := range strings.Split(raw, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid relative path")
		}
	}
	clean := path.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid relative path")
	}
	return clean, nil
}

func isUSBStorageMount(device, mountPoint string) bool {
	return (strings.HasPrefix(device, "/dev/sd") || strings.HasPrefix(device, "/dev/mmcblk")) &&
		(strings.HasPrefix(mountPoint, "/")) &&
		(strings.HasPrefix(mountPoint, "/mnt/") || strings.HasPrefix(mountPoint, "/media/")) &&
		filepath.Clean(mountPoint) != "/"
}

func unescapeMountField(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\134", "\\").Replace(value)
}

func usbMountExists(mountPoint string, mounts []string) bool {
	for _, line := range mounts {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		device := unescapeMountField(fields[0])
		root := unescapeMountField(fields[1])
		if root == mountPoint && isUSBStorageMount(device, root) {
			return true
		}
	}
	return false
}

func listStorageDirectory(rootPath, relativePath string) ([]storageFileEntry, bool, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()

	directory, err := root.Open(relativePath)
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()

	entries, err := directory.ReadDir(maxStorageDirectoryEntries + 1)
	if err != nil {
		return nil, false, err
	}
	truncated := len(entries) > maxStorageDirectoryEntries
	if truncated {
		entries = entries[:maxStorageDirectoryEntries]
	}

	files := make([]storageFileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, false, fmt.Errorf("inspect storage entry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		files = append(files, storageFileEntry{
			Name:      entry.Name(),
			Directory: info.IsDir(),
			Size:      info.Size(),
			Modified:  info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Directory != files[j].Directory {
			return files[i].Directory
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, truncated, nil
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}

	mountPoint := r.URL.Query().Get("mount")
	if !usbMountExists(mountPoint, readFileLines("/proc/mounts", 0)) {
		writeJSON(w, http.StatusBadRequest, false, "mount is not an attached USB storage volume")
		return
	}

	relativePath, err := cleanStorageRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	root, err := os.OpenRoot(mountPoint)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, false, "USB storage volume is unavailable")
		return
	}
	defer root.Close()

	if r.URL.Path == "/api/v1/files/download" || r.URL.Path == "/api/v1/files/preview" {
		file, err := root.Open(relativePath)
		if err != nil {
			writeJSON(w, http.StatusNotFound, false, "file not found")
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			writeJSON(w, http.StatusBadRequest, false, "requested path is not a regular file")
			return
		}
		if r.URL.Path == "/api/v1/files/preview" {
			contentType, ok := photoContentType(filepath.Ext(info.Name()))
			if !ok {
				writeJSON(w, http.StatusUnsupportedMediaType, false, "only supported raster images can be previewed")
				return
			}
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'")
			w.Header().Set("Cache-Control", "private, max-age=60")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": info.Name()}))
			w.Header().Set("Content-Type", contentType)
			http.ServeContent(w, r, info.Name(), info.ModTime(), file)
			return
		}
		contentType := mime.TypeByExtension(filepath.Ext(info.Name()))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
		return
	}
	if r.URL.Path != "/api/v1/files" {
		http.NotFound(w, r)
		return
	}

	files, truncated, err := listStorageDirectory(mountPoint, relativePath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, "directory is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"mount":     mountPoint,
		"path":      relativePath,
		"entries":   files,
		"truncated": truncated,
	})
}

// GET /api/v1/modules reports observed service and storage readiness.
func handleModules(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}

	type moduleStatus struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	processes := runningProcessNames()
	serviceStatus := func(id, process, initScript, detail string) moduleStatus {
		if processes[process] {
			return moduleStatus{ID: id, Status: "active", Detail: detail}
		}
		if _, err := os.Stat(initScript); err == nil {
			return moduleStatus{ID: id, Status: "installed", Detail: "Installed but no running process was detected."}
		}
		return moduleStatus{ID: id, Status: "not-installed", Detail: "Service package is not present in this firmware."}
	}

	adblock := moduleStatus{ID: "adblock", Status: "not-installed", Detail: "Adblock Fast package is not present in this firmware."}
	if _, err := os.Stat("/etc/init.d/adblock-fast"); err == nil {
		adblock.Status = "installed"
		adblock.Detail = "Adblock Fast is installed; active DNS filtering has not been independently probed."
		if enabled, _ := filepath.Glob("/etc/rc.d/S??adblock-fast"); len(enabled) > 0 {
			adblock.Status = "enabled"
			adblock.Detail = "Adblock Fast is enabled at boot; active DNS filtering has not been independently probed."
		}
	}

	nas := serviceStatus("nas", "ksmbd.mountd", "/etc/init.d/ksmbd", "ksmbd process is running.")
	if nas.Status != "active" && (processes["ksmbd"] || processes["ksmbd.mountd"]) {
		nas.Status = "active"
		nas.Detail = "ksmbd process is running."
	}

	mounts := make([]string, 0)
	for _, line := range readFileLines("/proc/mounts", 0) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && isUSBStorageMount(unescapeMountField(fields[0]), unescapeMountField(fields[1])) {
			mounts = append(mounts, unescapeMountField(fields[1]))
		}
	}
	photoStatus := "not-installed"
	photoDetail := "Photo Library requires a mounted removable USB volume."
	if len(mounts) > 0 {
		photoStatus = "ready"
		photoDetail = fmt.Sprintf("Read-only photo browsing is available on %d mounted USB volume(s).", len(mounts))
	}

	telegram := moduleStatus{ID: "telegram", Status: "not-configured", Detail: "Configure a bot token and allowlisted chat ID to enable URL intake."}
	if telegramDownloader != nil {
		status := telegramDownloader.statusSnapshot()
		switch {
		case status.Connected:
			telegram.Status = "active"
			telegram.Detail = "Telegram Bot API polling is connected for the configured chat."
		case status.Configured:
			telegram.Status = "configured"
			telegram.Detail = "Telegram settings are saved; polling or aria2 may be waiting."
		}
	}

	modules := []moduleStatus{
		serviceStatus("ssh", "dropbear", "/etc/init.d/dropbear", "Dropbear SSH process is running."),
		serviceStatus("dns", "dnsmasq", "/etc/init.d/dnsmasq", "dnsmasq process is running."),
		adblock,
		telegram,
		serviceStatus("downloader", "aria2c", "/etc/init.d/r3p-aria2", "aria2 download service is running."),
		nas,
		serviceStatus("ksmbd", "ksmbd.mountd", "/etc/init.d/ksmbd", "ksmbd package is installed but its shares need separate configuration."),
		{ID: "photos", Status: photoStatus, Detail: photoDetail},
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{"modules": modules})
}

// GET /api/v1/services
func handleServices(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	running := runningProcessNames()
	type svcStatus struct {
		Name    string `json:"name"`
		Running bool   `json:"running"`
	}
	var services []svcStatus
	for name := range allowedServices {
		services = append(services, svcStatus{
			Name:    name,
			Running: running[name],
		})
	}
	writeJSON(w, 200, true, map[string]interface{}{
		"services": services,
	})
}

// PATCH /api/v1/services/{name}  — body: {"action":"start"} or {"action":"stop"}
func handleServiceToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}

	// Extract service name from path: /api/v1/services/{name}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/services/"), "/")
	svcName := pathParts[0]
	if svcName == "" {
		writeJSON(w, http.StatusBadRequest, false, "missing service name")
		return
	}
	if _, ok := allowedServices[svcName]; !ok {
		writeJSON(w, http.StatusForbidden, false, "service not in whitelist")
		return
	}

	var body struct {
		Action string `json:"action"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1024))
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON body")
		return
	}
	if body.Action != "start" && body.Action != "stop" &&
		!(svcName == "adblock-fast" && (body.Action == "enable" || body.Action == "disable")) {
		writeJSON(w, http.StatusBadRequest, false, "unsupported action for this service")
		return
	}

	initScript := "/etc/init.d/" + svcName
	out, err := runCmd(5, initScript, body.Action)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, fmt.Sprintf("exec error: %v — %s", err, out))
		return
	}
	writeJSON(w, 200, true, map[string]interface{}{
		"service": svcName,
		"action":  body.Action,
		"output":  out,
	})
}

// GET /api/v1/logs
func handleLogs(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	if r.Method == http.MethodDelete || (r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/clear")) {
		_, _ = runCmd(5, "sh", "-c", "/etc/init.d/log restart; dmesg -c >/dev/null 2>&1")
		writeJSON(w, http.StatusOK, true, map[string]string{
			"message": "Đã xóa sạch bộ đệm nhật ký hệ thống thành công!",
		})
		return
	}
	// Try /var/log/messages first
	lines := readFileLines("/var/log/messages", 100)
	source := "/var/log/messages"

	if len(lines) == 0 {
		// Fallback to logread
		out, err := runCmd(5, "logread")
		if err == nil && out != "" {
			all := strings.Split(out, "\n")
			if len(all) > 100 {
				all = all[len(all)-100:]
			}
			lines = all
			source = "logread"
		} else if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, false, "system log service is unavailable")
			return
		}
	}
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, 200, true, map[string]interface{}{
		"source": source,
		"count":  len(lines),
		"lines":  lines,
	})
}

// GET /api/v1/monitor/stream — SSE real-time dashboard
func handleMonitorStream(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	for _, path := range []string{"/proc/stat", "/proc/meminfo", "/proc/net/dev"} {
		if _, err := os.Stat(path); err != nil {
			http.Error(w, "system metrics are unavailable on this host", http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Gather metrics — quick samples
			cpuAll := cpuPercentAll(200)
			m := memInfo()
			snap1 := parseNetDev()
			time.Sleep(200 * time.Millisecond)
			snap2 := parseNetDev()

			var netRxRate, netTxRate float64
			for name, s2 := range snap2 {
				if name == "lo" {
					continue
				}
				s1, ok := snap1[name]
				if !ok {
					continue
				}
				netRxRate += float64(counterDelta(s2.rxBytes, s1.rxBytes)) / 0.2
				netTxRate += float64(counterDelta(s2.txBytes, s1.txBytes)) / 0.2
			}

			total := m["MemTotal"]
			avail := availableMemory(m)
			memPct := 0.0
			if total > 0 {
				memPct = float64(counterDelta(total, avail)) / float64(total) * 100
			}

			up, _ := procUptime()
			load := parseLoadAvg()
			hostname, _ := os.Hostname()
			openwrt := parseOpenwrtRelease()
			kernel := readFile("/proc/sys/kernel/osrelease")
			temp := interface{}(nil)
			temps := readTemperatures()
			if len(temps) > 0 {
				temp = temps[0]["temp_celsius"]
			}
			payload := map[string]interface{}{
				"ts":                      time.Now().Unix(),
				"cpu_percent":             cpuAll["cpu"],
				"mem_used_percent":        memPct,
				"net_rx_bytes_per_second": netRxRate,
				"net_tx_bytes_per_second": netTxRate,
				"mem_total_mb":            float64(total) / 1024,
				"mem_used_mb":             float64(counterDelta(total, avail)) / 1024,
				"uptime_seconds":          up,
				"load_avg":                []float64{load.Load1, load.Load5, load.Load15},
				"hostname":                hostname,
				"kernel":                  kernel,
				"arch":                    runtime.GOARCH,
				"firmware_version":        openwrt["DISTRIB_RELEASE"],
				"temp_celsius":            temp,
				"internet":                getInternetStatusSnapshot(),
			}
			data, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// ─── Static file server with SPA fallback ────────────────────────────────

// spaHandler serves static files from dir; falls back to index.html for unknown paths.
type spaHandler struct {
	dir string
}

func (s spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean the path and build absolute file path
	urlPath := filepath.Clean("/" + r.URL.Path)
	fullPath := filepath.Join(s.dir, urlPath)

	info, err := os.Stat(fullPath)
	if err == nil && !info.IsDir() {
		// File exists — serve it directly
		http.ServeFile(w, r, fullPath)
		return
	}

	// SPA fallback — serve index.html
	indexPath := filepath.Join(s.dir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}

	http.NotFound(w, r)
}

// ─── Router: dispatch /api/v1/services with or without name segment ────────

func servicesRouter(w http.ResponseWriter, r *http.Request) {
	// Strip leading /api/v1/services
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/services")
	suffix = strings.TrimPrefix(suffix, "/")
	if suffix == "" {
		// GET /api/v1/services
		cors(handleServices, "GET")(w, r)
		return
	}
	// /api/v1/services/{name}
	cors(handleServiceToggle, "PATCH")(w, r)
}

// ─── Main ─────────────────────────────────────────────────────────────────

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[r3p-api] ")

	if len(os.Args) > 1 {
		if len(os.Args) != 4 || os.Args[1] != "generate-cert" {
			log.Fatal("usage: r3p-api [generate-cert CERT_PATH KEY_PATH]")
		}
		if err := generateSelfSignedCertificate(os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("generate TLS certificate: %v", err)
		}
		return
	}

	apiToken, tokenErr := loadAPIToken()
	if tokenErr != nil {
		log.Printf("API authentication is unavailable: %v", tokenErr)
	}
	auth := newAuthenticator(apiToken)
	ensureUSBHotplugAndTuning()
	startInternetMonitor()
	initAdblockOnStartup()
	_ = ensureSystemBlueprintFile()
	dlManager.init()
	initR3pDownloadRuntime()
	initGDriveSync()
	if err := telegramDownloader.load(); err != nil {
		log.Printf("Telegram auto-download configuration was not loaded: %v", err)
	}

	port := "80"
	if p := os.Getenv("R3P_PORT"); p != "" {
		port = p
	}
	bind := "0.0.0.0"
	if b := os.Getenv("R3P_BIND"); b != "" {
		bind = b
	}
	staticDir := "/www"
	if d := os.Getenv("R3P_STATIC_DIR"); d != "" {
		staticDir = d
	}
	tlsCert := os.Getenv("R3P_TLS_CERT")
	tlsKey := os.Getenv("R3P_TLS_KEY")
	if (tlsCert == "") != (tlsKey == "") {
		log.Fatal("R3P_TLS_CERT and R3P_TLS_KEY must be configured together")
	}
	listenAddr := bind + ":" + port

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/v1/health", cors(handleHealth, "GET"))
	mux.HandleFunc("/api/v1/auth/session", cors(auth.handleSession, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/system", cors(handleSystem, "GET"))
	mux.HandleFunc("/api/v1/system/ping", cors(handleSystemPing, "GET"))
	mux.HandleFunc("/api/v1/system/nslookup", cors(handleSystemNslookup, "GET"))
	mux.HandleFunc("/api/v1/system/backup", cors(handleSystemBackup, "GET"))
	mux.HandleFunc("/api/v1/system/restore", cors(handleSystemRestore, "POST"))
	mux.HandleFunc("/api/v1/system/reboot", cors(handleSystemReboot, "POST"))
	mux.HandleFunc("/api/v1/system/settings", cors(handleSystemSettings, "PUT"))
	mux.HandleFunc("/api/v1/system/drop-caches", cors(handleSystemDropCaches, "POST"))
	mux.HandleFunc("/api/v1/system/firmware/status", cors(handleFirmwareStatus, "GET"))
	mux.HandleFunc("/api/v1/system/firmware/config", cors(handleFirmwareConfigPut, "PUT"))
	mux.HandleFunc("/api/v1/system/firmware/github-check", cors(handleFirmwareGitHubCheck, "GET"))
	mux.HandleFunc("/api/v1/system/firmware/github-update", cors(handleFirmwareGitHubUpdate, "POST"))
	mux.HandleFunc("/api/v1/system/firmware/upload", cors(handleFirmwareManualUpload, "POST"))
	mux.HandleFunc("/api/v1/system/firmware/apply", cors(handleFirmwareApply, "POST"))
	mux.HandleFunc("/api/v1/memory", cors(handleMemory, "GET"))
	mux.HandleFunc("/api/v1/network", cors(handleNetwork, "GET"))
	mux.HandleFunc("/api/v1/network/details", cors(handleNetworkDetails, "GET"))
	mux.HandleFunc("/api/v1/network/internet", cors(handleInternetStatus, "GET"))
	mux.HandleFunc("/api/v1/network/interfaces-config", cors(handleNetworkInterfacesConfig, "GET", "PUT"))
	mux.HandleFunc("/api/v1/network/interfaces-config/action", cors(handleNetworkInterfaceAction, "POST"))
	mux.HandleFunc("/api/v1/adblock", cors(handleAdblockGet, "GET"))
	mux.HandleFunc("/api/v1/adblock/config", cors(handleAdblockConfigPut, "PUT"))
	mux.HandleFunc("/api/v1/adblock/update", cors(handleAdblockUpdateNow, "POST"))
	mux.HandleFunc("/api/v1/adblock/lists", cors(handleAdblockLists, "POST", "PATCH", "DELETE"))
	mux.HandleFunc("/api/v1/adblock/domains", cors(handleAdblockDomains, "POST", "DELETE"))
	mux.HandleFunc("/api/v1/adblock/test", cors(handleAdblockTestDomain, "POST"))
	mux.HandleFunc("/api/v1/usb/status", cors(handleUSBStatus, "GET"))
	mux.HandleFunc("/api/v1/usb/disks", cors(handleUSBStatus, "GET"))
	mux.HandleFunc("/api/v1/usb/mount", cors(handleUSBMount, "POST"))
	mux.HandleFunc("/api/v1/usb/format", cors(handleUSBFormat, "POST"))
	mux.HandleFunc("/api/v1/usb/benchmark", cors(handleUSBBenchmark, "POST"))
	mux.HandleFunc("/api/v1/wireless", cors(handleWirelessFull, "GET", "PUT"))
	mux.HandleFunc("/api/v1/wireless/restart", cors(handleWirelessRestart, "POST"))
	mux.HandleFunc("/api/v1/wireless/scan", cors(handleWirelessScan, "GET"))
	mux.HandleFunc("/api/v1/wireless/client", cors(handleWirelessClient, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/wireless/saved", cors(handleWirelessSaved, "GET"))
	mux.HandleFunc("/api/v1/wireless/saved/forget", cors(handleWirelessSavedForget, "POST"))
	mux.HandleFunc("/api/v1/wireless/saved/connect", cors(handleWirelessSavedConnect, "POST"))
	mux.HandleFunc("/api/v1/ai/config", cors(handleAIConfig, "GET", "PUT"))
	mux.HandleFunc("/api/v1/ai/chat", cors(handleAIChat, "POST", "DELETE"))
	mux.HandleFunc("/api/v1/ai/sessions", cors(handleAISessions, "GET", "POST", "PUT", "DELETE"))
	mux.HandleFunc("/api/v1/ai/memory", cors(handleAIMemory, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/ai/blueprint", cors(handleAIBlueprint, "GET", "PUT"))
	mux.HandleFunc("/api/v1/ai/quick-commands", cors(handleAIQuickCommands, "GET", "PUT", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/adblock/streams", cors(handleAdblockStreams, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/explorer/list", cors(handleExplorerList, "GET"))
	mux.HandleFunc("/api/v1/explorer/read", cors(handleExplorerRead, "GET"))
	mux.HandleFunc("/api/v1/explorer/raw", cors(handleExplorerRaw, "GET"))
	mux.HandleFunc("/api/v1/explorer/write", cors(handleExplorerWrite, "POST"))
	mux.HandleFunc("/api/v1/explorer/create", cors(handleExplorerCreate, "POST"))
	mux.HandleFunc("/api/v1/explorer/delete", cors(handleExplorerDelete, "DELETE"))
	// All downloader routes run locally on R3P via authenticated /api/v1/download/* handlers.
	mux.HandleFunc("/api/v1/firewall", cors(handleFirewallGet, "GET"))
	mux.HandleFunc("/api/v1/firewall/redirect", cors(handleFirewallRedirect, "POST", "PATCH", "DELETE"))
	mux.HandleFunc("/api/v1/firewall/rule", cors(handleFirewallRule, "POST", "PATCH", "DELETE"))
	mux.HandleFunc("/api/v1/clients", cors(handleClients, "GET"))
	mux.HandleFunc("/api/v1/storage", cors(handleStorage, "GET"))
	mux.HandleFunc("/api/v1/nas", cors(handleNASOverview, "GET"))
	mux.HandleFunc("/api/v1/nas/mkdir", cors(handleNASMkdir, "POST"))
	mux.HandleFunc("/api/v1/nas/upload", cors(handleNASUpload, "POST"))
	mux.HandleFunc("/api/v1/nas/file", cors(handleNASDeleteFile, "DELETE"))
	mux.HandleFunc("/api/v1/nas/download", cors(handleNASDownloadFile, "GET"))
	mux.HandleFunc("/api/v1/nas/shares", cors(handleNASShares, "POST", "PUT", "DELETE"))
	mux.HandleFunc("/api/v1/nas/services", cors(handleNASServices, "GET", "PUT"))
	mux.HandleFunc("/api/v1/nas/services/samba", cors(handleNASServices, "PUT"))
	mux.HandleFunc("/api/v1/nas/services/ftp", cors(handleNASServices, "PUT"))
	mux.HandleFunc("/api/v1/nas/users", cors(handleNASUsers, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/nas/gdrive-sync", cors(handleGDriveSync, "GET", "POST", "PUT"))
	mux.HandleFunc("/api/v1/nas/gdrive-sync/run", cors(handleGDriveSyncRun, "POST"))
	mux.HandleFunc("/api/v1/nas/gdrive-sync/refresh", cors(handleGDriveSyncRefresh, "POST"))
	mux.HandleFunc("/api/v1/nas/gdrive-sync/clear-log", cors(handleGDriveSyncClearLog, "POST"))
	mux.HandleFunc("/api/v1/usb/standby", cors(handleUSBStandby, "GET", "POST"))
	mux.HandleFunc("/api/v1/download/files", cors(handleDownloadFiles, "GET"))
	mux.HandleFunc("/api/v1/download/file", cors(handleDownloadFileServe, "GET"))
	mux.HandleFunc("/api/v1/download/settings", cors(handleDownloadSettings, "GET", "POST", "PUT"))
	mux.HandleFunc("/api/v1/download/jobs", cors(handleDownloadJobs, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/download/jobs/", cors(handleDownloadJobItem, "POST", "DELETE"))
	mux.HandleFunc("/api/v1/download/debug", cors(handleDownloadDebug, "GET", "POST"))
	mux.HandleFunc("/api/v1/download/config", cors(handleDownloadConfig, "GET", "PUT"))
	mux.HandleFunc("/api/v1/download/telegram/log", cors(handleDownloadTelegramLog, "GET", "DELETE"))
	mux.HandleFunc("/api/v1/download/telegram/test", cors(handleDownloadTelegramTest, "POST"))
	mux.HandleFunc("/api/v1/download/telegram/check", cors(handleDownloadTelegramCheck, "POST"))
	mux.HandleFunc("/api/v1/download/telegram/analyze", cors(handleDownloadTelegramAnalyze, "POST"))
	mux.HandleFunc("/api/v1/download/schedules", cors(handleDownloadSchedules, "GET", "POST"))
	mux.HandleFunc("/api/v1/download/schedules/", cors(handleDownloadScheduleItem, "POST", "DELETE"))
	mux.HandleFunc("/api/v1/network/speedtest", cors(handleInterfaceSpeedtest, "POST"))
	mux.HandleFunc("/api/v1/access", cors(handleAccessGet, "GET"))
	mux.HandleFunc("/api/v1/access/ssh", cors(handleAccessSSH, "PUT"))
	mux.HandleFunc("/api/v1/access/dhcp", cors(handleAccessDHCP, "PUT"))
	mux.HandleFunc("/api/v1/access/static-lease", cors(handleAccessStaticLease, "POST", "DELETE"))
	mux.HandleFunc("/api/v1/access/password", cors(handleAccessPassword, "POST"))
	mux.HandleFunc("/api/v1/access/ddns", cors(handleAccessDDNS, "GET", "PUT"))
	mux.HandleFunc("/api/v1/access/ddns/update", cors(handleAccessDDNSUpdate, "POST"))
	mux.HandleFunc("/api/v1/automation/cron", cors(handleAutomationCron, "GET", "POST", "PATCH", "DELETE"))
	mux.HandleFunc("/api/v1/automation/run", cors(handleAutomationRun, "POST"))
	mux.HandleFunc("/api/v1/files", cors(handleFiles, "GET"))
	mux.HandleFunc("/api/v1/files/download", cors(handleFiles, "GET"))
	mux.HandleFunc("/api/v1/files/preview", cors(handleFiles, "GET"))
	mux.HandleFunc("/api/v1/modules", cors(handleModules, "GET"))
	mux.HandleFunc("/api/v1/downloads", cors(handleDownloadsUnified, "GET", "POST", "PUT"))
	mux.HandleFunc("/api/v1/downloads/action", cors(handleDownloadsAction, "POST"))
	mux.HandleFunc("/api/v1/downloads/file", cors(handleDownloadsFile, "GET"))
	mux.HandleFunc("/api/v1/telegram", cors(handleTelegram, "GET", "PUT", "DELETE"))
	mux.HandleFunc("/api/v1/telegram/test", cors(handleTelegramTest, "POST"))
	mux.HandleFunc("/api/v1/services", servicesRouter)
	mux.HandleFunc("/api/v1/services/", servicesRouter)
	mux.HandleFunc("/api/v1/services/all", cors(handleServicesAll, "GET"))
	mux.HandleFunc("/api/v1/services/control", cors(handleServicesControl, "POST"))
	mux.HandleFunc("/api/v1/packages", cors(handlePackagesGet, "GET"))
	mux.HandleFunc("/api/v1/packages/repos", cors(handlePackageRepos, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/packages/search", cors(handlePackagesSearch, "GET"))
	mux.HandleFunc("/api/v1/packages/update-feeds", cors(handlePackagesUpdateFeeds, "POST"))
	mux.HandleFunc("/api/v1/packages/install", cors(handlePackagesInstall, "POST"))
	mux.HandleFunc("/api/v1/packages/remove", cors(handlePackagesRemove, "POST"))
	mux.HandleFunc("/api/v1/iot", cors(handleIoT, "GET", "POST", "PUT", "DELETE"))
	mux.HandleFunc("/api/v1/iot/ping", cors(handleIoTPing, "POST"))
	mux.HandleFunc("/api/v1/plugins", cors(handlePlugins, "GET", "POST"))
	mux.HandleFunc("/api/v1/config", cors(handleUCIConfig, "GET", "PUT"))
	mux.HandleFunc("/api/v1/logs", cors(handleLogs, "GET", "POST", "DELETE"))
	mux.HandleFunc("/api/v1/system/logs/clear", cors(handleSystemLogsClear, "POST", "DELETE"))
	mux.HandleFunc("/api/v1/monitor/stream", cors(handleMonitorStream, "GET"))

	// Static file server — catch-all; must be last
	mux.Handle("/", spaHandler{dir: staticDir})

	srv := &http.Server{
		Addr:           listenAddr,
		Handler:        corsMiddleware(auth.middleware(mux)),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   0, // SSE endpoint needs no write timeout
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	if tlsCert != "" {
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	initDDNSBackgroundRunner()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("R3P API %s (build: %s) starting on %s (arch=%s, os=%s) static=%s",
			Version, BuildDate, listenAddr, runtime.GOARCH, runtime.GOOS, staticDir)
		var err error
		if tlsCert != "" {
			err = srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	// Also listen on HTTP :80 when primary port is not 80 so http://192.168.2.1 works out-of-the-box
	var http80Srv *http.Server
	if port != "80" {
		http80Srv = &http.Server{
			Addr:           bind + ":80",
			Handler:        corsMiddleware(auth.middleware(mux)),
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   0,
			IdleTimeout:    120 * time.Second,
			MaxHeaderBytes: 1 << 20,
		}
		go func() {
			log.Printf("R3P HTTP Web UI & API mirror starting on %s:80", bind)
			if err := http80Srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP :80 listener notice: %v", err)
			}
		}()
	}

	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if http80Srv != nil {
		_ = http80Srv.Shutdown(ctx)
	}
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	wg.Wait()
	log.Println("stopped.")
}
