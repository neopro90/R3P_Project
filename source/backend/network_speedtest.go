package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var speedtestMu sync.Mutex

type speedtestTarget struct {
	Name         string
	LatencyURL   string
	DownloadURL  string
	UploadURL    string
	UploadServer string
}

type speedtestUbusStatus struct {
	Up          bool   `json:"up"`
	L3Device    string `json:"l3_device"`
	Device      string `json:"device"`
	IPv4Address []struct {
		Address string `json:"address"`
		Mask    int    `json:"mask"`
	} `json:"ipv4-address"`
}

type speedtestEnv struct {
	requireOpenWrt func(http.ResponseWriter) bool
	queryUbus      func(iface string) (speedtestUbusStatus, error)
	newClient      func(localIP string, timeout time.Duration) *http.Client
	targets        []speedtestTarget
}

func defaultSpeedtestEnv() speedtestEnv {
	return speedtestEnv{
		requireOpenWrt: requireOpenWrt,
		queryUbus:      queryOpenWrtInterfaceStatus,
		newClient: func(localIP string, timeout time.Duration) *http.Client {
			return newPublicHTTPClient(publicClientOptions{
				Timeout:            timeout,
				DialTimeout:        5 * time.Second,
				LocalIP:            localIP,
				DisableCompression: true,
			})
		},
		targets: []speedtestTarget{
			{
				Name:         "Cloudflare Edge",
				LatencyURL:   "https://speed.cloudflare.com/__down?bytes=0",
				DownloadURL:  "https://speed.cloudflare.com/__down?bytes=10000000",
				UploadURL:    "https://speed.cloudflare.com/__up",
				UploadServer: "Cloudflare Edge",
			},
			{
				Name:         "Hetzner Singapore",
				LatencyURL:   "https://sin-speed.hetzner.com/100MB.bin",
				DownloadURL:  "https://sin-speed.hetzner.com/100MB.bin",
				UploadURL:    "https://speed.cloudflare.com/__up",
				UploadServer: "Cloudflare Edge",
			},
			{
				Name:         "Hetzner Falkenstein",
				LatencyURL:   "https://fsn1-speed.hetzner.com/100MB.bin",
				DownloadURL:  "https://fsn1-speed.hetzner.com/100MB.bin",
				UploadURL:    "https://speed.cloudflare.com/__up",
				UploadServer: "Cloudflare Edge",
			},
			{
				Name:         "OVH Proof Mirror",
				LatencyURL:   "https://proof.ovh.net/files/10Mb.dat",
				DownloadURL:  "https://proof.ovh.net/files/10Mb.dat",
				UploadURL:    "https://speed.cloudflare.com/__up",
				UploadServer: "Cloudflare Edge",
			},
		},
	}
}

func queryOpenWrtInterfaceStatus(iface string) (speedtestUbusStatus, error) {
	var st speedtestUbusStatus
	ubusOut, err := runCmd(3, "ubus", "call", "network.interface."+iface, "status")
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal([]byte(ubusOut), &st); err != nil {
		return st, err
	}
	return st, nil
}

// zeroRAMPatternReader streams targetBytes of incompressible pseudo-random seed
// data using a small fixed 32KB pattern ring so uploads use negligible RAM on MT7621.
type zeroRAMPatternReader struct {
	ctx       context.Context
	remaining int64
	pattern   []byte
	offset    int
}

func newZeroRAMPatternReader(ctx context.Context, size int64) *zeroRAMPatternReader {
	pat := make([]byte, 32*1024)
	if _, err := rand.Read(pat); err != nil {
		for i := range pat {
			pat[i] = byte((i*31 + 17) & 0xff)
		}
	}
	return &zeroRAMPatternReader{
		ctx:       ctx,
		remaining: size,
		pattern:   pat,
	}
}

