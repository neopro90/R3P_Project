package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNetworkIPv4OverlapAndGatewayResolution(t *testing.T) {
	if !isValidIPv4Addr("192.168.2.1") {
		t.Fatal("expected valid IPv4 to pass")
	}
	for _, bad := range []string{"", "256.1.1.1", "::1", "not-an-ip"} {
		if isValidIPv4Addr(bad) {
			t.Fatalf("expected %q to fail isValidIPv4Addr", bad)
		}
	}

	if !ipv4SubnetsOverlap("192.168.1.1", "255.255.255.0", "192.168.1.254", "255.255.255.0") {
		t.Fatal("expected identical /24 subnets to overlap")
	}
	if ipv4SubnetsOverlap("192.168.2.1", "255.255.255.0", "192.168.1.1", "255.255.255.0") {
		t.Fatal("expected distinct /24 subnets not to overlap")
	}
	if !ipv4SubnetsOverlap("10.0.0.1", "255.255.0.0", "10.0.5.10", "255.255.255.0") {
		t.Fatal("expected /16 and contained /24 subnet to overlap")
	}

	cases := map[string]string{
		"wan":       "wan",
		"eth0.2":    "wan",
		"pppoe-wan": "wan",
		"usb0":      "usbwan",
		"wwan0":     "usbwan",
		"apclix0":   "wwan",
		"wlan1":     "wwan",
		"br-lan":    "lan",
	}
	for dev, want := range cases {
		if got := resolveGatewayQuickTarget(dev); got != want {
			t.Errorf("resolveGatewayQuickTarget(%q) = %q, want %q", dev, got, want)
		}
	}
}

func TestFirewallAndConfigSafetyGuards(t *testing.T) {
	for _, act := range []string{"start", "stop", "restart", "reload"} {
		if !isAllowedInitAction(act) {
			t.Errorf("expected init action %q allowed", act)
		}
	}
	for _, act := range []string{"enable", "disable", "exec", "start;id"} {
		if isAllowedInitAction(act) {
			t.Errorf("expected init action %q rejected", act)
		}
	}

	if escaped := escapeUCIQuote("a'b\r\nc"); stringsContainsNewline(escaped) || escaped != "a'\\''bc" {
		t.Fatalf("unexpected UCI escape output: %q", escaped)
	}

	// Non-OpenWrt environment cleanly rejects config & network handlers with 503 JSON
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/interfaces-config", nil)
	rr := httptest.NewRecorder()
	handleNetworkInterfacesConfig(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on non-OpenWrt network config GET, got %d", rr.Code)
	}

	bakReq := httptest.NewRequest(http.MethodGet, "/api/v1/config/backups", nil)
	bakRR := httptest.NewRecorder()
	handleUCIConfigBackups(bakRR, bakReq)
	if bakRR.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on non-OpenWrt config backups GET, got %d", bakRR.Code)
	}

	restReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/restore", bytes.NewBufferString(`{"file":"network"}`))
	restRR := httptest.NewRecorder()
	handleUCIConfigRestore(restRR, restReq)
	if restRR.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on non-OpenWrt config restore POST, got %d", restRR.Code)
	}
}

func stringsContainsNewline(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}
