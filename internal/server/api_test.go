package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIServesOpenAPIJSON(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/api/v1/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/openapi+json") {
		t.Errorf("content type = %q, want application/openapi+json", ct)
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decoding document: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("openapi = %q, want 3.1.0", doc.OpenAPI)
	}
	if doc.Info.Title != "homey API" {
		t.Errorf("title = %q, want homey API", doc.Info.Title)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "/api/v1" {
		t.Errorf("servers = %+v, want one server with url /api/v1", doc.Servers)
	}
}

func TestAPIServesOpenAPIYAML(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/api/v1/openapi.yaml")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Errorf("content type = %q, want a YAML type", ct)
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3.1.0") {
		t.Error("openapi.yaml does not look like an OpenAPI 3.1 document")
	}
}

func TestAPIServesDocsUI(t *testing.T) {
	rec := request(t, newTestHandler(t), http.MethodGet, "/api/v1/docs")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "stoplight") {
		t.Error("docs UI should be rendered with Stoplight Elements")
	}
}

// TestOpenAPIArtifactIsCurrent fails whenever the generated document and the
// committed api/openapi.yaml drift apart, so local tests enforce the same
// rule as CI.
func TestOpenAPIArtifactIsCurrent(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repository root: %v", err)
	}

	artifact, err := os.ReadFile(filepath.Join(root, "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}

	api := NewAPI(http.NewServeMux(), Deps{})
	generated, err := api.OpenAPI().YAML()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	if string(artifact) != string(generated) {
		t.Fatalf("api/openapi.yaml is out of date: run `go run ./cmd/openapi > api/openapi.yaml` and commit the result")
	}
}

// findRepoRoot walks up from the current directory until it finds go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