func (z *zeroRAMPatternReader) Read(p []byte) (int, error) {
	if z.remaining <= 0 {
		return 0, io.EOF
	}
	if z.ctx != nil && z.ctx.Err() != nil {
		return 0, z.ctx.Err()
	}
	if int64(len(p)) > z.remaining {
		p = p[:z.remaining]
	}
	copied := 0
	for copied < len(p) {
		n := copy(p[copied:], z.pattern[z.offset:])
		copied += n
		z.offset = (z.offset + n) % len(z.pattern)
	}
	z.remaining -= int64(copied)
	return copied, nil
}

// POST /api/v1/network/speedtest?interface=wwan - Measures latency, jitter, download and upload
func handleInterfaceSpeedtest(w http.ResponseWriter, r *http.Request) {
	handleInterfaceSpeedtestWithEnv(w, r, defaultSpeedtestEnv())
}

func handleInterfaceSpeedtestWithEnv(w http.ResponseWriter, r *http.Request, env speedtestEnv) {
	if env.requireOpenWrt != nil && !env.requireOpenWrt(w) {
		return
	}
	if !speedtestMu.TryLock() {
		writeJSON(w, http.StatusConflict, false, "Đang có tiến trình đo tốc độ mạng khác chạy, vui lòng đợi hoàn tất")
		return
	}
	defer speedtestMu.Unlock()

	iface := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("interface")))
	if iface == "" {
		iface = "wwan"
	}
	if !safeTokenKeyPattern.MatchString(iface) || strings.HasPrefix(iface, "-") {
		writeJSON(w, http.StatusBadRequest, false, "Tên interface không hợp lệ")
		return
	}

	ubusData, err := env.queryUbus(iface)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Không tìm thấy hoặc không thể truy vấn cổng %s trên ubus", strings.ToUpper(iface)))
		return
	}
	if !ubusData.Up {
		writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Cổng %s hiện đang ngắt kết nối (down), không thể đo tốc độ", strings.ToUpper(iface)))
		return
	}

	dev := iface
	if ubusData.L3Device != "" && safeTokenKeyPattern.MatchString(ubusData.L3Device) {
		dev = ubusData.L3Device
	} else if ubusData.Device != "" && safeTokenKeyPattern.MatchString(ubusData.Device) {
		dev = ubusData.Device
	}

	localIP := ""
	for _, a := range ubusData.IPv4Address {
		if parsed := net.ParseIP(strings.TrimSpace(a.Address)); parsed != nil && parsed.To4() != nil {
			localIP = parsed.To4().String()
			break
		}
	}
	if localIP == "" && dev != "" {
		if netIf, ifErr := net.InterfaceByName(dev); ifErr == nil {
			if addrs, addrErr := netIf.Addrs(); addrErr == nil {
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok {
						if v4 := ipnet.IP.To4(); v4 != nil && !v4.IsLoopback() {
							localIP = v4.String()
							break
						}
					}
				}
			}
		}
	}

	startTotal := time.Now()
	parentCtx := r.Context()

	var selected speedtestTarget
	var latencyMs, jitterMs float64
	var downMbps float64
	var dlBytes int64
	var dlSuccess bool

	// Reused 32KB sink buffer per test
	sinkBuf := make([]byte, 32*1024)

	for _, target := range env.targets {
		if parentCtx.Err() != nil {
			writeJSON(w, http.StatusRequestTimeout, false, "Đã huỷ phép đo tốc độ")
			return
		}

		// 1. Latency + Jitter (1 warmup + up to 4 keep-alive RTT samples)
		probeClient := env.newClient(localIP, 4*time.Second)
		if target.LatencyURL != "" {
			lat, jit := measureLatencyAndJitter(parentCtx, probeClient, target.LatencyURL)
			if lat > 0 {
				latencyMs = lat
				jitterMs = jit
			}
		}
		probeClient.CloseIdleConnections()

		// 2. Streaming Download (capped at 8 seconds so MT7621 remains responsive)
		dlCtx, dlCancel := context.WithTimeout(parentCtx, 8*time.Second)
		dlClient := env.newClient(localIP, 10*time.Second)
		req, reqErr := http.NewRequestWithContext(dlCtx, http.MethodGet, target.DownloadURL, nil)
		if reqErr == nil {
			req.Header.Set("User-Agent", "R3P-Speedtest/2.0")
			req.Header.Set("Cache-Control", "no-store")
			resp, respErr := dlClient.Do(req)
			if respErr == nil {
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					dlStart := time.Now()
					var nTotal int64
					for {
						n, readErr := resp.Body.Read(sinkBuf)
						if n > 0 {
							nTotal += int64(n)
						}
						if readErr != nil {
							break
						}
					}
					elapsed := time.Since(dlStart)
					resp.Body.Close()
					dlCancel()
					dlClient.CloseIdleConnections()

					if nTotal >= 16384 && elapsed >= 10*time.Millisecond {
						downMbps = (float64(nTotal) * 8.0) / (elapsed.Seconds() * 1_000_000.0)
						dlBytes = nTotal
						selected = target
						dlSuccess = true
						if latencyMs <= 0 {
							latencyMs = mathRound(elapsed.Seconds()*1000.0, 1)
						}
						break
					}
					log.Printf("[speedtest] %s short sample (%d bytes in %s), trying next mirror", target.Name, nTotal, elapsed.Round(time.Millisecond))
					continue
				} else {
					log.Printf("[speedtest] %s HTTP %d on download", target.Name, resp.StatusCode)
					resp.Body.Close()
				}
			} else {
				log.Printf("[speedtest] %s download request error: %v", target.Name, respErr)
			}
		}
		dlCancel()
		dlClient.CloseIdleConnections()
	}

	if !dlSuccess || downMbps <= 0 {
		writeJSON(w, http.StatusBadGateway, false, fmt.Sprintf("Không thể đo băng thông tải xuống qua cổng %s (%s): kiểm tra kết nối Internet hoặc DNS của cổng này", strings.ToUpper(iface), dev))
		return
	}

	// 3. Adaptive Streaming Upload (approx 2.5s window, between 512KB and 6MB)
	upMbps := 0.0
	var ulBytes int64
	uploadSupported := false

	uploadURL := selected.UploadURL
	if uploadURL == "" && len(env.targets) > 0 {
		uploadURL = env.targets[0].UploadURL
	}
	if uploadURL != "" && parentCtx.Err() == nil {
		// Target ~2.5s of upload based on conservative 40% of downMbps (or 1MB default)
		estBytes := int64(downMbps * 125_000 * 0.4 * 2.5)
		if estBytes < 512*1024 {
			estBytes = 512 * 1024
		}
		if estBytes > 6*1024*1024 {
			estBytes = 6 * 1024 * 1024
		}

		// Try adaptive upload, falling back to 512KB if asymmetric WAN times out
		for idx, trySize := range []int64{estBytes, 512 * 1024} {
			if idx > 0 && estBytes <= 512*1024 {
				break
			}
			ulCtx, ulCancel := context.WithTimeout(parentCtx, 8*time.Second)
			ulClient := env.newClient(localIP, 9*time.Second)
			// Pre-warm TCP+TLS connection to upload host before measuring payload transmission
			warmupURL := ""
			if strings.Contains(uploadURL, "speed.cloudflare.com") {
				warmupURL = "https://speed.cloudflare.com/__down?bytes=0"
			} else if selected.LatencyURL != "" && strings.Contains(selected.LatencyURL, "bytes=0") {
				warmupURL = selected.LatencyURL
			}
			if warmupURL != "" {
				if wReq, wErr := http.NewRequestWithContext(ulCtx, http.MethodGet, warmupURL, nil); wErr == nil {
					if wResp, wResErr := ulClient.Do(wReq); wResErr == nil {
						_, _ = io.Copy(io.Discard, io.LimitReader(wResp.Body, 4096))
						wResp.Body.Close()
					}
				}
			}

			bodyReader := newZeroRAMPatternReader(ulCtx, trySize)
			req, reqErr := http.NewRequestWithContext(ulCtx, http.MethodPost, uploadURL, bodyReader)
			if reqErr == nil {
				req.ContentLength = trySize
				req.Header.Set("Content-Type", "application/octet-stream")
				req.Header.Set("User-Agent", "R3P-Speedtest/2.0")
				ulStart := time.Now()
				resp, respErr := ulClient.Do(req)
				elapsed := time.Since(ulStart)
				if respErr == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
					resp.Body.Close()
					if resp.StatusCode >= 200 && resp.StatusCode < 300 && elapsed >= 20*time.Millisecond {
						ulBytes = trySize
						upMbps = (float64(trySize) * 8.0) / (elapsed.Seconds() * 1_000_000.0)
						uploadSupported = true
						ulCancel()
						ulClient.CloseIdleConnections()
						break
					}
					log.Printf("[speedtest] upload HTTP %d for %d bytes (%s)", resp.StatusCode, trySize, elapsed.Round(time.Millisecond))
				} else if !errors.Is(respErr, context.DeadlineExceeded) {
					log.Printf("[speedtest] upload error for %d bytes: %v", trySize, respErr)
					ulCancel()
					ulClient.CloseIdleConnections()
					break
				}
			}
			ulCancel()
			ulClient.CloseIdleConnections()
		}
	}

	uploadServer := selected.UploadServer
	if uploadServer == "" {
		uploadServer = selected.Name
		if strings.Contains(uploadURL, "speed.cloudflare.com") {
			uploadServer = "Cloudflare Edge"
		}
	}

	totalMs := time.Since(startTotal).Milliseconds()
	downRounded := mathRound(downMbps, 2)
	upRounded := mathRound(upMbps, 2)
	latRounded := mathRound(latencyMs, 1)
	jitRounded := mathRound(jitterMs, 1)

	msg := fmt.Sprintf("Đo tốc độ %s (%s): ↓ %.1f Mbps · ↑ %.1f Mbps · Ping %.1f ms (Jitter %.1f ms)",
		strings.ToUpper(iface), dev, downRounded, upRounded, latRounded, jitRounded)
	if !uploadSupported {
		msg = fmt.Sprintf("Đo tốc độ %s (%s): ↓ %.1f Mbps · Ping %.1f ms (Upload không khả dụng)",
			strings.ToUpper(iface), dev, downRounded, latRounded)
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"interface":        iface,
		"device":           dev,
		"local_ip":         localIP,
		"server":           selected.Name,
		"upload_server":    uploadServer,
		"download_mbps":    downRounded,
		"upload_mbps":      upRounded,
		"upload_supported": uploadSupported,
		"latency_ms":       latRounded,
		"jitter_ms":        jitRounded,
		"download_bytes":   dlBytes,
		"upload_bytes":     ulBytes,
		"duration_ms":      totalMs,
		"tested_at":        time.Now().Format("15:04:05"),
		"message":          msg,
	})
}

