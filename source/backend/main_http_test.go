package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGzipMiddlewareAndSPACacheHeaders(t *testing.T) {
	staticDir := t.TempDir()
	assetsDir := filepath.Join(staticDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		t.Fatal(err)
	}
	indexHTML := "<!doctype html><html><body>R3P Dashboard</body></html>"
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte(indexHTML), 0644); err != nil {
		t.Fatal(err)
	}
	jsPayload := strings.Repeat("console.log('r3p-router-asset');\n", 30)
	if err := os.WriteFile(filepath.Join(assetsDir, "app-ABC123.js"), []byte(jsPayload), 0644); err != nil {
		t.Fatal(err)
	}
	pngStub := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	if err := os.WriteFile(filepath.Join(assetsDir, "logo.png"), pngStub, 0644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, true, map[string]string{
			"status": "ok",
			"detail": strings.Repeat("telemetry-json-payload-", 20),
		})
	})
	mux.HandleFunc("/api/v1/monitor/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"ok\":true}\n\n"))
	})
	mux.Handle("/", spaHandler{dir: staticDir})

	handler := gzipMiddleware(mux)

	// 1. Hashed asset JS gets immutable cache + gzip compression
	req := httptest.NewRequest(http.MethodGet, "/assets/app-ABC123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for JS asset, got %d", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected Cache-Control for /assets/: %q", got)
	}
	if got := rr.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected gzip Content-Encoding for JS asset, got %q", got)
	}
	gr, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatalf("failed to open gzip reader: %v", err)
	}
	decompressed, _ := io.ReadAll(gr)
	_ = gr.Close()
	if string(decompressed) != jsPayload {
		t.Fatalf("decompressed JS mismatch: got len=%d want len=%d", len(decompressed), len(jsPayload))
	}

	// 2. SPA fallback gets no-cache + gzip
	reqSPA := httptest.NewRequest(http.MethodGet, "/wireless", nil)
	reqSPA.Header.Set("Accept-Encoding", "gzip")
	rrSPA := httptest.NewRecorder()
	handler.ServeHTTP(rrSPA, reqSPA)
	if rrSPA.Code != http.StatusOK {
		t.Fatalf("expected 200 for SPA fallback, got %d", rrSPA.Code)
	}
	if got := rrSPA.Header().Get("Cache-Control"); got != "no-cache, must-revalidate" {
		t.Fatalf("unexpected Cache-Control for SPA index fallback: %q", got)
	}
	if got := rrSPA.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected gzip Content-Encoding for SPA HTML, got %q", got)
	}

	// 3. HEAD request must NOT emit gzip body or Content-Encoding: gzip
	reqHead := httptest.NewRequest(http.MethodHead, "/assets/app-ABC123.js", nil)
	reqHead.Header.Set("Accept-Encoding", "gzip")
	rrHead := httptest.NewRecorder()
	handler.ServeHTTP(rrHead, reqHead)
	if rrHead.Body.Len() != 0 {
		t.Fatalf("HEAD request must return empty body, got %d bytes", rrHead.Body.Len())
	}

	// 4. SSE stream must NOT be gzip-buffered
	reqSSE := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/stream", nil)
	reqSSE.Header.Set("Accept-Encoding", "gzip")
	rrSSE := httptest.NewRecorder()
	handler.ServeHTTP(rrSSE, reqSSE)
	if got := rrSSE.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("SSE stream must not be gzipped, got %q", got)
	}

	// 5. Binary image must NOT be gzipped
	reqPNG := httptest.NewRequest(http.MethodGet, "/assets/logo.png", nil)
	reqPNG.Header.Set("Accept-Encoding", "gzip")
	rrPNG := httptest.NewRecorder()
	handler.ServeHTTP(rrPNG, reqPNG)
	if got := rrPNG.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("PNG asset must not be gzipped, got %q", got)
	}

	// 6. JSON API response gets gzipped cleanly
	reqAPI := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	reqAPI.Header.Set("Accept-Encoding", "gzip")
	rrAPI := httptest.NewRecorder()
	handler.ServeHTTP(rrAPI, reqAPI)
	if got := rrAPI.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected gzip Content-Encoding for /api/v1/health, got %q", got)
	}
}
