package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGDriveValidationAndSecretMasking(t *testing.T) {
	validDirs := []string{"/mnt/sda1/gdrive", "/overlay/gdrive", "/overlay/gdrive/media/2026"}
	for _, d := range validDirs {
		clean, err := validateGDriveLocalDir(d)
		if err != nil {
			t.Errorf("expected local dir %q valid, got %v", d, err)
		}
		if !strings.HasPrefix(clean, "/") {
			t.Errorf("expected cleaned path %q to be POSIX rooted", clean)
		}
	}

	invalidDirs := []string{"", "/", "/etc/r3p", "/mnt", "/mnt/../etc", `C:\temp`, "/mnt/usb:evil"}
	for _, d := range invalidDirs {
		if _, err := validateGDriveLocalDir(d); err == nil {
			t.Errorf("expected invalid local dir %q to be rejected", d)
		}
	}

	if got := extractGDriveFolderID("https://drive.google.com/drive/folders/1AbCdEfGhIjKlMnOpQr_99?usp=sharing"); got != "1AbCdEfGhIjKlMnOpQr_99" {
		t.Fatalf("unexpected extracted Drive folder ID: %q", got)
	}

	maskedCfg := publicGDriveConfig(GDriveSyncConfig{
		Enabled:      true,
		AccessToken:  "ya29.secret_access_9876",
		RefreshToken: "1//refresh_secret_4321",
		ClientSecret: "GOCSPX-secret_1111",
		APIKey:       "AIzaSyDummyKey5678",
	})
	for _, field := range []string{maskedCfg.AccessToken, maskedCfg.RefreshToken, maskedCfg.ClientSecret, maskedCfg.APIKey} {
		if strings.Contains(field, "secret") || !strings.HasPrefix(field, "••••") {
			t.Fatalf("expected secret masked with •••• prefix, got %q", field)
		}
	}
}

func TestFileExplorerSecurityAndExtensions(t *testing.T) {
	if isImageFile(".svg") {
		t.Fatal("expected .svg NOT to be classified as previewable image (XSS safeguard)")
	}
	for _, img := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		if !isImageFile(img) {
			t.Errorf("expected %q to be image file", img)
		}
	}
	if !isFileEditable("network.conf") || !isFileEditable("hosts") || isFileEditable("video.mp4") || isFileEditable("logo.png") {
		t.Fatal("unexpected isFileEditable classification")
	}

	if clean := sanitizeExplorerPath(" \x00//overlay/share/../test.txt "); clean != "/overlay/test.txt" {
		t.Fatalf("unexpected sanitizeExplorerPath result: %q", clean)
	}

	sensitive := []string{
		"/etc/shadow",
		"/etc/dropbear/authorized_keys",
		"/etc/r3p/r3p.db",
		"/etc/r3p/r3p.db.lock",
		"/etc/r3p/api-token",
		"/etc/r3p/sessions.json",
		"/etc/r3p/ai.json",
		"/etc/r3p/ai_config.json",
		"/etc/r3p/gdrive_sync.json",
		"/overlay/upper/etc/r3p/r3p.db",
		"/overlay/upper/etc/r3p/telegram.json",
	}
	for _, p := range sensitive {
		if !isSensitiveExplorerPath(p) || !isProtectedExplorerPath(p) {
			t.Errorf("expected sensitive path %q to be blocked", p)
		}
	}

	protectedSys := []string{"/", "/etc", "/etc/r3p", "/overlay", "/proc/cpuinfo", "/sys/class/net"}
	for _, p := range protectedSys {
		if !isProtectedExplorerPath(p) {
			t.Errorf("expected system root/path %q to be protected", p)
		}
	}

	if hasSymlinkAncestor(filepath.Join(t.TempDir(), "regular.txt")) {
		t.Fatal("expected temp directory path without symlinks to return false")
	}
}

