package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── LAN / WAN / WWAN / USBWAN Interface & Gateway Configuration ───────────

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
	Password      string   `json:"password"`       // PPPoE password
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
			Password:    netUCI["network."+sec+".password"],
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
			gw := strings.TrimSpace(body.Gateway)
			dev := strings.TrimSpace(body.Device)
			if dev == "" {
				dev = "br-lan"
			}
			if gw != "" {
				if net.ParseIP(gw) == nil {
					writeJSON(w, http.StatusBadRequest, false, "invalid gateway IPv4 address")
					return
				}
				_, _ = runCmd(4, "uci", "set", "network.lan.gateway="+gw)
				_, _ = runCmd(4, "ip", "route", "replace", "default", "via", gw, "dev", dev)
			} else {
				_, _ = runCmd(4, "uci", "delete", "network.lan.gateway")
			}

			var dnsList []string
			for _, d := range body.DNS {
				if net.ParseIP(strings.TrimSpace(d)) != nil {
					dnsList = append(dnsList, strings.TrimSpace(d))
				}
			}
			for _, d := range strings.Fields(body.DNSString) {
				if net.ParseIP(strings.TrimSpace(d)) != nil {
					dnsList = append(dnsList, strings.TrimSpace(d))
				}
			}
			if len(dnsList) > 0 {
				_, _ = runCmd(4, "uci", "set", "network.lan.dns="+strings.Join(dnsList, " "))
				_, _ = runCmd(4, "uci", "set", "network.wan.peerdns=0")
				_, _ = runCmd(4, "uci", "set", "network.wan.dns="+strings.Join(dnsList, " "))
			}
			_, _ = runCmd(5, "uci", "commit", "network")
			go func() {
				_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
				time.Sleep(1 * time.Second)
				refreshInternetStatus(true)
			}()
			ifaces, defaultGW, defaultDev, activeDNS := readNetworkInterfacesConfig()
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"saved":           true,
				"message":         "Đã cập nhật Default Gateway & DNS thành công!",
				"interfaces":      ifaces,
				"default_gateway": defaultGW,
				"default_device":  defaultDev,
				"active_dns":      activeDNS,
			})
			return
		}

		proto := strings.ToLower(strings.TrimSpace(body.Proto))
		if proto != "static" && proto != "dhcp" && proto != "pppoe" {
			proto = "dhcp"
		}

		// Ensure section exists
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s=interface", sec))
		if body.Device != "" && safeWirelessVal.MatchString(body.Device) {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.device=%s", sec, body.Device))
		} else if sec == "lan" {
			_, _ = runCmd(4, "uci", "set", "network.lan.device=br-lan")
		} else if sec == "wan" {
			_, _ = runCmd(4, "uci", "set", "network.wan.device=wan")
		}

		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.proto=%s", sec, proto))

		if proto == "static" {
			ip := strings.TrimSpace(body.IPAddr)
			if net.ParseIP(ip) == nil {
				writeJSON(w, http.StatusBadRequest, false, "Địa chỉ IPv4 không hợp lệ cho chế độ Static IP")
				return
			}
			nm := strings.TrimSpace(body.Netmask)
			if net.ParseIP(nm) == nil {
				nm = "255.255.255.0"
			}
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.ipaddr=%s", sec, ip))
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.netmask=%s", sec, nm))

			gw := strings.TrimSpace(body.Gateway)
			if gw != "" {
				if net.ParseIP(gw) == nil {
					writeJSON(w, http.StatusBadRequest, false, "Địa chỉ Gateway không hợp lệ")
					return
				}
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
			if body.Password != "" {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.password=%s", sec, body.Password))
			}
		} else {
			// dhcp
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.ipaddr", sec))
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.netmask", sec))
			gw := strings.TrimSpace(body.Gateway)
			if gw != "" && net.ParseIP(gw) != nil {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.gateway=%s", sec, gw))
			} else {
				_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.gateway", sec))
			}
		}

		// DNS servers
		var validDNS []string
		for _, d := range body.DNS {
			for _, part := range strings.Fields(strings.ReplaceAll(d, ",", " ")) {
				if net.ParseIP(part) != nil {
					validDNS = append(validDNS, part)
				}
			}
		}
		if body.DNSString != "" {
			for _, part := range strings.Fields(strings.ReplaceAll(body.DNSString, ",", " ")) {
				if net.ParseIP(part) != nil {
					validDNS = append(validDNS, part)
				}
			}
		}
		if len(validDNS) > 0 {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.peerdns=0", sec))
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.dns=%s", sec, strings.Join(validDNS, " ")))
		} else {
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.peerdns", sec))
			_, _ = runCmd(4, "uci", "delete", fmt.Sprintf("network.%s.dns", sec))
		}

		if body.MTU != "" {
			if mtu, err := strconv.Atoi(body.MTU); err == nil && mtu >= 576 && mtu <= 9000 {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.mtu=%d", sec, mtu))
			}
		}
		if body.Metric != "" {
			if metric, err := strconv.Atoi(body.Metric); err == nil && metric >= 0 && metric <= 1000 {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.metric=%d", sec, metric))
			}
		}
		if body.MACAddr != "" {
			if _, err := net.ParseMAC(body.MACAddr); err == nil {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("network.%s.macaddr=%s", sec, strings.ToUpper(body.MACAddr)))
			}
		}

		_, _ = runCmd(5, "uci", "commit", "network")

		// If LAN, also update DHCP settings if provided
		if sec == "lan" {
			if body.DHCPEnabled != nil {
				if *body.DHCPEnabled {
					_, _ = runCmd(4, "uci", "delete", "dhcp.lan.ignore")
				} else {
					_, _ = runCmd(4, "uci", "set", "dhcp.lan.ignore=1")
				}
			}
			if start, err := strconv.Atoi(body.DHCPStart); err == nil && start >= 2 && start <= 250 {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.start=%d", start))
			}
			if limit, err := strconv.Atoi(body.DHCPLimit); err == nil && limit >= 1 && limit <= 250 {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.limit=%d", limit))
			}
			if body.DHCPLeaseTime != "" && safeTokenKeyPattern.MatchString(body.DHCPLeaseTime) {
				_, _ = runCmd(4, "uci", "set", fmt.Sprintf("dhcp.lan.leasetime=%s", body.DHCPLeaseTime))
			}
			_, _ = runCmd(5, "uci", "commit", "dhcp")
		}

		// Apply changes asynchronously so HTTP response returns cleanly before interface reload
		go func(targetSec, targetGW string) {
			time.Sleep(300 * time.Millisecond)
			_, _ = runCmd(8, "/sbin/ifup", targetSec)
			if targetSec == "lan" {
				_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
				_, _ = runCmd(8, "/etc/init.d/samba4", "restart")
				_, _ = runCmd(8, "/etc/init.d/vsftpd", "restart")
				if targetGW != "" && net.ParseIP(targetGW) != nil {
					_, _ = runCmd(4, "ip", "route", "replace", "default", "via", targetGW, "dev", "br-lan")
				}
			}
			time.Sleep(2 * time.Second)
			refreshInternetStatus(true)
		}(sec, strings.TrimSpace(body.Gateway))

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
		go func() {
			_, _ = runCmd(10, "/sbin/ifup", sec)
			time.Sleep(2 * time.Second)
			refreshInternetStatus(true)
		}()
	case "ifdown":
		if sec == "lan" {
			writeJSON(w, http.StatusBadRequest, false, "Không thể tắt cổng LAN chính để tránh mất kết nối quản trị")
			return
		}
		go func() {
			_, _ = runCmd(8, "/sbin/ifdown", sec)
			time.Sleep(1 * time.Second)
			refreshInternetStatus(true)
		}()
	case "reload":
		go func() {
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
