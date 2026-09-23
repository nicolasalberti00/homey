package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicolasalberti00/homey/internal/config"
	"github.com/nicolasalberti00/homey/internal/storage"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, _ := newTestHandlerCfg(t, defaultTestConfig(t))
	return handler
}

// newTestHandlerUI builds the production chain with an injected UI build, so
// routing tests do not depend on the embedded files.
func newTestHandlerUI(t *testing.T, ui fs.FS) http.Handler {
	t.Helper()
	return newHandler(defaultTestConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)), newTestDB(t), ui)
}

// defaultTestConfig loads the standard configuration (defaults only).
func defaultTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(nil, nil, io.Discard)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg
}

// newTestHandlerCfg builds the production handler chain for cfg against a
// fresh database, so middleware tests exercise the real wiring.
func newTestHandlerCfg(t *testing.T, cfg *config.Config) (http.Handler, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(cfg, logger, db), db
}

// newTestDB opens a migrated database in the test's temporary directory.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "homey.db")
	if err := storage.MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func request(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body = %v, want status ok", body)
	}
}

func TestReadyzWithHealthyDatabase(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadyzWithClosedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "homey.db")
	if err := storage.MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := request(t, NewHandler(defaultTestConfig(t), logger, db), http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHealthzRejectsPost(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodPost, "/healthz")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/api/v1/nothing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	// The SPA fallback must not swallow API paths: the answer stays a JSON
	// problem document (Step 4.9).
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Errorf("Content-Type = %q, want a problem document", got)
	}
}

func TestClientRouteIsServedByTheUI(t *testing.T) {
	// The UI fixture keeps the assertion independent of the embedded build.
	rec := request(t, newTestHandlerUI(t, testUI()), http.MethodGet, "/rooms/1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want the shell", got)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Errorf("body = %q, want the shell", rec.Body.String())
	}
}
