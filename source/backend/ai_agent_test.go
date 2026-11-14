package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateAIBaseURLSSRFGuards(t *testing.T) {
	allowed := []string{
		"",
		"https://generativelanguage.googleapis.com/v1beta/openai",
		"https://api.openai.com/v1",
		"http://127.0.0.1:11434/v1",
		"http://192.168.2.50:11434/v1",
	}
	for _, u := range allowed {
		if err := validateAIBaseURL(u); err != nil {
			t.Errorf("expected AI BaseURL %q to be allowed, got %v", u, err)
		}
	}

	blocked := []string{
		"ftp://example.com/v1",
		"file:///etc/passwd",
		"https://user:secret@api.openai.com/v1",
		"http://169.254.169.254/latest/meta-data",
		"http://0.0.0.0:8080/v1",
		"http://224.0.0.1:8080/v1",
		"http://8.8.8.8/v1",
	}
	for _, u := range blocked {
		if err := validateAIBaseURL(u); err == nil {
			t.Errorf("expected unsafe AI BaseURL %q to be rejected", u)
		}
	}
}

func TestAIProvidersAndBareURLFiltering(t *testing.T) {
	if got := defaultBaseURLForProvider("gemini"); !strings.Contains(got, "generativelanguage.googleapis.com") {
		t.Fatalf("unexpected gemini BaseURL: %q", got)
	}
	if got := defaultBaseURLForProvider("deepseek"); !strings.Contains(got, "deepseek.com") {
		t.Fatalf("unexpected deepseek BaseURL: %q", got)
	}
	if got := defaultModelForProvider("gemini"); got == "" {
		t.Fatal("expected non-empty default gemini model")
	}

	if !isBareURLMessage("https://www.youtube.com/watch?v=dQw4w9WgXcQ") {
		t.Fatal("expected bare YouTube URL to route to Downloader")
	}
	if isBareURLMessage("Kiểm tra ping và RAM của router giúp tôi") {
		t.Fatal("expected normal conversation text not to match bare URL route")
	}

	msgs := []AIChatMessage{
		{Role: "user", Content: "https://youtu.be/abc12345"},
		{Role: "user", Content: "Báo cáo trạng thái WiFi"},
		{Role: "assistant", Content: "Đã kiểm tra xong."},
	}
	filtered := filterOutBareURLMessages(msgs)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 non-URL messages after filtering, got %d", len(filtered))
	}
	if filtered[0].Content != "Báo cáo trạng thái WiFi" {
		t.Fatalf("unexpected filtered message order/content: %+v", filtered)
	}
}

