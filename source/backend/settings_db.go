package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

const (
	settingsDBPrimaryPath = "/etc/r3p/r3p.db"

	bucketSettings    = "settings"
	bucketCollections = "collections"
	bucketSystemMeta  = "system_meta"
	bucketAuditLogs   = "audit_logs"
)

var (
	settingsDBMu   sync.RWMutex
	settingsDB     *bbolt.DB
	settingsDBPath string
)

type SettingsDBKeyInfo struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	SizeBytes int    `json:"size_bytes"`
}

type SettingsDBSummary struct {
	Path               string              `json:"path"`
	Engine             string              `json:"engine"`
	Initialized        bool                `json:"initialized"`
	SizeBytes          int64               `json:"size_bytes"`
	SchemaVersion      int                 `json:"schema_version"`
	MigratedAt         string              `json:"migrated_at,omitempty"`
	LastMigrationCount int                 `json:"last_migration_count"`
	Buckets            map[string]int      `json:"buckets"`
	TotalKeys          int                 `json:"total_keys"`
	Keys               []SettingsDBKeyInfo `json:"keys"`
}

type UIPreferences struct {
	Theme            string                 `json:"theme,omitempty"`
	SidebarCollapsed bool                   `json:"sidebar_collapsed,omitempty"`
	DefaultTab       string                 `json:"default_tab,omitempty"`
	Preferences      map[string]interface{} `json:"preferences,omitempty"`
	UpdatedAt        string                 `json:"updated_at,omitempty"`
}

type legacyDBMigrationItem struct {
	Bucket   string
	Key      string
	FilePath string
	IsJSON   bool
}

