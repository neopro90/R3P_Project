package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
	data, err := os.ReadFile(s.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Telegram configuration: %w", err)
	}
	var config telegramConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode Telegram configuration: %w", err)
	}
	if err := validateTelegramConfig(config.BotToken, config.ChatID); err != nil {
		return fmt.Errorf("invalid Telegram configuration: %w", err)
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
	s.startLocked()
	s.mu.Unlock()
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
	defer s.mu.Unlock()
	if config.BotToken == "" && s.config != nil {
		config.BotToken = s.config.BotToken
	}
	if err := validateTelegramConfig(config.BotToken, config.ChatID); err != nil {
		return err
	}
	if err := writePrivateJSON(s.configPath, config); err != nil {
		return fmt.Errorf("save Telegram configuration: %w", err)
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
	s.startLocked()
	return nil
}

func (s *telegramService) clearConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if err := os.Remove(s.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Telegram configuration: %w", err)
	}
	s.config = nil
	s.status = telegramServiceStatus{}
	return nil
}

func (s *telegramService) startLocked() {
	if s.config == nil || !s.config.Enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status.Running = true
	config := *s.config
	go s.poll(ctx, config)
}

func (s *telegramService) updatePollStatus(connected bool, lastError string) {
	s.mu.Lock()
	s.status.Connected = connected
	s.status.LastPoll = time.Now().UTC()
	s.status.LastError = lastError
	s.mu.Unlock()
}

func (s *telegramService) poll(ctx context.Context, config telegramConfig) {
	var offset int64
	var queuedUpdateID int64 = -1
	queuedForUpdate := make(map[string]struct{})
	for {
		updates, err := telegramGetUpdates(ctx, config.BotToken, offset)
		if err != nil {
			s.updatePollStatus(false, "Telegram polling failed; check connectivity, token, or webhook configuration.")
			if !waitContext(ctx, 5*time.Second) {
				return
			}
			continue
		}
		s.updatePollStatus(true, "")
		retry := false
		for _, update := range updates {
			if queuedUpdateID != update.UpdateID {
				queuedUpdateID = update.UpdateID
				queuedForUpdate = make(map[string]struct{})
			}
			message := update.Message
			if message == nil {
				message = update.ChannelPost
			}
			if telegramMessageMatchesChat(message, config.ChatID) {
				text := strings.TrimSpace(message.Text + "\n" + message.Caption)
				urls := parseDownloadURLs(text)
				destination, destinationErr := validateDownloadDestination(config.Mount, config.Path)
				if len(urls) > 0 && destinationErr != nil {
					s.updatePollStatus(false, "Telegram destination is unavailable; verify the mounted USB folder.")
					retry = true
					break
				}
				allQueued := true
				for _, downloadURL := range urls {
					if _, exists := queuedForUpdate[downloadURL]; exists {
						continue
					}
					if _, err := dlManager.addTask(downloadURL, destination); err != nil {
						s.updatePollStatus(false, "Downloader could not queue Telegram URL: "+err.Error())
						allQueued = false
						break
					}
					queuedForUpdate[downloadURL] = struct{}{}
				}
				if !allQueued {
					retry = true
					break
				}
			}
			offset = update.UpdateID + 1
		}
		if retry && !waitContext(ctx, 5*time.Second) {
			return
		}
	}
}

func telegramMessageMatchesChat(message *telegramMessage, allowedChatID string) bool {
	return message != nil && message.Chat.ID.String() == allowedChatID
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type telegramUpdate struct {
	UpdateID    int64            `json:"update_id"`
	Message     *telegramMessage `json:"message"`
	ChannelPost *telegramMessage `json:"channel_post"`
}

type telegramMessage struct {
	Chat struct {
		ID json.Number `json:"id"`
	} `json:"chat"`
	Text    string `json:"text"`
	Caption string `json:"caption"`
}

func telegramGetUpdates(ctx context.Context, token string, offset int64) ([]telegramUpdate, error) {
	endpoint := "https://api.telegram.org/bot" + token + "/getUpdates"
	query := url.Values{}
	query.Set("timeout", "15")
	query.Set("allowed_updates", `["message","channel_post"]`)
	if offset > 0 {
		query.Set("offset", strconv.FormatInt(offset, 10))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, errors.New("create Telegram request")
	}
	client := &http.Client{Timeout: 22 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("Telegram request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Telegram returned an unsuccessful status")
	}
	var result struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil || !result.OK {
		return nil, errors.New("Telegram returned an invalid response")
	}
	return result.Result, nil
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

func handleDownloads(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		items, err := fetchAria2Downloads(ctx)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, false, "download service is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"downloads": items})
	case http.MethodPost:
		var body struct {
			URL   string `json:"url"`
			Mount string `json:"mount"`
			Path  string `json:"path"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid download request")
			return
		}
		if err := validateDownloadURL(body.URL); err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		destination, err := validateDownloadDestination(body.Mount, body.Path)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, false, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		gid, err := queueAria2URL(ctx, body.URL, destination)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, false, "download service could not queue this URL")
			return
		}
		writeJSON(w, http.StatusAccepted, true, map[string]string{"gid": gid})
	default:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}
