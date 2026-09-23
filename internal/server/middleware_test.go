package server

// Unit tests for the middleware pieces: the fixed-window counter that backs
// both rate limits, the over-limit problem contract, and the pass-through
// behaviour when a limit is disabled (0).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nicolasalberti00/homey/internal/storage"
)

func TestFixedWindowEnforcesLimitAndReportsRetryAfter(t *testing.T) {
	f := newFixedWindow(time.Minute, 2)
	for i := 0; i < 2; i++ {
		retryAfter, ok := f.allow("ip")
		if !ok {
			t.Fatalf("allow %d: over the limit early", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("allow %d: retryAfter = %d, want 0 while under the limit", i+1, retryAfter)
		}
	}
	retryAfter, ok := f.allow("ip")
	if ok {
		t.Fatal("allow 3: expected to be over the limit")
	}
	if retryAfter <= 0 || retryAfter > 60 {
		t.Errorf("retryAfter = %d, want seconds remaining in the window", retryAfter)
	}
	if retryAfter := f.retryAfter("ip"); retryAfter <= 0 || retryAfter > 60 {
		t.Errorf("retryAfter = %d, want seconds remaining in the window", retryAfter)
	}
	// Other keys are unaffected.
	if _, ok := f.allow("other"); !ok {
		t.Error("another key was limited by the first key's window")
	}
	if retryAfter := f.retryAfter("other"); retryAfter != 0 {
		t.Errorf("retryAfter for an unblocked key = %d, want 0", retryAfter)
	}
}

func TestFixedWindowRolloverResets(t *testing.T) {
	// No sleeps: the window is long, and the rollover is forced
	// deterministically by moving the counter's reset into the past, so CI
	// timing can never flake this test.
	f := newFixedWindow(time.Minute, 1)
	if _, ok := f.allow("ip"); !ok {
		t.Fatal("first allow should pass")
	}
	if _, ok := f.allow("ip"); ok {
		t.Fatal("second allow within the window should fail")
	}
	if f.retryAfter("ip") == 0 {
		t.Fatal("retryAfter should be positive at the limit")
	}

	// Force the window to expire: blocked clears and allow opens a fresh
	// window — the old over-limit state was rolled over.
	f.mu.Lock()
	f.keys["ip"].reset = time.Now().Add(-time.Millisecond)
	f.mu.Unlock()
	if retryAfter := f.retryAfter("ip"); retryAfter != 0 {
		t.Errorf("retryAfter = %d, want 0 once the window has expired", retryAfter)
	}
	if _, ok := f.allow("ip"); !ok {
		t.Error("allow after the window rolled over should pass")
	}
}

func TestFixedWindowCleanupSweepsExpiredKeys(t *testing.T) {
	f := newFixedWindow(time.Minute, 1)
	f.mu.Lock()
	for i := 0; i < 4100; i++ {
		f.keys[fmt.Sprintf("old-%d", i)] = &windowCounter{reset: time.Now().Add(-time.Second)}
	}
	f.mu.Unlock()

	if _, ok := f.allow("fresh"); !ok {
		t.Fatal("fresh key should pass")
	}
	f.mu.Lock()
	size := len(f.keys)
	f.mu.Unlock()
	if size > 10 {
		t.Errorf("map size after cleanup = %d, want expired entries swept", size)
	}
}

func TestDisabledLimitsArePassThrough(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.RateLimitWrites = 0
	cfg.RateLimitAuthFailures = 0
	handler, db := newTestHandlerCfg(t, cfg)
	token := strings.TrimPrefix(makeTokenHeader(t, storage.NewTokenStore(db), "rw", "read,write"), "Authorization: ")

	// Repeated writes and repeated failed authentications are never limited.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"name":"R`+string(rune('A'+i))+`"}`))
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("write %d: status = %d, want 201 with the write limit disabled", i+1, rec.Code)
		}
	}
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
		req.Header.Set("Authorization", "Bearer homey_wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("bad auth %d: status = %d, want 401 with the auth limit disabled", i+1, rec.Code)
		}
	}
}

// assertProblem429 checks the full over-limit contract: status, problem
// media type, RFC 9457 fields and the Retry-After hint.
func assertProblem429(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	if retry := rec.Header().Get("Retry-After"); retry == "" {
		t.Error("Retry-After missing on 429")
	} else if seconds, err := strconv.Atoi(retry); err != nil || seconds <= 0 {
		t.Errorf("Retry-After = %q, want positive seconds", retry)
	}
	var problem struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Status != http.StatusTooManyRequests || problem.Title == "" || problem.Detail == "" {
		t.Errorf("problem = %+v, want status 429 with title and detail", problem)
	}
}

func TestRateLimitProblemContract(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.RateLimitWrites = 1
	handler, db := newTestHandlerCfg(t, cfg)
	token := strings.TrimPrefix(makeTokenHeader(t, storage.NewTokenStore(db), "rw", "read,write"), "Authorization: ")

	// First write passes, second is over the limit and must be a problem.
	for i, want := range []int{http.StatusCreated, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"name":"Room`+string(rune('A'+i))+`"}`))
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("write %d: status = %d, want %d; body: %s", i+1, rec.Code, want, rec.Body.String())
		}
		if want == http.StatusTooManyRequests {
			assertProblem429(t, rec)
		}
	}
}

func TestAuthFailureProblemContract(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.RateLimitAuthFailures = 1
	handler, _ := newTestHandlerCfg(t, cfg)

	// First failure is a plain 401 (an API problem), the next is blocked.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	req.Header.Set("Authorization", "Bearer homey_wrong")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("first failure: status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	req.Header.Set("Authorization", "Bearer homey_wrong")
	handler.ServeHTTP(rec, req)
	assertProblem429(t, rec)
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:54321"
	if got := clientIP(req); got != "192.0.2.10" {
		t.Errorf("clientIP = %q, want host part without the port", got)
	}
	req.RemoteAddr = "unix-socket-peer"
	if got := clientIP(req); got != "unix-socket-peer" {
		t.Errorf("clientIP = %q, want the raw address when there is no port", got)
	}
}
