package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWirelessMaskingAndIoTBrokerProbe(t *testing.T) {
	if got := maskDDNSToken("wifi-secret-9876"); got != "••••9876" {
		t.Fatalf("unexpected masked Wi-Fi key: %q", got)
	}
	if got := maskDDNSToken("1234"); got != "••••" {
		t.Fatalf("expected short key masked as ••••, got %q", got)
	}

	// Start ephemeral local TCP listener to test broker_alive=true when enabled and reachable
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	defer ln.Close()

	go func() {
		for {
			conn, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	if !probeIoTBrokerAlive(IoTBrokerConfig{Host: "127.0.0.1", Port: port, Enabled: true}) {
		t.Fatal("expected probeIoTBrokerAlive=true for live listener")
	}
	if probeIoTBrokerAlive(IoTBrokerConfig{Host: "127.0.0.1", Port: port, Enabled: false}) {
		t.Fatal("expected probeIoTBrokerAlive=false when broker is disabled")
	}
	ln.Close()
	if probeIoTBrokerAlive(IoTBrokerConfig{Host: "127.0.0.1", Port: port, Enabled: true}) {
		t.Fatal("expected probeIoTBrokerAlive=false after closing listener")
	}
}

func TestPluginsAllowlistAndAppsPackageGuards(t *testing.T) {
	expectedPresets := []string{"ram_flush", "lan_probe", "nand_health", "dns_reload", "disk_usage", "uptime_check"}
	for _, p := range expectedPresets {
		if !allowedPluginPresets[p] {
			t.Errorf("expected preset %q in allowedPluginPresets", p)
		}
	}
	if allowedPluginPresets["sh -c id"] || allowedPluginPresets["rm -rf /"] {
		t.Fatal("arbitrary shell command must never be in allowedPluginPresets")
	}

	if packageAliases["samba-server"] != "samba4-server" || packageAliases["ftp"] != "vsftpd" {
		t.Fatalf("unexpected packageAliases mapping: %+v", packageAliases)
	}
	if !safePkgNamePattern.MatchString("wireguard-tools") || safePkgNamePattern.MatchString("pkg;rm -rf /") {
		t.Fatal("unexpected safePkgNamePattern behavior")
	}

	owner, repo, ok := parseGitHubOwnerRepo("https://github.com/neopro90/R3P_Project.git")
	if !ok || owner != "neopro90" || repo != "R3P_Project" {
		t.Fatalf("unexpected parseGitHubOwnerRepo: ok=%t owner=%q repo=%q", ok, owner, repo)
	}
	if _, _, badOK := parseGitHubOwnerRepo("single-segment"); badOK {
		t.Fatal("expected single segment GitHub URL to fail")
	}

	if err := validateDownloadURL("http://192.168.2.1/evil.ipk"); err == nil {
		t.Fatal("expected private IP package download URL to fail validateDownloadURL")
	}

	for _, act := range []string{"stop", "disable", "restart", "reload"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/services/control",
			bytes.NewBufferString(`{"service":"r3p-api","action":"`+act+`"}`))
		rr := httptest.NewRecorder()
		handleServicesControl(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 rejecting r3p-api %s, got %d", act, rr.Code)
		}
	}

	for _, handler := range []http.HandlerFunc{handleWirelessFull, handleIoT, handlePlugins, handlePackagesInstall} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler(rr, req)
		if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 503/405 outside OpenWrt, got %d", rr.Code)
		}
	}
}
