package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsDBInitPersistMigrateAndSummary(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "r3p.db")
	legacyDDNS := filepath.Join(tmp, "ddns_legacy.json")
	if err := os.WriteFile(legacyDDNS, []byte(`{"enabled":true,"provider":"duckdns","domain":"testr3p","token":"tok-123"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := initSettingsDBAt(dbPath); err != nil {
		t.Fatalf("initSettingsDBAt failed: %v", err)
	}
	defer closeSettingsDB()

	type sampleCfg struct {
		Name   string `json:"name"`
		Secret string `json:"secret,omitempty"`
	}
	if err := settingsDBSaveJSON(bucketSettings, "adblock", "", 0644, sampleCfg{Name: "adblock-v1"}); err != nil {
		t.Fatalf("settingsDBSaveJSON adblock failed: %v", err)
	}
	if err := settingsDBSaveJSON(bucketSettings, "gdrive_sync", "", 0600, sampleCfg{Name: "gd", Secret: "ULTRA_SECRET_TOKEN"}); err != nil {
		t.Fatalf("settingsDBSaveJSON gdrive_sync failed: %v", err)
	}

	// Re-open DB and confirm persistence
	_ = closeSettingsDB()
	if err := initSettingsDBAt(dbPath); err != nil {
		t.Fatalf("re-open DB failed: %v", err)
	}
	var got sampleCfg
	if !settingsDBLoadJSON(bucketSettings, "adblock", "", &got) || got.Name != "adblock-v1" {
		t.Fatalf("expected persisted adblock setting, got %+v", got)
	}

	// Lazy backfill from fallback file when key is not in DB yet
	var ddnsLoaded DDNSConfig
	if !settingsDBLoadJSON(bucketSettings, "custom_ddns_probe", legacyDDNS, &ddnsLoaded) || ddnsLoaded.Domain != "testr3p" {
		t.Fatalf("expected lazy JSON migration to populate DDNSConfig, got %+v", ddnsLoaded)
	}

	// Public snapshot must NEVER include secret keys like gdrive_sync
	snap, err := buildPublicSettingsDBSnapshotJSON()
	if err != nil {
		t.Fatalf("buildPublicSettingsDBSnapshotJSON failed: %v", err)
	}
	if strings.Contains(string(snap), "ULTRA_SECRET_TOKEN") || strings.Contains(string(snap), "gdrive_sync") {
		t.Fatalf("public DB snapshot leaked secret entry: %s", string(snap))
	}
	if !strings.Contains(string(snap), "adblock-v1") {
		t.Fatalf("public DB snapshot missing adblock config: %s", string(snap))
	}

	summary := getSettingsDBSummary()
	if !summary.Initialized || summary.Engine != "bbolt" || summary.TotalKeys < 3 {
		t.Fatalf("unexpected settings DB summary: %+v", summary)
	}

	// UI Preferences HTTP handler roundtrip
	putBody := bytes.NewBufferString(`{"theme":"dark","sidebar_collapsed":true,"default_tab":"/nas","preferences":{"density":"compact"}}`)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/ui/preferences", putBody)
	putRec := httptest.NewRecorder()
	handleUIPreferences(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/ui/preferences status=%d body=%s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/ui/preferences", nil)
	getRec := httptest.NewRecorder()
	handleUIPreferences(getRec, getReq)
	var envelope struct {
		Ok   bool          `json:"ok"`
		Data UIPreferences `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &envelope); err != nil || !envelope.Ok || !envelope.Data.SidebarCollapsed || envelope.Data.DefaultTab != "/nas" {
		t.Fatalf("GET /api/v1/ui/preferences unexpected response: %s (err=%v)", getRec.Body.String(), err)
	}
}
