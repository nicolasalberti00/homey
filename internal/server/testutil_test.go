package server

// Shared helpers for the API handler tests: they build an API with the
// production configuration (apiConfig) and the full set of operations wired
// to a fresh SQLite database, mirroring the production wiring.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/nicolasalberti00/homey/internal/storage"
)

func newTestAPI(t *testing.T) humatest.TestAPI {
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

	_, api := humatest.New(t, apiConfig())
	registerOperations(api, Deps{Repos: storage.NewRepos(db), Logger: logger})
	return api
}

// decodeProblem decodes an RFC 9457 problem document, including the optional
// per-field details.
func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) (status int, title string, detail string, locations []string) {
	t.Helper()
	var problem struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
		Errors []struct {
			Location string `json:"location"`
			Message  string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decoding problem: %v; body: %s", err, rec.Body.String())
	}
	for _, e := range problem.Errors {
		locations = append(locations, e.Location)
	}
	return problem.Status, problem.Title, problem.Detail, locations
}
