package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