func TestGDriveRefreshPlaygroundBase64AndMidSyncRetry(t *testing.T) {
	if err := initSettingsDBAt(filepath.Join(t.TempDir(), "r3p.db")); err != nil {
		t.Fatalf("initSettingsDBAt failed: %v", err)
	}
	defer closeSettingsDB()

	origOAuth := gdriveOAuthTokenEndpoint
	origPg := gdrivePlaygroundRefreshEndpoint
	origAPI := gdriveAPIBaseURL
	gdriveConfigLock.RLock()
	origCfg := currentGDriveCfg
	gdriveConfigLock.RUnlock()
	defer func() {
		gdriveOAuthTokenEndpoint = origOAuth
		gdrivePlaygroundRefreshEndpoint = origPg
		gdriveAPIBaseURL = origAPI
		gdriveConfigLock.Lock()
		currentGDriveCfg = origCfg
		gdriveConfigLock.Unlock()
	}()

	var pgCalls int32
	pgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&pgCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			// Transient 502 first, should be retried once and succeed on call 2 via base64 message-body
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
		case 2:
			inner := `{"access_token":"ya29.pg_fresh_tok","refresh_token":"1//rotated_refresh","expires_in":3600}`
			b64 := base64.StdEncoding.EncodeToString([]byte(inner))
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success":     true,
				"Status-Code": 200,
				"Response": map[string]interface{}{
					"Status-Code":  200,
					"message-body": b64,
				},
			})
		default:
			// HTTP 200 with Status-Code 400 invalid_grant (real Google Playground 24h revocation envelope)
			inner := `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`
			b64 := base64.StdEncoding.EncodeToString([]byte(inner))
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success":     true,
				"Status-Code": 400,
				"error":       "invalid_grant: Token has been expired or revoked.",
				"Response": map[string]interface{}{
					"Status-Code":  400,
					"message-body": b64,
				},
			})
		}
	}))
	defer pgServer.Close()

	var oauthCalls int32
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&oauthCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ya29.custom_oauth_tok","expires_in":3599}`))
	}))
	defer oauthServer.Close()

	gdrivePlaygroundRefreshEndpoint = pgServer.URL
	gdriveOAuthTokenEndpoint = oauthServer.URL

	// 1. Playground transient retry + base64 message-body decode + rotated refresh_token
	gdriveConfigLock.Lock()
	currentGDriveCfg = GDriveSyncConfig{
		AccessToken:  "ya29.old_expired",
		RefreshToken: "1//initial_pg_refresh",
	}
	gdriveConfigLock.Unlock()

	tok, err := refreshGDriveToken()
	if err != nil || tok != "ya29.pg_fresh_tok" {
		t.Fatalf("expected ya29.pg_fresh_tok after 502 retry + base64 decode, got tok=%q err=%v", tok, err)
	}
	gdriveConfigLock.RLock()
	if currentGDriveCfg.RefreshToken != "1//rotated_refresh" || currentGDriveCfg.TokenAuthMode != "playground" || currentGDriveCfg.TokenExpiresAt == 0 {
		gdriveConfigLock.RUnlock()
		t.Fatalf("unexpected updated gdrive config after refresh: %+v", currentGDriveCfg)
	}
	gdriveConfigLock.RUnlock()

	// 2. HTTP 200 Playground invalid_grant surfaces actionable 24h expiry diagnostic
	if _, err := refreshGDriveToken(); err == nil || !strings.Contains(err.Error(), "24 giờ") {
		t.Fatalf("expected Playground 24h revocation explanation, got err=%v", err)
	}
	gdriveConfigLock.RLock()
	if !strings.Contains(currentGDriveCfg.LastRefreshError, "invalid_grant") {
		gdriveConfigLock.RUnlock()
		t.Fatalf("expected LastRefreshError saved, got %q", currentGDriveCfg.LastRefreshError)
	}
	gdriveConfigLock.RUnlock()

	// 3. Mid-sync 401 triggers custom OAuth2 refresh and retries operation transparently
	gdriveConfigLock.Lock()
	currentGDriveCfg.AccessToken = "ya29.stale_mid_sync"
	currentGDriveCfg.ClientID = "123.apps.googleusercontent.com"
	currentGDriveCfg.ClientSecret = "GOCSPX-testsecret"
	gdriveConfigLock.Unlock()

	activeTok := "ya29.stale_mid_sync"
	attempts := 0
	err = retryOnGDriveAuth(&activeTok, "1//rotated_refresh", func(useTok string) error {
		attempts++
		if useTok == "ya29.stale_mid_sync" {
			return &gdriveTestHTTPError{"HTTP 401: Invalid Credentials"}
		}
		return nil
	})
	if err != nil || attempts != 2 || activeTok != "ya29.custom_oauth_tok" {
		t.Fatalf("expected mid-sync 401 retry with custom oauth token, got attempts=%d tok=%q err=%v", attempts, activeTok, err)
	}
}

type gdriveTestHTTPError struct {
	msg string
}

func (e *gdriveTestHTTPError) Error() string { return e.msg }
