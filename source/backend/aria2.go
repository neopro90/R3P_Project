package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxDownloadURLLength = 4096
	maxTelegramURLs      = 20
	aria2RPCAddress      = "http://127.0.0.1:6800/jsonrpc"
)

var downloadURLPattern = regexp.MustCompile(`(?i)(https?://[^\s<>]+|magnet:\?[^\s<>]+)`)
var telegramTokenPattern = regexp.MustCompile(`^[0-9]{5,15}:[A-Za-z0-9_-]{20,128}$`)
var telegramChatIDPattern = regexp.MustCompile(`^-?[0-9]{1,20}$`)

type downloadItem struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     string `json:"totalLength"`
	CompletedLength string `json:"completedLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
	Files           []struct {
		Path string `json:"path"`
	} `json:"files"`
}

func parseDownloadURLs(text string) []string {
	seen := make(map[string]struct{})
	urls := make([]string, 0)
	for _, match := range downloadURLPattern.FindAllString(text, maxTelegramURLs+1) {
		candidate := strings.TrimRight(match, ".,;:!?)]}\"'")
		if _, exists := seen[candidate]; exists {
			continue
		}
		if err := validateDownloadURL(candidate); err != nil {
			continue
		}
		seen[candidate] = struct{}{}
		urls = append(urls, candidate)
		if len(urls) == maxTelegramURLs {
			break
		}
	}
	return urls
}

func validateTelegramConfig(token, chatID string) error {
	if !telegramTokenPattern.MatchString(token) {
		return errors.New("bot token format is invalid")
	}
	if !telegramChatIDPattern.MatchString(chatID) {
		return errors.New("chat ID must be a numeric Telegram chat ID")
	}
	if _, err := strconv.ParseInt(chatID, 10, 64); err != nil {
		return errors.New("chat ID is outside the supported numeric range")
	}
	return nil
}

func validateDownloadURL(raw string) error {
	if len(raw) == 0 || len(raw) > maxDownloadURLLength || strings.ContainsAny(raw, "\r\n\t") {
		return errors.New("URL is empty or too long")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("URL format is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "magnet":
		if parsed.Opaque == "" && parsed.RawQuery == "" {
			return errors.New("magnet link is invalid")
		}
		return nil
	case "http", "https":
		host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		if host == "" || strings.Contains(host, "%") ||
			host == "localhost" || strings.HasSuffix(host, ".localhost") ||
			strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") ||
			strings.HasSuffix(host, ".internal") {
			return errors.New("URL host is not allowed")
		}
		if ip := net.ParseIP(host); ip != nil && !isPublicDownloadIP(ip) {
			return errors.New("private or local IP addresses are not allowed")
		}
		return nil
	default:
		return errors.New("only HTTP, HTTPS, and magnet URLs are supported")
	}
}

func isPublicDownloadIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	for _, block := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32",
	} {
		_, network, _ := net.ParseCIDR(block)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func validatePublicDownloadHost(ctx context.Context, rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("URL format is invalid")
	}
	if parsed.Scheme == "magnet" {
		return nil
	}
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicDownloadIP(ip) {
			return errors.New("private or special-use IP addresses are not allowed")
		}
		return nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("download host did not resolve to public addresses")
	}
	return validateResolvedDownloadAddresses(addresses)
}

func validateResolvedDownloadAddresses(addresses []net.IPAddr) error {
	for _, address := range addresses {
		if !isPublicDownloadIP(address.IP) {
			return errors.New("download host resolves to a private or special-use IP address")
		}
	}
	return nil
}

func validateDownloadDestination(mountPoint, relativePath string) (string, error) {
	if mountPoint == "/overlay" || mountPoint == "/tmp" {
		cleaned, err := cleanStorageRelativePath(relativePath)
		if err != nil {
			return "", err
		}
		target := filepath.Join(mountPoint, filepath.FromSlash(cleaned))
		if err := os.MkdirAll(target, 0755); err != nil {
			return "", errors.New("could not create destination directory")
		}
		return target, nil
	}
	if !usbMountExists(mountPoint, readFileLines("/proc/mounts", 0)) {
		return "", errors.New("destination must be a currently mounted removable USB volume")
	}
	cleaned, err := cleanStorageRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(mountPoint)
	if err != nil {
		return "", errors.New("USB volume is unavailable")
	}
	defer root.Close()
	directory, err := root.Open(cleaned)
	if err != nil {
		return "", errors.New("destination directory does not exist")
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil || !info.IsDir() {
		return "", errors.New("destination path is not a directory")
	}
	return filepath.Join(mountPoint, filepath.FromSlash(cleaned)), nil
}

func loadAria2Token() (string, error) {
	tokenPath := strings.TrimSpace(os.Getenv("R3P_ARIA2_RPC_TOKEN_FILE"))
	if tokenPath == "" {
		tokenPath = "/etc/r3p/aria2-rpc-token"
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("read aria2 RPC credential: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 128 {
		return "", errors.New("aria2 RPC credential has an invalid length")
	}
	return token, nil
}

func callAria2(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	token, err := loadAria2Token()
	if err != nil {
		return nil, err
	}
	rpcParams := make([]interface{}, 0, len(params)+1)
	rpcParams = append(rpcParams, "token:"+token)
	rpcParams = append(rpcParams, params...)
	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "r3p",
		"method":  method,
		"params":  rpcParams,
	})
	if err != nil {
		return nil, fmt.Errorf("encode aria2 request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, aria2RPCAddress, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create aria2 request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("aria2 RPC service is unavailable")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, errors.New("could not read aria2 response")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aria2 returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, errors.New("aria2 returned invalid JSON")
	}
	if result.Error != nil {
		return nil, fmt.Errorf("aria2 request failed: %s", result.Error.Message)
	}
	if len(result.Result) == 0 {
		return nil, errors.New("aria2 returned no result")
	}
	return result.Result, nil
}

func queueAria2URL(ctx context.Context, rawURL, destination string) (string, error) {
	if err := validateDownloadURL(rawURL); err != nil {
		return "", err
	}
	downloadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := validatePublicDownloadHost(downloadCtx, rawURL); err != nil {
		return "", err
	}
	options := map[string]string{
		"dir":          destination,
		"max-redirect": "0",
		"continue":     "true",
	}
	result, err := callAria2(downloadCtx, "aria2.addUri", []string{rawURL}, options)
	if err != nil {
		return "", err
	}
	var gid string
	if err := json.Unmarshal(result, &gid); err != nil || gid == "" {
		return "", errors.New("aria2 returned an invalid download ID")
	}
	return gid, nil
}

func fetchAria2Downloads(ctx context.Context) ([]downloadItem, error) {
	methods := []struct {
		name   string
		params []interface{}
	}{
		{name: "aria2.tellActive"},
		{name: "aria2.tellWaiting", params: []interface{}{0, 100}},
		{name: "aria2.tellStopped", params: []interface{}{0, 100}},
	}
	items := make([]downloadItem, 0)
	for _, method := range methods {
		result, err := callAria2(ctx, method.name, method.params...)
		if err != nil {
			return nil, err
		}
		var current []downloadItem
		if err := json.Unmarshal(result, &current); err != nil {
			return nil, errors.New("aria2 returned an invalid download list")
		}
		items = append(items, current...)
	}
	return items, nil
}