var legacyMigrationCatalog = []legacyDBMigrationItem{
	{Bucket: bucketSettings, Key: "adblock", FilePath: "/etc/r3p/adblock.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "ai_config", FilePath: "/etc/r3p/ai_config.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "ai_memory", FilePath: "/etc/r3p/ai_memory.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "ai_sessions", FilePath: "/etc/r3p/ai_sessions.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "ai_quick_commands", FilePath: "/etc/r3p/quick_commands.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "app_repos", FilePath: "/etc/r3p/app_repos.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "ddns", FilePath: "/etc/r3p/ddns.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "downloads_state", FilePath: "/etc/r3p/downloads_state.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "downloader", FilePath: "/etc/r3p/downloader.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "downloader_queue", FilePath: "/etc/r3p/downloader-queue.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "downloader_schedules", FilePath: "/etc/r3p/downloader-schedules.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "gdrive_sync", FilePath: "/etc/r3p/gdrive_sync.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "iot_devices", FilePath: "/etc/r3p/iot_devices.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "iot_broker", FilePath: "/etc/r3p/iot_broker.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "plugins", FilePath: "/etc/r3p/plugins.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "saved_wifi", FilePath: "/etc/r3p/saved-wifi.json", IsJSON: true},
	{Bucket: bucketCollections, Key: "shares", FilePath: "/etc/r3p/shares.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "telegram", FilePath: "/etc/r3p/telegram.json", IsJSON: true},
	{Bucket: bucketSettings, Key: "version", FilePath: "/etc/r3p/version.json", IsJSON: true},
	{Bucket: bucketSystemMeta, Key: "ai_blueprint", FilePath: "/etc/r3p/SYSTEM_BLUEPRINT.md", IsJSON: false},
}

func resolveSettingsDBPath() string {
	if envPath := strings.TrimSpace(os.Getenv("R3P_SETTINGS_DB")); envPath != "" {
		return envPath
	}
	if st, err := os.Stat("/etc/openwrt_release"); err == nil && !st.IsDir() {
		return settingsDBPrimaryPath
	}
	if err := os.MkdirAll("/etc/r3p", 0755); err == nil {
		probe := filepath.Join("/etc/r3p", ".r3p_write_probe")
		if f, wErr := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600); wErr == nil {
			_ = f.Close()
			_ = os.Remove(probe)
			return settingsDBPrimaryPath
		}
	}
	return filepath.Join(os.TempDir(), "r3p-dev.db")
}

func initSettingsDB() error {
	return initSettingsDBAt(resolveSettingsDBPath())
}

func initSettingsDBAt(dbPath string) error {
	settingsDBMu.Lock()
	defer settingsDBMu.Unlock()

	if settingsDB != nil {
		_ = settingsDB.Close()
		settingsDB = nil
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return err
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		for _, b := range []string{bucketSettings, bucketCollections, bucketSystemMeta, bucketAuditLogs} {
			if _, cErr := tx.CreateBucketIfNotExists([]byte(b)); cErr != nil {
				return cErr
			}
		}
		meta := tx.Bucket([]byte(bucketSystemMeta))
		if meta.Get([]byte("schema_version")) == nil {
			_ = meta.Put([]byte("schema_version"), []byte("1"))
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return err
	}

	settingsDB = db
	settingsDBPath = dbPath
	_, _ = migrateLegacyJSONToDBLocked(db, false)
	return nil
}

func closeSettingsDB() error {
	settingsDBMu.Lock()
	defer settingsDBMu.Unlock()
	if settingsDB == nil {
		return nil
	}
	err := settingsDB.Close()
	settingsDB = nil
	return err
}

func syncSettingsDB() error {
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	if db == nil {
		return nil
	}
	return db.Sync()
}

func migrateLegacyJSONToDBLocked(db *bbolt.DB, overwriteExisting bool) (int, error) {
	if db == nil {
		return 0, nil
	}
	migrated := 0
	var migratedKeys []string

	err := db.Update(func(tx *bbolt.Tx) error {
		for _, item := range legacyMigrationCatalog {
			bkt := tx.Bucket([]byte(item.Bucket))
			if bkt == nil {
				var cErr error
				bkt, cErr = tx.CreateBucketIfNotExists([]byte(item.Bucket))
				if cErr != nil {
					return cErr
				}
			}
			if !overwriteExisting && bkt.Get([]byte(item.Key)) != nil {
				continue
			}
			raw, rErr := os.ReadFile(item.FilePath)
			if rErr != nil || len(bytes.TrimSpace(raw)) == 0 {
				continue
			}
			raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
			if item.IsJSON && !json.Valid(raw) {
				continue
			}
			if pErr := bkt.Put([]byte(item.Key), raw); pErr != nil {
				return pErr
			}
			migrated++
			migratedKeys = append(migratedKeys, item.Bucket+"/"+item.Key)
		}

		meta := tx.Bucket([]byte(bucketSystemMeta))
		if meta != nil && migrated > 0 {
			now := time.Now().Format(time.RFC3339Nano)
			_ = meta.Put([]byte("migrated_at"), []byte(now))
			_ = meta.Put([]byte("last_migration_count"), []byte(strconv.Itoa(migrated)))
			if logs := tx.Bucket([]byte(bucketAuditLogs)); logs != nil {
				entry, _ := json.Marshal(map[string]interface{}{
					"event": "legacy_json_migration",
					"count": migrated,
					"keys":  migratedKeys,
					"at":    now,
				})
				_ = logs.Put([]byte(now), entry)
			}
		}
		return nil
	})
	return migrated, err
}

var safePublicDBSnapshotKeys = map[string]bool{
	bucketSettings + "/adblock":               true,
	bucketSettings + "/ui_preferences":        true,
	bucketSettings + "/version":               true,
	bucketSettings + "/iot_broker":            true,
	bucketCollections + "/app_repos":          true,
	bucketCollections + "/plugins":            true,
	bucketCollections + "/shares":             true,
	bucketCollections + "/iot_devices":        true,
	bucketCollections + "/ai_memory":          true,
	bucketCollections + "/ai_quick_commands":  true,
	bucketCollections + "/downloader_schedules": true,
}

func importPublicSettingsDBSnapshot(raw []byte) (int, error) {
	trimmed := bytes.TrimPrefix(bytes.TrimSpace(raw), []byte("\xef\xbb\xbf"))
	if len(trimmed) == 0 {
		return 0, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return 0, err
	}
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	if db == nil {
		return 0, os.ErrNotExist
	}
	imported := 0
	var keys []string
	err := db.Update(func(tx *bbolt.Tx) error {
		for fullKey, val := range payload {
			if !safePublicDBSnapshotKeys[fullKey] {
				continue
			}
			clean := bytes.TrimSpace(val)
			if len(clean) == 0 || len(clean) > 2<<20 || !json.Valid(clean) {
				continue
			}
			parts := strings.SplitN(fullKey, "/", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				continue
			}
			bkt, err := tx.CreateBucketIfNotExists([]byte(parts[0]))
			if err != nil {
				return err
			}
			if err := bkt.Put([]byte(parts[1]), clean); err != nil {
				return err
			}
			imported++
			keys = append(keys, fullKey)
		}
		if imported > 0 {
			if logs, err := tx.CreateBucketIfNotExists([]byte(bucketAuditLogs)); err == nil {
				now := time.Now().Format(time.RFC3339Nano)
				entry, _ := json.Marshal(map[string]interface{}{
					"event": "snapshot_restore",
					"count": imported,
					"keys":  keys,
					"at":    now,
				})
				_ = logs.Put([]byte(now), entry)
			}
		}
		return nil
	})
	return imported, err
}

func settingsDBGetRaw(bucket, key string) ([]byte, bool) {
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	if db == nil {
		return nil, false
	}
	var out []byte
	found := false
	_ = db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return nil
		}
		v := bkt.Get([]byte(key))
		if v != nil {
			out = append([]byte(nil), v...)
			found = true
		}
		return nil
	})
	return out, found
}