func TestAIToolBlocksRespectsAllowExecFalse(t *testing.T) {
	_, cmdLogs, cmdOutputs, _ := executeAIToolBlocks("Tôi đề xuất lệnh:\n<exec_cmd>rm -rf /tmp/nonexistent</exec_cmd>", false)
	if len(cmdLogs) != 0 {
		t.Fatalf("expected zero command executions when allowExec=false, got %d", len(cmdLogs))
	}
	if len(cmdOutputs) != 0 {
		t.Fatalf("expected zero command outputs when allowExec=false, got %d", len(cmdOutputs))
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", bytes.NewBufferString(`{"message":"hello"}`))
	rr := httptest.NewRecorder()
	handleAIChat(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from handleAIChat outside OpenWrt, got %d", rr.Code)
	}
}

func TestUnquoteUCIValueAndParseDirective(t *testing.T) {
	unquoteCases := []struct {
		in   string
		want string
	}{
		{"'Hello'", "Hello"},
		{`"My WiFi"`, "My WiFi"},
		{`''\''Hello'\'''`, `'Hello'`},
		{`'abc'\''def'`, `abc'def`},
		{"'0'", "0"},
		{`""`, ""},
		{"Hello", "Hello"},
	}
	for _, tc := range unquoteCases {
		if got := unquoteUCIValue(tc.in); got != tc.want {
			t.Errorf("unquoteUCIValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	dir, ok, err := parseUCISetDirective("wireless.default_radio0.ssid='Hello'")
	if err != nil || !ok || dir.action != "set" || dir.pkg != "wireless" || dir.key != "wireless.default_radio0.ssid" || dir.val != "Hello" {
		t.Fatalf("unexpected parse result: dir=%+v ok=%v err=%v", dir, ok, err)
	}

	dir, ok, err = parseUCISetDirective(`uci set wireless.default_radio0.disabled="0"`)
	if err != nil || !ok || dir.action != "set" || dir.val != "0" {
		t.Fatalf("unexpected uci set parse result: dir=%+v ok=%v err=%v", dir, ok, err)
	}

	dir, ok, err = parseUCISetDirective("uci -q delete wireless.wwan_sta.bssid")
	if err != nil || !ok || dir.action != "delete" || dir.key != "wireless.wwan_sta.bssid" || dir.hasVal {
		t.Fatalf("unexpected delete parse result: dir=%+v ok=%v err=%v", dir, ok, err)
	}

	if _, ok, err := parseUCISetDirective("uci commit wireless"); err != nil || ok {
		t.Fatalf("expected commit line to be skipped cleanly, got ok=%v err=%v", ok, err)
	}
	if _, _, err := parseUCISetDirective("bad key=value"); err == nil {
		t.Fatal("expected invalid UCI path with space to fail")
	}
	if _, _, err := parseUCISetDirective("wireless.default_radio0.ssid=bad\x00val"); err == nil {
		t.Fatal("expected NUL byte in UCI value to fail")
	}
}

func TestBlueprintAutoUpgradeAndCustomPreservation(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "r3p.db")
	if err := initSettingsDBAt(dbPath); err != nil {
		t.Fatalf("initSettingsDBAt failed: %v", err)
	}
	defer closeSettingsDB()

	origBlueprintFile := aiBlueprintFile
	aiBlueprintFile = filepath.Join(tmp, "SYSTEM_BLUEPRINT.md")
	defer func() { aiBlueprintFile = origBlueprintFile }()

	legacy := "# TÀI LIỆU\n512MB DDR3 (RAM盘 /tmp)\nwireless.default_radio0.ssid='TenWiFiMoi'\n"
	if err := settingsDBPutRaw(bucketSystemMeta, "ai_blueprint", []byte(legacy)); err != nil {
		t.Fatal(err)
	}

	upgraded := ensureSystemBlueprintFile()
	if strings.Contains(upgraded, "RAM盘") || strings.Contains(upgraded, "ssid='TenWiFiMoi'") {
		t.Fatalf("expected legacy blueprint to auto-upgrade, still got outdated content")
	}
	if !strings.Contains(upgraded, "/etc/r3p/r3p.db") || !strings.Contains(upgraded, "fw4") || !strings.Contains(upgraded, "radio0.disabled") {
		t.Fatalf("expected upgraded blueprint to document r3p.db, fw4, and radio0 topology")
	}
	if rev, ok := settingsDBGetRaw(bucketSystemMeta, "ai_blueprint_rev"); !ok || string(rev) != currentSystemBlueprintRev {
		t.Fatalf("expected ai_blueprint_rev=%q, got %q", currentSystemBlueprintRev, string(rev))
	}

	custom := "# Custom Router Blueprint\nKeep my manual notes"
	writeSystemBlueprintToStorage(custom, currentSystemBlueprintRev, true)
	if got := ensureSystemBlueprintFile(); got != custom {
		t.Fatalf("expected custom user blueprint to be preserved, got %q", got)
	}
}

func TestFormatWirelessAndArchitecturePromptTruthfulness(t *testing.T) {
	tmp := t.TempDir()
	if err := initSettingsDBAt(filepath.Join(tmp, "r3p.db")); err != nil {
		t.Fatalf("initSettingsDBAt failed: %v", err)
	}
	defer closeSettingsDB()

	origBlueprintFile := aiBlueprintFile
	aiBlueprintFile = filepath.Join(tmp, "SYSTEM_BLUEPRINT.md")
	defer func() { aiBlueprintFile = origBlueprintFile }()

	wifiRaw := strings.Join([]string{
		"wireless.radio0=wifi-device",
		"wireless.radio0.disabled='1'",
		"wireless.radio0.channel='1'",
		"wireless.default_radio0=wifi-iface",
		`wireless.default_radio0.ssid=''\''Hello'\'''`,
		"wireless.default_radio0.encryption='psk2'",
		"wireless.default_radio0.disabled='0'",
	}, "\n")
	summary := formatWirelessSummaryForPrompt(wifiRaw)
	if !strings.Contains(summary, `ssid="'Hello'"`) {
		t.Fatalf("expected decoded ssid=\"'Hello'\", got:\n%s", summary)
	}
	if !strings.Contains(summary, "OFF (radio chip disabled=1)") {
		t.Fatalf("expected parent radio disabled warning in wireless summary, got:\n%s", summary)
	}

	prompt := buildSystemArchitecturePrompt("extra test note")
	if !strings.Contains(prompt, "r3p.db -> collections/ai_memory") || !strings.Contains(prompt, "wireless.default_radio0.ssid=TenMoi") {
		t.Fatalf("expected updated DB and unquoted <uci_set> instructions in system prompt")
	}
}

func TestWriteFileGuardAndSessionPruning(t *testing.T) {
	blocked := []string{"/etc/r3p/r3p.db", "/etc/r3p/api-token", "/etc/shadow", "/etc/config/../passwd", "/usr/bin/r3p-api"}
	for _, p := range blocked {
		if err := validateAIWriteFilePath(p); err == nil {
			t.Errorf("expected validateAIWriteFilePath(%q) to reject protected path", p)
		}
	}
	allowed := []string{"/tmp/test_r3p.sh", "/etc/config/wireless", "/etc/dnsmasq.conf"}
	for _, p := range allowed {
		if err := validateAIWriteFilePath(p); err != nil {
			t.Errorf("expected validateAIWriteFilePath(%q) to pass, got %v", p, err)
		}
	}

	evts, _, outs, _ := executeAIToolBlocks(`<write_file path="/etc/r3p/r3p.db">bad</write_file>`, true)
	if len(evts) != 1 || evts[0].Success || len(outs) != 1 || !strings.Contains(outs[0], "từ chối") {
		t.Fatalf("expected <write_file> targeting r3p.db to be blocked cleanly, got evts=%+v outs=%+v", evts, outs)
	}

	tmp := t.TempDir()
	if err := initSettingsDBAt(filepath.Join(tmp, "r3p.db")); err != nil {
		t.Fatal(err)
	}
	defer closeSettingsDB()
	resetAIMemoryCaches()

	for i := 0; i < 105; i++ {
		addAIMemoryEntry("fact", fmt.Sprintf("memory item %03d", i))
	}
	mems := loadAIMemories()
	if len(mems) > 100 || mems[0].Content != "memory item 104" {
		t.Fatalf("expected newest memory item 104 retained at index 0 within 100 limit, got len=%d top=%q", len(mems), mems[0].Content)
	}

	hugeOut := strings.Repeat("A", 2000)
	for i := 0; i < 34; i++ {
		appendMessageToSession("default", AIChatMessage{
			Role:       "assistant",
			Content:    fmt.Sprintf("turn %d", i),
			Commands:   []AICmdLog{{Command: "cmd", Output: hugeOut}},
			ToolEvents: []AIToolAction{{Tool: "exec_cmd", Command: "cmd", Output: hugeOut, Success: true}},
		})
	}
	_, sessMsgs := getActiveSessionMessages("default")
	if len(sessMsgs) < 34 {
		t.Fatalf("expected at least 34 messages, got %d", len(sessMsgs))
	}
	last := sessMsgs[len(sessMsgs)-1]
	if len(last.Commands) == 0 || len(last.Commands[0].Output) >= len(hugeOut) {
		t.Fatalf("expected latest command output to be trimmed for storage, got len=%d", len(last.Commands[0].Output))
	}
	if len(sessMsgs[1].Commands) != 0 || len(sessMsgs[1].ToolEvents) != 0 {
		t.Fatalf("expected old session message command blobs to be pruned")
	}
}

func TestPromptTokenOptimizationAndModelCooldown(t *testing.T) {
	if !needsFullUCIContext("đổi SSID wifi 2.4GHz") {
		t.Fatal("expected Wi-Fi SSID query to include full UCI context")
	}
	if needsFullUCIContext("kiểm tra dung lượng RAM trống") {
		t.Fatal("expected RAM query to omit verbose raw UCI dumps")
	}

	tmp := t.TempDir()
	if err := initSettingsDBAt(filepath.Join(tmp, "r3p.db")); err != nil {
		t.Fatal(err)
	}
	defer closeSettingsDB()
	resetAIMemoryCaches()

	origBlueprintFile := aiBlueprintFile
	aiBlueprintFile = filepath.Join(tmp, "SYSTEM_BLUEPRINT.md")
	defer func() { aiBlueprintFile = origBlueprintFile }()

	shortPrompt := buildSystemArchitecturePromptForQuery("", "kiểm tra dung lượng RAM trống")
	fullPrompt := buildSystemArchitecturePromptForQuery("", "đổi wifi ssid")
	if !strings.Contains(shortPrompt, "đã rút gọn để tiết kiệm token") {
		t.Fatalf("expected compact telemetry hint for non-UCI query")
	}
	if strings.Contains(fullPrompt, "đã rút gọn để tiết kiệm token") {
		t.Fatalf("expected full UCI telemetry section for Wi-Fi query")
	}

	now := time.Now()
	models := []string{"gemini-3.1-flash-lite", "gemini-3-flash-preview"}
	recordModelCooldown("gemini-3.1-flash-lite", http.StatusTooManyRequests, "RESOURCE_EXHAUSTED limit 20 per day", now)
	filtered := filterModelsByCooldown(models, now.Add(time.Minute))
	if len(filtered) != 1 || filtered[0] != "gemini-3-flash-preview" {
		t.Fatalf("expected cooled-down model skipped, got %v", filtered)
	}
	recordModelCooldown("gemini-3-flash-preview", http.StatusServiceUnavailable, "overloaded", now)
	allDown := filterModelsByCooldown(models, now.Add(10*time.Second))
	if len(allDown) != 2 {
		t.Fatalf("expected fallback to full candidate list when all models are in cooldown, got %v", allDown)
	}
	clearModelCooldown("gemini-3.1-flash-lite")
	clearModelCooldown("gemini-3-flash-preview")
}
