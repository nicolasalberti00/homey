package server

// Tests for the transport hardening middlewares: security headers, CORS
// (off by default), the write rate limit and the failed-authentication
// block. They run against the production handler chain.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nicolasalberti00/homey/internal/storage"
)

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestCORSIsOffByDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	newTestHandler(t).ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want none (same-origin only)", got)
	}
}

func TestCORSAllowsConfiguredOrigins(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.CORSOrigins = []string{"https://app.example"}
	handler, _ := newTestHandlerCfg(t, cfg)

	// An allowed origin is tagged on regular responses.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if vary := rec.Header().Get("Vary"); vary != "Origin" {
		t.Errorf("Vary = %q, want Origin", vary)
	}

	// A different origin is not tagged.
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://other.example")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q for a disallowed origin", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.CORSOrigins = []string{"https://app.example"}
	handler, _ := newTestHandlerCfg(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/rooms", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PATCH, DELETE" {
		t.Errorf("Allow-Methods = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Errorf("Allow-Headers = %q", got)
	}
	if rec.Header().Get("Access-Control-Max-Age") == "" {
		t.Error("Max-Age missing")
	}
}

func TestWriteRateLimit(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.RateLimitWrites = 2
	cfg.RateLimitAuthFailures = 0 // isolate this test
	handler, db := newTestHandlerCfg(t, cfg)
	// makeTokenHeader returns a full "Name: value" line for the humatest
	// helpers; httptest needs only the value.
	token := strings.TrimPrefix(makeTokenHeader(t, storage.NewTokenStore(db), "test write", "read,write"), "Authorization: ")

	for i, name := range []string{"Garage", "Kitchen"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"name":"`+name+`"}`))
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("write %d: status = %d, want 201; body: %s", i+1, rec.Code, rec.Body.String())
		}
	}

	// Third write in the window: 429 with Retry-After.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"name":"Office"}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body: %s", rec.Code, rec.Body.String())
	}
	if retry := rec.Header().Get("Retry-After"); retry == "" {
		t.Error("Retry-After missing on 429")
	} else if _, err := strconv.Atoi(retry); err != nil {
		t.Errorf("Retry-After = %q, want seconds", retry)
	}

	// Reads are not limited.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read after 429: status = %d, want 200", rec.Code)
	}
}

func TestAuthFailureRateLimit(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.RateLimitAuthFailures = 2
	handler, _ := newTestHandlerCfg(t, cfg)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
		req.Header.Set("Authorization", "Bearer homey_guessed")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	// Third failed attempt is blocked outright.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	req.Header.Set("Authorization", "Bearer homey_guessed")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after repeated auth failures; body: %s", rec.Code, rec.Body.String())
	}
}