func settingsDBPutRaw(bucket, key string, data []byte) error {
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	if db == nil {
		return os.ErrNotExist
	}
	return db.Update(func(tx *bbolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return bkt.Put([]byte(key), data)
	})
}

func settingsDBDelete(bucket, key string) error {
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	if db == nil {
		return nil
	}
	return db.Update(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return nil
		}
		return bkt.Delete([]byte(key))
	})
}

func settingsDBLoadJSON(bucket, key string, fallbackFile string, dst interface{}) bool {
	if raw, ok := settingsDBGetRaw(bucket, key); ok && len(bytes.TrimSpace(raw)) > 0 {
		trimmed := bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
		if json.Unmarshal(trimmed, dst) == nil {
			return true
		}
	}
	if strings.TrimSpace(fallbackFile) != "" {
		if raw, err := os.ReadFile(fallbackFile); err == nil && len(bytes.TrimSpace(raw)) > 0 {
			trimmed := bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
			if json.Unmarshal(trimmed, dst) == nil {
				// Lazily backfill into bbolt if DB is open
				_ = settingsDBPutRaw(bucket, key, trimmed)
				return true
			}
		}
	}
	return false
}

func settingsDBSaveJSON(bucket, key string, fallbackFile string, mode os.FileMode, val interface{}) error {
	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return err
	}
	settingsDBMu.RLock()
	hasDB := settingsDB != nil
	settingsDBMu.RUnlock()

	if hasDB {
		return settingsDBPutRaw(bucket, key, data)
	}
	if strings.TrimSpace(fallbackFile) != "" {
		_ = os.MkdirAll(filepath.Dir(fallbackFile), 0755)
		return atomicWriteFile(fallbackFile, data, mode)
	}
	return os.ErrNotExist
}

