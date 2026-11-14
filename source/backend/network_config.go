package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var networkConfigMu sync.Mutex

func isValidIPv4Addr(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	return ip != nil && ip.To4() != nil
}

func ipv4SubnetsOverlap(ipStrA, maskStrA, ipStrB, maskStrB string) bool {
	ipA := net.ParseIP(strings.TrimSpace(ipStrA)).To4()
	maskA := net.ParseIP(strings.TrimSpace(maskStrA)).To4()
	ipB := net.ParseIP(strings.TrimSpace(ipStrB)).To4()
	maskB := net.ParseIP(strings.TrimSpace(maskStrB)).To4()
	if ipA == nil || maskA == nil || ipB == nil || maskB == nil {
		return false
	}
	uA := uint32(ipA[0])<<24 | uint32(ipA[1])<<16 | uint32(ipA[2])<<8 | uint32(ipA[3])
	mA := uint32(maskA[0])<<24 | uint32(maskA[1])<<16 | uint32(maskA[2])<<8 | uint32(maskA[3])
	uB := uint32(ipB[0])<<24 | uint32(ipB[1])<<16 | uint32(ipB[2])<<8 | uint32(ipB[3])
	mB := uint32(maskB[0])<<24 | uint32(maskB[1])<<16 | uint32(maskB[2])<<8 | uint32(maskB[3])
	commonMask := mA & mB
	if mA > mB {
		commonMask = mB
	} else {
		commonMask = mA
	}
	return (uA & commonMask) == (uB & commonMask)
}

func isValidIPv4Netmask(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s)).To4()
	if ip == nil {
		return false
	}
	mask := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	if mask == 0 {
		return false
	}
	inv := ^mask
	return (inv & (inv + 1)) == 0
}

func resolveGatewayQuickTarget(dev string) string {
	d := strings.ToLower(strings.TrimSpace(dev))
	switch {
	case d == "wan" || strings.HasPrefix(d, "eth0.2") || strings.HasPrefix(d, "pppoe"):
		return "wan"
	case d == "usbwan" || strings.HasPrefix(d, "usb") || strings.HasPrefix(d, "wwan0") || strings.HasPrefix(d, "wwan1"):
		return "usbwan"
	case d == "wwan" || strings.Contains(d, "sta") || strings.HasPrefix(d, "wlan") || strings.HasPrefix(d, "apcli"):
		return "wwan"
	case d == "br-lan" || d == "lan":
		return "lan"
	default:
		return "wan"
	}
}

type NetworkInterfaceConfig struct {
	Name          string   `json:"name"`           // lan, wan, wwan, usbwan
	Label         string   `json:"label"`          // Human-readable name
	Device        string   `json:"device"`         // br-lan, wan, phy0-sta0, usb0
	Proto         string   `json:"proto"`          // static, dhcp, pppoe
	IPAddr        string   `json:"ipaddr"`         // e.g. 192.168.2.1
	Netmask       string   `json:"netmask"`        // e.g. 255.255.255.0
	Gateway       string   `json:"gateway"`        // e.g. 192.168.1.1
	DNS           []string `json:"dns"`            // e.g. ["1.1.1.1", "8.8.8.8"]
	MTU           string   `json:"mtu"`            // e.g. 1500
	Metric        string   `json:"metric"`         // route metric
	MACAddr       string   `json:"macaddr"`        // optional MAC override
	Username      string   `json:"username"`       // PPPoE username
	Password      string   `json:"password"`       // Never populated in GET responses
	HasPassword   bool     `json:"has_password"`   // True if PPPoE password is stored in UCI
	DHCPEnabled   bool     `json:"dhcp_enabled"`   // For LAN: whether dnsmasq DHCP is active
	DHCPStart     string   `json:"dhcp_start"`     // e.g. 100
	DHCPLimit     string   `json:"dhcp_limit"`     // e.g. 150
	DHCPLeaseTime string   `json:"dhcp_leasetime"` // e.g. 12h
	LiveUp        bool     `json:"live_up"`
	LiveIPv4      string   `json:"live_ipv4"`
	LiveMAC       string   `json:"live_mac"`
	LiveGateway   string   `json:"live_gateway"`
}

