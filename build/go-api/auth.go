package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName = "r3p_session"
	sessionDuration   = 8 * time.Hour
	maxAuthBody       = 1024
)

type authenticator struct {
	tokenHash       [sha256.Size]byte
	tokenConfigured bool
	mu              sync.Mutex
	sessions        map[string]time.Time
	now             func() time.Time
}

func newAuthenticator(token string) *authenticator {
	token = strings.TrimSpace(token)
	return &authenticator{
		tokenHash:       sha256.Sum256([]byte(token)),
		tokenConfigured: token != "",
		sessions:        make(map[string]time.Time),
		now:             time.Now,
	}
}

func loadAPIToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("R3P_API_TOKEN")); token != "" {
		return token, nil
	}

	tokenPath := strings.TrimSpace(os.Getenv("R3P_API_TOKEN_FILE"))
	if tokenPath == "" {
		tokenPath = "/etc/r3p/api-token"
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("read API token file %q: %w", tokenPath, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("API token file is empty")
	}
	return token, nil
}

func (a *authenticator) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/api/v1/health" ||
			r.Method == http.MethodOptions ||
			(r.URL.Path == "/api/v1/auth/session" && r.Method == http.MethodPost) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && a.validSession(cookie.Value) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, false, "authentication required")
	})
}

func (a *authenticator) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, true, map[string]bool{"authenticated": true})
	case http.MethodPost:
		a.createSession(w, r)
	case http.MethodDelete:
		a.deleteSession(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

func (a *authenticator) createSession(w http.ResponseWriter, r *http.Request) {
	if !a.tokenConfigured {
		writeJSON(w, http.StatusServiceUnavailable, false, "API token is not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		Token string `json:"token"`
	}
	if err := decoder.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON body")
		return
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, false, "request must contain one JSON object")
		return
	}

	providedHash := sha256.Sum256([]byte(body.Token))
	if subtle.ConstantTimeCompare(providedHash[:], a.tokenHash[:]) != 1 {
		writeJSON(w, http.StatusUnauthorized, false, "invalid credentials")
		return
	}

	rawID := make([]byte, 32)
	if _, err := rand.Read(rawID); err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "could not create session")
		return
	}
	sessionID := hex.EncodeToString(rawID)
	expiresAt := a.now().Add(sessionDuration)
	a.mu.Lock()
	a.removeExpiredSessionsLocked(a.now())
	a.sessions[sessionID] = expiresAt
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/api/v1",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"authenticated": true,
		"expires_at":    expiresAt.UTC(),
	})
}

func (a *authenticator) deleteSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/api/v1",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, true, map[string]bool{"authenticated": false})
}

func (a *authenticator) validSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.removeExpiredSessionsLocked(now)
	expiresAt, ok := a.sessions[sessionID]
	return ok && now.Before(expiresAt)
}

func (a *authenticator) removeExpiredSessionsLocked(now time.Time) {
	for sessionID, expiresAt := range a.sessions {
		if !now.Before(expiresAt) {
			delete(a.sessions, sessionID)
		}
	}
}

func sameHostOrigin(origin, requestHost string) bool {
	originURL, err := url.Parse(origin)
	if err != nil || (originURL.Scheme != "http" && originURL.Scheme != "https") ||
		originURL.User != nil || (originURL.Path != "" && originURL.Path != "/") ||
		originURL.RawQuery != "" || originURL.Fragment != "" {
		return false
	}
	requestURL, err := url.Parse("//" + requestHost)
	if err != nil || requestURL.Hostname() == "" {
		return false
	}
	return strings.EqualFold(
		strings.TrimSuffix(originURL.Hostname(), "."),
		strings.TrimSuffix(requestURL.Hostname(), "."),
	)
}
