// Package webui carries the built single-page app inside the server binary.
//
// The dist/ tree is filled by the build pipeline: the Dockerfile copies
// web/build into it, and scripts/embed-ui.sh does the same for a local
// single-binary build. The committed .gitkeep keeps the directory — and
// therefore `go build` — working when the UI has not been built; the server
// then answers with a clean 404 instead of pretending the UI is there.
package webui

import (
	"embed"
	"io/fs"
)

// all: is required: SvelteKit's assets live under _app/, which a plain
// pattern would skip.
//
//go:embed all:dist
var assets embed.FS

// Assets returns the SPA root: index.html, _app/…, favicon and friends.
func Assets() fs.FS {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		// Unreachable: the embed directive above guarantees the directory.
		panic(err)
	}
	return dist
}
