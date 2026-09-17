package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nicolasalberti00/homey/internal/storage"
)

func newTestHandler(t *testing.T) http.Handler {
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(logger, db)
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
	rec := request(t, NewHandler(logger, db), http.MethodGet, "/readyz")
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
}
