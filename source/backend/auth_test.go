package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionLoginProtectsAndLogoutRevokes(t *testing.T) {
	auth := newAuthenticator("router-secret")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/session", auth.handleSession)
	mux.HandleFunc("/api/v1/protected", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	server := auth.middleware(mux)

	unauthorized := httptest.NewRecorder()
	server.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"router-secret"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	server.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", loginResponse.Code, http.StatusOK, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login returned %d cookies, want 1", len(cookies))
	}
	session := cookies[0]
	if !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Secure {
		t.Fatalf("unexpected development session cookie attributes: %+v", session)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.AddCookie(session)
	protected := httptest.NewRecorder()
	server.ServeHTTP(protected, request)
	if protected.Code != http.StatusNoContent {
		t.Fatalf("authenticated request status = %d, want %d", protected.Code, http.StatusNoContent)
	}

	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	logout.AddCookie(session)
	logoutResponse := httptest.NewRecorder()
	server.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d", logoutResponse.Code, http.StatusOK)
	}

	revoked := httptest.NewRecorder()
	server.ServeHTTP(revoked, request)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d, want %d", revoked.Code, http.StatusUnauthorized)
	}
}

func TestSessionCookieIsSecureOverTLS(t *testing.T) {
	auth := newAuthenticator("router-secret")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/session", auth.handleSession)
	server := httptest.NewTLSServer(auth.middleware(mux))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/auth/session", strings.NewReader(`{"token":"router-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("TLS login request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("TLS login status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("unexpected TLS session cookie: %+v", cookies)
	}
}

func TestSessionRejectsInvalidAndMalformedCredentials(t *testing.T) {
	auth := newAuthenticator("router-secret")
	for _, body := range []string{
		`{"token":"wrong"}`,
		`{"token":"router-secret","extra":true}`,
		`{"token":"router-secret"} {}`,
		`not-json`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(body))
		response := httptest.NewRecorder()
		auth.handleSession(response, request)
		if response.Code == http.StatusOK {
			t.Fatalf("accepted invalid login body %q", body)
		}
	}
}

func TestSessionUnavailableWithoutConfiguredToken(t *testing.T) {
	auth := newAuthenticator("")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"anything"}`))
	response := httptest.NewRecorder()
	auth.handleSession(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("login status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestSessionExpires(t *testing.T) {
	auth := newAuthenticator("router-secret")
	now := time.Now()
	auth.now = func() time.Time { return now }
	auth.sessions["session-id"] = now.Add(time.Second)
	if !auth.validSession("session-id") {
		t.Fatal("session should be valid before expiry")
	}
	now = now.Add(time.Second)
	if auth.validSession("session-id") {
		t.Fatal("session should be expired at expiry time")
	}
}

func TestSameHostOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin string
		host   string
		want   bool
	}{
		{"http://router.lan", "router.lan:8080", true},
		{"https://192.168.1.1", "192.168.1.1:8080", true},
		{"https://evil.example", "router.lan:8080", false},
		{"null", "router.lan:8080", false},
		{"http://router.lan.attacker", "router.lan:8080", false},
		{"http://router.lan/path", "router.lan:8080", false},
	} {
		if got := sameHostOrigin(tc.origin, tc.host); got != tc.want {
			t.Errorf("sameHostOrigin(%q, %q) = %t, want %t", tc.origin, tc.host, got, tc.want)
		}
	}
}

func TestCorsRejectsForeignOriginAndAllowsSameHostPreflight(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/test", cors(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, "GET", "POST"))
	handler := corsMiddleware(mux)

	disallowed := httptest.NewRequest(http.MethodGet, "http://router.lan:8080/api/v1/system", nil)
	disallowed.Header.Set("Origin", "https://attacker.example")
	disallowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(disallowedResponse, disallowed)
	if disallowedResponse.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status = %d, want %d", disallowedResponse.Code, http.StatusForbidden)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "http://router.lan:8080/api/v1/test", nil)
	preflight.Header.Set("Origin", "http://router.lan")
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("same-host preflight status = %d, want %d", preflightResponse.Code, http.StatusNoContent)
	}
	if got := preflightResponse.Header().Get("Access-Control-Allow-Origin"); got != "http://router.lan" {
		t.Fatalf("allowed origin header = %q", got)
	}
	if got := preflightResponse.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q, want true", got)
	}
}

func TestUnauthenticatedResponseIncludesCorsHeaders(t *testing.T) {
	auth := newAuthenticator("router-secret")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/private", cors(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, "GET"))
	handler := corsMiddleware(auth.middleware(mux))
	request := httptest.NewRequest(http.MethodGet, "http://router.lan:8080/api/v1/private", nil)
	request.Header.Set("Origin", "http://router.lan")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://router.lan" {
		t.Fatalf("allow origin header = %q", got)
	}
}

func TestSessionResponseIsStructuredJSON(t *testing.T) {
	auth := newAuthenticator("router-secret")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"router-secret"}`))
	response := httptest.NewRecorder()
	auth.handleSession(response, request)
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("response is not successful: %v", body)
	}
}

