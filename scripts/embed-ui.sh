#!/bin/sh
# Build the SPA and copy it into the tree the Go binary embeds
# (internal/webui/dist), so `go build ./cmd/server` yields one self-contained
# binary that serves the UI itself. The Docker image does the same in two
# stages (see Dockerfile).
#
# Usage: scripts/embed-ui.sh [--no-build]
#   --no-build  reuse the existing web/build instead of running npm run build
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${1:-}" != "--no-build" ]; then
	(cd web && npm run build)
fi

if [ ! -f web/build/index.html ]; then
	echo "web/build/index.html is missing: build the UI first" >&2
	exit 1
fi

# Replace the whole tree so stale assets from an earlier build cannot survive,
# then restore the placeholder that keeps `go build` working without a UI.
rm -rf internal/webui/dist
cp -R web/build internal/webui/dist
touch internal/webui/dist/.gitkeep

echo "UI embedded from web/build into internal/webui/dist"
echo "next: go build ./cmd/server"
