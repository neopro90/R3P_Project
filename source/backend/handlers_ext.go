package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var safeHostPattern = regexp.MustCompile(`^[a-zA-Z0-9.-]{1,128}$`)
var safeTokenKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
var nasUsersMu sync.Mutex

// ─── 1. Network Diagnostics & Extended Info ───────────────────────────────

// GET /api/v1/network/details
func handleNetworkDetails(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	ifaces := netRates(400)
	for _, item := range ifaces {
		if name, ok := item["iface"].(string); ok {
			if netIf, err := net.InterfaceByName(name); err == nil {
				item["mtu"] = netIf.MTU
			}
		}
	}

	// Parse routes from `ip route`
	routeOut, _ := runCmd(4, "ip", "route")
	type routeEntry struct {
		Destination string `json:"destination"`
		Gateway     string `json:"gateway"`
		Device      string `json:"device"`
		Source      string `json:"source"`
		Raw         string `json:"raw"`
	}
	var routes []routeEntry
	for _, line := range strings.Split(routeOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		re := routeEntry{Destination: fields[0], Gateway: "0.0.0.0", Raw: line}
		for i := 1; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				re.Gateway = fields[i+1]
			case "dev":
				re.Device = fields[i+1]
			case "src":
				re.Source = fields[i+1]
			}
		}
		routes = append(routes, re)
	}
	if routes == nil {
		routes = []routeEntry{}
	}

	// Parse DNS servers from /tmp/resolv.conf.d/resolv.conf.auto and /etc/resolv.conf
	dnsSet := map[string]bool{}
	var dnsServers []string
	for _, p := range []string{"/tmp/resolv.conf.d/resolv.conf.auto", "/etc/resolv.conf"} {
		for _, line := range readFileLines(p, 50) {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "nameserver" {
				if !dnsSet[fields[1]] {
					dnsSet[fields[1]] = true
					dnsServers = append(dnsServers, fields[1])
				}
			}
		}
	}
	if dnsServers == nil {
		dnsServers = []string{"127.0.0.1"}
	}

	// Parse ARP table from /proc/net/arp
	type arpEntry struct {
		IP     string `json:"ip"`
		MAC    string `json:"mac"`
		Device string `json:"device"`
		Flags  string `json:"flags"`
		Vendor string `json:"vendor"`
	}
	var arpList []arpEntry
	arpLines := readFileLines("/proc/net/arp", 200)
	if len(arpLines) > 1 {
		for _, line := range arpLines[1:] {
			f := strings.Fields(line)
			if len(f) >= 6 && f[3] != "00:00:00:00:00:00" {
				arpList = append(arpList, arpEntry{
					IP:     f[0],
					Flags:  f[2],
					MAC:    f[3],
					Device: f[5],
					Vendor: ouiLookup(f[3]),
				})
			}
		}
	}
	if arpList == nil {
		arpList = []arpEntry{}
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"interfaces":  ifaces,
		"routes":      routes,
		"dns_servers": dnsServers,
		"arp_table":   arpList,
	})
}

// GET /api/v1/system/ping?host=...&count=...
func handleSystemPing(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if !safeHostPattern.MatchString(host) || strings.HasPrefix(host, "-") {
		writeJSON(w, http.StatusBadRequest, false, "invalid host or IP address")
		return
	}
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil || count < 1 || count > 10 {
		count = 4
	}
	out, cmdErr := runCmd(15, "ping", "-c", strconv.Itoa(count), "-W", "2", host)
	if out == "" && cmdErr != nil {
		out = fmt.Sprintf("ping %s failed: %v", host, cmdErr)
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"host":    host,
		"count":   count,
		"success": cmdErr == nil,
		"output":  out,
	})
}

// GET /api/v1/system/nslookup?host=...
func handleSystemNslookup(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if !safeHostPattern.MatchString(host) || strings.HasPrefix(host, "-") {
		writeJSON(w, http.StatusBadRequest, false, "invalid domain or host")
		return
	}
	out, cmdErr := runCmd(8, "nslookup", host)
	if out == "" && cmdErr != nil {
		out = fmt.Sprintf("nslookup %s failed: %v", host, cmdErr)
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"host":    host,
		"success": cmdErr == nil,
		"output":  out,
	})
}

// ─── 2. Wireless Management (UCI + iwinfo / iw) ───────────────────────────

func parseUCIConfigMap(pkg string) map[string]string {
	out, _ := runCmd(5, "uci", "-X", "show", pkg)
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			res[k] = v
		}
	}
	return res
}

