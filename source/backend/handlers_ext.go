package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var safeHostPattern = regexp.MustCompile(`^[a-zA-Z0-9.-]{1,128}$`)
var safeTokenKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

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
						Signal:   "-60 dBm",
						TxRate:   "144 Mbps",
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
		if name == "" || body.SrcDport == "" || body.DestIP == "" {
			writeJSON(w, http.StatusBadRequest, false, "name, external port, and internal IP are required")
			return
		}
		if net.ParseIP(body.DestIP) == nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid destination IP address")
			return
		}
		if body.DestPort == "" {
			body.DestPort = body.SrcDport
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
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.src_dport=%s", secID, body.SrcDport))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest_ip=%s", secID, body.DestIP))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest_port=%s", secID, body.DestPort))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.target=DNAT", secID))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=1", secID))
		_, _ = runCmd(5, "uci", "commit", "firewall")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
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
		val := "0"
		if body.Enabled {
			val = "1"
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=%s", body.ID, val))
		_, _ = runCmd(5, "uci", "commit", "firewall")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"id": body.ID, "enabled": body.Enabled})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if !safeTokenKeyPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, false, "invalid rule ID")
			return
		}
		_, _ = runCmd(4, "uci", "delete", "firewall."+id)
		_, _ = runCmd(5, "uci", "commit", "firewall")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
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
		if name == "" {
			writeJSON(w, http.StatusBadRequest, false, "rule name is required")
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
		if body.DestPort != "" {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.dest_port=%s", secID, body.DestPort))
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.target=%s", secID, target))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=1", secID))
		_, _ = runCmd(5, "uci", "commit", "firewall")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
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
		val := "0"
		if body.Enabled {
			val = "1"
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("firewall.%s.enabled=%s", body.ID, val))
		_, _ = runCmd(5, "uci", "commit", "firewall")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"id": body.ID, "enabled": body.Enabled})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if !safeTokenKeyPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, false, "invalid rule ID")
			return
		}
		_, _ = runCmd(4, "uci", "delete", "firewall."+id)
		_, _ = runCmd(5, "uci", "commit", "firewall")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
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
			{Name: "ESP32_Downloads", Path: "/mnt/sda1/esp32-downloads", ReadOnly: false, GuestOk: true, Comment: "ESP32 Auto-Downloader Media Output"},
			{Name: "USB_Downloads", Path: "/mnt/sda1/downloads", ReadOnly: false, GuestOk: true, Comment: "USB 3.0 High-Speed Downloads"},
			{Name: "NAND_Share", Path: "/overlay/share", ReadOnly: false, GuestOk: true, Comment: "Persistent NAND Shared Storage"},
			{Name: "Downloads", Path: "/overlay/downloads", ReadOnly: false, GuestOk: true, Comment: "NAND Download Folder"},
			{Name: "RAM_Scratch", Path: "/tmp/downloads", ReadOnly: false, GuestOk: true, Comment: "High-Speed RAM Scratch Disk"},
		}
		saveNASShares(list)
		return list
	}
	hasUSB := false
	for _, s := range list {
		if strings.HasPrefix(s.Path, "/mnt/sda1") {
			hasUSB = true
			break
		}
	}
	if !hasUSB {
		if st, err := os.Stat("/mnt/sda1"); err == nil && st.IsDir() {
			list = append([]ShareItem{
				{Name: "USB_500G_HDD", Path: "/mnt/sda1", ReadOnly: false, GuestOk: true, Comment: "USB 3.0 500GB External HDD Root"},
				{Name: "ESP32_Downloads", Path: "/mnt/sda1/esp32-downloads", ReadOnly: false, GuestOk: true, Comment: "ESP32 Auto-Downloader Media Output"},
				{Name: "USB_Downloads", Path: "/mnt/sda1/downloads", ReadOnly: false, GuestOk: true, Comment: "USB 3.0 High-Speed Downloads"},
			}, list...)
			saveNASShares(list)
		}
	}
	return list
}

func saveNASShares(list []ShareItem) {
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(sharesConfigFile, b, 0644)
	}
}

func validateNASPath(rawDir string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(rawDir))
	if strings.HasPrefix(clean, "/overlay/share") ||
		strings.HasPrefix(clean, "/overlay/downloads") ||
		strings.HasPrefix(clean, "/tmp/downloads") ||
		strings.HasPrefix(clean, "/mnt/") ||
		strings.HasPrefix(clean, "/media/") {
		_ = os.MkdirAll(clean, 0755)
		return clean, nil
	}
	return "", fmt.Errorf("path must be inside /overlay/share, /overlay/downloads, /tmp/downloads, or /mnt/")
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
			if mp == "/" || mp == "/dev" {
				continue
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
				label = "Internal UBI NAND Storage (/overlay)"
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
	})
}

