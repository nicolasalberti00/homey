package server

// The built single-page app ships inside the binary (Step 4.9). The handler
// serves the embedded files, falls back to index.html so client-side routes
// survive a reload, and keeps unknown /api/ paths JSON: an API client must
// never receive the HTML shell.
//
// Only GET and HEAD reach it — the root mux registers the pattern as
// "GET /" — so unknown methods on API paths keep the mux's 405.

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const (
	// immutablePrefix marks the content-hashed part of a SvelteKit build.
	immutablePrefix = "_app/immutable/"
	// notBuiltHint explains an empty UI build; see internal/webui.
	notBuiltHint = "the web UI is not part of this binary: run scripts/embed-ui.sh (see README)"
)

// newSPAHandler serves the SPA rooted at fsys. The /api/ rule comes first so
// it holds for every build, UI embedded or not.
func newSPAHandler(fsys fs.FS) http.Handler {
	shell := newShellHandler(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "api" || strings.HasPrefix(name, "api/") {
			// The API owns /api/: an unmatched path is a problem document,
			// not a page a browser can render.
			writeProblem(w, http.StatusNotFound, "no API operation is registered for this path")
			return
		}
		shell.ServeHTTP(w, r)
	})
}

// newShellHandler serves the embedded build, or a hint when the binary
// carries no UI (a development build).
func newShellHandler(fsys fs.FS) http.Handler {
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		// Say so instead of serving a blank page or a confusing 500.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, notBuiltHint, http.StatusNotFound)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		if name != "" && name != "." {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				serveUI(w, r, fsys, name)
				return
			}
		}
		// The root, a client-side route or an unknown file: hand over the
		// shell and let the router decide.
		serveUI(w, r, fsys, "index.html")
	})
}

// serveUI writes one embedded file with its cache policy. SvelteKit
// fingerprints everything under _app/immutable/, so those files can be cached
// forever; the shell must be revalidated, because it points at the new hashes
// after a deployment.
func serveUI(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	switch {
	case strings.HasPrefix(name, immutablePrefix):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasSuffix(name, ".html"):
		w.Header().Set("Cache-Control", "no-cache")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.ServeFileFS(w, r, fsys, name)
}