// GET / PUT /api/v1/wireless
func handleWirelessFull(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		uciMap := parseUCIConfigMap("wireless")
		type radioInfo struct {
			Device       string `json:"device"`
			IfaceSection string `json:"iface_section"`
			Band         string `json:"band"`
			Chip         string `json:"chip"`
			Enabled      bool   `json:"enabled"`
			SSID         string `json:"ssid"`
			Encryption   string `json:"encryption"`
			Key          string `json:"key"`
			Channel      string `json:"channel"`
			HTMode       string `json:"htmode"`
			TxPower      string `json:"txpower"`
			Country      string `json:"country"`
			Mode         string `json:"mode"`
		}

		buildRadio := func(dev, ifaceSec, defaultBand, chip string) radioInfo {
			bandCode := uciMap["wireless."+dev+".band"]
			bandLabel := defaultBand
			if bandCode == "2g" {
				bandLabel = "2.4GHz"
			} else if bandCode == "5g" {
				bandLabel = "5GHz"
			}
			devDisabled := uciMap["wireless."+dev+".disabled"] == "1"
			ifDisabled := uciMap["wireless."+ifaceSec+".disabled"] == "1"
			ch := uciMap["wireless."+dev+".channel"]
			if ch == "" {
				ch = "auto"
			}
			ht := uciMap["wireless."+dev+".htmode"]
			if ht == "" {
				ht = "HT20"
			}
			ssid := uciMap["wireless."+ifaceSec+".ssid"]
			if ssid == "" {
				ssid = "OpenWrt"
			}
			enc := uciMap["wireless."+ifaceSec+".encryption"]
			if enc == "" {
				enc = "none"
			}
			txp := uciMap["wireless."+dev+".txpower"]
			if txp == "" {
				txp = "20"
			}
			country := uciMap["wireless."+dev+".country"]
			if country == "" {
				country = "VN"
			}
			return radioInfo{
				Device:       dev,
				IfaceSection: ifaceSec,
				Band:         bandLabel,
				Chip:         chip,
				Enabled:      !devDisabled && !ifDisabled,
				SSID:         ssid,
				Encryption:   enc,
				Key:          uciMap["wireless."+ifaceSec+".key"],
				Channel:      ch,
				HTMode:       ht,
				TxPower:      txp,
				Country:      country,
				Mode:         uciMap["wireless."+ifaceSec+".mode"],
			}
		}

		radios := []radioInfo{
			buildRadio("radio0", "default_radio0", "2.4GHz", "MediaTek MT7615N 4x4 (2.4G)"),
			buildRadio("radio1", "default_radio1", "5GHz", "MediaTek MT7615N 4x4 MU-MIMO (5G)"),
		}

		// Parse connected wireless stations across all wlan/phy interfaces
		type stationEntry struct {
			MAC      string `json:"mac"`
			IP       string `json:"ip"`
			Hostname string `json:"hostname"`
			Iface    string `json:"iface"`
			Band     string `json:"band"`
			Signal   string `json:"signal"`
			RxRate   string `json:"rx_rate"`
			TxRate   string `json:"tx_rate"`
		}
		leases := parseDHCPLeases("/tmp/dhcp.leases")
		leaseByMAC := map[string]dhcpLease{}
		for _, l := range leases {
			leaseByMAC[strings.ToLower(l.MAC)] = l
		}

		var stations []stationEntry
		iwDevsOut, _ := runCmd(4, "iw", "dev")
		var wlanIfaces []string
		for _, line := range strings.Split(iwDevsOut, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Interface ") {
				wlanIfaces = append(wlanIfaces, strings.TrimSpace(strings.TrimPrefix(trimmed, "Interface ")))
			}
		}
		for _, wif := range wlanIfaces {
			dump, _ := runCmd(4, "iw", "dev", wif, "station", "dump")
			var cur *stationEntry
			for _, line := range strings.Split(dump, "\n") {
				t := strings.TrimSpace(line)
				if strings.HasPrefix(t, "Station ") {
					if cur != nil {
						stations = append(stations, *cur)
					}
					fields := strings.Fields(t)
					mac := ""
					if len(fields) >= 2 {
						mac = strings.ToLower(fields[1])
					}
					l := leaseByMAC[mac]
					band := "2.4GHz"
					if strings.Contains(wif, "1") {
						band = "5GHz"
					}
					ip := l.IP
					if ip == "" {
						ip = "DHCP Pending"
					}
					hn := l.Hostname
					if hn == "" || hn == "*" {
						hn = ouiLookup(mac)
					}
					cur = &stationEntry{
						MAC:      mac,
						IP:       ip,
						Hostname: hn,
						Iface:    wif,
						Band:     band,
						Signal:   "N/A",
						TxRate:   "N/A",
					}
				} else if cur != nil {
					if strings.HasPrefix(t, "signal:") {
						cur.Signal = strings.TrimSpace(strings.TrimPrefix(t, "signal:"))
					} else if strings.HasPrefix(t, "tx bitrate:") {
						cur.TxRate = strings.TrimSpace(strings.TrimPrefix(t, "tx bitrate:"))
					} else if strings.HasPrefix(t, "rx bitrate:") {
						cur.RxRate = strings.TrimSpace(strings.TrimPrefix(t, "rx bitrate:"))
					}
				}
			}
			if cur != nil {
				stations = append(stations, *cur)
			}
		}
		if stations == nil {
			stations = []stationEntry{}
		}

		iwinfoOut, _ := runCmd(4, "iwinfo")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"radios":        radios,
			"stations":      stations,
			"station_count": len(stations),
			"uci_config":    uciMap,
			"iwinfo_raw":    iwinfoOut,
		})

	case http.MethodPut:
		var body struct {
			Device     string `json:"device"`
			Enabled    bool   `json:"enabled"`
			SSID       string `json:"ssid"`
			Encryption string `json:"encryption"`
			Key        string `json:"key"`
			Channel    string `json:"channel"`
			HTMode     string `json:"htmode"`
			TxPower    string `json:"txpower"`
			Country    string `json:"country"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid wireless config JSON")
			return
		}
		if body.Device != "radio0" && body.Device != "radio1" {
			writeJSON(w, http.StatusBadRequest, false, "device must be radio0 or radio1")
			return
		}
		ifaceSec := "default_" + body.Device
		ssid := strings.TrimSpace(body.SSID)
		if ssid == "" || len(ssid) > 32 {
			writeJSON(w, http.StatusBadRequest, false, "SSID must be between 1 and 32 characters")
			return
		}
		enc := body.Encryption
		if enc != "none" && enc != "psk2" && enc != "psk-mixed" && enc != "sae" && enc != "sae-mixed" {
			enc = "psk2"
		}
		if enc != "none" && len(body.Key) > 0 && len(body.Key) < 8 {
			writeJSON(w, http.StatusBadRequest, false, "Wi-Fi password must be at least 8 characters")
			return
		}

		disabledVal := "1"
		if body.Enabled {
			disabledVal = "0"
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.disabled=%s", body.Device, disabledVal))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.disabled=%s", ifaceSec, disabledVal))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.ssid=%s", ifaceSec, ssid))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.encryption=%s", ifaceSec, enc))
		if enc != "none" && body.Key != "" {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.key=%s", ifaceSec, body.Key))
		}
		if body.Channel != "" && safeTokenKeyPattern.MatchString(body.Channel) {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.channel=%s", body.Device, body.Channel))
		}
		if body.HTMode != "" && safeTokenKeyPattern.MatchString(body.HTMode) {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.htmode=%s", body.Device, body.HTMode))
		}
		if body.TxPower != "" && safeTokenKeyPattern.MatchString(body.TxPower) {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.txpower=%s", body.Device, body.TxPower))
		}
		if body.Country != "" && len(body.Country) == 2 {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.country=%s", body.Device, strings.ToUpper(body.Country)))
		}
		_, _ = runCmd(5, "uci", "commit", "wireless")
		go func() {
			_, _ = runCmd(10, "wifi", "reload")
		}()

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"device":  body.Device,
			"enabled": body.Enabled,
			"ssid":    ssid,
			"message": "Wireless configuration saved and wifi reload triggered.",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/wireless/restart
func handleWirelessRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	out, err := runCmd(12, "wifi", "reload")
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"restarted": err == nil,
		"output":    out,
	})
}

// ─── 3. Firewall Management (UCI + fw4 / nftables) ────────────────────────

type uciSection struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
}

func parseUCISections(pkg string) []uciSection {
	out, _ := runCmd(5, "uci", "-X", "show", pkg)
	secMap := map[string]*uciSection{}
	var order []string

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		keyPath := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
		segs := strings.Split(keyPath, ".")
		if len(segs) == 2 {
			secID := segs[1]
			if _, ok := secMap[secID]; !ok {
				secMap[secID] = &uciSection{ID: secID, Type: val, Options: map[string]string{}}
				order = append(order, secID)
			}
		} else if len(segs) >= 3 {
			secID := segs[1]
			optName := strings.Join(segs[2:], ".")
			if s, ok := secMap[secID]; ok {
				s.Options[optName] = val
			}
		}
	}

	res := make([]uciSection, 0, len(order))
	for _, id := range order {
		res = append(res, *secMap[id])
	}
	return res
}

// GET /api/v1/firewall
func handleFirewallGet(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	sections := parseUCISections("firewall")

	defaults := map[string]string{
		"syn_flood": "1",
		"input":     "REJECT",
		"output":    "ACCEPT",
		"forward":   "REJECT",
	}
	var zones []map[string]interface{}
	var redirects []map[string]interface{}
	var rules []map[string]interface{}

	for _, s := range sections {
		switch s.Type {
		case "defaults":
			for k, v := range s.Options {
				defaults[k] = v
			}
		case "zone":
			zones = append(zones, map[string]interface{}{
				"id":      s.ID,
				"name":    s.Options["name"],
				"input":   s.Options["input"],
				"output":  s.Options["output"],
				"forward": s.Options["forward"],
				"masq":    s.Options["masq"] == "1",
				"mtu_fix": s.Options["mtu_fix"] == "1",
			})
		case "redirect":
			redirects = append(redirects, map[string]interface{}{
				"id":        s.ID,
				"name":      s.Options["name"],
				"src":       s.Options["src"],
				"src_dport": s.Options["src_dport"],
				"dest":      s.Options["dest"],
				"dest_ip":   s.Options["dest_ip"],
				"dest_port": s.Options["dest_port"],
				"proto":     s.Options["proto"],
				"enabled":   s.Options["enabled"] != "0",
			})
		case "rule":
			rules = append(rules, map[string]interface{}{
				"id":        s.ID,
				"name":      s.Options["name"],
				"src":       s.Options["src"],
				"dest":      s.Options["dest"],
				"proto":     s.Options["proto"],
				"dest_port": s.Options["dest_port"],
				"target":    s.Options["target"],
				"family":    s.Options["family"],
				"enabled":   s.Options["enabled"] != "0",
			})
		}
	}
	if zones == nil {
		zones = []map[string]interface{}{}
	}
	if redirects == nil {
		redirects = []map[string]interface{}{}
	}
	if rules == nil {
		rules = []map[string]interface{}{}
	}

	connCount, _ := strconv.Atoi(readFile("/proc/sys/net/netfilter/nf_conntrack_count"))
	connMax, _ := strconv.Atoi(readFile("/proc/sys/net/netfilter/nf_conntrack_max"))
	if connMax == 0 {
		connMax = 16384
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"defaults":        defaults,
		"zones":           zones,
		"redirects":       redirects,
		"rules":           rules,
		"conntrack_count": connCount,
		"conntrack_max":   connMax,
	})
}

func isValidPortSpec(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	parts := strings.Split(s, "-")
	if len(parts) > 2 {
		return false
	}
	prev := 0
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > 65535 {
			return false
		}
		if prev > 0 && n < prev {
			return false
		}
		prev = n
	}
	return true
}

func uciFirewallSectionType(id string) string {
	out, err := runCmd(3, "uci", "-q", "get", "firewall."+id)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// POST / PATCH / DELETE /api/v1/firewall/redirect
func handleFirewallRedirect(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name     string `json:"name"`
			Proto    string `json:"proto"`
			SrcDport string `json:"src_dport"`
			DestIP   string `json:"dest_ip"`
			DestPort string `json:"dest_port"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" || strings.ContainsAny(name, "\r\n") || !isValidPortSpec(body.SrcDport) || net.ParseIP(body.DestIP) == nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid redirect rule fields (valid name, src_dport, dest_ip required)")
			return
		}
		if body.DestPort == "" {
			body.DestPort = body.SrcDport
		} else if !isValidPortSpec(body.DestPort) {
			writeJSON(w, http.StatusBadRequest, false, "invalid destination port range")
			return
		}
		proto := strings.ToLower(strings.TrimSpace(body.Proto))
		if proto != "tcp" && proto != "udp" && proto != "tcp udp" {
			proto = "tcp"
		}
		secID, err := runCmd(4, "uci", "add", "firewall", "redirect")
		if err != nil || secID == "" {
			writeJSON(w, http.StatusInternalServerError, false, "failed to create UCI redirect section")
			return
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.name=%s", secID, name))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.src=wan", secID))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest=lan", secID))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.proto=%s", secID, proto))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.src_dport=%s", secID, strings.TrimSpace(body.SrcDport)))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest_ip=%s", secID, strings.TrimSpace(body.DestIP)))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest_port=%s", secID, strings.TrimSpace(body.DestPort)))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.target=DNAT", secID))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=1", secID))
		if out, err := runCmd(5, "uci", "commit", "firewall"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit firewall failed: "+out)
			return
		}
		if out, err := runCmd(8, "/etc/init.d/firewall", "reload"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "firewall reload failed: "+out)
			return
		}
		writeJSON(w, http.StatusCreated, true, map[string]string{"id": secID})

	case http.MethodPatch:
		var body struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil || !safeTokenKeyPattern.MatchString(body.ID) {
			writeJSON(w, http.StatusBadRequest, false, "invalid request")
			return
		}
		if uciFirewallSectionType(body.ID) != "redirect" {
			writeJSON(w, http.StatusBadRequest, false, "section is not a firewall redirect rule")
			return
		}
		val := "0"
		if body.Enabled {
			val = "1"
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=%s", body.ID, val))
		if out, err := runCmd(5, "uci", "commit", "firewall"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit firewall failed: "+out)
			return
		}
		if out, err := runCmd(8, "/etc/init.d/firewall", "reload"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "firewall reload failed: "+out)
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"id": body.ID, "enabled": body.Enabled})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if !safeTokenKeyPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, false, "invalid rule ID")
			return
		}
		if uciFirewallSectionType(id) != "redirect" {
			writeJSON(w, http.StatusBadRequest, false, "section is not a firewall redirect rule")
			return
		}
		_, _ = runCmd(4, "uci", "delete", "firewall."+id)
		if out, err := runCmd(5, "uci", "commit", "firewall"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit firewall failed: "+out)
			return
		}
		if out, err := runCmd(8, "/etc/init.d/firewall", "reload"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "firewall reload failed: "+out)
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]string{"deleted": id})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST / PATCH / DELETE /api/v1/firewall/rule
func handleFirewallRule(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name     string `json:"name"`
			Src      string `json:"src"`
			Proto    string `json:"proto"`
			DestPort string `json:"dest_port"`
			Target   string `json:"target"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" || strings.ContainsAny(name, "\r\n") {
			writeJSON(w, http.StatusBadRequest, false, "valid rule name is required")
			return
		}
		if strings.TrimSpace(body.DestPort) != "" && !isValidPortSpec(body.DestPort) {
			writeJSON(w, http.StatusBadRequest, false, "invalid dest_port")
			return
		}
		target := strings.ToUpper(body.Target)
		if target != "ACCEPT" && target != "DROP" && target != "REJECT" {
			target = "ACCEPT"
		}
		src := body.Src
		if src != "wan" && src != "lan" {
			src = "wan"
		}
		proto := strings.ToLower(body.Proto)
		if proto != "tcp" && proto != "udp" && proto != "icmp" && proto != "all" {
			proto = "tcp"
		}
		secID, err := runCmd(4, "uci", "add", "firewall", "rule")
		if err != nil || secID == "" {
			writeJSON(w, http.StatusInternalServerError, false, "failed to add firewall rule")
			return
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.name=%s", secID, name))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.src=%s", secID, src))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.proto=%s", secID, proto))
		if strings.TrimSpace(body.DestPort) != "" {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest_port=%s", secID, strings.TrimSpace(body.DestPort)))
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.target=%s", secID, target))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=1", secID))
		if out, err := runCmd(5, "uci", "commit", "firewall"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit firewall failed: "+out)
			return
		}
		if out, err := runCmd(8, "/etc/init.d/firewall", "reload"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "firewall reload failed: "+out)
			return
		}
		writeJSON(w, http.StatusCreated, true, map[string]string{"id": secID})

	case http.MethodPatch:
		var body struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil || !safeTokenKeyPattern.MatchString(body.ID) {
			writeJSON(w, http.StatusBadRequest, false, "invalid request")
			return
		}
		if uciFirewallSectionType(body.ID) != "rule" {
			writeJSON(w, http.StatusBadRequest, false, "section is not a firewall rule")
			return
		}
		val := "0"
		if body.Enabled {
			val = "1"
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=%s", body.ID, val))
		if out, err := runCmd(5, "uci", "commit", "firewall"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit firewall failed: "+out)
			return
		}
		if out, err := runCmd(8, "/etc/init.d/firewall", "reload"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "firewall reload failed: "+out)
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"id": body.ID, "enabled": body.Enabled})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if !safeTokenKeyPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, false, "invalid rule ID")
			return
		}
		if uciFirewallSectionType(id) != "rule" {
			writeJSON(w, http.StatusBadRequest, false, "section is not a firewall rule")
			return
		}
		_, _ = runCmd(4, "uci", "delete", "firewall."+id)
		if out, err := runCmd(5, "uci", "commit", "firewall"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit firewall failed: "+out)
			return
		}
		if out, err := runCmd(8, "/etc/init.d/firewall", "reload"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "firewall reload failed: "+out)
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]string{"deleted": id})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// ─── 4. NAS & Storage File Manager ────────────────────────────────────────

type ShareItem struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	ReadOnly  bool   `json:"read_only"`
	GuestOk   bool   `json:"guest_ok"`
	Comment   string `json:"comment"`
	Users     string `json:"users,omitempty"`
	Password  string `json:"password,omitempty"`
}

const sharesConfigFile = "/etc/r3p/shares.json"

func loadNASShares() []ShareItem {
	_ = os.MkdirAll("/overlay/share", 0755)
	_ = os.MkdirAll("/overlay/downloads", 0755)
	var list []ShareItem
	data, err := os.ReadFile(sharesConfigFile)
	if err == nil {
		_ = json.Unmarshal(data, &list)
	}
	if len(list) == 0 {
		list = []ShareItem{
			{Name: "USB_500G_HDD", Path: "/mnt/sda1", ReadOnly: false, GuestOk: true, Comment: "USB 3.0 500GB External HDD Root"},
			{Name: "USB_Downloads", Path: "/mnt/sda1/downloads", ReadOnly: false, GuestOk: true, Comment: "R3P Auto-Downloader & USB High-Speed Downloads"},
			{Name: "NAND_Share", Path: "/overlay/share", ReadOnly: false, GuestOk: true, Comment: "Persistent NAND Shared Storage"},
			{Name: "Downloads", Path: "/overlay/downloads", ReadOnly: false, GuestOk: true, Comment: "NAND Download Folder"},
			{Name: "RAM_Scratch", Path: "/tmp/downloads", ReadOnly: false, GuestOk: true, Comment: "High-Speed RAM Scratch Disk"},
		}
		saveNASShares(list)
		return list
	}
	hasUSB := false
	for i := range list {
		list[i].Password = ""
		if strings.HasPrefix(list[i].Path, "/mnt/sda1") {
			hasUSB = true
		}
	}
	if !hasUSB {
		if st, err := os.Stat("/mnt/sda1"); err == nil && st.IsDir() {
			list = append([]ShareItem{
				{Name: "USB_500G_HDD", Path: "/mnt/sda1", ReadOnly: false, GuestOk: true, Comment: "USB 3.0 500GB External HDD Root"},
				{Name: "USB_Downloads", Path: "/mnt/sda1/downloads", ReadOnly: false, GuestOk: true, Comment: "R3P Auto-Downloader & USB High-Speed Downloads"},
			}, list...)
			saveNASShares(list)
		}
	}
	return list
}

func saveNASShares(list []ShareItem) {
	_ = os.MkdirAll("/etc/r3p", 0755)
	cleanList := make([]ShareItem, len(list))
	for i, s := range list {
		s.Password = ""
		cleanList[i] = s
	}
	if b, err := json.MarshalIndent(cleanList, "", "  "); err == nil {
		_ = os.WriteFile(sharesConfigFile, b, 0644)
	}
	_ = syncSambaConfigFile(list)
}

func escapeUCIQuote(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return strings.ReplaceAll(v, "'", "'\\''")
}

func getServerLanIP() string {
	return getRouterLanIP()
}

func syncSambaConfigFile(shares []ShareItem) error {
	// 1. Ensure 'invalid users = root' is commented out in /etc/samba/smb.conf.template only if present
	if tmpl, err := os.ReadFile("/etc/samba/smb.conf.template"); err == nil {
		sTmpl := string(tmpl)
		if strings.Contains(sTmpl, "invalid users = root") && !strings.Contains(sTmpl, "#invalid users = root") {
			sTmpl = strings.ReplaceAll(sTmpl, "invalid users = root", "#invalid users = root")
			_ = os.WriteFile("/etc/samba/smb.conf.template", []byte(sTmpl), 0644)
		}
	}

	sambaUCI := parseUCIConfigMap("samba4")
	workgroup := sambaUCI["samba4.@samba[0].workgroup"]
	if workgroup == "" {
		workgroup = "WORKGROUP"
	}
	desc := sambaUCI["samba4.@samba[0].description"]
	if desc == "" {
		desc = "R3P Mini Server NAS"
	}
	tuning := sambaUCI["samba4.@samba[0].enable_extra_tuning"]
	if tuning == "" {
		tuning = "1"
	}

	var sb strings.Builder
	sb.WriteString("config samba\n")
	sb.WriteString(fmt.Sprintf("\toption workgroup '%s'\n", escapeUCIQuote(workgroup)))
	sb.WriteString("\toption charset 'UTF-8'\n")
	sb.WriteString(fmt.Sprintf("\toption description '%s'\n", escapeUCIQuote(desc)))
	sb.WriteString("\toption interface 'lan'\n")
	sb.WriteString(fmt.Sprintf("\toption enable_extra_tuning '%s'\n", escapeUCIQuote(tuning)))

	for _, s := range shares {
		name := sanitizeFilename(s.Name)
		if name == "" || s.Path == "" {
			continue
		}
		_ = os.MkdirAll(s.Path, 0755)
		ro := "no"
		if s.ReadOnly {
			ro = "yes"
		}
		guest := "yes"
		if !s.GuestOk {
			guest = "no"
		}
		sb.WriteString("\nconfig sambashare\n")
		sb.WriteString(fmt.Sprintf("\toption name '%s'\n", escapeUCIQuote(name)))
		sb.WriteString(fmt.Sprintf("\toption path '%s'\n", escapeUCIQuote(s.Path)))
		sb.WriteString(fmt.Sprintf("\toption read_only '%s'\n", ro))
		sb.WriteString(fmt.Sprintf("\toption guest_ok '%s'\n", guest))
		if !s.GuestOk && strings.TrimSpace(s.Users) != "" {
			usr := strings.TrimSpace(s.Users)
			sb.WriteString(fmt.Sprintf("\toption users '%s'\n", escapeUCIQuote(usr)))
		}
		sb.WriteString("\toption browseable 'yes'\n")
		sb.WriteString("\toption create_mask '0666'\n")
		sb.WriteString("\toption dir_mask '0777'\n")
	}

	err := os.WriteFile("/etc/config/samba4", []byte(sb.String()), 0644)
	if err == nil {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_, _ = runCmd(8, "/etc/init.d/samba4", "restart")
		}()
	}
	return err
}

// GET /api/v1/nas/services
// PUT /api/v1/nas/services/samba
// PUT /api/v1/nas/services/ftp
func handleNASServices(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	lanIP := getServerLanIP()
	sambaUCI := parseUCIConfigMap("samba4")

	smbRunning := false
	ftpRunning := false
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entries {
			if _, err := strconv.Atoi(e.Name()); err == nil {
				cmd := readFile("/proc/" + e.Name() + "/cmdline")
				if strings.Contains(cmd, "smbd") {
					smbRunning = true
				}
				if strings.Contains(cmd, "vsftpd") {
					ftpRunning = true
				}
			}
		}
	}

	smbEnabledMatches, _ := filepath.Glob("/etc/rc.d/S*samba4")
	ftpEnabledMatches, _ := filepath.Glob("/etc/rc.d/S*vsftpd")

	ftpPort := 21
	ftpRootDir := "/mnt/sda1"
	ftpWriteEnable := true
	ftpAnonEnable := false
	ftpLocalEnable := true

	for _, line := range readFileLines("/etc/vsftpd.conf", 100) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		switch k {
		case "listen_port":
			if p, err := strconv.Atoi(v); err == nil && p > 0 {
				ftpPort = p
			}
		case "local_root", "anon_root":
			if v != "" {
				ftpRootDir = v
			}
		case "write_enable":
			ftpWriteEnable = strings.ToUpper(v) == "YES"
		case "anonymous_enable":
			ftpAnonEnable = strings.ToUpper(v) == "YES"
		case "local_enable":
			ftpLocalEnable = strings.ToUpper(v) == "YES"
		}
	}

	workgroup := sambaUCI["samba4.@samba[0].workgroup"]
	if workgroup == "" {
		workgroup = "WORKGROUP"
	}
	desc := sambaUCI["samba4.@samba[0].description"]
	if desc == "" {
		desc = "R3P Mini Server NAS"
	}
	extraTuning := sambaUCI["samba4.@samba[0].enable_extra_tuning"] != "0"

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"server_ip": lanIP,
			"samba": map[string]interface{}{
				"running":      smbRunning,
				"enabled":      len(smbEnabledMatches) > 0,
				"workgroup":    workgroup,
				"description":  desc,
				"extra_tuning": extraTuning,
				"allow_guest":  true,
			},
			"ftp": map[string]interface{}{
				"running":          ftpRunning,
				"enabled":          len(ftpEnabledMatches) > 0,
				"port":             ftpPort,
				"root_dir":         ftpRootDir,
				"write_enable":     ftpWriteEnable,
				"anonymous_enable": ftpAnonEnable,
				"local_enable":     ftpLocalEnable,
			},
		})

	case http.MethodPut:
		path := r.URL.Path
		if strings.HasSuffix(path, "/samba") {
			var body struct {
				Enabled     *bool  `json:"enabled"`
				Running     *bool  `json:"running"`
				Action      string `json:"action"`
				Workgroup   string `json:"workgroup"`
				Description string `json:"description"`
				ExtraTuning *bool  `json:"extra_tuning"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
				return
			}
			if body.Workgroup != "" && safeTokenKeyPattern.MatchString(body.Workgroup) {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("samba4.@samba[0].workgroup=%s", body.Workgroup))
			}
			if body.Description != "" {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("samba4.@samba[0].description=%s", body.Description))
			}
			if body.ExtraTuning != nil {
				tunVal := "0"
				if *body.ExtraTuning {
					tunVal = "1"
				}
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("samba4.@samba[0].enable_extra_tuning=%s", tunVal))
			}
			_, _ = runCmd(4, "uci", "commit", "samba4")

			if body.Enabled != nil {
				if *body.Enabled {
					_, _ = runCmd(4, "/etc/init.d/samba4", "enable")
				} else {
					_, _ = runCmd(4, "/etc/init.d/samba4", "disable")
				}
			}

			_ = syncSambaConfigFile(loadNASShares())

			act := body.Action
			if act == "" {
				if body.Running != nil {
					if *body.Running {
						act = "restart"
					} else {
						act = "stop"
					}
				} else {
					act = "restart"
				}
			}
			_, _ = runCmd(8, "/etc/init.d/samba4", act)

			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"saved":   true,
				"message": "Đã lưu và áp dụng cấu hình Samba (SMB) thành công!",
			})
			return

		} else if strings.HasSuffix(path, "/ftp") {
			var body struct {
				Enabled         *bool  `json:"enabled"`
				Running         *bool  `json:"running"`
				Action          string `json:"action"`
				Port            int    `json:"port"`
				RootDir         string `json:"root_dir"`
				WriteEnable     *bool  `json:"write_enable"`
				AnonymousEnable *bool  `json:"anonymous_enable"`
				LocalEnable     *bool  `json:"local_enable"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
				return
			}
			if body.Port > 0 && body.Port <= 65535 {
				ftpPort = body.Port
			}
			if body.RootDir != "" {
				validPath, err := validateNASPath(body.RootDir)
				if err == nil {
					ftpRootDir = validPath
				}
			}
			if body.WriteEnable != nil {
				ftpWriteEnable = *body.WriteEnable
			}
			if body.AnonymousEnable != nil {
				ftpAnonEnable = *body.AnonymousEnable
			}
			if body.LocalEnable != nil {
				ftpLocalEnable = *body.LocalEnable
			}

			anonStr := "NO"
			if ftpAnonEnable {
				anonStr = "YES"
			}
			localStr := "NO"
			if ftpLocalEnable {
				localStr = "YES"
			}
			writeStr := "NO"
			if ftpWriteEnable {
				writeStr = "YES"
			}

			var sb strings.Builder
			sb.WriteString("background=NO\nlisten=YES\n")
			sb.WriteString(fmt.Sprintf("listen_port=%d\n", ftpPort))
			sb.WriteString(fmt.Sprintf("anonymous_enable=%s\n", anonStr))
			sb.WriteString(fmt.Sprintf("local_enable=%s\n", localStr))
			sb.WriteString(fmt.Sprintf("write_enable=%s\n", writeStr))
			sb.WriteString("local_umask=022\ncheck_shell=NO\nsession_support=NO\n")
			sb.WriteString(fmt.Sprintf("local_root=%s\n", ftpRootDir))
			sb.WriteString(fmt.Sprintf("anon_root=%s\n", ftpRootDir))

			_ = os.WriteFile("/etc/vsftpd.conf", []byte(sb.String()), 0644)

			if body.Enabled != nil {
				if *body.Enabled {
					_, _ = runCmd(4, "/etc/init.d/vsftpd", "enable")
				} else {
					_, _ = runCmd(4, "/etc/init.d/vsftpd", "disable")
				}
			}

			act := body.Action
			if act == "" {
				if body.Running != nil {
					if *body.Running {
						act = "restart"
					} else {
						act = "stop"
					}
				} else {
					act = "restart"
				}
			}
			_, _ = runCmd(8, "/etc/init.d/vsftpd", act)

			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"saved":   true,
				"message": fmt.Sprintf("Đã lưu và áp dụng cấu hình máy chủ FTP (Cổng %d, thư mục %s) thành công!", ftpPort, ftpRootDir),
			})
			return
		}

		writeJSON(w, http.StatusBadRequest, false, "endpoint path must end with /samba or /ftp")

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

func isMountedOrValidNASDir(clean string) bool {
	clean = filepath.ToSlash(filepath.Clean(clean))
	return isUnderBaseDir(clean, "/overlay/share") ||
		isUnderBaseDir(clean, "/overlay/downloads") ||
		isUnderBaseDir(clean, "/tmp/downloads") ||
		(isUnderBaseDir(clean, "/mnt") && clean != "/mnt") ||
		(isUnderBaseDir(clean, "/media") && clean != "/media")
}

func validateNASPath(rawDir string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rawDir)))
	if !isMountedOrValidNASDir(clean) {
		return "", fmt.Errorf("path must be inside /overlay/share, /overlay/downloads, /tmp/downloads, /mnt/*, or /media/*")
	}
	if linfo, err := os.Lstat(clean); err == nil && linfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symlinks are not permitted in NAS paths")
	}
	return clean, nil
}

// GET /api/v1/nas
func handleNASOverview(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	dfOut, _ := runCmd(4, "df", "-k")
	type diskVolume struct {
		Device     string  `json:"device"`
		MountPoint string  `json:"mount_point"`
		Label      string  `json:"label"`
		TotalKB    uint64  `json:"total_kb"`
		UsedKB     uint64  `json:"used_kb"`
		AvailKB    uint64  `json:"avail_kb"`
		Percent    float64 `json:"percent"`
	}
	var volumes []diskVolume
	lines := strings.Split(dfOut, "\n")
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			f := strings.Fields(line)
			if len(f) < 6 {
				continue
			}
			dev := f[0]
			mp := f[5]
			if mp == "/dev" {
				continue
			}
			// In OpenWrt, the rootfs overlay is mounted at "/" with device "overlayfs:/overlay" or "ubi0:rootfs_data"
			if mp == "/" {
				if strings.Contains(dev, "overlay") || strings.Contains(dev, "ubi") {
					mp = "/overlay" // Map to /overlay so UI and charts identify NAND Flash accurately
				} else {
					continue
				}
			}
			tot, _ := strconv.ParseUint(f[1], 10, 64)
			used, _ := strconv.ParseUint(f[2], 10, 64)
			avail, _ := strconv.ParseUint(f[3], 10, 64)
			pct := 0.0
			if tot > 0 {
				pct = float64(used) / float64(tot) * 100
			}
			label := "System Partition"
			switch {
			case mp == "/overlay":
				label = "NAND Flash System (/overlay)"
			case mp == "/tmp":
				label = "RAM Disk (tmpfs /tmp)"
			case mp == "/rom":
				label = "Read-Only SquashFS (/rom)"
			case strings.HasPrefix(dev, "/dev/sd"):
				label = "USB 3.0 External Storage (" + dev + ")"
			}
			volumes = append(volumes, diskVolume{
				Device:     dev,
				MountPoint: mp,
				Label:      label,
				TotalKB:    tot,
				UsedKB:     used,
				AvailKB:    avail,
				Percent:    pct,
			})
		}
	}

	// Browse requested directory
	browseDir := r.URL.Query().Get("dir")
	if browseDir == "" {
		if st, err := os.Stat("/mnt/sda1"); err == nil && st.IsDir() {
			browseDir = "/mnt/sda1"
		} else {
			browseDir = "/overlay/share"
		}
	}
	validDir, err := validateNASPath(browseDir)
	if err != nil {
		validDir = "/overlay/share"
	}

	type fileItem struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		IsDir     bool   `json:"is_dir"`
		Size      int64  `json:"size"`
		UpdatedAt string `json:"updated_at"`
	}
	var files []fileItem
	if entries, err := os.ReadDir(validDir); err == nil {
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, fileItem{
				Name:      e.Name(),
				Path:      filepath.Join(validDir, e.Name()),
				IsDir:     e.IsDir(),
				Size:      info.Size(),
				UpdatedAt: info.ModTime().Format("2006-01-02 15:04"),
			})
		}
	}
	if files == nil {
		files = []fileItem{}
	}

	shares := loadNASShares()
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"volumes":     volumes,
		"shares":      shares,
		"current_dir": validDir,
		"files":       files,
		"server_ip":   getServerLanIP(),
	})
}

// POST /api/v1/nas/mkdir
func ensureSafeNASParentDir(baseDir string, createSystemRoots bool) error {
	if createSystemRoots && (isUnderBaseDir(baseDir, "/overlay/share") || isUnderBaseDir(baseDir, "/overlay/downloads") || isUnderBaseDir(baseDir, "/tmp/downloads")) {
		if err := os.MkdirAll(baseDir, 0755); err != nil {
			return err
		}
	}
	if realDir, err := filepath.EvalSymlinks(baseDir); err == nil {
		if !isMountedOrValidNASDir(filepath.Clean(realDir)) {
			return fmt.Errorf("resolved parent path escapes NAS root")
		}
	} else {
		return err
	}
	return nil
}

func handleNASMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Dir  string `json:"dir"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	baseDir, err := validateNASPath(body.Dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	if err := ensureSafeNASParentDir(baseDir, true); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "base directory unavailable: "+err.Error())
		return
	}
	folderName := sanitizeFilename(body.Name)
	if folderName == "" || folderName == "." || folderName == ".." {
		writeJSON(w, http.StatusBadRequest, false, "invalid folder name")
		return
	}
	target := filepath.Join(baseDir, folderName)
	if linfo, lerr := os.Lstat(target); lerr == nil && linfo.Mode()&os.ModeSymlink != 0 {
		writeJSON(w, http.StatusBadRequest, false, "cannot create directory over symlink")
		return
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, false, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, true, map[string]string{"path": target})
}

