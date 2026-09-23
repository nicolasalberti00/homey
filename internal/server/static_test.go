package server

// Tests for the embedded SPA handler: the routing rules are the
// interesting part — the shell fallback, the cache policy of hashed assets,
// JSON for unknown API paths and the honest 404 of a UI-less build.

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

// testUI is a miniature build: a shell, one hashed asset, one static file.
func testUI() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                    {Data: []byte("<!doctype html><title>homey</title>")},
		"_app/immutable/entry/start.js": {Data: []byte("console.log('start')")},
		"favicon.svg":                   {Data: []byte("<svg/>")},
	}
}

func TestSPAHandler(t *testing.T) {
	handler := newSPAHandler(testUI())

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCache  string
		wantType   string
		wantBody   string
	}{
		{
			name:       "root serves the shell",
			method:     http.MethodGet,
			path:       "/",
			wantStatus: http.StatusOK,
			wantCache:  "no-cache",
			wantType:   "text/html",
			wantBody:   "<!doctype html>",
		},
		{
			name:       "client-side route serves the shell",
			method:     http.MethodGet,
			path:       "/rooms/1",
			wantStatus: http.StatusOK,
			wantCache:  "no-cache",
			wantBody:   "<!doctype html>",
		},
		{
			name:       "hashed asset is immutable",
			method:     http.MethodGet,
			path:       "/_app/immutable/entry/start.js",
			wantStatus: http.StatusOK,
			wantCache:  "public, max-age=31536000, immutable",
			wantType:   "text/javascript",
			wantBody:   "console.log",
		},
		{
			name:       "other files get a short ttl",
			method:     http.MethodGet,
			path:       "/favicon.svg",
			wantStatus: http.StatusOK,
			wantCache:  "public, max-age=3600",
			wantBody:   "<svg/>",
		},
		{
			name:       "head carries headers and no body",
			method:     http.MethodHead,
			path:       "/",
			wantStatus: http.StatusOK,
			wantCache:  "no-cache",
			wantBody:   "",
		},
		{
			name:       "unknown api path is a problem document",
			method:     http.MethodGet,
			path:       "/api/v1/nope",
			wantStatus: http.StatusNotFound,
			wantType:   "application/problem+json",
			wantBody:   "no API operation is registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := request(t, handler, tt.method, tt.path)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if tt.wantType != "" {
				if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.wantType) {
					t.Errorf("Content-Type = %q, want prefix %q", got, tt.wantType)
				}
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// A development build has no UI: every path answers 404 with the hint, never
// the shell of a build that is not there.
func TestSPAHandlerWithoutUI(t *testing.T) {
	handler := newSPAHandler(fstest.MapFS{})

	for _, path := range []string{"/", "/rooms/1", "/_app/immutable/entry/start.js"} {
		rec := request(t, handler, http.MethodGet, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "not part of this binary") {
			t.Errorf("%s: body = %q, want the build hint", path, rec.Body.String())
		}
	}
}
