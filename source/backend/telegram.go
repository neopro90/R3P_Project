package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

type TelegramMessageRoute string

const (
	TelegramRouteIgnore     TelegramMessageRoute = "ignore"
	TelegramRouteHelp       TelegramMessageRoute = "help"
	TelegramRouteDownloader TelegramMessageRoute = "downloader"
	TelegramRouteAI         TelegramMessageRoute = "ai"
)

type TelegramMessageAnalysis struct {
	Route     TelegramMessageRoute `json:"route"`
	Reason    string               `json:"reason"`
	URLs      []string             `json:"urls,omitempty"`
	CleanText string               `json:"clean_text,omitempty"`
}

var telegramURLRegex = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)

type telegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	Mount    string `json:"mount"`
	Path     string `json:"path"`
	Enabled  bool   `json:"enabled"`
}

type telegramServiceStatus struct {
	Configured bool      `json:"configured"`
	Enabled    bool      `json:"enabled"`
	Running    bool      `json:"running"`
	Connected  bool      `json:"connected"`
	ChatID     string    `json:"chat_id,omitempty"`
	Mount      string    `json:"mount,omitempty"`
	Path       string    `json:"path,omitempty"`
	LastPoll   time.Time `json:"last_poll,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
}

type telegramService struct {
	mu         sync.Mutex
	configPath string
	config     *telegramConfig
	status     telegramServiceStatus
	cancel     context.CancelFunc
}

var telegramDownloader = newTelegramService()

func newTelegramService() *telegramService {
	configPath := strings.TrimSpace(os.Getenv("R3P_TELEGRAM_CONFIG"))
	if configPath == "" {
		configPath = "/etc/r3p/telegram.json"
	}
	return &telegramService{configPath: configPath}
}

func (s *telegramService) load() error {
	var config telegramConfig
	isDefaultPath := s.configPath == "/etc/r3p/telegram.json"
	if isDefaultPath {
		if raw, ok := settingsDBGetRaw(bucketSettings, "telegram"); ok && len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &config); err != nil {
				return fmt.Errorf("decode Telegram configuration: %w", err)
			}
		} else {
			data, err := os.ReadFile(s.configPath)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read Telegram configuration: %w", err)
			}
			if err := json.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("decode Telegram configuration: %w", err)
			}
		}
	} else {
		data, err := os.ReadFile(s.configPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Telegram configuration: %w", err)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("decode Telegram configuration: %w", err)
		}
	}
	if err := validateTelegramConfig(config.BotToken, config.ChatID); err != nil {
		return fmt.Errorf("invalid Telegram configuration: %w", err)
	}
	if isDefaultPath {
		_ = settingsDBSaveJSON(bucketSettings, "telegram", "", 0600, config)
	}
	s.mu.Lock()
	s.config = &config
	s.status = telegramServiceStatus{
		Configured: true,
		Enabled:    config.Enabled,
		ChatID:     config.ChatID,
		Mount:      config.Mount,
		Path:       config.Path,
	}
	tok, chatID, enabled, mount, subPath, shouldSync := s.startLocked()
	s.mu.Unlock()
	if shouldSync {
		syncTelegramConfigToDownloader(tok, chatID, enabled, mount, subPath)
	}
	return nil
}

func (s *telegramService) statusSnapshot() telegramServiceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *telegramService) currentToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config == nil {
		return ""
	}
	return s.config.BotToken
}

func (s *telegramService) setConfig(config telegramConfig) error {
	s.mu.Lock()
	if config.BotToken == "" && s.config != nil {
		config.BotToken = s.config.BotToken
	}
	if err := validateTelegramConfig(config.BotToken, config.ChatID); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.configPath == "/etc/r3p/telegram.json" {
		if err := settingsDBSaveJSON(bucketSettings, "telegram", s.configPath, 0600, config); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("save Telegram configuration: %w", err)
		}
	} else {
		if err := writePrivateJSON(s.configPath, config); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("save Telegram configuration: %w", err)
		}
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.config = &config
	s.status = telegramServiceStatus{
		Configured: true,
		Enabled:    config.Enabled,
		ChatID:     config.ChatID,
		Mount:      config.Mount,
		Path:       config.Path,
	}
	tok, chatID, enabled, mount, subPath, shouldSync := s.startLocked()
	s.mu.Unlock()
	if shouldSync {
		syncTelegramConfigToDownloader(tok, chatID, enabled, mount, subPath)
	}
	return nil
}

func (s *telegramService) clearConfig() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	isDefaultPath := s.configPath == "/etc/r3p/telegram.json"
	if isDefaultPath {
		_ = settingsDBDelete(bucketSettings, "telegram")
	}
	if err := os.Remove(s.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.mu.Unlock()
		return fmt.Errorf("remove Telegram configuration: %w", err)
	}
	s.config = nil
	s.status = telegramServiceStatus{}
	s.mu.Unlock()
	if isDefaultPath {
		syncTelegramConfigToDownloader("", "", false, "", "")
	}
	return nil
}

func (s *telegramService) startLocked() (tok, chatID string, enabled bool, mount, subPath string, shouldSync bool) {
	if s.config == nil {
		return "", "", false, "", "", false
	}
	// Unified polling runs in r3pTelegramPollerLoop so /api/v1/telegram and
	// /api/v1/download/* never spawn competing getUpdates requests (HTTP 409).
	s.status.Running = s.config.Enabled
	if !s.config.Enabled {
		s.status.Connected = false
	}
	if s.configPath == "/etc/r3p/telegram.json" {
		return s.config.BotToken, s.config.ChatID, s.config.Enabled, s.config.Mount, s.config.Path, true
	}
	return "", "", false, "", "", false
}

func (s *telegramService) syncFromDownloader(botToken, chatID string, enabled bool, defaultMount, defaultPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	mount := "/mnt/sda1"
	path := "downloads"
	if strings.TrimSpace(defaultMount) != "" {
		mount = defaultMount
	}
	if strings.TrimSpace(defaultPath) != "" {
		path = defaultPath
	}
	if s.config != nil {
		if s.config.Mount != "" {
			mount = s.config.Mount
		}
		if s.config.Path != "" {
			path = s.config.Path
		}
	}
	configured := botToken != ""
	s.config = &telegramConfig{
		BotToken: botToken,
		ChatID:   chatID,
		Mount:    mount,
		Path:     path,
		Enabled:  enabled,
	}
	s.status.Configured = configured
	s.status.Enabled = enabled
	s.status.Running = configured && enabled
	s.status.ChatID = chatID
	s.status.Mount = mount
	s.status.Path = path
	if !configured || !enabled {
		s.status.Connected = false
	}
}

func (s *telegramService) updatePollStatus(connected bool, lastError string) {
	s.mu.Lock()
	s.status.Connected = connected
	s.status.LastPoll = time.Now().UTC()
	s.status.LastError = lastError
	s.mu.Unlock()
}

// analyzeTelegramMessage inspects incoming Telegram text and decides whether it
// belongs to the Downloader (URL-only messages) or the Root AI Agent (normal text,
// questions referencing URLs, or ! shell commands).
func analyzeTelegramMessage(text string) TelegramMessageAnalysis {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return TelegramMessageAnalysis{Route: TelegramRouteIgnore, Reason: "empty"}
	}

	fields := strings.Fields(trimmed)
	if len(fields) > 0 && strings.HasPrefix(fields[0], "/") {
		cmdBase := strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
		if cmdBase == "/start" || cmdBase == "/id" {
			return TelegramMessageAnalysis{
				Route:     TelegramRouteHelp,
				Reason:    "help_cmd",
				CleanText: trimmed,
			}
		}
		if cmdBase == "/exec" || cmdBase == "/sh" {
			return TelegramMessageAnalysis{
				Route:     TelegramRouteAI,
				Reason:    "shell_cmd",
				CleanText: trimmed,
			}
		}
	}
	if strings.HasPrefix(trimmed, "!") {
		return TelegramMessageAnalysis{
			Route:     TelegramRouteAI,
			Reason:    "shell_cmd",
			CleanText: trimmed,
		}
	}

	rawMatches := telegramURLRegex.FindAllString(trimmed, -1)
	seen := make(map[string]struct{}, len(rawMatches))
	urls := make([]string, 0, len(rawMatches))
	for _, m := range rawMatches {
		u := trimExtractedURL(m)
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	if len(urls) > 0 {
		leftover := telegramURLRegex.ReplaceAllString(trimmed, " ")
		onlySeparators := true
		for _, r := range leftover {
			if unicode.IsSpace(r) {
				continue
			}
			switch r {
			case ',', ';', '|', '<', '>', '(', ')', '[', ']', '{', '}', '"', '\'', '`':
				continue
			default:
				onlySeparators = false
			}
			if !onlySeparators {
				break
			}
		}
		if onlySeparators {
			return TelegramMessageAnalysis{
				Route:     TelegramRouteDownloader,
				Reason:    "url_only",
				URLs:      urls,
				CleanText: trimmed,
			}
		}
		return TelegramMessageAnalysis{
			Route:     TelegramRouteAI,
			Reason:    "text_with_url",
			URLs:      urls,
			CleanText: trimmed,
		}
	}

	return TelegramMessageAnalysis{
		Route:     TelegramRouteAI,
		Reason:    "normal_text",
		CleanText: trimmed,
	}
}