// POST /api/v1/nas/upload
func handleNASUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // up to 64MB upload
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "failed to parse upload form")
		return
	}
	dir := r.FormValue("dir")
	baseDir, err := validateNASPath(dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	if err := ensureSafeNASParentDir(baseDir, true); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "target directory unavailable: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, "missing file")
		return
	}
	defer file.Close()

	safeName := sanitizeFilename(header.Filename)
	destPath := filepath.Join(baseDir, safeName)
	if linfo, lerr := os.Lstat(destPath); lerr == nil && (linfo.Mode()&os.ModeSymlink != 0 || linfo.IsDir()) {
		writeJSON(w, http.StatusBadRequest, false, "refusing to overwrite symlink or directory")
		return
	}
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "could not create file: "+err.Error())
		return
	}
	defer out.Close()
	written, err := io.Copy(out, file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "write failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, true, map[string]interface{}{
		"name": safeName,
		"path": destPath,
		"size": written,
	})
}

// DELETE /api/v1/nas/file?path=...
func handleNASDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" {
		writeJSON(w, http.StatusBadRequest, false, "path is required")
		return
	}
	clean := filepath.ToSlash(filepath.Clean(rawPath))
	parent := filepath.ToSlash(filepath.Dir(clean))
	if clean == "/overlay/share" || clean == "/overlay/downloads" || clean == "/tmp/downloads" || parent == "/mnt" || parent == "/media" {
		writeJSON(w, http.StatusBadRequest, false, "cannot delete root share or mount folder")
		return
	}
	if _, err := validateNASPath(parent); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	if err := ensureSafeNASParentDir(parent, false); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	if _, err := os.Lstat(clean); err != nil {
		writeJSON(w, http.StatusNotFound, false, "file or folder not found")
		return
	}
	if err := os.RemoveAll(clean); err != nil {
		writeJSON(w, http.StatusInternalServerError, false, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]string{"deleted": clean})
}

