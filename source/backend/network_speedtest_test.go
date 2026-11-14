package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpeedtestValidatesInterfaceAndUbusState(t *testing.T) {
	t.Run("invalid interface name", func(t *testing.T) {
		env := speedtestEnv{
			requireOpenWrt: func(http.ResponseWriter) bool { return true },
			queryUbus:      func(string) (speedtestUbusStatus, error) { return speedtestUbusStatus{}, nil },
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/network/speedtest?interface=-bad", nil)
		rec := httptest.NewRecorder()
		handleInterfaceSpeedtestWithEnv(rec, req, env)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("interface down fails fast", func(t *testing.T) {
		env := speedtestEnv{
			requireOpenWrt: func(http.ResponseWriter) bool { return true },
			queryUbus: func(iface string) (speedtestUbusStatus, error) {
				return speedtestUbusStatus{Up: false, L3Device: "phy0-sta0"}, nil
			},
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/network/speedtest?interface=wwan", nil)
		rec := httptest.NewRecorder()
		handleInterfaceSpeedtestWithEnv(rec, req, env)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("ubus missing interface returns 400", func(t *testing.T) {
		env := speedtestEnv{
			requireOpenWrt: func(http.ResponseWriter) bool { return true },
			queryUbus: func(iface string) (speedtestUbusStatus, error) {
				return speedtestUbusStatus{}, errors.New("not found")
			},
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/network/speedtest?interface=usbwan", nil)
		rec := httptest.NewRecorder()
		handleInterfaceSpeedtestWithEnv(rec, req, env)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestSpeedtestRejectsConcurrentRun(t *testing.T) {
	if !speedtestMu.TryLock() {
		t.Fatal("expected lock to be free before test")
	}
	defer speedtestMu.Unlock()

	env := speedtestEnv{
		requireOpenWrt: func(http.ResponseWriter) bool { return true },
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/speedtest?interface=wan", nil)
	rec := httptest.NewRecorder()
	handleInterfaceSpeedtestWithEnv(rec, req, env)
	if rec.Code != http.StatusConflict {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestSpeedtestMeasuresDownloadUploadLatencyAndJitter(t *testing.T) {
	payload := bytes.Repeat([]byte("R"), 512*1024) // 512 KB
	var receivedUpload int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lat":
			time.Sleep(15 * time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		case "/down":
			w.WriteHeader(http.StatusOK)
			half := len(payload) / 2
			_, _ = w.Write(payload[:half])
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			time.Sleep(45 * time.Millisecond)
			_, _ = w.Write(payload[half:])
		case "/up":
			time.Sleep(35 * time.Millisecond)
			n, _ := io.Copy(io.Discard, r.Body)
			receivedUpload = n
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var capturedLocalIP string
	env := speedtestEnv{
		requireOpenWrt: func(http.ResponseWriter) bool { return true },
		queryUbus: func(iface string) (speedtestUbusStatus, error) {
			st := speedtestUbusStatus{
				Up:       true,
				L3Device: "eth0.2",
			}
			st.IPv4Address = []struct {
				Address string `json:"address"`
				Mask    int    `json:"mask"`
			}{{Address: "100.64.10.5", Mask: 24}}
			return st, nil
		},
		newClient: func(localIP string, timeout time.Duration) *http.Client {
			capturedLocalIP = localIP
			return &http.Client{Timeout: timeout}
		},
		targets: []speedtestTarget{
			{
				Name:        "Broken Mirror",
				LatencyURL:  srv.URL + "/missing",
				DownloadURL: srv.URL + "/missing",
			},
			{
				Name:        "Mock Test Edge",
				LatencyURL:  srv.URL + "/lat",
				DownloadURL: srv.URL + "/down",
				UploadURL:   srv.URL + "/up",
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/speedtest?interface=wan", nil)
	rec := httptest.NewRecorder()
	handleInterfaceSpeedtestWithEnv(rec, req, env)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, body=%s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		OK   bool `json:"ok"`
		Data struct {
			Interface       string  `json:"interface"`
			Device          string  `json:"device"`
			LocalIP         string  `json:"local_ip"`
			Server          string  `json:"server"`
			DownloadMbps    float64 `json:"download_mbps"`
			UploadMbps      float64 `json:"upload_mbps"`
			UploadSupported bool    `json:"upload_supported"`
			LatencyMs       float64 `json:"latency_ms"`
			JitterMs        float64 `json:"jitter_ms"`
			DownloadBytes   int64   `json:"download_bytes"`
			UploadBytes     int64   `json:"upload_bytes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !parsed.OK {
		t.Fatal("expected ok=true")
	}
	if capturedLocalIP != "100.64.10.5" || parsed.Data.LocalIP != "100.64.10.5" {
		t.Fatalf("unexpected local IP: captured=%q resp=%q", capturedLocalIP, parsed.Data.LocalIP)
	}
	if parsed.Data.Device != "eth0.2" || parsed.Data.Server != "Mock Test Edge" {
		t.Fatalf("unexpected device/server metadata: %+v", parsed.Data)
	}
	if parsed.Data.DownloadBytes != int64(len(payload)) || parsed.Data.DownloadMbps <= 0 {
		t.Fatalf("invalid download measurement: %+v", parsed.Data)
	}
	if !parsed.Data.UploadSupported || parsed.Data.UploadBytes <= 0 || parsed.Data.UploadMbps <= 0 || receivedUpload != parsed.Data.UploadBytes {
		t.Fatalf("invalid upload measurement: %+v received=%d", parsed.Data, receivedUpload)
	}
	if parsed.Data.LatencyMs <= 0 {
		t.Fatalf("expected positive latency_ms, got %v", parsed.Data.LatencyMs)
	}
}

func TestPublicHTTPClientBlocksPrivateDestinationsAndRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:8080/secret", http.StatusFound)
	}))
	defer srv.Close()

	client := newPublicHTTPClient(publicClientOptions{
		Timeout:   2 * time.Second,
		AllowHTTP: true,
	})
	if _, err := client.Get("http://127.0.0.1:1/test"); err == nil {
		t.Fatal("expected direct loopback URL to be blocked")
	}
	if _, err := client.Get("https://192.168.1.1/api"); err == nil {
		t.Fatal("expected RFC1918 LAN IP to be blocked")
	}
	if err := validatePublicHTTPURL(context.Background(), "http://169.254.169.254/latest/meta-data"); err == nil {
		t.Fatal("expected link-local metadata IP to be blocked")
	}
	if err := validatePublicHTTPURL(context.Background(), srv.URL); err == nil {
		t.Fatal("expected validatePublicHTTPURL to reject loopback server URL")
	}
	if got := sanitizeTelegramError("123456:ABCdef_SecretToken", errors.New(`Post "https://api.telegram.org/bot123456:ABCdef_SecretToken/sendMessage": EOF`)); strings.Contains(got, "ABCdef_SecretToken") {
		t.Fatalf("token leaked in sanitized error: %s", got)
	}
}