// POST /api/v1/nas/mkdir
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
	folderName := sanitizeFilename(body.Name)
	target := filepath.Join(baseDir, folderName)
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
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, "missing file")
		return
	}
	defer file.Close()

	safeName := sanitizeFilename(header.Filename)
	destPath := filepath.Join(baseDir, safeName)
	out, err := os.Create(destPath)
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
	rawPath := r.URL.Query().Get("path")
	clean := filepath.Clean(rawPath)
	if clean == "/overlay/share" || clean == "/overlay/downloads" || clean == "/tmp/downloads" {
		writeJSON(w, http.StatusBadRequest, false, "cannot delete root share folder")
		return
	}
	if _, err := validateNASPath(filepath.Dir(clean)); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
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
	rawPath := r.URL.Query().Get("path")
	clean := filepath.Clean(rawPath)
	if _, err := validateNASPath(filepath.Dir(clean)); err != nil {
		writeJSON(w, http.StatusBadRequest, false, err.Error())
		return
	}
	info, err := os.Stat(clean)
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, false, "file not found")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name()))
	http.ServeFile(w, r, clean)
}

// POST / DELETE /api/v1/nas/shares
func handleNASShares(w http.ResponseWriter, r *http.Request) {
	shares := loadNASShares()
	switch r.Method {
	case http.MethodPost:
		var body ShareItem
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		body.Name = sanitizeFilename(body.Name)
		validPath, err := validateNASPath(body.Path)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		body.Path = validPath
		shares = append(shares, body)
		saveNASShares(shares)
		writeJSON(w, http.StatusCreated, true, shares)

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		var updated []ShareItem
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
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.%s.mac=%s", secID, strings.ToLower(body.MAC)))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.%s.ip=%s", secID, body.IP))
		_, _ = runCmd(5, "uci", "commit", "dhcp")
		_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
		writeJSON(w, http.StatusCreated, true, map[string]string{"id": secID})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if !safeTokenKeyPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, false, "invalid ID")
			return
		}
		_, _ = runCmd(4, "uci", "delete", "dhcp."+id)
		_, _ = runCmd(5, "uci", "commit", "dhcp")
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
	_ = os.MkdirAll("/etc/r3p", 0700)
	_ = os.WriteFile("/etc/r3p/api-token", []byte(pw+"\n"), 0600)
	if globalAuth != nil {
		globalAuth.updateToken(pw)
	}
	_, _ = runCmd(5, "sh", "-c", fmt.Sprintf("printf '%%s\\n%%s\\n' '%s' '%s' | passwd root", pw, pw))
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
		if len(strings.Fields(body.Schedule)) != 5 || strings.TrimSpace(body.Command) == "" {
			writeJSON(w, http.StatusBadRequest, false, "schedule must have 5 cron fields and command is required")
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
		for i := range entries {
			if entries[i].Index == body.Index || i == body.Index {
				entries[i].Enabled = body.Enabled
			}
		}
		saveRootCrontab(entries)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"jobs": loadRootCrontab()})

	case http.MethodDelete:
		idx, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid index")
			return
		}
		var updated []CronEntry
		for i, e := range entries {
			if e.Index != idx && i != idx {
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
		Command string `json:"command"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	cmd := strings.TrimSpace(body.Command)
	if cmd == "" {
		writeJSON(w, http.StatusBadRequest, false, "empty command")
		return
	}
	out, err := runCmd(10, "sh", "-c", cmd)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"command": cmd,
		"output":  out,
		"error":   errStr,
	})
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
	if body.Service == "r3p-api" && body.Action == "stop" {
		writeJSON(w, http.StatusBadRequest, false, "cannot stop r3p-api from the web UI")
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

func loadIoTDevices() []IoTDevice {
	data, err := os.ReadFile(iotDevicesFile)
	if err == nil {
		var list []IoTDevice
		if json.Unmarshal(data, &list) == nil {
			return list
		}
	}
	// Auto-seed from current DHCP leases
	leases := parseDHCPLeases("/tmp/dhcp.leases")
	var seeded []IoTDevice
	for _, l := range leases {
		seeded = append(seeded, IoTDevice{
			ID:       "dev-" + strings.ReplaceAll(l.MAC, ":", "")[6:],
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
	if seeded == nil {
		seeded = []IoTDevice{}
	}
	saveIoTDevices(seeded)
	return seeded
}

func saveIoTDevices(list []IoTDevice) {
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
		Host:        "192.168.2.1",
		Port:        1883,
		TopicPrefix: "r3p/home",
		Enabled:     true,
	}
}

// GET / POST / DELETE /api/v1/iot
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
		if body.Name == "" || net.ParseIP(body.IP) == nil {
			writeJSON(w, http.StatusBadRequest, false, "device name and valid IPv4 address are required")
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
		// Probe immediately
		start := time.Now()
		_, err := runCmd(2, "ping", "-c", "1", "-W", "1", body.IP)
		body.Online = err == nil
		if body.Online {
			body.Latency = fmt.Sprintf("%d ms", time.Since(start).Milliseconds())
		} else {
			body.Latency = "Offline"
		}
		body.LastSeen = time.Now().Format("15:04:05")
		devices = append(devices, body)
		saveIoTDevices(devices)
		writeJSON(w, http.StatusCreated, true, map[string]interface{}{"devices": devices})

	case http.MethodPut:
		var cfg IoTBrokerConfig
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid broker JSON")
			return
		}
		if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.WriteFile(iotBrokerFile, b, 0644)
		}
		writeJSON(w, http.StatusOK, true, cfg)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		var updated []IoTDevice
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
	for i := range devices {
		if net.ParseIP(devices[i].IP) == nil {
			continue
		}
		start := time.Now()
		_, err := runCmd(2, "ping", "-c", "1", "-W", "1", devices[i].IP)
		devices[i].Online = err == nil
		if err == nil {
			ms := time.Since(start).Milliseconds()
			if ms < 1 {
				devices[i].Latency = "< 1 ms"
			} else {
				devices[i].Latency = fmt.Sprintf("%d ms", ms)
			}
			devices[i].LastSeen = time.Now().Format("15:04:05")
		} else {
			devices[i].Latency = "Timeout"
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
			Command:     "mkdir -p /tmp/hosts && printf '0.0.0.0 doubleclick.net\\n0.0.0.0 googleadservices.com\\n0.0.0.0 pagead2.googlesyndication.com\\n' > /tmp/hosts/r3p-adblock && /etc/init.d/dnsmasq reload && echo 'AdBlock hosts active (3 rules loaded)'",
		},
		{
			ID:          "plugin-ram-optimizer",
			Name:        "Memory Cache Optimizer",
			Version:     "1.0.4",
			Author:      "R3P Core",
			Description: "Flushes reclaimable pagecache, dentries and inodes on MT7621",
			Enabled:     true,
			Command:     "sync && echo 3 > /proc/sys/vm/drop_caches && free -m",
		},
		{
			ID:          "plugin-wan-watchdog",
			Name:        "Gateway & LAN Connectivity Probe",
			Version:     "1.1.0",
			Author:      "R3P Core",
			Description: "Verifies local bridge and ARP neighbor reachability",
			Enabled:     true,
			Command:     "ip addr show br-lan && ip neigh show",
		},
		{
			ID:          "plugin-nand-health",
			Name:        "ESMT SLC NAND UBI Inspector",
			Version:     "1.0.2",
			Author:      "R3P Core",
			Description: "Reads UBI volume wear-leveling and bad block counters from /sys/class/ubi/ubi0",
			Enabled:     true,
			Command:     "head -n 1 /sys/class/ubi/ubi0/bad_peb_count /sys/class/ubi/ubi0/max_ec /sys/class/ubi/ubi0/avail_eraseblocks",
		},
	}
	savePlugins(defaults)
	return defaults
}

func savePlugins(list []PluginManifest) {
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
			for i := range plugins {
				if plugins[i].ID == body.ID {
					plugins[i].Enabled = !plugins[i].Enabled
				}
			}
			savePlugins(plugins)
		case "run":
			for i := range plugins {
				if plugins[i].ID == body.ID {
					out, err := runCmd(10, "sh", "-c", plugins[i].Command)
					if err != nil && out == "" {
						out = err.Error()
					}
					plugins[i].LastOutput = out
					plugins[i].LastRun = time.Now().Format("15:04:05")
				}
			}
			savePlugins(plugins)
		case "create":
			if body.Name == "" || body.Command == "" {
				writeJSON(w, http.StatusBadRequest, false, "name and command are required")
				return
			}
			plugins = append(plugins, PluginManifest{
				ID:          "plugin-" + generateGID()[:6],
				Name:        body.Name,
				Version:     "1.0.0",
				Author:      "Admin",
				Description: body.Description,
				Enabled:     true,
				Command:     body.Command,
			})
			savePlugins(plugins)
		case "delete":
			var updated []PluginManifest
			for _, p := range plugins {
				if p.ID != body.ID {
					updated = append(updated, p)
				}
			}
			plugins = updated
			savePlugins(plugins)
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
	backupPath := "/tmp/r3p-config-backup.tar.gz"
	_ = os.Remove(backupPath)
	if _, err := runCmd(15, "sysupgrade", "-b", backupPath); err != nil {
		// Fallback tar of /etc/config and /etc/r3p
		_, _ = runCmd(10, "tar", "-czf", backupPath, "/etc/config", "/etc/r3p", "/etc/crontabs")
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"r3p-backup.tar.gz\"")
	http.ServeFile(w, r, backupPath)
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
		_, _ = runCmd(4, "uci", "set", "system.@system[0].timezone="+body.Timezone)
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
		_ = os.MkdirAll("/etc/r3p/backups", 0700)
		if old, err := os.ReadFile(targetPath); err == nil {
			_ = os.WriteFile("/etc/r3p/backups/"+body.File+".bak", old, 0600)
		}
		content := strings.ReplaceAll(body.Content, "\r\n", "\n")
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(targetPath, []byte(content), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "failed to write config: "+err.Error())
			return
		}
		_, _ = runCmd(5, "uci", "commit", body.File)
		writeJSON(w, http.StatusOK, true, map[string]string{
			"file":    body.File,
			"message": fmt.Sprintf("/etc/config/%s saved and committed cleanly.", body.File),
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}