// GET /api/v1/nas/download?path=...
func handleNASDownloadFile(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	clean := filepath.Clean(rawPath)
	parent := filepath.Dir(clean)
	if _, err := validateNASPath(parent); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	if err := ensureSafeNASParentDir(parent, false); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	info, err := os.Lstat(clean)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		writeJSON(w, http.StatusNotFound, false, "file not found")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name()))
	http.ServeFile(w, r, clean)
}

// POST / PUT / DELETE /api/v1/nas/shares
func handleNASShares(w http.ResponseWriter, r *http.Request) {
	shares := loadNASShares()
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var body ShareItem
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		body.Name = sanitizeFilename(body.Name)
		if body.Name == "" {
			writeJSON(w, http.StatusBadRequest, false, "share name is required")
			return
		}
		validPath, err := validateNASPath(body.Path)
		if err != nil {
			// If updating existing share and path is unchanged/valid, keep original path
			foundPath := ""
			for _, s := range shares {
				if s.Name == body.Name {
					foundPath = s.Path
					break
				}
			}
			if foundPath != "" {
				body.Path = foundPath
			} else {
				writeJSON(w, http.StatusBadRequest, false, err.Error())
				return
			}
		} else {
			body.Path = validPath
		}

		sharePass := strings.TrimSpace(body.Password)
		body.Password = ""
		if sharePass != "" && strings.TrimSpace(body.Users) != "" {
			for _, rawU := range strings.FieldsFunc(body.Users, func(c rune) bool { return c == ',' || c == ' ' || c == ';' }) {
				u := strings.TrimSpace(rawU)
				if u == "" {
					continue
				}
				if !safeTokenKeyPattern.MatchString(u) {
					writeJSON(w, http.StatusBadRequest, false, "invalid share username")
					return
				}
				if uErr := ensureUnixAndSambaUser(u, sharePass); uErr != nil {
					writeJSON(w, http.StatusInternalServerError, false, "failed to provision share user: "+uErr.Error())
					return
				}
			}
		}

		found := false
		for i, s := range shares {
			if s.Name == body.Name {
				shares[i] = body
				found = true
				break
			}
		}
		if !found {
			shares = append(shares, body)
		}
		saveNASShares(shares)
		writeJSON(w, http.StatusOK, true, shares)

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		updated := []ShareItem{}
		for _, s := range shares {
			if s.Name != name {
				updated = append(updated, s)
			}
		}
		saveNASShares(updated)
		writeJSON(w, http.StatusOK, true, updated)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/usb/standby - Puts NVMe USB SSD into standby/sleep or configures auto-standby timeout
func handleUSBStandby(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	if r.Method == http.MethodGet {
		minsStr, _ := runCmd(2, "uci", "-q", "get", "hd-idle.@hd-idle[0].idle_time_interval")
		minsStr = strings.TrimSpace(minsStr)
		enabledStr, _ := runCmd(2, "uci", "-q", "get", "hd-idle.@hd-idle[0].enabled")
		enabledStr = strings.TrimSpace(enabledStr)
		mins, _ := strconv.Atoi(minsStr)
		if mins <= 0 {
			mins = 10
		}
		enabled := enabledStr != "0"
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"enabled": enabled,
			"minutes": mins,
		})
		return
	}
	var body struct {
		Action  string `json:"action"` // "standby_now" or "set_timeout"
		Minutes int    `json:"minutes"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)

	if body.Action == "set_timeout" {
		mins := body.Minutes
		if mins < 0 {
			mins = 10
		}
		if mins == 0 {
			_, _ = runCmd(3, "uci", "set", "hd-idle.@hd-idle[0].enabled=0")
		} else {
			_, _ = runCmd(3, "uci", "set", "hd-idle.@hd-idle[0].enabled=1")
			_, _ = runCmd(3, "uci", "set", fmt.Sprintf("hd-idle.@hd-idle[0].idle_time_interval=%d", mins))
		}
		_, _ = runCmd(3, "uci", "commit", "hd-idle")
		_, _ = runCmd(5, "/etc/init.d/hd-idle", "restart")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message": fmt.Sprintf("Đã đặt thời gian tự động nghỉ của ổ cứng SSD là %d phút!", mins),
			"enabled": mins > 0,
			"minutes": mins,
		})
		return
	}

	// Immediate standby
	_, _ = runCmd(3, "sync")
	out, err := runCmd(4, "hdparm", "-y", "/dev/sda")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Không thể chuyển ổ đĩa sang chế độ nghỉ: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"standby": true,
		"output":  strings.TrimSpace(out),
		"message": "Đã cho ổ cứng SSD NVMe chuyển sang trạng thái nghỉ (Standby/Sleep) thành công để hạ nhiệt!",
	})
}

func ensureUnixAndSambaUser(u, p string) error {
	if !safeTokenKeyPattern.MatchString(u) {
		return fmt.Errorf("invalid username")
	}
	if p == "" || strings.ContainsAny(p, "\r\n\x00") {
		return fmt.Errorf("invalid password characters")
	}
	nasUsersMu.Lock()
	defer nasUsersMu.Unlock()

	// 1. Ensure user is present in /etc/passwd and /etc/group (OpenWrt Busybox lacks adduser/useradd)
	pContent, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return fmt.Errorf("failed to read /etc/passwd: %w", err)
	}
	lines := strings.Split(string(pContent), "\n")
	found := false
	maxUID := 1000
	for _, l := range lines {
		parts := strings.Split(l, ":")
		if len(parts) >= 3 {
			if parts[0] == u {
				found = true
			}
			if uid, uidErr := strconv.Atoi(parts[2]); uidErr == nil && uid >= maxUID {
				maxUID = uid + 1
			}
		}
	}
	if !found {
		entry := fmt.Sprintf("%s:x:%d:%d:%s:/var:/bin/false\n", u, maxUID, maxUID, u)
		f, err := os.OpenFile("/etc/passwd", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to update /etc/passwd: %w", err)
		}
		if _, werr := f.WriteString(entry); werr != nil {
			_ = f.Close()
			return fmt.Errorf("failed to write /etc/passwd: %w", werr)
		}
		if cerr := f.Close(); cerr != nil {
			return fmt.Errorf("failed to close /etc/passwd: %w", cerr)
		}

		gEntry := fmt.Sprintf("%s:x:%d:\n", u, maxUID)
		fg, gerr := os.OpenFile("/etc/group", os.O_APPEND|os.O_WRONLY, 0644)
		if gerr != nil {
			return fmt.Errorf("failed to update /etc/group: %w", gerr)
		}
		if _, werr := fg.WriteString(gEntry); werr != nil {
			_ = fg.Close()
			return fmt.Errorf("failed to write /etc/group: %w", werr)
		}
		if cerr := fg.Close(); cerr != nil {
			return fmt.Errorf("failed to close /etc/group: %w", cerr)
		}
	}

	// 2. Add / Update password in Samba passdb
	cmdAdd := exec.Command("smbpasswd", "-a", "-s", u)
	cmdAdd.Stdin = strings.NewReader(p + "\n" + p + "\n")
	if out, err := cmdAdd.CombinedOutput(); err != nil {
		// If already added, try updating password without -a
		cmdSet := exec.Command("smbpasswd", "-s", u)
		cmdSet.Stdin = strings.NewReader(p + "\n" + p + "\n")
		if outSet, errSet := cmdSet.CombinedOutput(); errSet != nil {
			return fmt.Errorf("smbpasswd failed: %s / %s", string(out), string(outSet))
		}
	}
	return nil
}

func removeUnixUser(u string) {
	if u == "" || u == "root" || !safeTokenKeyPattern.MatchString(u) {
		return
	}
	nasUsersMu.Lock()
	defer nasUsersMu.Unlock()
	if pContent, err := os.ReadFile("/etc/passwd"); err == nil {
		lines := strings.Split(string(pContent), "\n")
		var newLines []string
		for _, l := range lines {
			if strings.HasPrefix(l, u+":") {
				continue
			}
			newLines = append(newLines, l)
		}
		_ = os.WriteFile("/etc/passwd", []byte(strings.Join(newLines, "\n")), 0644)
	}
	if gContent, err := os.ReadFile("/etc/group"); err == nil {
		lines := strings.Split(string(gContent), "\n")
		var newLines []string
		for _, l := range lines {
			if strings.HasPrefix(l, u+":") {
				continue
			}
			newLines = append(newLines, l)
		}
		_ = os.WriteFile("/etc/group", []byte(strings.Join(newLines, "\n")), 0644)
	}
}

// GET / POST / DELETE /api/v1/nas/users - Manage Samba/System users and passwords
func handleNASUsers(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		type UserInfo struct {
			Username string `json:"username"`
			IsRoot   bool   `json:"is_root"`
			Status   string `json:"status"`
		}
		var users []UserInfo
		users = append(users, UserInfo{Username: "root", IsRoot: true, Status: "Sẵn sàng (System Admin)"})
		if out, err := runCmd(3, "pdbedit", "-L"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				parts := strings.Split(line, ":")
				if len(parts) >= 1 {
					uname := strings.TrimSpace(parts[0])
					if uname != "" && uname != "root" {
						users = append(users, UserInfo{
							Username: uname,
							IsRoot:   false,
							Status:   "Samba User",
						})
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"users": users})

	case http.MethodPost:
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
			writeJSON(w, http.StatusBadRequest, false, "Username và Password không được để trống")
			return
		}
		u := strings.TrimSpace(body.Username)
		p := strings.TrimSpace(body.Password)
		if !safeTokenKeyPattern.MatchString(u) {
			writeJSON(w, http.StatusBadRequest, false, "Tên tài khoản không hợp lệ (chỉ chữ cái, số, gạch dưới)")
			return
		}

		if err := ensureUnixAndSambaUser(u, p); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi tạo tài khoản Samba: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"created":  true,
			"username": u,
			"message":  fmt.Sprintf("Đã tạo/cập nhật mật khẩu tài khoản '%s' thành công!", u),
		})

	case http.MethodDelete:
		u := r.URL.Query().Get("username")
		if u == "" || u == "root" {
			writeJSON(w, http.StatusBadRequest, false, "Không thể xóa tài khoản root")
			return
		}
		_, _ = runCmd(3, "smbpasswd", "-x", u)
		removeUnixUser(u)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"deleted": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/network/speedtest?interface=wwan - Measures bandwidth through chosen interface
func handleInterfaceSpeedtest(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	iface := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("interface")))
	if iface == "" {
		iface = "wwan"
	}
	if !safeTokenKeyPattern.MatchString(iface) || strings.HasPrefix(iface, "-") {
		writeJSON(w, http.StatusBadRequest, false, "invalid network interface")
		return
	}

	// Resolve UCI logical interface name (wan/wwan/usbwan) to its active Linux l3_device
	dev := iface
	if ubusOut, err := runCmd(3, "ubus", "call", "network.interface."+iface, "status"); err == nil {
		var ubusData struct {
			Up       bool   `json:"up"`
			L3Device string `json:"l3_device"`
			Device   string `json:"device"`
		}
		if json.Unmarshal([]byte(ubusOut), &ubusData) == nil {
			if ubusData.L3Device != "" && safeTokenKeyPattern.MatchString(ubusData.L3Device) {
				dev = ubusData.L3Device
			} else if ubusData.Device != "" && safeTokenKeyPattern.MatchString(ubusData.Device) {
				dev = ubusData.Device
			}
		}
	}

	parseCurlStats := func(raw string) (float64, float64) {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			return 0, 0
		}
		bps, _ := strconv.ParseFloat(fields[0], 64)
		var connectSec float64
		if len(fields) >= 2 {
			connectSec, _ = strconv.ParseFloat(fields[1], 64)
		}
		return bps, connectSec * 1000.0
	}

	probeURL := func(targetURL string, maxSecs string) (float64, float64, error) {
		args := []string{"-fsSL", "-w", "%{speed_download} %{time_connect}", "-o", "/dev/null", "--max-time", maxSecs}
		if dev != "" {
			args = append(args, "--interface", dev)
		}
		args = append(args, targetURL)
		out, err := runCmd(12, "curl", args...)
		bps, latMs := parseCurlStats(out)
		return bps, latMs, err
	}

	bytesPerSec, latencyMs, err := probeURL("https://speed.cloudflare.com/__down?bytes=5000000", "8")
	if err != nil || bytesPerSec <= 0 {
		bytesPerSec, latencyMs, err = probeURL("http://speedtest.tele2.net/1MB.zip", "8")
	}
	if bytesPerSec <= 0 {
		writeJSON(w, http.StatusBadGateway, false, fmt.Sprintf("Không thể đo băng thông qua cổng %s (%s): kiểm tra kết nối Internet của interface này", strings.ToUpper(iface), dev))
		return
	}

	downMbps := (bytesPerSec * 8) / (1024 * 1024)
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"interface":     iface,
		"device":        dev,
		"download_mbps": mathRound(downMbps, 1),
		"upload_mbps":   0.0,
		"latency_ms":    mathRound(latencyMs, 1),
		"tested_at":     time.Now().Format("15:04:05"),
		"message":       fmt.Sprintf("Đã đo tốc độ cổng %s (%s): ↓ %.1f Mbps (Latency %.1f ms)", strings.ToUpper(iface), dev, mathRound(downMbps, 1), mathRound(latencyMs, 1)),
	})
}

func mathRound(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

// ─── 4b. Download Bot & USB Video Storage API ──────────────────────────────

type DownloadFileItem struct {
	Name     string `json:"name"`
	SubPath  string `json:"sub_path"`
	FullPath string `json:"full_path"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
	Category string `json:"category"`
}

