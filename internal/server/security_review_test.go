package server

// Security review findings that had no guard: the size of a request
// body, the write rate limit over the MCP endpoint, and the promise that no log
// line carries a token. They run against the production handler chain.

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicolasalberti00/homey/internal/storage"
)

// TestRequestBodyIsCapped checks the body limit: a client that announces an
// oversized body is refused before anything is read, a client that streams one
// is cut while it flows, and neither leaves a mutation behind.
func TestRequestBodyIsCapped(t *testing.T) {
	handler, db := newTestHandlerCfg(t, defaultTestConfig(t))
	token := bearerValue(makeTokenHeader(t, storage.NewTokenStore(db), "test write", "read,write"))

	huge := `{"name":"Flood","description":"` + strings.Repeat("x", int(maxRequestBodyBytes)) + `"}`

	// The honest client: Content-Length says it is over the cap.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(huge))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: status = %d, want 413; body: %s", rec.Code, rec.Body.String())
	}

	// The streaming client: no Content-Length, so only the cap can stop it.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(huge))
	req.ContentLength = -1
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated {
		t.Fatalf("a streamed body past the cap was accepted; body: %s", rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("streamed oversized body: status = %d, want a 4xx", rec.Code)
	}

	// Neither attempt wrote anything.
	var rooms int
	if err := db.QueryRow(`SELECT count(*) FROM rooms`).Scan(&rooms); err != nil {
		t.Fatalf("counting rooms: %v", err)
	}
	if rooms != 0 {
		t.Fatalf("rooms = %d, want none: the refusals must not mutate", rooms)
	}

	// A body within the cap still goes through.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"name":"Garage"}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("a normal write: status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
}

// TestMCPPostsCountTowardTheWriteLimit closes the gap the review found: the
// write limit covered /api/v1 only, so the endpoint an agent drives mutations
// through was the one endpoint not limited.
func TestMCPPostsCountTowardTheWriteLimit(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.RateLimitWrites = 2
	cfg.RateLimitAuthFailures = 0 // isolate this test
	handler, _ := newTestHandlerCfg(t, cfg)

	for i := 1; i <= 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"initialize"}`))
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("POST /mcp %d: status = %d, want 401 (unauthenticated, but counted)", i, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"initialize"}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third POST /mcp: status = %d, want 429; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing on 429")
	}

	// The long-lived event stream is a read and stays open: GET /mcp is not a
	// write, so it is not limited.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("the event stream was rate limited; only POST counts")
	}
}

// TestLogsNeverCarryAToken pins the audit promise: what a request presented as
// a credential never reaches a log line, neither when it works nor when it is
// rejected. The logs carry the reason and the path instead.
func TestLogsNeverCarryAToken(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := NewHandler(defaultTestConfig(t), logger, newTestDB(t))
	token := bearerValue(makeTokenHeader(t, storage.NewTokenStore(newTestDB(t)), "test write", "read,write"))

	write := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"name":"Garage"}`))
	write.Header.Set("Authorization", token)
	write.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), write)

	rejected := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	rejected.Header.Set("Authorization", "Bearer homey_wrong_guess")
	handler.ServeHTTP(httptest.NewRecorder(), rejected)

	captured := logs.String()
	if !strings.Contains(captured, "/api/v1/rooms") {
		t.Fatalf("logs = %q, want the request to be there at all", captured)
	}
	if plaintext := strings.TrimPrefix(token, "Bearer "); plaintext != "" {
		if strings.Contains(captured, plaintext) {
			t.Errorf("a valid token reached the logs:\n%s", captured)
		}
	}
	if strings.Contains(captured, "homey_wrong_guess") {
		t.Errorf("a presented token reached the logs:\n%s", captured)
	}
}

// bearerValue turns the "Authorization: Bearer …" line the token helpers return
// into the header value an http request carries.
func bearerValue(line string) string {
	return strings.TrimPrefix(line, "Authorization: ")
}