func measureLatencyAndJitter(parent context.Context, client *http.Client, targetURL string) (float64, float64) {
	useRangeByteProbe := !strings.Contains(targetURL, "bytes=0")
	var samples []float64
	// 1 warmup + 4 measured probes
	for i := 0; i < 5; i++ {
		if parent.Err() != nil {
			break
		}
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			cancel()
			continue
		}
		if useRangeByteProbe {
			req.Header.Set("Range", "bytes=0-0")
		}
		req.Header.Set("Cache-Control", "no-store")
		t0 := time.Now()
		resp, err := client.Do(req)
		rttMs := float64(time.Since(t0).Microseconds()) / 1000.0
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
		}
		cancel()
		if err != nil || resp == nil || resp.StatusCode >= 400 {
			continue
		}
		if i > 0 || len(samples) == 0 {
			samples = append(samples, rttMs)
		}
	}
	// Drop initial handshake warmup if we collected at least 2 follow-up RTTs
	if len(samples) >= 3 {
		samples = samples[1:]
	}
	if len(samples) == 0 {
		return 0, 0
	}
	var jitter float64
	if len(samples) > 1 {
		var diffSum float64
		for i := 1; i < len(samples); i++ {
			diffSum += math.Abs(samples[i] - samples[i-1])
		}
		jitter = diffSum / float64(len(samples)-1)
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	return median, jitter
}

func mathRound(val float64, precision int) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