func handleDownloadFiles(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	dlMu.Lock()
	baseDir := dlConfig.OutputDirectory
	dlMu.Unlock()
	if baseDir == "" {
		baseDir = getDefaultDownloadOutputDir()
	}

	var files []DownloadFileItem
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || strings.HasSuffix(info.Name(), ".part") || strings.HasSuffix(info.Name(), ".ytdl") {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)
		cat := "Khác"
		if strings.HasPrefix(rel, "Youtube") || strings.Contains(strings.ToLower(rel), "youtube") {
			cat = "YouTube"
		} else if strings.HasPrefix(rel, "TikTok") || strings.Contains(strings.ToLower(rel), "tiktok") {
			cat = "TikTok"
		} else if strings.HasPrefix(rel, "Facebook") || strings.Contains(strings.ToLower(rel), "facebook") {
			cat = "Facebook"
		}
		files = append(files, DownloadFileItem{
			Name:     info.Name(),
			SubPath:  filepath.ToSlash(rel),
			FullPath: filepath.ToSlash(path),
			Size:     info.Size(),
			ModTime:  info.ModTime().Format(time.RFC3339),
			Category: cat,
		})
		return nil
	})

	// Sort newest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime > files[j].ModTime
	})

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"output_directory": baseDir,
		"total":            len(files),
		"files":            files,
	})
}

func handleDownloadFileServe(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	dlMu.Lock()
	baseDir := dlConfig.OutputDirectory
	dlMu.Unlock()
	if baseDir == "" {
		baseDir = getDefaultDownloadOutputDir()
	}

	cleanBase := filepath.Clean(baseDir)
	sub := strings.TrimSpace(r.URL.Query().Get("sub_path"))
	if sub == "" {
		sub = strings.TrimSpace(r.URL.Query().Get("path"))
	}
	if sub == "" || strings.Contains(sub, "\x00") {
		writeJSON(w, http.StatusBadRequest, false, "Invalid path")
		return
	}
	subClean := filepath.Clean(filepath.FromSlash(strings.TrimLeft(sub, "/\\")))
	if strings.Contains(subClean, "..") {
		writeJSON(w, http.StatusBadRequest, false, "Invalid path")
		return
	}
	fullPath := filepath.Clean(filepath.Join(cleanBase, subClean))
	rel, relErr := filepath.Rel(cleanBase, fullPath)
	if relErr != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		writeJSON(w, http.StatusBadRequest, false, "Invalid path")
		return
	}
	info, err := os.Lstat(fullPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		writeJSON(w, http.StatusNotFound, false, "File not found")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name()))
	http.ServeFile(w, r, fullPath)
}

func handleDownloadSettings(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	if r.Method == http.MethodGet {
		dlMu.Lock()
		outDir := dlConfig.OutputDirectory
		dlMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"output_directory": outDir,
		})
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var body struct {
			OutputDir string `json:"output_dir"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
		target := strings.TrimSpace(body.OutputDir)
		if target == "" {
			writeJSON(w, http.StatusBadRequest, false, "Đường dẫn thư mục không được để trống")
			return
		}
		target = filepath.Clean(target)
		_ = os.MkdirAll(target, 0755)
		dlMu.Lock()
		dlConfig.OutputDirectory = target
		saveDownloaderConfig()
		dlMu.Unlock()
		syncR3pDownloaderSettings()

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"saved":            true,
			"output_directory": target,
			"message":          fmt.Sprintf("Đã thay đổi thư mục lưu trữ thành '%s'!", target),
		})
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
}

// ─── 5. Access, SSH, DHCP & Admin Password ────────────────────────────────

// GET /api/v1/access
func handleAccessGet(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	dropbearMap := parseUCIConfigMap("dropbear")
	dhcpSections := parseUCISections("dhcp")

	sshPort := "22"
	sshPassAuth := true
	sshRootAuth := true
	for k, v := range dropbearMap {
		if strings.HasSuffix(k, ".Port") {
			sshPort = v
		} else if strings.HasSuffix(k, ".PasswordAuth") {
			sshPassAuth = v != "off" && v != "0"
		} else if strings.HasSuffix(k, ".RootPasswordAuth") {
			sshRootAuth = v != "off" && v != "0"
		}
	}

	dhcpLan := map[string]string{
		"start":     "100",
		"limit":     "150",
		"leasetime": "12h",
	}
	var staticLeases []map[string]string
	for _, s := range dhcpSections {
		if s.ID == "lan" {
			for k, v := range s.Options {
				dhcpLan[k] = v
			}
		} else if s.Type == "host" {
			staticLeases = append(staticLeases, map[string]string{
				"id":   s.ID,
				"name": s.Options["name"],
				"mac":  s.Options["mac"],
				"ip":   s.Options["ip"],
			})
		}
	}
	if staticLeases == nil {
		staticLeases = []map[string]string{}
	}

	leases := parseDHCPLeases("/tmp/dhcp.leases")
	if leases == nil {
		leases = []dhcpLease{}
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"ssh": map[string]interface{}{
			"port":               sshPort,
			"password_auth":      sshPassAuth,
			"root_password_auth": sshRootAuth,
		},
		"dhcp_lan":      dhcpLan,
		"static_leases": staticLeases,
		"active_leases": leases,
		"now_unix":      time.Now().Unix(),
		"ddns":          loadDDNSConfig(),
		"external_ip":   fetchPublicIPv4(),
	})
}

// PUT /api/v1/access/ssh
func handleAccessSSH(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Port             string `json:"port"`
		PasswordAuth     bool   `json:"password_auth"`
		RootPasswordAuth bool   `json:"root_password_auth"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	port, err := strconv.Atoi(body.Port)
	if err != nil || port < 1 || port > 65535 {
		port = 22
	}
	pa := "on"
	if !body.PasswordAuth {
		pa = "off"
	}
	rpa := "on"
	if !body.RootPasswordAuth {
		rpa = "off"
	}
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dropbear.@dropbear[0].Port=%d", port))
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dropbear.@dropbear[0].PasswordAuth=%s", pa))
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dropbear.@dropbear[0].RootPasswordAuth=%s", rpa))
	_, _ = runCmd(5, "uci", "commit", "dropbear")
	_, _ = runCmd(8, "/etc/init.d/dropbear", "reload")
	writeJSON(w, http.StatusOK, true, map[string]interface{}{"port": port, "saved": true})
}

