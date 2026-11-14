package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdblockDomainNormalizationAndWhitelist(t *testing.T) {
	if got := normalizeDomain("0.0.0.0 Ads.DoubleClick.Net # tracker"); got != "ads.doubleclick.net" {
		t.Fatalf("unexpected normalizeDomain hosts output: %q", got)
	}
	if got := normalizeDomain("||tracking.example.com^"); got != "tracking.example.com" {
		t.Fatalf("unexpected normalizeDomain AdGuard rule output: %q", got)
	}
	for _, ignore := range []string{"localhost", "127.0.0.1", "0.0.0.0", "::1", "invalid domain", "bad;host"} {
		if got := normalizeDomain(ignore); got != "" {
			t.Errorf("expected %q to normalize to empty, got %q", ignore, got)
		}
	}

	whitelist := []string{"youtube.com", "trusted.org"}
	if !isDomainWhitelisted("youtube.com", whitelist) || !isDomainWhitelisted("m.youtube.com", whitelist) {
		t.Fatal("expected exact and subdomain match to be whitelisted")
	}
	if isDomainWhitelisted("notyoutube.com", whitelist) {
		t.Fatal("expected suffix spoof domain not to be whitelisted")
	}

	m, h := parseAdblockScheduleHM("30 4 * * *")
	if m != 30 || h != 4 {
		t.Fatalf("expected (30,4), got (%d,%d)", m, h)
	}
	dm, dh := parseAdblockScheduleHM("invalid")
	if dm != 0 || dh != 3 {
		t.Fatalf("expected default (0,3) on invalid schedule, got (%d,%d)", dm, dh)
	}
}

func TestAdblockStreamClassifierAndSSRFGuard(t *testing.T) {
	adStreamsMu.Lock()
	origEvents := adStreamsEvents
	origStats := map[string]int{}
	for k, v := range adStreamStats {
		origStats[k] = v
	}
	adStreamsEvents = nil
	adStreamStats = map[string]int{"blocked": 0, "analyzed": 0, "video_ads": 0, "trackers": 0}
	adStreamsMu.Unlock()

	t.Cleanup(func() {
		adStreamsMu.Lock()
		adStreamsEvents = origEvents
		adStreamStats = origStats
		adStreamsMu.Unlock()
	})

	evtVid := inspectDomainStreamWithBlock("r3.googlevideo.com", "192.168.2.50", true)
	if evtVid.Action != "BLOCKED" || evtVid.Category != "Video Ad Stream" {
		t.Fatalf("unexpected video ad event: %+v", evtVid)
	}

	evtDNS := inspectDomainStreamWithBlock("ads.custom-domain.example", "192.168.2.51", true)
	if evtDNS.Action != "BLOCKED" || evtDNS.Category != "DNS Rule Blocklist" {
		t.Fatalf("unexpected DNS blocklist event: %+v", evtDNS)
	}

	evtAllow := inspectDomainStreamWithBlock("openwrt.org", "192.168.2.52", false)
	if evtAllow.Action != "ALLOWED" {
		t.Fatalf("unexpected allowed event: %+v", evtAllow)
	}

	adStreamsMu.Lock()
	if adStreamStats["analyzed"] != 3 || adStreamStats["blocked"] != 2 || adStreamStats["video_ads"] != 1 || adStreamStats["trackers"] != 1 {
		t.Fatalf("unexpected adStreamStats: %+v", adStreamStats)
	}
	adStreamsMu.Unlock()

	if err := validatePublicHTTPURL(context.Background(), "http://192.168.2.1/hosts.txt"); err == nil {
		t.Fatal("expected local/private AdBlock source URL to fail SSRF check")
	}

	// Method guard on handleAdblockConfigPut
	req := httptest.NewRequest(http.MethodGet, "/api/v1/adblock/config", nil)
	rr := httptest.NewRecorder()
	handleAdblockConfigPut(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed on GET /adblock/config, got %d", rr.Code)
	}
}