func trimExtractedURL(raw string) string {
	u := strings.TrimSpace(raw)
	for u != "" {
		last := u[len(u)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?', '"', '\'', '`':
			u = u[:len(u)-1]
		case ')':
			if strings.Count(u, "(") < strings.Count(u, ")") {
				u = u[:len(u)-1]
			} else {
				goto parsed
			}
		case ']':
			if strings.Count(u, "[") < strings.Count(u, "]") {
				u = u[:len(u)-1]
			} else {
				goto parsed
			}
		case '}':
			if strings.Count(u, "{") < strings.Count(u, "}") {
				u = u[:len(u)-1]
			} else {
				goto parsed
			}
		default:
			goto parsed
		}
	}
parsed:
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return u
}

func isAllowedTelegramChat(allowedList, chatID string) bool {
	chatID = strings.TrimSpace(chatID)
	allowedList = strings.TrimSpace(allowedList)
	if chatID == "" || allowedList == "" {
		return false
	}
	tokens := strings.FieldsFunc(allowedList, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';' || r == '|'
	})
	for _, tok := range tokens {
		if tok == chatID {
			return true
		}
	}
	return false
}

func telegramMessageMatchesChat(message *telegramMessage, allowedChatID string) bool {
	if message == nil {
		return false
	}
	return isAllowedTelegramChat(allowedChatID, message.Chat.ID.String())
}