// PUT /api/v1/access/dhcp
func handleAccessDHCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Start     string `json:"start"`
		Limit     string `json:"limit"`
		Leasetime string `json:"leasetime"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	start, err1 := strconv.Atoi(body.Start)
	limit, err2 := strconv.Atoi(body.Limit)
	if err1 != nil || err2 != nil || start < 2 || start > 250 || limit < 1 || limit > 250 {
		writeJSON(w, http.StatusBadRequest, false, "invalid DHCP start or limit range")
		return
	}
	lt := strings.TrimSpace(body.Leasetime)
	if !safeTokenKeyPattern.MatchString(lt) {
		lt = "12h"
	}
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.start=%d", start))
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.limit=%d", limit))
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.leasetime=%s", lt))
	_, _ = runCmd(5, "uci", "commit", "dhcp")
	_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
	writeJSON(w, http.StatusOK, true, map[string]interface{}{"saved": true})
}

// POST / DELETE /api/v1/access/static-lease
func handleAccessStaticLease(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
			MAC  string `json:"mac"`
			IP   string `json:"ip"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		if _, err := net.ParseMAC(body.MAC); err != nil || net.ParseIP(body.IP) == nil {
			writeJSON(w, http.StatusBadRequest, false, "valid MAC and IPv4 address are required")
			return
		}
		secID, err := runCmd(4, "uci", "add", "dhcp", "host")
		if err != nil || secID == "" {
			writeJSON(w, http.StatusInternalServerError, false, "failed to add static lease")
			return
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.%s.name=%s", secID, sanitizeFilename(body.Name)))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.%s.mac=%s", secID, strings.ToLower(strings.TrimSpace(body.MAC))))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.%s.ip=%s", secID, strings.TrimSpace(body.IP)))
		if out, err := runCmd(5, "uci", "commit", "dhcp"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit dhcp failed: "+out)
			return
		}
		_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
		writeJSON(w, http.StatusCreated, true, map[string]string{"id": secID})

	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			var body struct {
				ID string `json:"id"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
			id = strings.TrimSpace(body.ID)
		}
		if !safeTokenKeyPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, false, "invalid ID")
			return
		}
		secType, err := runCmd(3, "uci", "-q", "get", "dhcp."+id)
		if err != nil || strings.TrimSpace(secType) != "host" {
			writeJSON(w, http.StatusBadRequest, false, "section is not a DHCP static host lease")
			return
		}
		_, _ = runCmd(4, "uci", "delete", "dhcp."+id)
		if out, err := runCmd(5, "uci", "commit", "dhcp"); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "uci commit dhcp failed: "+out)
			return
		}
		_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
		writeJSON(w, http.StatusOK, true, map[string]string{"deleted": id})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/access/password
func handleAccessPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	pw := strings.TrimSpace(body.Password)
	if len(pw) < 4 || len(pw) > 64 || strings.ContainsAny(pw, "\r\n'\"\\") {
		writeJSON(w, http.StatusBadRequest, false, "password must be 4–64 safe characters")
		return
	}
	if strings.ContainsAny(pw, "\r\n\x00") {
		writeJSON(w, http.StatusBadRequest, false, "Mật khẩu không được chứa ký tự xuống dòng hoặc NUL")
		return
	}
	if _, err := os.Stat("/etc/shadow"); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "passwd", "root")
		cmd.Stdin = strings.NewReader(pw + "\n" + pw + "\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Không thể cập nhật mật khẩu root hệ thống: "+strings.TrimSpace(string(out))+" ("+err.Error()+")")
			return
		}
	}
	_ = os.MkdirAll("/etc/r3p", 0700)
	if err := os.WriteFile("/etc/r3p/api-token", []byte(pw+"\n"), 0600); err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Không thể lưu api-token: "+err.Error())
		return
	}
	if globalAuth != nil {
		globalAuth.updateToken(pw)
	}
	writeJSON(w, http.StatusOK, true, map[string]bool{"updated": true})
}

// ─── 6. Automation & Cron (/etc/crontabs/root) ────────────────────────────

const rootCrontabPath = "/etc/crontabs/root"

type CronEntry struct {
	Index       int    `json:"index"`
	Schedule    string `json:"schedule"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

func loadRootCrontab() []CronEntry {
	_ = os.MkdirAll("/etc/crontabs", 0755)
	lines := readFileLines(rootCrontabPath, 200)
	if len(lines) == 0 {
		defaultLines := []string{
			"0 4 * * * sync; echo 3 > /proc/sys/vm/drop_caches # Daily RAM cache flush",
			"*/30 * * * * /usr/sbin/ntpd -q -p pool.ntp.org # Periodic NTP time sync",
		}
		_ = os.WriteFile(rootCrontabPath, []byte(strings.Join(defaultLines, "\n")+"\n"), 0600)
		lines = defaultLines
	}
	var res []CronEntry
	for idx, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		enabled := true
		if strings.HasPrefix(line, "#DISABLED#") {
			enabled = false
			line = strings.TrimSpace(strings.TrimPrefix(line, "#DISABLED#"))
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		desc := "Scheduled command"
		cmdPart := line
		if cIdx := strings.Index(line, " # "); cIdx != -1 {
			desc = strings.TrimSpace(line[cIdx+3:])
			cmdPart = strings.TrimSpace(line[:cIdx])
		}
		fields := strings.Fields(cmdPart)
		if len(fields) < 6 {
			continue
		}
		sched := strings.Join(fields[:5], " ")
		cmd := strings.Join(fields[5:], " ")
		res = append(res, CronEntry{
			Index:       idx,
			Schedule:    sched,
			Command:     cmd,
			Description: desc,
			Enabled:     enabled,
		})
	}
	if res == nil {
		res = []CronEntry{}
	}
	return res
}

func saveRootCrontab(entries []CronEntry) {
	var lines []string
	for _, e := range entries {
		prefix := ""
		if !e.Enabled {
			prefix = "#DISABLED# "
		}
		desc := strings.TrimSpace(e.Description)
		if desc == "" {
			desc = "Scheduled task"
		}
		lines = append(lines, fmt.Sprintf("%s%s %s # %s", prefix, e.Schedule, e.Command, desc))
	}
	_ = os.MkdirAll("/etc/crontabs", 0755)
	_ = os.WriteFile(rootCrontabPath, []byte(strings.Join(lines, "\n")+"\n"), 0600)
	_, _ = runCmd(5, "/etc/init.d/cron", "reload")
}

var cronFieldPattern = regexp.MustCompile(`^[0-9*,/-]+$`)

func isValidCronSchedule(sched string) bool {
	if strings.ContainsAny(sched, "\r\n#") {
		return false
	}
	fields := strings.Fields(sched)
	if len(fields) != 5 {
		return false
	}
	for _, f := range fields {
		if !cronFieldPattern.MatchString(f) {
			return false
		}
	}
	return true
}

func runSafeSystemPreset(presetID string) (string, string, error) {
	switch presetID {
	case "flush_ram_cache", "ram_flush", "plugin-ram-optimizer":
		_, _ = runCmd(3, "sync")
		_ = os.WriteFile("/proc/sys/vm/drop_caches", []byte("3\n"), 0644)
		out, err := runCmd(5, "free", "-m")
		return "sync && drop_caches && free -m", out, err
	case "listening_sockets":
		out, err := runCmd(5, "netstat", "-tulnp")
		if err != nil && out == "" {
			out, err = runCmd(5, "ss", "-tuln")
		}
		return "netstat -tulnp", out, err
	case "reload_dnsmasq":
		out, err := runCmd(8, "/etc/init.d/dnsmasq", "reload")
		if err == nil && strings.TrimSpace(out) == "" {
			out = "dnsmasq reloaded cleanly"
		}
		return "/etc/init.d/dnsmasq reload", out, err
	case "dns_reload", "plugin-dns-adblock":
		_ = os.MkdirAll("/tmp/hosts", 0755)
		rules := "0.0.0.0 doubleclick.net\n0.0.0.0 googleadservices.com\n0.0.0.0 pagead2.googlesyndication.com\n"
		if werr := os.WriteFile("/tmp/hosts/r3p-adblock", []byte(rules), 0644); werr != nil {
			return "adblock hosts update", "", werr
		}
		out, err := runCmd(8, "/etc/init.d/dnsmasq", "reload")
		if err == nil {
			out = strings.TrimSpace("AdBlock hosts active (3 rules loaded)\n" + out)
		}
		return "adblock hosts + dnsmasq reload", out, err
	case "ntp_sync":
		out, err := runCmd(10, "/usr/sbin/ntpd", "-q", "-p", "pool.ntp.org")
		return "/usr/sbin/ntpd -q -p pool.ntp.org", out, err
	case "lan_probe", "plugin-wan-watchdog":
		out1, err1 := runCmd(5, "ip", "addr", "show", "br-lan")
		out2, _ := runCmd(5, "ip", "neigh", "show")
		return "ip addr show br-lan && ip neigh show", strings.TrimSpace(out1 + "\n" + out2), err1
	case "nand_health", "plugin-nand-health":
		var parts []string
		for _, k := range []string{"bad_peb_count", "max_ec", "avail_eraseblocks"} {
			b, err := os.ReadFile("/sys/class/ubi/ubi0/" + k)
			if err != nil {
				parts = append(parts, k+": N/A ("+err.Error()+")")
			} else {
				parts = append(parts, k+": "+strings.TrimSpace(string(b)))
			}
		}
		return "ubi0 health counters", strings.Join(parts, "\n"), nil
	case "disk_usage":
		out, err := runCmd(5, "df", "-h")
		return "df -h", out, err
	case "uptime_check":
		out, err := runCmd(4, "uptime")
		return "uptime", out, err
	default:
		return "", "", fmt.Errorf("unknown or unsupported execution preset %q", presetID)
	}
}

// GET / POST / PATCH / DELETE /api/v1/automation/cron
func handleAutomationCron(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	entries := loadRootCrontab()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"jobs":     entries,
			"telegram": telegramDownloader.statusSnapshot(),
		})

	case http.MethodPost:
		var body CronEntry
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		body.Schedule = strings.TrimSpace(body.Schedule)
		body.Command = strings.TrimSpace(body.Command)
		body.Description = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(body.Description), "\r", ""), "\n", "")
		if !isValidCronSchedule(body.Schedule) || body.Command == "" || strings.ContainsAny(body.Command, "\r\n") {
			writeJSON(w, http.StatusBadRequest, false, "valid 5-field cron schedule and single-line command are required")
			return
		}
		body.Enabled = true
		entries = append(entries, body)
		saveRootCrontab(entries)
		writeJSON(w, http.StatusCreated, true, map[string]interface{}{"jobs": loadRootCrontab()})

	case http.MethodPatch:
		var body struct {
			Index   int  `json:"index"`
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		matched := false
		for i := range entries {
			if entries[i].Index == body.Index {
				entries[i].Enabled = body.Enabled
				matched = true
			}
		}
		if !matched {
			writeJSON(w, http.StatusNotFound, false, "cron job index not found")
			return
		}
		saveRootCrontab(entries)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"jobs": loadRootCrontab()})

	case http.MethodDelete:
		idx, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid index")
			return
		}
		updated := []CronEntry{}
		for _, e := range entries {
			if e.Index != idx {
				updated = append(updated, e)
			}
		}
		saveRootCrontab(updated)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"jobs": loadRootCrontab()})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/automation/run
func handleAutomationRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Preset    string `json:"preset"`
		CronIndex *int   `json:"cron_index"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}

	preset := strings.TrimSpace(body.Preset)
	if preset == "" && body.CronIndex != nil {
		for _, entry := range loadRootCrontab() {
			if entry.Index != *body.CronIndex {
				continue
			}
			cmdLower := strings.ToLower(entry.Command)
			switch {
			case strings.Contains(cmdLower, "drop_caches"):
				preset = "flush_ram_cache"
			case strings.Contains(cmdLower, "ntpd"):
				preset = "ntp_sync"
			case strings.Contains(cmdLower, "dnsmasq") && strings.Contains(cmdLower, "reload"):
				preset = "reload_dnsmasq"
			default:
				parts := strings.Fields(entry.Command)
				if len(parts) > 0 && !strings.ContainsAny(entry.Command, ";|&><$`\r\n") &&
					(strings.HasPrefix(parts[0], "/usr/bin/") || strings.HasPrefix(parts[0], "/usr/sbin/") || strings.HasPrefix(parts[0], "/etc/init.d/")) {
					if st, serr := os.Stat(parts[0]); serr == nil && !st.IsDir() {
						out, err := runCmd(10, parts[0], parts[1:]...)
						errStr := ""
						if err != nil {
							errStr = err.Error()
						}
						writeJSON(w, http.StatusOK, true, map[string]interface{}{
							"command": entry.Command,
							"output":  out,
							"error":   errStr,
						})
						return
					}
				}
			}
			break
		}
	}

	if preset == "" {
		writeJSON(w, http.StatusBadRequest, false, "arbitrary shell execution is disabled; use a supported maintenance preset or verified binary cron task")
		return
	}
	cmdLabel, out, err := runSafeSystemPreset(preset)
	if strings.HasPrefix(errString(err), "unknown or unsupported") {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"command": cmdLabel,
		"output":  out,
		"error":   errString(err),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// POST /api/v1/telegram/test
func handleTelegramTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	token := strings.TrimSpace(body.BotToken)
	if token == "" {
		token = telegramDownloader.currentToken()
	}
	chatID := strings.TrimSpace(body.ChatID)
	if chatID == "" {
		chatID = telegramDownloader.statusSnapshot().ChatID
	}
	if err := validateTelegramConfig(token, chatID); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}

	hostname, _ := os.Hostname()
	msg := fmt.Sprintf("✅ R3P Home Server OS (%s) — Test notification at %s", hostname, time.Now().Format("15:04:05"))
	payload, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    msg,
	})
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, false, "Telegram request failed (check WAN internet access): "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Telegram API returned HTTP %d", resp.StatusCode))
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]string{"message": "Telegram test notification sent successfully!"})
}

// ─── 7. Applications, Services & Installed APK Packages ───────────────────

var serviceDescriptions = map[string]string{
	"dropbear":        "Lightweight SSH server for remote terminal & SCP",
	"dnsmasq":         "Local DNS caching resolver and DHCP lease server",
	"firewall":        "OpenWrt fw4 nftables packet filter & NAT engine",
	"network":         "Netifd network interface and DSA switch manager",
	"cron":            "Periodic scheduled task execution daemon",
	"sysntpd":         "Network Time Protocol (NTP) client/server",
	"odhcpd":          "IPv6 DHCPv6 & Router Advertisement daemon",
	"uhttpd":          "Secondary lightweight HTTP server",
	"wpad":            "IEEE 802.1X / WPA2 / WPA3 wireless authenticator",
	"ubihealthd":      "NAND flash wear-leveling and UBI health daemon",
	"r3p-api":         "R3P Home Server OS Core Backend & Web GUI",
	"r3p-aria2":       "Aria2 multi-protocol high-speed download daemon",
	"urngd":           "Hardware entropy & random number generator",
	"log":             "System ring buffer and kernel log daemon",
	"led":             "Hardware status LED controller",
	"packet_steering": "Multi-core network packet RPS/RFS steering",
}