func getSettingsDBSummary() SettingsDBSummary {
	settingsDBMu.RLock()
	db := settingsDB
	path := settingsDBPath
	settingsDBMu.RUnlock()

	summary := SettingsDBSummary{
		Path:          path,
		Engine:        "bbolt",
		Initialized:   db != nil,
		SchemaVersion: 1,
		Buckets:       map[string]int{},
		Keys:          []SettingsDBKeyInfo{},
	}
	if db == nil {
		return summary
	}
	if st, err := os.Stat(path); err == nil {
		summary.SizeBytes = st.Size()
	}

	_ = db.View(func(tx *bbolt.Tx) error {
		if meta := tx.Bucket([]byte(bucketSystemMeta)); meta != nil {
			if sv := meta.Get([]byte("schema_version")); len(sv) > 0 {
				if n, err := strconv.Atoi(string(sv)); err == nil && n > 0 {
					summary.SchemaVersion = n
				}
			}
			if ma := meta.Get([]byte("migrated_at")); len(ma) > 0 {
				summary.MigratedAt = string(ma)
			}
			if mc := meta.Get([]byte("last_migration_count")); len(mc) > 0 {
				if n, err := strconv.Atoi(string(mc)); err == nil {
					summary.LastMigrationCount = n
				}
			}
		}
		for _, bName := range []string{bucketSettings, bucketCollections, bucketSystemMeta, bucketAuditLogs} {
			bkt := tx.Bucket([]byte(bName))
			if bkt == nil {
				summary.Buckets[bName] = 0
				continue
			}
			cnt := 0
			_ = bkt.ForEach(func(k, v []byte) error {
				cnt++
				summary.Keys = append(summary.Keys, SettingsDBKeyInfo{
					Bucket:    bName,
					Key:       string(k),
					SizeBytes: len(v),
				})
				return nil
			})
			summary.Buckets[bName] = cnt
			summary.TotalKeys += cnt
		}
		return nil
	})
	sort.Slice(summary.Keys, func(i, j int) bool {
		if summary.Keys[i].Bucket == summary.Keys[j].Bucket {
			return summary.Keys[i].Key < summary.Keys[j].Key
		}
		return summary.Keys[i].Bucket < summary.Keys[j].Bucket
	})
	return summary
}

func buildPublicSettingsDBSnapshotJSON() ([]byte, error) {
	settingsDBMu.RLock()
	db := settingsDB
	settingsDBMu.RUnlock()
	if db == nil {
		return nil, os.ErrNotExist
	}

	safeKeys := safePublicDBSnapshotKeys

	out := map[string]json.RawMessage{}
	_ = db.View(func(tx *bbolt.Tx) error {
		for _, bName := range []string{bucketSettings, bucketCollections} {
			bkt := tx.Bucket([]byte(bName))
			if bkt == nil {
				continue
			}
			_ = bkt.ForEach(func(k, v []byte) error {
				fullKey := bName + "/" + string(k)
				if safeKeys[fullKey] && json.Valid(v) {
					cp := append([]byte(nil), v...)
					out[fullKey] = json.RawMessage(cp)
				}
				return nil
			})
		}
		return nil
	})
	return json.MarshalIndent(out, "", "  ")
}

// GET / POST /api/v1/system/db
func handleSystemDB(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, getSettingsDBSummary())
	case http.MethodPost:
		var body struct {
			Action string `json:"action"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body)
		switch strings.TrimSpace(body.Action) {
		case "migrate":
			settingsDBMu.Lock()
			db := settingsDB
			var count int
			var err error
			if db != nil {
				count, err = migrateLegacyJSONToDBLocked(db, false)
			}
			settingsDBMu.Unlock()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, false, "migration failed: "+err.Error())
				return
			}
			_ = syncSettingsDB()
			writeJSON(w, http.StatusOK, true, map[string]interface{}{
				"migrated": count,
				"summary":  getSettingsDBSummary(),
			})
		case "sync", "":
			if err := syncSettingsDB(); err != nil {
				writeJSON(w, http.StatusInternalServerError, false, "sync failed: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, true, getSettingsDBSummary())
		default:
			writeJSON(w, http.StatusBadRequest, false, "unsupported action")
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// GET / PUT /api/v1/ui/preferences
func handleUIPreferences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var prefs UIPreferences
		if !settingsDBLoadJSON(bucketSettings, "ui_preferences", "", &prefs) {
			prefs = UIPreferences{Theme: "dark", DefaultTab: "/", Preferences: map[string]interface{}{}}
		}
		if prefs.Preferences == nil {
			prefs.Preferences = map[string]interface{}{}
		}
		writeJSON(w, http.StatusOK, true, prefs)
	case http.MethodPut, http.MethodPost:
		var prefs UIPreferences
		if err := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&prefs); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid preferences JSON")
			return
		}
		if prefs.Preferences == nil {
			prefs.Preferences = map[string]interface{}{}
		}
		prefs.UpdatedAt = time.Now().Format(time.RFC3339)
		if err := settingsDBSaveJSON(bucketSettings, "ui_preferences", "", 0600, prefs); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "failed to save preferences: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, true, prefs)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}