// splitTelegramMessage divides long replies into Telegram-safe UTF-8 chunks (<4096 chars).
func splitTelegramMessage(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 3800
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			chunks = append(chunks, strings.TrimSpace(string(runes[start:])))
			break
		}
		splitAt := end
		minBreak := start + (maxRunes / 2)
		for i := end; i > minBreak; i-- {
			if runes[i-1] == '\n' {
				splitAt = i
				break
			}
		}
		if splitAt == end {
			for i := end; i > minBreak; i-- {
				if runes[i-1] == ' ' || runes[i-1] == '\t' {
					splitAt = i
					break
				}
			}
		}
		part := strings.TrimSpace(string(runes[start:splitAt]))
		if part != "" {
			chunks = append(chunks, part)
		}
		start = splitAt
		for start < len(runes) && (runes[start] == '\n' || runes[start] == ' ' || runes[start] == '\t') {
			start++
		}
	}
	return chunks
}

type telegramUpdate struct {
	UpdateID    int64            `json:"update_id"`
	Message     *telegramMessage `json:"message"`
	ChannelPost *telegramMessage `json:"channel_post"`
}

type telegramMessage struct {
	MessageID int64 `json:"message_id"`
	Chat      struct {
		ID json.Number `json:"id"`
	} `json:"chat"`
	Text    string `json:"text"`
	Caption string `json:"caption"`
}

func writePrivateJSON(path string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode private configuration: %w", err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("create private configuration directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return fmt.Errorf("secure private configuration directory: %w", err)
	}
	temp, err := os.CreateTemp(directory, ".telegram-*.tmp")
	if err != nil {
		return fmt.Errorf("create private configuration file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("secure private configuration file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write private configuration file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync private configuration file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close private configuration file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("activate private configuration file: %w", err)
	}
	return nil
}

func handleTelegram(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, telegramDownloader.statusSnapshot())
	case http.MethodPut:
		if r.TLS == nil {
			writeJSON(w, http.StatusUpgradeRequired, false, "HTTPS is required to configure Telegram credentials")
			return
		}
		var body telegramConfig
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid Telegram configuration")
			return
		}
		if body.BotToken == "" {
			body.BotToken = telegramDownloader.currentToken()
		}
		if err := validateTelegramConfig(body.BotToken, body.ChatID); err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		if body.Mount == "" {
			body.Mount = "/overlay"
		}
		if body.Path == "" {
			body.Path = "downloads"
		}
		if _, err := validateDownloadDestination(body.Mount, body.Path); err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		if err := telegramDownloader.setConfig(body); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "could not securely save Telegram configuration")
			return
		}
		writeJSON(w, http.StatusOK, true, telegramDownloader.statusSnapshot())
	case http.MethodDelete:
		if r.TLS == nil {
			writeJSON(w, http.StatusUpgradeRequired, false, "HTTPS is required to change Telegram configuration")
			return
		}
		if err := telegramDownloader.clearConfig(); err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "could not remove Telegram configuration")
			return
		}
		writeJSON(w, http.StatusOK, true, telegramDownloader.statusSnapshot())
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}