// GET /api/v1/services/all
func handleServicesAll(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	type procDetail struct {
		PID   int
		MemKB uint64
	}
	procMap := map[string]procDetail{}
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}
			statusRaw := readFile("/proc/" + e.Name() + "/status")
			var name string
			var rss uint64
			for _, line := range strings.Split(statusRaw, "\n") {
				if strings.HasPrefix(line, "Name:") {
					name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
				} else if strings.HasPrefix(line, "VmRSS:") {
					f := strings.Fields(line)
					if len(f) >= 2 {
						rss, _ = strconv.ParseUint(f[1], 10, 64)
					}
				}
			}
			if name != "" {
				procMap[name] = procDetail{PID: pid, MemKB: rss}
			}
		}
	}

	type serviceItem struct {
		Name        string `json:"name"`
		Running     bool   `json:"running"`
		Enabled     bool   `json:"enabled"`
		PID         int    `json:"pid"`
		MemoryKB    uint64 `json:"memory_kb"`
		Description string `json:"description"`
	}

	initEntries, _ := os.ReadDir("/etc/init.d")
	var list []serviceItem
	for _, ie := range initEntries {
		svc := ie.Name()
		if svc == "boot" || svc == "done" || svc == "umount" || svc == "sysfixtime" || svc == "sysctl" || svc == "bootcount" || svc == "gpio_switch" {
			continue
		}
		p, ok := procMap[svc]
		if !ok {
			switch svc {
			case "cron":
				p, ok = procMap["crond"]
			case "sysntpd":
				p, ok = procMap["ntpd"]
			case "log":
				p, ok = procMap["logd"]
			case "r3p-aria2":
				p, ok = procMap["aria2c"]
			case "network":
				p, ok = procMap["netifd"]
			case "wpad":
				p, ok = procMap["hostapd"]
			}
		}
		// firewall and led are oneshot active services if enabled
		running := ok
		if !running && (svc == "firewall" || svc == "led" || svc == "packet_steering") {
			if m, _ := filepath.Glob("/etc/rc.d/S??" + svc); len(m) > 0 {
				running = true
			}
		}
		enabledMatches, _ := filepath.Glob("/etc/rc.d/S??" + svc)
		desc := serviceDescriptions[svc]
		if desc == "" {
			desc = "OpenWrt procd system service"
		}
		list = append(list, serviceItem{
			Name:        svc,
			Running:     running,
			Enabled:     len(enabledMatches) > 0,
			PID:         p.PID,
			MemoryKB:    p.MemKB,
			Description: desc,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Running != list[j].Running {
			return list[i].Running
		}
		return list[i].Name < list[j].Name
	})

	writeJSON(w, http.StatusOK, true, map[string]interface{}{"services": list})
}

// POST /api/v1/services/control
func handleServicesControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Service string `json:"service"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	if !safeTokenKeyPattern.MatchString(body.Service) {
		writeJSON(w, http.StatusBadRequest, false, "invalid service name")
		return
	}
	if body.Service == "r3p-api" && (body.Action == "stop" || body.Action == "disable") {
		writeJSON(w, http.StatusBadRequest, false, "cannot stop or disable r3p-api from the web UI")
		return
	}
	switch body.Action {
	case "start", "stop", "restart", "reload", "enable", "disable":
	default:
		writeJSON(w, http.StatusBadRequest, false, "unsupported action")
		return
	}
	initPath := "/etc/init.d/" + body.Service
	if _, err := os.Stat(initPath); err != nil {
		writeJSON(w, http.StatusNotFound, false, "init script not found")
		return
	}
	out, err := runCmd(10, initPath, body.Action)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"service": body.Service,
		"action":  body.Action,
		"output":  out,
		"error":   errStr,
	})
}

// GET /api/v1/packages
func handlePackagesGet(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	out, err := runCmd(6, "apk", "info", "-v")
	if err != nil || out == "" {
		out, _ = runCmd(6, "opkg", "list-installed")
	}
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "WARNING:") {
			pkgs = append(pkgs, t)
		}
	}
	sort.Strings(pkgs)
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"count":    len(pkgs),
		"packages": pkgs,
	})
}

// ─── 8. IoT Device Registry & LAN Discovery ───────────────────────────────

type IoTDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Protocol string `json:"protocol"` // ICMP, HTTP, MQTT, TCP
	Topic    string `json:"topic"`
	Online   bool   `json:"online"`
	Latency  string `json:"latency"`
	LastSeen string `json:"last_seen"`
}

type IoTBrokerConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	TopicPrefix string `json:"topic_prefix"`
	Enabled     bool   `json:"enabled"`
}

const iotDevicesFile = "/etc/r3p/iot_devices.json"
const iotBrokerFile = "/etc/r3p/iot_broker.json"

func macShortID(mac string) string {
	hexOnly := strings.ReplaceAll(strings.TrimSpace(mac), ":", "")
	if len(hexOnly) >= 6 {
		return hexOnly[len(hexOnly)-6:]
	}
	if hexOnly != "" {
		return hexOnly
	}
	return generateGID()[:6]
}

func loadIoTDevices() []IoTDevice {
	data, err := os.ReadFile(iotDevicesFile)
	if err == nil {
		var list []IoTDevice
		if json.Unmarshal(data, &list) == nil {
			if list == nil {
				return []IoTDevice{}
			}
			return list
		}
	}
	// Auto-seed from current DHCP leases
	leases := parseDHCPLeases("/tmp/dhcp.leases")
	seeded := []IoTDevice{}
	for _, l := range leases {
		seeded = append(seeded, IoTDevice{
			ID:       "dev-" + macShortID(l.MAC),
			Name:     l.Hostname,
			IP:       l.IP,
			MAC:      l.MAC,
			Protocol: "ICMP / LAN",
			Topic:    "r3p/lan/" + l.Hostname,
			Online:   true,
			Latency:  "< 1 ms",
			LastSeen: time.Now().Format("15:04:05"),
		})
	}
	saveIoTDevices(seeded)
	return seeded
}

func saveIoTDevices(list []IoTDevice) {
	if list == nil {
		list = []IoTDevice{}
	}
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(iotDevicesFile, b, 0644)
	}
}

func loadIoTBroker() IoTBrokerConfig {
	data, err := os.ReadFile(iotBrokerFile)
	if err == nil {
		var cfg IoTBrokerConfig
		if json.Unmarshal(data, &cfg) == nil && cfg.Host != "" {
			return cfg
		}
	}
	return IoTBrokerConfig{
		Host:        getRouterLanIP(),
		Port:        1883,
		TopicPrefix: "r3p/home",
		Enabled:     true,
	}
}

func probeIoTDevice(ip, protocol string, brokerPort int) (bool, string) {
	protoUpper := strings.ToUpper(strings.TrimSpace(protocol))
	if strings.Contains(protoUpper, "MQTT") || strings.Contains(protoUpper, "TCP") || strings.Contains(protoUpper, "HTTP") {
		port := 1883
		if strings.Contains(protoUpper, "HTTP") {
			port = 80
		} else if brokerPort > 0 && brokerPort <= 65535 {
			port = brokerPort
		}
		start := time.Now()
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), 1200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			ms := time.Since(start).Milliseconds()
			if ms < 1 {
				return true, fmt.Sprintf("< 1 ms (:%d)", port)
			}
			return true, fmt.Sprintf("%d ms (:%d)", ms, port)
		}
		if protoUpper == "MQTT" || protoUpper == "TCP" || protoUpper == "HTTP" {
			return false, "Timeout"
		}
	}
	start := time.Now()
	_, err := runCmd(2, "ping", "-c", "1", "-W", "1", ip)
	if err != nil {
		return false, "Timeout"
	}
	ms := time.Since(start).Milliseconds()
	if ms < 1 {
		return true, "< 1 ms"
	}
	return true, fmt.Sprintf("%d ms", ms)
}

// GET / POST / PUT / DELETE /api/v1/iot
func handleIoT(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	devices := loadIoTDevices()
	broker := loadIoTBroker()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"devices": devices,
			"broker":  broker,
		})

	case http.MethodPost:
		var body IoTDevice
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		body.IP = strings.TrimSpace(body.IP)
		if body.Name == "" || net.ParseIP(body.IP) == nil {
			writeJSON(w, http.StatusBadRequest, false, "device name and valid IP address are required")
			return
		}
		if body.ID == "" {
			body.ID = "iot-" + generateGID()[:6]
		}
		if body.Protocol == "" {
			body.Protocol = "MQTT / ICMP"
		}
		if body.Topic == "" {
			body.Topic = broker.TopicPrefix + "/" + sanitizeFilename(body.Name)
		}
		online, latency := probeIoTDevice(body.IP, body.Protocol, broker.Port)
		body.Online = online
		if online {
			body.Latency = latency
			body.LastSeen = time.Now().Format("15:04:05")
		} else {
			body.Latency = "Offline"
		}
		devices = append(devices, body)
		saveIoTDevices(devices)
		writeJSON(w, http.StatusCreated, true, map[string]interface{}{"devices": devices})

	case http.MethodPut:
		var cfg IoTBrokerConfig
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid broker JSON")
			return
		}
		cfg.Host = strings.TrimSpace(cfg.Host)
		if cfg.Host == "" || strings.ContainsAny(cfg.Host+cfg.Username+cfg.TopicPrefix, "\r\n\x00") || cfg.Port < 1 || cfg.Port > 65535 {
			writeJSON(w, http.StatusBadRequest, false, "invalid broker host or port (1-65535)")
			return
		}
		if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.MkdirAll("/etc/r3p", 0755)
			_ = os.WriteFile(iotBrokerFile, b, 0644)
		}
		writeJSON(w, http.StatusOK, true, cfg)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		updated := []IoTDevice{}
		for _, d := range devices {
			if d.ID != id {
				updated = append(updated, d)
			}
		}
		saveIoTDevices(updated)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"devices": updated})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/iot/ping
func handleIoTPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	devices := loadIoTDevices()
	broker := loadIoTBroker()
	for i := range devices {
		if net.ParseIP(devices[i].IP) == nil {
			continue
		}
		online, latency := probeIoTDevice(devices[i].IP, devices[i].Protocol, broker.Port)
		devices[i].Online = online
		devices[i].Latency = latency
		if online {
			devices[i].LastSeen = time.Now().Format("15:04:05")
		}
	}
	saveIoTDevices(devices)
	writeJSON(w, http.StatusOK, true, map[string]interface{}{"devices": devices})
}

// ─── 9. Plugins & Extensions System ───────────────────────────────────────

type PluginManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Command     string `json:"command"`
	LastOutput  string `json:"last_output"`
	LastRun     string `json:"last_run"`
}

const pluginsFile = "/etc/r3p/plugins.json"

func loadPlugins() []PluginManifest {
	data, err := os.ReadFile(pluginsFile)
	if err == nil {
		var list []PluginManifest
		if json.Unmarshal(data, &list) == nil && len(list) > 0 {
			return list
		}
	}
	defaults := []PluginManifest{
		{
			ID:          "plugin-dns-adblock",
			Name:        "DNS AdBlock Shield (dnsmasq)",
			Version:     "1.2.0",
			Author:      "R3P Core",
			Description: "Blocks telemetry & ad domains via /tmp/hosts/r3p-adblock and reloads dnsmasq",
			Enabled:     true,
			Command:     "dns_reload",
		},
		{
			ID:          "plugin-ram-optimizer",
			Name:        "Memory Cache Optimizer",
			Version:     "1.0.4",
			Author:      "R3P Core",
			Description: "Flushes reclaimable pagecache, dentries and inodes on MT7621",
			Enabled:     true,
			Command:     "ram_flush",
		},
		{
			ID:          "plugin-wan-watchdog",
			Name:        "Gateway & LAN Connectivity Probe",
			Version:     "1.1.0",
			Author:      "R3P Core",
			Description: "Verifies local bridge and ARP neighbor reachability",
			Enabled:     true,
			Command:     "lan_probe",
		},
		{
			ID:          "plugin-nand-health",
			Name:        "ESMT SLC NAND UBI Inspector",
			Version:     "1.0.2",
			Author:      "R3P Core",
			Description: "Reads UBI volume wear-leveling and bad block counters from /sys/class/ubi/ubi0",
			Enabled:     true,
			Command:     "nand_health",
		},
	}
	savePlugins(defaults)
	return defaults
}

var allowedPluginPresets = map[string]bool{
	"ram_flush":         true,
	"flush_ram_cache":   true,
	"lan_probe":         true,
	"nand_health":       true,
	"dns_reload":        true,
	"reload_dnsmasq":    true,
	"disk_usage":        true,
	"uptime_check":      true,
	"listening_sockets": true,
	"ntp_sync":          true,
}

func savePlugins(list []PluginManifest) {
	if list == nil {
		list = []PluginManifest{}
	}
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(pluginsFile, b, 0644)
	}
}

// GET / POST /api/v1/plugins
func handlePlugins(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	plugins := loadPlugins()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"plugins": plugins})

	case http.MethodPost:
		var body struct {
			Action      string `json:"action"` // toggle, run, create, delete
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Command     string `json:"command"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		switch body.Action {
		case "toggle":
			found := false
			for i := range plugins {
				if plugins[i].ID == body.ID {
					plugins[i].Enabled = !plugins[i].Enabled
					found = true
				}
			}
			if !found {
				writeJSON(w, http.StatusNotFound, false, "plugin not found")
				return
			}
			savePlugins(plugins)
		case "run":
			found := false
			for i := range plugins {
				if plugins[i].ID != body.ID {
					continue
				}
				found = true
				if !plugins[i].Enabled {
					writeJSON(w, http.StatusBadRequest, false, "plugin is disabled; enable it before running")
					return
				}
				presetKey := strings.TrimSpace(plugins[i].Command)
				if !allowedPluginPresets[presetKey] {
					presetKey = plugins[i].ID
				}
				_, out, err := runSafeSystemPreset(presetKey)
				if err != nil {
					if strings.HasPrefix(err.Error(), "unknown or unsupported") {
						writeJSON(w, http.StatusBadRequest, false, "plugin uses unsupported shell execution; recreate using a safe preset hook")
						return
					}
					if out == "" {
						out = err.Error()
					}
				}
				plugins[i].LastOutput = out
				plugins[i].LastRun = time.Now().Format("15:04:05")
			}
			if !found {
				writeJSON(w, http.StatusNotFound, false, "plugin not found")
				return
			}
			savePlugins(plugins)
		case "create":
			body.Name = strings.TrimSpace(body.Name)
			body.Command = strings.TrimSpace(body.Command)
			if body.Name == "" || !allowedPluginPresets[body.Command] {
				writeJSON(w, http.StatusBadRequest, false, "name is required and command must be a supported safe preset hook")
				return
			}
			plugins = append(plugins, PluginManifest{
				ID:          "plugin-" + generateGID()[:6],
				Name:        body.Name,
				Version:     "1.0.0",
				Author:      "Admin",
				Description: strings.TrimSpace(body.Description),
				Enabled:     true,
				Command:     body.Command,
			})
			savePlugins(plugins)
		case "delete":
			updated := []PluginManifest{}
			for _, p := range plugins {
				if p.ID != body.ID {
					updated = append(updated, p)
				}
			}
			plugins = updated
			savePlugins(plugins)
		default:
			writeJSON(w, http.StatusBadRequest, false, "unsupported plugin action")
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"plugins": plugins})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// ─── 10. System Backup, Reboot, Settings & Cache Flush ────────────────────

