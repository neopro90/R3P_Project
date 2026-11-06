package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestParseNetDevData(t *testing.T) {
	data := `Inter-| Receive | Transmit
 face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed
  eth0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
 malformed: 1 2`
	got := parseNetDevData(data)
	if len(got) != 1 {
		t.Fatalf("got %d interfaces, want 1", len(got))
	}
	if iface := got["eth0"]; iface.rxBytes != 1000 || iface.rxPkts != 10 || iface.txBytes != 2000 || iface.txPkts != 20 {
		t.Fatalf("unexpected eth0 counters: %+v", iface)
	}
}

func TestCounterDeltaHandlesCounterReset(t *testing.T) {
	if got := counterDelta(120, 100); got != 20 {
		t.Fatalf("counterDelta(120, 100) = %d, want 20", got)
	}
	if got := counterDelta(3, 100); got != 0 {
		t.Fatalf("counterDelta after reset = %d, want 0", got)
	}
}

func TestAvailableMemoryFallsBackAndClamps(t *testing.T) {
	info := map[string]uint64{"MemTotal": 1000, "MemFree": 100, "Buffers": 50, "Cached": 150}
	if got := availableMemory(info); got != 300 {
		t.Fatalf("fallback available memory = %d, want 300", got)
	}
	info["MemAvailable"] = 400
	if got := availableMemory(info); got != 400 {
		t.Fatalf("MemAvailable = %d, want 400", got)
	}
	info["MemAvailable"] = 2000
	if got := availableMemory(info); got != 1000 {
		t.Fatalf("clamped MemAvailable = %d, want 1000", got)
	}
}

func TestParseWirelessDataKeepsDecimalFields(t *testing.T) {
	data := `Inter-| sta-|   Quality        |   Discarded packets               | Missed | WE
 face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
 wlan0: 0000 70.0 -45.0 -95.0 0 0 0 0 0 0`
	got := parseWirelessData(data)
	if len(got) != 1 {
		t.Fatalf("got %d wireless interfaces, want 1", len(got))
	}
	if got[0].Name != "wlan0" || got[0].Status != 0 || got[0].Link != 70 || got[0].Level != -45 || got[0].Noise != -95 {
		t.Fatalf("unexpected wireless metrics: %+v", got[0])
	}
}

func TestParseDHCPLeaseDataMarksLeaseActivity(t *testing.T) {
	data := `2000 aa:bb:cc:dd:ee:01 192.168.1.10 laptop *
900 aa:bb:cc:dd:ee:02 192.168.1.11 expired *
0 aa:bb:cc:dd:ee:03 192.168.1.12 static *
bad not-a-mac not-an-ip malformed *`
	got := parseDHCPLeaseData(data, 1000)
	if len(got) != 3 {
		t.Fatalf("got %d leases, want 3 valid leases", len(got))
	}
	if !got[0].LeaseActive || got[1].LeaseActive || !got[2].LeaseActive {
		t.Fatalf("unexpected lease activity: %+v", got)
	}
}

func TestInterfaceType(t *testing.T) {
	tests := map[string]string{
		"lo":        "loopback",
		"wan":       "wan",
		"pppoe-wan": "wan",
		"br-lan":    "lan",
		"phy0-ap0":  "wlan",
		"wlan0":     "wlan",
		"eth0":      "other",
	}
	for name, want := range tests {
		if got := interfaceType(name); got != want {
			t.Errorf("interfaceType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestMonitorStreamRejectsNonOpenWrtWithSingleJSONResponse(t *testing.T) {
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		t.Skip("running on OpenWrt; host-only rejection does not apply")
	}

	response := httptest.NewRecorder()
	handleMonitorStream(response, httptest.NewRequest(http.MethodGet, "/api/v1/monitor/stream", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response must be a single JSON envelope: %v", err)
	}
	if body["ok"] != false {
		t.Fatalf("response should report unavailable: %v", body)
	}
}
