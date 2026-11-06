package main

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestParseDownloadURLs(t *testing.T) {
	text := "Queue https://downloads.example/file.zip, magnet:?xt=urn:btih:0123456789012345678901234567890123456789&dn=test and duplicate https://downloads.example/file.zip"
	want := []string{
		"https://downloads.example/file.zip",
		"magnet:?xt=urn:btih:0123456789012345678901234567890123456789&dn=test",
	}
	if got := parseDownloadURLs(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDownloadURLs() = %#v, want %#v", got, want)
	}
}

func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "https", url: "https://downloads.example/file.zip", want: true},
		{name: "magnet", url: "magnet:?xt=urn:btih:0123456789012345678901234567890123456789", want: true},
		{name: "ftp rejected", url: "ftp://downloads.example/file", want: false},
		{name: "local host rejected", url: "http://router.local/file", want: false},
		{name: "private IP rejected", url: "http://192.168.1.1/file", want: false},
		{name: "loopback rejected", url: "http://127.0.0.1/file", want: false},
		{name: "embedded credentials rejected", url: "https://user:pass@downloads.example/file", want: false},
		{name: "control chars rejected", url: "https://downloads.example/file\n", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDownloadURL(test.url)
			if (err == nil) != test.want {
				t.Fatalf("validateDownloadURL(%q) error = %v; want allowed=%t", test.url, err, test.want)
			}
		})
	}
}

func TestResolvedDownloadAddressesRejectPrivateNetworks(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		wantErr   bool
	}{
		{name: "public", addresses: []string{"1.1.1.1"}},
		{name: "private", addresses: []string{"1.1.1.1", "10.0.0.2"}, wantErr: true},
		{name: "carrier NAT", addresses: []string{"100.64.0.1"}, wantErr: true},
		{name: "documentation range", addresses: []string{"203.0.113.1"}, wantErr: true},
		{name: "IPv6 documentation", addresses: []string{"2001:db8::1"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addresses := make([]net.IPAddr, 0, len(test.addresses))
			for _, value := range test.addresses {
				addresses = append(addresses, net.IPAddr{IP: net.ParseIP(value)})
			}
			err := validateResolvedDownloadAddresses(addresses)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateResolvedDownloadAddresses(%v) error = %v; wantErr=%t", test.addresses, err, test.wantErr)
			}
		})
	}
}

func TestValidateTelegramConfig(t *testing.T) {
	if err := validateTelegramConfig("123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", "-10012345"); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	if err := validateTelegramConfig("bad-token", "-10012345"); err == nil {
		t.Fatal("invalid bot token accepted")
	}
	if err := validateTelegramConfig("123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", "not-a-chat"); err == nil {
		t.Fatal("non-numeric chat ID accepted")
	}
}

func TestTelegramMessageMatchesOnlyConfiguredChat(t *testing.T) {
	message := &telegramMessage{}
	message.Chat.ID = "111222333"
	if !telegramMessageMatchesChat(message, "111222333") {
		t.Fatal("configured chat was rejected")
	}
	if telegramMessageMatchesChat(message, "111222334") {
		t.Fatal("message from an unconfigured chat was accepted")
	}
	if telegramMessageMatchesChat(nil, "111222333") {
		t.Fatal("missing Telegram message was accepted")
	}
}

func TestWritePrivateJSONUsesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filesystem does not expose POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "private", "telegram.json")
	if err := writePrivateJSON(path, telegramConfig{BotToken: "secret", ChatID: "12345"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("configuration permissions = %04o, want 0600", got)
	}
}