// GET /api/v1/system/backup
func handleSystemBackup(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	// Guarantee /etc/r3p is preserved in sysupgrade.conf
	if rawConf, err := os.ReadFile("/etc/sysupgrade.conf"); err == nil || os.IsNotExist(err) {
		lines := strings.Split(string(rawConf), "\n")
		hasR3p := false
		for _, ln := range lines {
			if strings.TrimSpace(ln) == "/etc/r3p" {
				hasR3p = true
				break
			}
		}
		if !hasR3p {
			content := strings.TrimRight(string(rawConf), "\r\n")
			if content != "" {
				content += "\n"
			}
			content += "/etc/r3p\n"
			_ = os.WriteFile("/etc/sysupgrade.conf", []byte(content), 0644)
		}
	}

	tmpF, err := os.CreateTemp("", "r3p-backup-*.tar.gz")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Không thể tạo file tạm sao lưu: "+err.Error())
		return
	}
	backupPath := tmpF.Name()
	tmpF.Close()
	defer os.Remove(backupPath)

	if _, err := runCmd(15, "sysupgrade", "-b", backupPath); err != nil {
		// Fallback tar of /etc/config, /etc/r3p, /etc/crontabs, /etc/passwd, /etc/shadow, /etc/dropbear
		if out, tarErr := runCmd(10, "tar", "-czf", backupPath, "/etc/config", "/etc/r3p", "/etc/crontabs", "/etc/passwd", "/etc/shadow", "/etc/dropbear"); tarErr != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Không thể tạo gói sao lưu cấu hình: "+out)
			return
		}
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"r3p-backup.tar.gz\"")
	http.ServeFile(w, r, backupPath)
}

// POST /api/v1/system/restore
func handleSystemRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}

	tmpRestore, err := os.CreateTemp("", "r3p-restore-*.tar.gz")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Không thể tạo file tạm /tmp: "+err.Error())
		return
	}
	restorePath := tmpRestore.Name()
	tmpRestore.Close()
	defer os.Remove(restorePath)

	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)

	var origFilename string
	var doReboot bool

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Không thể đọc file backup: "+err.Error())
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, "Vui lòng chọn file backup (.tar.gz)")
			return
		}
		defer file.Close()
		origFilename = filepath.Base(header.Filename)
		doReboot = r.FormValue("reboot") == "true" || r.FormValue("reboot") == "1"

		out, err := os.OpenFile(restorePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Không thể tạo file tạm /tmp: "+err.Error())
			return
		}
		_, copyErr := io.Copy(out, file)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi khi lưu file backup")
			return
		}
	} else {
		origFilename = filepath.Base(r.Header.Get("X-Filename"))
		if origFilename == "" || origFilename == "." {
			origFilename = r.URL.Query().Get("filename")
		}
		if origFilename == "" {
			origFilename = "r3p-backup.tar.gz"
		} else {
			origFilename = filepath.Base(origFilename)
		}
		doReboot = r.URL.Query().Get("reboot") == "true" || r.URL.Query().Get("reboot") == "1"

		out, err := os.OpenFile(restorePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Không thể tạo file tạm /tmp: "+err.Error())
			return
		}
		_, copyErr := io.Copy(out, r.Body)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Lỗi khi nhận dữ liệu file backup")
			return
		}
	}

	// Verify tar archive safety in pure Go (rejects path traversal, devices, hardlinks, unsafe symlinks)
	if _, safetyErr := inspectTarGzSafety(restorePath, true, nil); safetyErr != nil {
		writeJSON(w, http.StatusBadRequest, false, "File tải lên không hợp lệ hoặc không an toàn: "+safetyErr.Error())
		return
	}

	// Extract to /
	extractOut, extractErr := runCmd(15, "tar", "-xzf", restorePath, "-C", "/")
	if extractErr != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Lỗi khi giải nén khôi phục cấu hình: "+extractOut)
		return
	}

	if doReboot {
		go func() {
			time.Sleep(1 * time.Second)
			_, _ = runCmd(5, "sync")
			_, _ = runCmd(5, "reboot")
		}()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message":  "Đã khôi phục cấu hình thành công từ " + origFilename + "! Router đang khởi động lại trong 45 giây...",
			"reboot":   true,
			"filename": origFilename,
		})
	} else {
		go func() {
			time.Sleep(500 * time.Millisecond)
			_, _ = runCmd(10, "/etc/init.d/network", "reload")
			_, _ = runCmd(10, "wifi", "reload")
			_, _ = runCmd(10, "/etc/init.d/firewall", "restart")
			_, _ = runCmd(10, "/etc/init.d/dnsmasq", "restart")
			_, _ = runCmd(10, "/etc/init.d/r3p-api", "restart")
		}()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"message":  "Đã khôi phục cấu hình thành công từ " + origFilename + "! Các dịch vụ đang được áp dụng lại.",
			"reboot":   false,
			"filename": origFilename,
		})
	}
}

// POST /api/v1/system/reboot
func handleSystemReboot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]string{"status": "rebooting"})
	go func() {
		time.Sleep(500 * time.Millisecond)
		_, _ = runCmd(5, "sync")
		_, _ = runCmd(5, "reboot")
	}()
}

// PUT /api/v1/system/settings
func handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	var body struct {
		Hostname string `json:"hostname"`
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	hn := strings.TrimSpace(body.Hostname)
	if !safeHostPattern.MatchString(hn) {
		writeJSON(w, http.StatusBadRequest, false, "invalid hostname")
		return
	}
	_, _ = runCmd(4, "uci", "set", "system.@system[0].hostname="+hn)
	if body.Timezone != "" && !strings.ContainsAny(body.Timezone, "\r\n'\"\\") {
		tz := strings.TrimSpace(body.Timezone)
		_, _ = runCmd(4, "uci", "set", "system.@system[0].timezone="+tz)
		if tz == "ICT-7" {
			_, _ = runCmd(4, "uci", "set", "system.@system[0].zonename=Asia/Ho_Chi_Minh")
		}
		_ = os.WriteFile("/tmp/TZ", []byte(tz+"\n"), 0644)
		_ = os.WriteFile("/etc/TZ", []byte(tz+"\n"), 0644)
	}
	_, _ = runCmd(5, "uci", "commit", "system")
	_ = os.WriteFile("/proc/sys/kernel/hostname", []byte(hn+"\n"), 0644)
	_, _ = runCmd(5, "/etc/init.d/system", "reload")
	writeJSON(w, http.StatusOK, true, map[string]string{"hostname": hn})
}

// POST /api/v1/system/drop-caches
func handleSystemDropCaches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	_, _ = runCmd(4, "sync")
	_ = os.WriteFile("/proc/sys/vm/drop_caches", []byte("3\n"), 0644)
	writeJSON(w, http.StatusOK, true, map[string]bool{"flushed": true})
}

// POST /api/v1/system/logs/clear or DELETE /api/v1/logs
func handleSystemLogsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	// Clear both kernel dmesg ringbuffer and restart logd / syslog daemon
	_, _ = runCmd(5, "sh", "-c", "/etc/init.d/log restart; dmesg -c >/dev/null 2>&1")
	writeJSON(w, http.StatusOK, true, map[string]string{
		"message": "Đã xóa sạch bộ đệm nhật ký hệ thống (logread & dmesg) thành công!",
	})
}

// ─── 11. UCI Config File Editor (/etc/config/*) ───────────────────────────

var allowedConfigFiles = map[string]bool{
	"network":    true,
	"wireless":   true,
	"system":     true,
	"dropbear":   true,
	"dhcp":       true,
	"firewall":   true,
	"uhttpd":     true,
	"ubihealthd": true,
}

// GET / PUT /api/v1/config
func handleUCIConfig(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		name := r.URL.Query().Get("file")
		if name == "" {
			name = "network"
		}
		if !allowedConfigFiles[name] {
			writeJSON(w, http.StatusBadRequest, false, "unsupported config file")
			return
		}
		content := readFile("/etc/config/" + name)
		var available []string
		for k := range allowedConfigFiles {
			available = append(available, k)
		}
		sort.Strings(available)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"file":      name,
			"content":   content,
			"available": available,
		})

	case http.MethodPut:
		var body struct {
			File    string `json:"file"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		if !allowedConfigFiles[body.File] {
			writeJSON(w, http.StatusBadRequest, false, "unsupported config file")
			return
		}
		if strings.TrimSpace(body.Content) == "" {
			writeJSON(w, http.StatusBadRequest, false, "config content cannot be empty")
			return
		}
		targetPath := "/etc/config/" + body.File
		content := strings.ReplaceAll(body.Content, "\r\n", "\n")
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}

		// Validate candidate UCI syntax inside isolated temporary config directory before replacing live /etc/config/*
		tmpDir, err := os.MkdirTemp("", "r3p-ucicheck-*")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "failed to create temp validation dir: "+err.Error())
			return
		}
		defer os.RemoveAll(tmpDir)
		tmpCfg := filepath.Join(tmpDir, body.File)
		if err := os.WriteFile(tmpCfg, []byte(content), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "failed to write temp config: "+err.Error())
			return
		}
		if parseOut, parseErr := runCmd(4, "uci", "-c", tmpDir, "show", body.File); parseErr != nil {
			writeJSON(w, http.StatusBadRequest, false, "Lỗi cú pháp UCI ("+body.File+"): "+strings.TrimSpace(parseOut))
			return
		}

		_ = os.MkdirAll("/etc/r3p/backups", 0700)
		oldBytes, oldErr := os.ReadFile(targetPath)
		if oldErr == nil {
			_ = os.WriteFile("/etc/r3p/backups/"+body.File+".bak", oldBytes, 0600)
		}
		perm := os.FileMode(0644)
		if st, stErr := os.Stat(targetPath); stErr == nil {
			perm = st.Mode().Perm()
		}
		if err := os.WriteFile(targetPath, []byte(content), perm); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "failed to write config: "+err.Error())
			return
		}
		if out, err := runCmd(5, "uci", "commit", body.File); err != nil {
			if oldErr == nil {
				_ = os.WriteFile(targetPath, oldBytes, perm)
			}
			writeJSON(w, http.StatusInternalServerError, false, "uci commit failed (rolled back): "+strings.TrimSpace(out))
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]string{
			"file":    body.File,
			"message": fmt.Sprintf("/etc/config/%s saved and committed cleanly.", body.File),
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}