func TestLoadAPITokenFailsWithoutFileOrEnv(t *testing.T) {
	t.Setenv("R3P_API_TOKEN", "")
	t.Setenv("R3P_API_TOKEN_FILE", filepath.Join(t.TempDir(), "missing-token"))
	token, err := loadAPIToken()
	if err == nil || token != "" {
		t.Fatalf("expected error and empty token, got token=%q err=%v", token, err)
	}

	emptyFile := filepath.Join(t.TempDir(), "empty-token")
	if err := os.WriteFile(emptyFile, []byte(" \n "), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("R3P_API_TOKEN_FILE", emptyFile)
	token, err = loadAPIToken()
	if err == nil || token != "" {
		t.Fatalf("expected error for empty token file, got token=%q err=%v", token, err)
	}
}

func TestCryptShadowVectors(t *testing.T) {
	vectors := []struct {
		hash string
		pass string
		want bool
	}{
		{"$5$test1234$3qn9hN9IGIiA.t9YvawRvRqSaG73wotUNqDcB5uxD93", "mysecret", true},
		{"$5$test1234$3qn9hN9IGIiA.t9YvawRvRqSaG73wotUNqDcB5uxD93", "wrongpass", false},
		{"$1$abcd1234$yuVPdq603u5AD2TPDRBy71", "mysecret", true},
		{"$1$abcd1234$yuVPdq603u5AD2TPDRBy71", "wrongpass", false},
		{"$6$test1234$U7.iXcOsT9Mmt4x09aYtZ9vB4hb1JWJTdcr3T7.FX/Ov6m8RdjlRENe.yTzVO7ikkxjRXQqop3C89rmOoaYsP.", "mysecret", true},
		{"!", "root", false},
		{"*", "root", false},
		{"", "root", false},
	}
	for _, v := range vectors {
		if got := verifyCryptHash(v.pass, v.hash); got != v.want {
			t.Errorf("verifyCryptHash(%q, %q) = %v, want %v", v.pass, v.hash, got, v.want)
		}
	}
}

func TestShadowPasswordAuthenticationRejectsHardcodedDefault(t *testing.T) {
	shadow := filepath.Join(t.TempDir(), "shadow")
	if err := os.WriteFile(shadow, []byte("root:$5$test1234$3qn9hN9IGIiA.t9YvawRvRqSaG73wotUNqDcB5uxD93:19000:0:99999:7:::\n"), 0600); err != nil {
		t.Fatal(err)
	}
	auth := newAuthenticator("hex-random-token-64")
	auth.shadowPath = shadow

	for _, bad := range []string{
		`{"username":"root","password":"root"}`,
		`{"username":"admin","password":"admin"}`,
		`{"username":"root","password":"admin"}`,
		`{"username":"other","password":"mysecret"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(bad))
		req.RemoteAddr = "192.168.1.50:1234"
		rec := httptest.NewRecorder()
		auth.handleSession(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("body %s got status %d, want 401", bad, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"username":"root","password":"mysecret"}`))
	req.RemoteAddr = "192.168.1.51:1234"
	rec := httptest.NewRecorder()
	auth.handleSession(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid shadow login got status %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Simulate password rotation in /etc/shadow
	if err := os.WriteFile(shadow, []byte("root:$1$abcd1234$yuVPdq603u5AD2TPDRBy71:19000:0:99999:7:::\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Old hash format swapped but verify rejection on wrong password
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"username":"root","password":"oldpassword"}`))
	reqBad.RemoteAddr = "192.168.1.52:1234"
	recBad := httptest.NewRecorder()
	auth.handleSession(recBad, reqBad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("wrong shadow login got status %d, want 401", recBad.Code)
	}
}

func TestLoginRateLimitReturns429(t *testing.T) {
	auth := newAuthenticator("router-secret")
	auth.shadowPath = ""
	now := time.Now()
	auth.now = func() time.Time { return now }

	for i := 0; i < loginRateLimitMax; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"bad"}`))
		req.RemoteAddr = "10.0.0.5:4321"
		rec := httptest.NewRecorder()
		auth.handleSession(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"router-secret"}`))
	blockedReq.RemoteAddr = "10.0.0.5:4321"
	blockedRec := httptest.NewRecorder()
	auth.handleSession(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked attempt status = %d, want 429", blockedRec.Code)
	}
	if blockedRec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}

	// Different IP succeeds
	otherReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"router-secret"}`))
	otherReq.RemoteAddr = "10.0.0.6:4321"
	otherRec := httptest.NewRecorder()
	auth.handleSession(otherRec, otherReq)
	if otherRec.Code != http.StatusOK {
		t.Fatalf("other IP status = %d, want 200", otherRec.Code)
	}

	// Original IP recovers after window expires
	now = now.Add(loginRateLimitWindow + time.Second)
	recoverReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"router-secret"}`))
	recoverReq.RemoteAddr = "10.0.0.5:4321"
	recoverRec := httptest.NewRecorder()
	auth.handleSession(recoverRec, recoverReq)
	if recoverRec.Code != http.StatusOK {
		t.Fatalf("post-window recovery status = %d, want 200", recoverRec.Code)
	}
}
