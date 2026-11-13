package main

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAnalyzeTelegramMessage(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRoute  TelegramMessageRoute
		wantReason string
		wantURLs   []string
	}{
		{
			name:       "empty_whitespace",
			input:      "   \n\t  ",
			wantRoute:  TelegramRouteIgnore,
			wantReason: "empty",
		},
		{
			name:       "start_command",
			input:      "/start",
			wantRoute:  TelegramRouteHelp,
			wantReason: "help_cmd",
		},
		{
			name:       "id_command_with_bot_mention",
			input:      "/id@r3p_router_bot",
			wantRoute:  TelegramRouteHelp,
			wantReason: "help_cmd",
		},
		{
			name:       "bang_shell_command",
			input:      "!df -h",
			wantRoute:  TelegramRouteAI,
			wantReason: "shell_cmd",
		},
		{
			name:       "exec_shell_command",
			input:      "/exec uptime",
			wantRoute:  TelegramRouteAI,
			wantReason: "shell_cmd",
		},
		{
			name:       "sh_shell_command",
			input:      "/sh free -m",
			wantRoute:  TelegramRouteAI,
			wantReason: "shell_cmd",
		},
		{
			name:       "single_bare_url",
			input:      "https://www.tiktok.com/@creator/video/71234567890",
			wantRoute:  TelegramRouteDownloader,
			wantReason: "url_only",
			wantURLs:   []string{"https://www.tiktok.com/@creator/video/71234567890"},
		},
		{
			name:       "wrapped_and_trailing_punctuation_url",
			input:      "  (https://youtu.be/dQw4w9WgXcQ,)  ",
			wantRoute:  TelegramRouteDownloader,
			wantReason: "url_only",
			wantURLs:   []string{"https://youtu.be/dQw4w9WgXcQ"},
		},
		{
			name:       "wikipedia_balanced_parens_url",
			input:      "https://en.wikipedia.org/wiki/Router_(computing)",
			wantRoute:  TelegramRouteDownloader,
			wantReason: "url_only",
			wantURLs:   []string{"https://en.wikipedia.org/wiki/Router_(computing)"},
		},
		{
			name:       "multiple_urls_deduped",
			input:      "https://example.com/a.mp4\nhttps://example.com/b.mp4, https://example.com/a.mp4",
			wantRoute:  TelegramRouteDownloader,
			wantReason: "url_only",
			wantURLs:   []string{"https://example.com/a.mp4", "https://example.com/b.mp4"},
		},
		{
			name:       "plain_vietnamese_question",
			input:      "Kiểm tra nhiệt độ CPU và dung lượng USB hiện tại",
			wantRoute:  TelegramRouteAI,
			wantReason: "normal_text",
		},
		{
			name:       "question_mentioning_url_routes_to_ai",
			input:      "Tại sao link https://www.facebook.com/stories/12345/?view_single=1 không tải được?",
			wantRoute:  TelegramRouteAI,
			wantReason: "text_with_url",
			wantURLs:   []string{"https://www.facebook.com/stories/12345/?view_single=1"},
		},
		{
			name:       "natural_request_with_url_routes_to_ai",
			input:      "Kiểm tra giúp tôi link này có video không https://example.org/watch?v=1",
			wantRoute:  TelegramRouteAI,
			wantReason: "text_with_url",
			wantURLs:   []string{"https://example.org/watch?v=1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := analyzeTelegramMessage(tc.input)
			if got.Route != tc.wantRoute {
				t.Fatalf("Route = %q, want %q (analysis=%+v)", got.Route, tc.wantRoute, got)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if !reflect.DeepEqual(got.URLs, tc.wantURLs) {
				t.Fatalf("URLs = %#v, want %#v", got.URLs, tc.wantURLs)
			}
		})
	}
}

func TestIsAllowedTelegramChat(t *testing.T) {
	if !isAllowedTelegramChat("111222333", "111222333") {
		t.Fatal("exact allowed chat ID was rejected")
	}
	if !isAllowedTelegramChat("111222333, -100998877", "-100998877") {
		t.Fatal("group chat ID in comma-separated list was rejected")
	}
	if isAllowedTelegramChat("111222333", "1112") {
		t.Fatal("substring prefix chat ID must be rejected")
	}
	if isAllowedTelegramChat("111222333", "2223") {
		t.Fatal("substring middle chat ID must be rejected")
	}
	if isAllowedTelegramChat("", "111222333") {
		t.Fatal("empty allowlist matched in strict helper")
	}
	if isAllowedTelegramChat("111222333", "") {
		t.Fatal("empty chat ID was accepted")
	}
}

func TestIsBareURLMessageConsistency(t *testing.T) {
	if !isBareURLMessage("https://youtu.be/abc123") {
		t.Fatal("single bare URL should return true")
	}
	if isBareURLMessage("Tại sao https://youtu.be/abc123 lỗi?") {
		t.Fatal("sentence containing URL must route to AI (return false)")
	}
	if isBareURLMessage("xin chào router") {
		t.Fatal("plain text must return false")
	}
}

func TestSplitTelegramMessage(t *testing.T) {
	if got := splitTelegramMessage("  ", 3800); got != nil {
		t.Fatalf("empty text returned %#v, want nil", got)
	}
	short := "Trả lời ngắn từ Router R3P."
	if got := splitTelegramMessage(short, 3800); !reflect.DeepEqual(got, []string{short}) {
		t.Fatalf("short message split = %#v", got)
	}

	line := strings.Repeat("Router R3P kiểm tra tiếng Việt có dấu 1234567890.\n", 95)
	chunks := splitTelegramMessage(line, 1000)
	if len(chunks) < 2 {
		t.Fatalf("expected multi-chunk split, got %d chunk(s)", len(chunks))
	}
	for i, c := range chunks {
		if !utf8.ValidString(c) {
			t.Fatalf("chunk %d is not valid UTF-8", i)
		}
		if utf8.RuneCountInString(c) > 1000 {
			t.Fatalf("chunk %d exceeds limit: %d runes", i, utf8.RuneCountInString(c))
		}
	}
}
