package server

import (
	"net/http"
	"testing"
)

// TestBearerAuthentication covers the contract of the token middleware:
// 401 for missing or invalid tokens, 403 for read-only tokens on writes,
// 200 for valid tokens, and public paths that stay open.
func TestBearerAuthentication(t *testing.T) {
	api, tokens := newTestAPI(t)

	// Missing token: 401 with WWW-Authenticate and an RFC 9457 problem.
	rec := api.Get("/rooms")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
	if wa := rec.Header().Get("WWW-Authenticate"); wa == "" {
		t.Error("401 responses should carry a WWW-Authenticate header")
	}
	status, title, _, _ := decodeProblem(t, rec)
	if status != http.StatusUnauthorized || title != "Unauthorized" {
		t.Fatalf("problem = %d/%q", status, title)
	}

	// Malformed and unknown tokens: 401.
	rec = api.Get("/rooms", "Authorization: not-bearer")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a malformed authorization header", rec.Code)
	}
	rec = api.Get("/rooms", "Authorization: Bearer homey_forged")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for an unknown token", rec.Code)
	}

	// A read-only token may read...
	rec = api.Get("/rooms", tokens.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with a read token", rec.Code)
	}
	// ...but not write: 403.
	rec = api.Post("/rooms", tokens.Read, RoomInput{Name: "Garage"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 with a read-only token; body: %s", rec.Code, rec.Body.String())
	}
	status, title, _, _ = decodeProblem(t, rec)
	if status != http.StatusForbidden || title != "Forbidden" {
		t.Fatalf("problem = %d/%q", status, title)
	}

	// A write token can create.
	rec = api.Post("/rooms", tokens.Write, RoomInput{Name: "Garage"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 with a write token; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPublicPathsStayOpen(t *testing.T) {
	handler := newTestHandler(t)

	for _, path := range []string{"/api/v1/openapi.json", "/api/v1/openapi.yaml", "/api/v1/docs"} {
		rec := request(t, handler, http.MethodGet, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200 without a token", path, rec.Code)
		}
	}
}
