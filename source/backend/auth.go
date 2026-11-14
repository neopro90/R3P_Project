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
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName    = "r3p_session"
	sessionDuration      = 8 * time.Hour
	maxAuthBody          = 1024
	loginRateLimitWindow = 5 * time.Minute
	loginRateLimitMax    = 10
	maxTrackedAttemptIPs = 256
	maxActiveSessions    = 64
)

type loginAttemptState struct {
	failures  int
	windowEnd time.Time
}

type authenticator struct {
	tokenHash       [sha256.Size]byte
	tokenConfigured bool
	shadowPath      string
	mu              sync.Mutex
	sessions        map[string]time.Time
	attempts        map[string]loginAttemptState
	now             func() time.Time
}

var globalAuth *authenticator

func defaultShadowPath() string {
	if info, err := os.Stat("/etc/shadow"); err == nil && !info.IsDir() {
		return "/etc/shadow"
	}
	return ""
}

func newAuthenticator(token string) *authenticator {
	token = strings.TrimSpace(token)
	a := &authenticator{
		tokenHash:       sha256.Sum256([]byte(token)),
		tokenConfigured: token != "",
		shadowPath:      defaultShadowPath(),
		sessions:        make(map[string]time.Time),
		attempts:        make(map[string]loginAttemptState),
		now:             time.Now,
	}
	globalAuth = a
	return a
}

func (a *authenticator) updateToken(token string) {
	token = strings.TrimSpace(token)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokenHash = sha256.Sum256([]byte(token))
	a.tokenConfigured = token != ""
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
		return "", fmt.Errorf("read API token %s: %w", tokenPath, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("API token file is empty")
	}
	return token, nil
}

func (a *authenticator) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") ||
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

func clientRemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if addr := strings.TrimSpace(r.RemoteAddr); addr != "" {
		return addr
	}
	return "unknown"
}

func (a *authenticator) rateLimitRetryAfter(ip string, now time.Time) (int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range a.attempts {
		if !now.Before(v.windowEnd) {
			delete(a.attempts, k)
		}
	}
	st, ok := a.attempts[ip]
	if ok && st.failures >= loginRateLimitMax && now.Before(st.windowEnd) {
		sec := int(st.windowEnd.Sub(now).Seconds())
		if sec < 1 {
			sec = 1
		}
		return sec, true
	}
	return 0, false
}

func (a *authenticator) recordLoginFailure(ip string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.attempts) >= maxTrackedAttemptIPs {
		for k, v := range a.attempts {
			if !now.Before(v.windowEnd) {
				delete(a.attempts, k)
			}
		}
		if len(a.attempts) >= maxTrackedAttemptIPs {
			for k := range a.attempts {
				delete(a.attempts, k)
				if len(a.attempts) < maxTrackedAttemptIPs {
					break
				}
			}
		}
	}
	st, ok := a.attempts[ip]
	if !ok || !now.Before(st.windowEnd) {
		a.attempts[ip] = loginAttemptState{
			failures:  1,
			windowEnd: now.Add(loginRateLimitWindow),
		}
		return
	}
	st.failures++
	a.attempts[ip] = st
}

func (a *authenticator) clearLoginFailures(ip string) {
	a.mu.Lock()
	delete(a.attempts, ip)
	a.mu.Unlock()
}

func (a *authenticator) createSession(w http.ResponseWriter, r *http.Request) {
	if !a.tokenConfigured && a.shadowPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, false, "API token is not configured")
		return
	}

	now := a.now()
	ip := clientRemoteIP(r)
	if retryAfter, limited := a.rateLimitRetryAfter(ip, now); limited {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeJSON(w, http.StatusTooManyRequests, false, "too many failed login attempts")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
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

	authenticated := false
	if body.Token != "" {
		providedHash := sha256.Sum256([]byte(body.Token))
		a.mu.Lock()
		expectedHash := a.tokenHash
		a.mu.Unlock()
		if subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1 {
			authenticated = true
		}
	}
	if !authenticated && body.Password != "" {
		user := strings.ToLower(strings.TrimSpace(body.Username))
		if user == "" || user == "root" || user == "admin" {
			providedHash := sha256.Sum256([]byte(body.Password))
			a.mu.Lock()
			expectedHash := a.tokenHash
			shadowPath := a.shadowPath
			a.mu.Unlock()
			if subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1 {
				authenticated = true
			} else if (user == "" || user == "root" || user == "admin") && shadowPath != "" {
				authenticated = verifyShadowPassword(shadowPath, "root", body.Password)
			}
		}
	}

	if !authenticated {
		a.recordLoginFailure(ip, now)
		writeJSON(w, http.StatusUnauthorized, false, "invalid credentials")
		return
	}
	a.clearLoginFailures(ip)

	rawID := make([]byte, 32)
	if _, err := rand.Read(rawID); err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "could not create session")
		return
	}
	sessionID := hex.EncodeToString(rawID)
	expiresAt := a.now().Add(sessionDuration)
	a.mu.Lock()
	a.removeExpiredSessionsLocked(a.now())
	for len(a.sessions) >= maxActiveSessions {
		var oldestID string
		var oldestExp time.Time
		for sid, exp := range a.sessions {
			if oldestID == "" || exp.Before(oldestExp) {
				oldestID = sid
				oldestExp = exp
			}
		}
		if oldestID == "" {
			break
		}
		delete(a.sessions, oldestID)
	}
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

func sameOriginForRequest(origin string, r *http.Request) bool {
	if r == nil || !sameHostOrigin(origin, r.Host) {
		return false
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if r.TLS != nil && originURL.Scheme != "https" {
		return false
	}
	if op := originURL.Port(); op != "" {
		reqURL, err := url.Parse("//" + r.Host)
		if err != nil {
			return false
		}
		rp := reqURL.Port()
		if rp == "" {
			if r.TLS != nil {
				rp = "443"
			} else {
				rp = "80"
			}
		}
		if op != rp {
			return false
		}
	}
	return true
}
