package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestSearchIsAStable501Stub(t *testing.T) {
	api, bearer := newTestAPI(t)

	rec := api.Get("/search?q=trapano", bearer.Read)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Errorf("content type = %q, want application/problem+json", ct)
	}
	status, title, _, _ := decodeProblem(t, rec)
	if status != http.StatusNotImplemented || title != "Not Implemented" {
		t.Fatalf("problem = %d/%q", status, title)
	}

	// The query is part of the contract already.
	rec = api.Get("/search", bearer.Read)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 without q", rec.Code)
	}
}