func readNetworkInterfacesConfig() ([]NetworkInterfaceConfig, string, string, []string) {
	netUCI := parseUCIConfigMap("network")
	dhcpUCI := parseUCIConfigMap("dhcp")

	// Live default route
	routeOut, _ := runCmd(3, "ip", "route", "show", "default")
	defaultGW := ""
	defaultDev := ""
	for _, line := range strings.Split(routeOut, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		for i := 0; i < len(f)-1; i++ {
			if f[i] == "via" && defaultGW == "" {
				defaultGW = f[i+1]
			}
			if f[i] == "dev" && defaultDev == "" {
				defaultDev = f[i+1]
			}
		}
	}

	// Active DNS resolvers
	dnsSet := map[string]bool{}
	var activeDNS []string
	for _, p := range []string{"/tmp/resolv.conf.d/resolv.conf.auto", "/etc/resolv.conf"} {
		for _, line := range readFileLines(p, 50) {
			f := strings.Fields(line)
			if len(f) >= 2 && f[0] == "nameserver" && f[1] != "127.0.0.1" && f[1] != "::1" {
				if !dnsSet[f[1]] {
					dnsSet[f[1]] = true
					activeDNS = append(activeDNS, f[1])
				}
			}
		}
	}
	if len(activeDNS) == 0 {
		activeDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	buildIface := func(sec, label, defaultDevName, defaultProto string) NetworkInterfaceConfig {
		dev := netUCI["network."+sec+".device"]
		if dev == "" {
			dev = netUCI["network."+sec+".ifname"]
		}
		if dev == "" {
			dev = defaultDevName
		}
		proto := netUCI["network."+sec+".proto"]
		if proto == "" {
			proto = defaultProto
		}
		dnsRaw := netUCI["network."+sec+".dns"]
		var dnsList []string
		for _, d := range strings.Fields(dnsRaw) {
			if d != "" {
				dnsList = append(dnsList, d)
			}
		}
		if dnsList == nil {
			dnsList = []string{}
		}

		// Check live interface status
		liveDev := dev
		if sec == "wwan" {
			for _, cand := range []string{"phy0-sta0", "phy1-sta0", "wlan0-1", "wlan1-1", "apcli0"} {
				meta := interfaceMetadata(cand)
				if up, ok := meta["up"].(bool); ok && up {
					liveDev = cand
					break
				}
			}
		}
		meta := interfaceMetadata(liveDev)
		liveUp, _ := meta["up"].(bool)
		liveIP, _ := meta["ipv4"].(string)
		liveMAC, _ := meta["mac"].(string)

		cfg := NetworkInterfaceConfig{
			Name:        sec,
			Label:       label,
			Device:      dev,
			Proto:       proto,
			IPAddr:      netUCI["network."+sec+".ipaddr"],
			Netmask:     netUCI["network."+sec+".netmask"],
			Gateway:     netUCI["network."+sec+".gateway"],
			DNS:         dnsList,
			MTU:         netUCI["network."+sec+".mtu"],
			Metric:      netUCI["network."+sec+".metric"],
			MACAddr:     netUCI["network."+sec+".macaddr"],
			Username:    netUCI["network."+sec+".username"],
			Password:    "",
			HasPassword: netUCI["network."+sec+".password"] != "",
			LiveUp:      liveUp,
			LiveIPv4:    liveIP,
			LiveMAC:     liveMAC,
			LiveGateway: defaultGW,
		}
		if cfg.Netmask == "" && cfg.Proto == "static" {
			cfg.Netmask = "255.255.255.0"
		}
		if sec == "lan" {
			cfg.DHCPEnabled = dhcpUCI["dhcp.lan.ignore"] != "1"
			cfg.DHCPStart = dhcpUCI["dhcp.lan.start"]
			if cfg.DHCPStart == "" {
				cfg.DHCPStart = "100"
			}
			cfg.DHCPLimit = dhcpUCI["dhcp.lan.limit"]
			if cfg.DHCPLimit == "" {
				cfg.DHCPLimit = "150"
			}
			cfg.DHCPLeaseTime = dhcpUCI["dhcp.lan.leasetime"]
			if cfg.DHCPLeaseTime == "" {
				cfg.DHCPLeaseTime = "12h"
			}
		}
		return cfg
	}

	list := []NetworkInterfaceConfig{
		buildIface("lan", "LAN Bridge (Mạng Nội Bộ)", "br-lan", "static"),
		buildIface("wan", "WAN Ethernet (Cổng WAN Vật Lý)", "wan", "dhcp"),
		buildIface("wwan", "WWAN Wi-Fi Client (Thu Sóng Wi-Fi)", "phy0-sta0", "dhcp"),
		buildIface("usbwan", "USB WAN (USB Tethering / 4G LTE)", "usb0", "dhcp"),
	}

	return list, defaultGW, defaultDev, activeDNS
}

// GET / PUT /api/v1/network/interfaces-config
func handleNetworkInterfacesConfig(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		ifaces, defaultGW, defaultDev, activeDNS := readNetworkInterfacesConfig()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"interfaces":      ifaces,
			"default_gateway": defaultGW,
			"default_device":  defaultDev,
			"active_dns":      activeDNS,
		})

	case http.MethodPut:
		var body struct {
			Name          string   `json:"name"`
			Proto         string   `json:"proto"`
			Device        string   `json:"device"`
			IPAddr        string   `json:"ipaddr"`
			Netmask       string   `json:"netmask"`
			Gateway       string   `json:"gateway"`
			DNS           []string `json:"dns"`
			DNSString     string   `json:"dns_string"`
			MTU           string   `json:"mtu"`
			Metric        string   `json:"metric"`
			MACAddr       string   `json:"macaddr"`
			Username      string   `json:"username"`
			Password      string   `json:"password"`
			DHCPEnabled   *bool    `json:"dhcp_enabled"`
			DHCPStart     string   `json:"dhcp_start"`
			DHCPLimit     string   `json:"dhcp_limit"`
			DHCPLeaseTime string   `json:"dhcp_leasetime"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON payload")
			return
		}

		sec := strings.ToLower(strings.TrimSpace(body.Name))
		if sec != "lan" && sec != "wan" && sec != "wwan" && sec != "usbwan" && sec != "gateway_quick" {
			writeJSON(w, http.StatusBadRequest, false, "interface name must be lan, wan, wwan, usbwan, or gateway_quick")
			return
		}

		// Quick Gateway & DNS edit shortcut
		if sec == "gateway_quick" {
			networkConfigMu.Lock()
			defer networkConfigMu.Unlock()

			gw := strings.TrimSpace(body.Gateway)
			dev := strings.TrimSpace(body.Device)
			if dev == "" {
				dev = "br-lan"
			}
			if !safeTokenKeyPattern.MatchString(dev) || strings.HasPrefix(dev, "-") {
				writeJSON(w, http.StatusBadRequest, false, "invalid device name")
				return
			}
			if gw != "" && !isValidIPv4Addr(gw) {
				writeJSON(w, http.StatusBadRequest, false, "invalid gateway IPv4 address")
				return
			}
			var dnsList []string
			for _, d := range body.DNS {
				t := strings.TrimSpace(d)
				if t != "" {
					if !isValidIPv4Addr(t) {
						writeJSON(w, http.StatusBadRequest, false, "invalid DNS IPv4 address: "+t)
						return
					}
					dnsList = append(dnsList, t)
				}
			}
			for _, d := range strings.Fields(body.DNSString) {
				t := strings.TrimSpace(d)
				if !isValidIPv4Addr(t) {
					writeJSON(w, http.StatusBadRequest, false, "invalid DNS IPv4 address: "+t)
					return
				}
				dnsList = append(dnsList, t)
			}

			targetSec := resolveGatewayQuickTarget(dev)
			if gw != "" {
				if out, err := runCmd(4, "uci", "set", fmt.Sprintf("network.%s.gateway=%s", targetSec, gw)); err != nil {
					_, _ = runCmd(3, "uci", "revert", "network")
					writeJSON(w, http.StatusInternalServerError, false, "uci set gateway failed: "+out)
					return
				}
				_, _ = runCmd(4, "ip", "route", "replace", "default", "via", gw, "dev", dev)
			} else {
				_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.gateway", targetSec))
			}

			if len(dnsList) > 0 {
				if targetSec != "lan" {
					_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.peerdns=0", targetSec))
				}
				if out, err := runCmd(4, "uci", "set", fmt.Sprintf("network.%s.dns=%s", targetSec, strings.Join(dnsList, " "))); err != nil {
					_, _ = runCmd(3, "uci", "revert", "network")
					writeJSON(w, http.StatusInternalServerError, false, "uci set dns failed: "+out)
					return
				}
			}
			if out, err := runCmd(5, "uci", "commit", "network"); err != nil {
				_, _ = runCmd(3, "uci", "revert", "network")
				writeJSON(w, http.StatusInternalServerError, false, "uci commit network failed: "+out)
				return
			}
			go func() {
				_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
				time.Sleep(1 * time.Second)
				refreshInternetStatus(true)
			}()
			ifaces, defaultGW, defaultDev, activeDNS := readNetworkInterfacesConfig()
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"saved":           true,
				"message":         fmt.Sprintf("Đã cập nhật Default Gateway & DNS (%s / %s) thành công!", strings.ToUpper(targetSec), dev),
				"interfaces":      ifaces,
				"default_gateway": defaultGW,
				"default_device":  defaultDev,
				"active_dns":      activeDNS,
			})
			return
		}

		proto := strings.ToLower(strings.TrimSpace(body.Proto))
		if proto == "" {
			proto = "dhcp"
		}
		if proto != "static" && proto != "dhcp" && proto != "pppoe" {
			writeJSON(w, http.StatusBadRequest, false, "proto không hợp lệ (chỉ chấp nhận static, dhcp, pppoe)")
			return
		}

		// Pre-validate all fields before touching UCI
		ip := strings.TrimSpace(body.IPAddr)
		nm := strings.TrimSpace(body.Netmask)
		gw := strings.TrimSpace(body.Gateway)
		if proto == "static" {
			if !isValidIPv4Addr(ip) {
				writeJSON(w, http.StatusBadRequest, false, "Địa chỉ IPv4 không hợp lệ cho chế độ Static IP")
				return
			}
			if nm == "" {
				nm = "255.255.255.0"
			}
			if !isValidIPv4Netmask(nm) {
				writeJSON(w, http.StatusBadRequest, false, "Subnet Mask không hợp lệ cho chế độ Static IP")
				return
			}
			if gw != "" && !isValidIPv4Addr(gw) {
				writeJSON(w, http.StatusBadRequest, false, "Địa chỉ Gateway IPv4 không hợp lệ")
				return
			}
			// Guard against overlapping LAN/WAN static subnets
			netUCI := parseUCIConfigMap("network")
			for _, peer := range []string{"lan", "wan", "wwan", "usbwan"} {
				if peer == sec {
					continue
				}
				if (sec == "lan") != (peer == "lan") && netUCI["network."+peer+".proto"] == "static" {
					peerIP := netUCI["network."+peer+".ipaddr"]
					peerMask := netUCI["network."+peer+".netmask"]
					if peerMask == "" {
						peerMask = "255.255.255.0"
					}
					if ipv4SubnetsOverlap(ip, nm, peerIP, peerMask) {
						writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Dải mạng %s (%s/%s) bị trùng subnet với cổng %s (%s/%s)", strings.ToUpper(sec), ip, nm, strings.ToUpper(peer), peerIP, peerMask))
						return
					}
				}
			}
		} else if proto == "dhcp" && gw != "" && !isValidIPv4Addr(gw) {
			writeJSON(w, http.StatusBadRequest, false, "Địa chỉ Gateway IPv4 không hợp lệ")
			return
		}

		var validDNS []string
		for _, d := range body.DNS {
			for _, part := range strings.Fields(strings.ReplaceAll(d, ",", " ")) {
				if !isValidIPv4Addr(part) {
					writeJSON(w, http.StatusBadRequest, false, "DNS server IPv4 không hợp lệ: "+part)
					return
				}
				validDNS = append(validDNS, part)
			}
		}
		if body.DNSString != "" {
			for _, part := range strings.Fields(strings.ReplaceAll(body.DNSString, ",", " ")) {
				if !isValidIPv4Addr(part) {
					writeJSON(w, http.StatusBadRequest, false, "DNS server IPv4 không hợp lệ: "+part)
					return
				}
				validDNS = append(validDNS, part)
			}
		}

		if strings.TrimSpace(body.MTU) != "" {
			mtu, err := strconv.Atoi(strings.TrimSpace(body.MTU))
			if err != nil || mtu < 576 || mtu > 9000 {
				writeJSON(w, http.StatusBadRequest, false, "MTU phải từ 576 đến 9000")
				return
			}
		}
		if strings.TrimSpace(body.Metric) != "" {
			metric, err := strconv.Atoi(strings.TrimSpace(body.Metric))
			if err != nil || metric < 0 || metric > 1000 {
				writeJSON(w, http.StatusBadRequest, false, "Metric phải từ 0 đến 1000")
				return
			}
		}
		if strings.TrimSpace(body.MACAddr) != "" {
			if _, err := net.ParseMAC(strings.TrimSpace(body.MACAddr)); err != nil {
				writeJSON(w, http.StatusBadRequest, false, "Địa chỉ MAC không hợp lệ")
				return
			}
		}
		if sec == "lan" {
			startNum, hasStart := 0, strings.TrimSpace(body.DHCPStart) != ""
			limitNum, hasLimit := 0, strings.TrimSpace(body.DHCPLimit) != ""
			if hasStart {
				v, err := strconv.Atoi(strings.TrimSpace(body.DHCPStart))
				if err != nil || v < 2 || v > 250 {
					writeJSON(w, http.StatusBadRequest, false, "DHCP start phải trong khoảng 2–250")
					return
				}
				startNum = v
			}
			if hasLimit {
				v, err := strconv.Atoi(strings.TrimSpace(body.DHCPLimit))
				if err != nil || v < 1 || v > 250 {
					writeJSON(w, http.StatusBadRequest, false, "DHCP limit phải trong khoảng 1–250")
					return
				}
				limitNum = v
			}
			if hasStart && hasLimit && startNum+limitNum > 254 {
				writeJSON(w, http.StatusBadRequest, false, "Tổng DHCP start + limit vượt quá giới hạn 254 của subnet /24")
				return
			}
			if strings.TrimSpace(body.DHCPLeaseTime) != "" && !safeTokenKeyPattern.MatchString(strings.TrimSpace(body.DHCPLeaseTime)) {
				writeJSON(w, http.StatusBadRequest, false, "Thời hạn cấp DHCP leasetime không hợp lệ")
				return
			}
		}
		if body.Device != "" && (!safeWirelessVal.MatchString(body.Device) || strings.HasPrefix(body.Device, "-") || strings.Contains(body.Device, "..")) {
			writeJSON(w, http.StatusBadRequest, false, "Tên device không hợp lệ")
			return
		}

		networkConfigMu.Lock()
		defer networkConfigMu.Unlock()
		if sec == "lan" {
			dhcpMu.Lock()
			defer dhcpMu.Unlock()
		}

		prevNet, prevNetErr := runCmd(4, "uci", "export", "network")
		revertAndFail := func(msg string) {
			_, _ = runCmd(3, "uci", "revert", "network")
			_, _ = runCmd(3, "uci", "revert", "dhcp")
			writeJSON(w, http.StatusInternalServerError, false, msg)
		}

		if out, err := runCmd(4, "uci", "set", fmt.Sprintf("network.%s=interface", sec)); err != nil {
			revertAndFail("uci set interface failed: " + out)
			return
		}
		if body.Device != "" {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.device=%s", sec, body.Device))
		} else if sec == "lan" {
			_, _ = runCmd(4, "uci", "set", "network.lan.device=br-lan")
		} else if sec == "wan" {
			_, _ = runCmd(4, "uci", "set", "network.wan.device=wan")
		}

		if out, err := runCmd(4, "uci", "set", fmt.Sprintf("network.%s.proto=%s", sec, proto)); err != nil {
			revertAndFail("uci set proto failed: " + out)
			return
		}

		if proto == "static" {
			if out, err := runCmd(4, "uci", "set", fmt.Sprintf("network.%s.ipaddr=%s", sec, ip)); err != nil {
				revertAndFail("uci set ipaddr failed: " + out)
				return
			}
			if out, err := runCmd(4, "uci", "set", fmt.Sprintf("network.%s.netmask=%s", sec, nm)); err != nil {
				revertAndFail("uci set netmask failed: " + out)
				return
			}
			if gw != "" {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.gateway=%s", sec, gw))
			} else {
				_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.gateway", sec))
			}
		} else if proto == "pppoe" {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.ipaddr", sec))
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.netmask", sec))
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.gateway", sec))
			if body.Username != "" {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.username=%s", sec, strings.TrimSpace(body.Username)))
			}
			if body.Password != "" && !strings.HasPrefix(body.Password, "••••") {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.password=%s", sec, body.Password))
			}
		} else {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.ipaddr", sec))
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.netmask", sec))
			if gw != "" {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.gateway=%s", sec, gw))
			} else {
				_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.gateway", sec))
			}
		}

		if len(validDNS) > 0 {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.peerdns=0", sec))
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.dns=%s", sec, strings.Join(validDNS, " ")))
		} else {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.peerdns", sec))
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.dns", sec))
		}

		if strings.TrimSpace(body.MTU) != "" {
			mtu, _ := strconv.Atoi(strings.TrimSpace(body.MTU))
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.mtu=%d", sec, mtu))
		} else {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.mtu", sec))
		}
		if strings.TrimSpace(body.Metric) != "" {
			metric, _ := strconv.Atoi(strings.TrimSpace(body.Metric))
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.metric=%d", sec, metric))
		} else {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.metric", sec))
		}
		if strings.TrimSpace(body.MACAddr) != "" {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.macaddr=%s", sec, strings.ToUpper(strings.TrimSpace(body.MACAddr))))
		} else {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.macaddr", sec))
		}

		if out, err := runCmd(5, "uci", "commit", "network"); err != nil {
			revertAndFail("uci commit network failed: " + out)
			return
		}

		// If LAN, also update DHCP settings if provided
		if sec == "lan" {
			if body.DHCPEnabled != nil {
				if *body.DHCPEnabled {
					_, _ = runCmd(4, "uci", "delete", "dhcp.lan.ignore")
				} else {
					_, _ = runCmd(4, "uci", "set", "dhcp.lan.ignore=1")
				}
			}
			if body.DHCPStart != "" {
				start, _ := strconv.Atoi(strings.TrimSpace(body.DHCPStart))
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.start=%d", start))
			}
			if body.DHCPLimit != "" {
				limit, _ := strconv.Atoi(strings.TrimSpace(body.DHCPLimit))
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.limit=%d", limit))
			}
			if body.DHCPLeaseTime != "" {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.leasetime=%s", strings.TrimSpace(body.DHCPLeaseTime)))
			}
			if out, err := runCmd(5, "uci", "commit", "dhcp"); err != nil {
				_, _ = runCmd(3, "uci", "revert", "dhcp")
				if prevNetErr == nil && prevNet != "" {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					cmd := exec.CommandContext(ctx, "uci", "import", "network")
					cmd.Stdin = strings.NewReader(prevNet + "\n")
					_ = cmd.Run()
					cancel()
					_, _ = runCmd(5, "uci", "commit", "network")
				}
				writeJSON(w, http.StatusInternalServerError, false, "uci commit dhcp failed (network rolled back): "+out)
				return
			}
		}

		// Apply changes asynchronously so HTTP response returns cleanly before interface reload
		go func(targetSec, targetGW string) {
			netApplyMu.Lock()
			defer netApplyMu.Unlock()
			time.Sleep(300 * time.Millisecond)
			_, _ = runCmd(8, "/sbin/ifup", targetSec)
			if targetSec == "lan" {
				_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
				_, _ = runCmd(8, "/etc/init.d/samba4", "restart")
				_, _ = runCmd(8, "/etc/init.d/vsftpd", "restart")
				if targetGW != "" && isValidIPv4Addr(targetGW) {
					_, _ = runCmd(4, "ip", "route", "replace", "default", "via", targetGW, "dev", "br-lan")
				}
			}
			time.Sleep(2 * time.Second)
			refreshInternetStatus(true)
		}(sec, gw)

		ifaces, defaultGW, defaultDev, activeDNS := readNetworkInterfacesConfig()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"saved":           true,
			"interface":       sec,
			"message":         fmt.Sprintf("Đã lưu và áp dụng cấu hình mạng cho cổng %s!", strings.ToUpper(sec)),
			"interfaces":      ifaces,
			"default_gateway": defaultGW,
			"default_device":  defaultDev,
			"active_dns":      activeDNS,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

var netApplyMu sync.Mutex

// POST /api/v1/network/interfaces-config/action
func handleNetworkInterfaceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Name   string `json:"name"`
		Action string `json:"action"` // ifup, ifdown, reload
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	sec := strings.ToLower(strings.TrimSpace(body.Name))
	if sec != "lan" && sec != "wan" && sec != "wwan" && sec != "usbwan" && sec != "all" {
		writeJSON(w, http.StatusBadRequest, false, "invalid interface name")
		return
	}
	switch body.Action {
	case "ifup", "reconnect":
		if sec == "all" {
			go func() {
				netApplyMu.Lock()
				defer netApplyMu.Unlock()
				_, _ = runCmd(10, "/etc/init.d/network", "reload")
				time.Sleep(2 * time.Second)
				refreshInternetStatus(true)
			}()
		} else {
			go func() {
				netApplyMu.Lock()
				defer netApplyMu.Unlock()
				_, _ = runCmd(10, "/sbin/ifup", sec)
				time.Sleep(2 * time.Second)
				refreshInternetStatus(true)
			}()
		}
	case "ifdown":
		if sec == "lan" || sec == "all" {
			writeJSON(w, http.StatusBadRequest, false, "Không thể tắt cổng LAN hoặc toàn bộ giao diện để tránh mất kết nối quản trị")
			return
		}
		go func() {
			netApplyMu.Lock()
			defer netApplyMu.Unlock()
			_, _ = runCmd(8, "/sbin/ifdown", sec)
			time.Sleep(1 * time.Second)
			refreshInternetStatus(true)
		}()
	case "reload":
		go func() {
			netApplyMu.Lock()
			defer netApplyMu.Unlock()
			_, _ = runCmd(10, "/etc/init.d/network", "reload")
			time.Sleep(2 * time.Second)
			refreshInternetStatus(true)
		}()
	default:
		writeJSON(w, http.StatusBadRequest, false, "unsupported action")
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"interface": sec,
		"action":    body.Action,
		"message":   fmt.Sprintf("Đã gửi lệnh %s tới interface %s", body.Action, strings.ToUpper(sec)),
	})
}
