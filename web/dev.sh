#!/usr/bin/env bash
#
# Local UI development helper: builds and starts the API on a throwaway
# database, mints a fresh read/write token, then runs the Vite dev server.
# Ctrl-C stops everything (no orphan processes, no `go run` zombies).
#
#   ./dev.sh
#
# Environment:
#   HOMEY_TEST_DB        database path        (default: ~/homey-dev.db)
#   HOMEY_TEST_LISTEN    API listen address   (default: 127.0.0.1:8080)
#   HOMEY_TEST_WEB_PORT  Vite port            (default: 5173)
#   HOMEY_TEST_RESET=1   delete the database before starting (fresh state)

set -euo pipefail

cd "$(dirname "$0")/.." # repo root

DB_PATH="${HOMEY_TEST_DB:-$HOME/homey-dev.db}"
API_ADDR="${HOMEY_TEST_LISTEN:-127.0.0.1:8080}"
API_PORT="${API_ADDR##*:}"
WEB_PORT="${HOMEY_TEST_WEB_PORT:-5173}"

if [[ "${HOMEY_TEST_RESET:-0}" == "1" ]]; then
	echo "Resetting the test database: $DB_PATH"
	rm -f "$DB_PATH" "$DB_PATH-wal" "$DB_PATH-shm"
fi

port_busy() {
	command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1
}

if port_busy "$API_PORT"; then
	echo "error: port $API_PORT is already in use (a stale server? check: lsof -nP -iTCP:$API_PORT -sTCP:LISTEN)" >&2
	exit 1
fi
if port_busy "$WEB_PORT"; then
	echo "error: port $WEB_PORT is already in use (a stale dev server?)" >&2
	exit 1
fi

WORK_DIR="$(mktemp -d)"
API_PID=""
WEB_PID=""

# kill_tree terminates a process and its descendants: killing the npm
# wrapper alone would leave the actual vite node process behind.
kill_tree() {
	local pid="$1" child
	for child in $(pgrep -P "$pid" 2>/dev/null); do
		kill_tree "$child"
	done
	kill "$pid" 2>/dev/null || true
}

cleanup() {
	trap - EXIT INT TERM
	[[ -n "$API_PID" ]] && kill_tree "$API_PID"
	[[ -n "$WEB_PID" ]] && kill_tree "$WEB_PID"
	wait 2>/dev/null || true
	rm -rf "$WORK_DIR"
	echo
	echo "Stopped. Test data kept in $DB_PATH (delete it for a clean slate)."
}
trap cleanup EXIT INT TERM

# Build once and run the binary: killing the binary stops the server cleanly,
# unlike `go run`, which can leave an orphan process holding the port.
echo "Building the server…"
go build -o "$WORK_DIR/homey" ./cmd/server

echo "Starting the API on http://$API_ADDR (db: $DB_PATH)"
"$WORK_DIR/homey" serve --listen "$API_ADDR" --db "$DB_PATH" &
API_PID=$!

for _ in $(seq 1 40); do
	curl -sf "http://$API_ADDR/healthz" >/dev/null 2>&1 && break
	kill -0 "$API_PID" 2>/dev/null || {
		echo "error: the API exited during startup" >&2
		exit 1
	}
	sleep 0.25
done

# Fresh token each run; previous "dev" tokens are revoked to keep the list tidy.
while read -r id; do
	[[ -n "$id" ]] && "$WORK_DIR/homey" token revoke --db "$DB_PATH" "$id" >/dev/null
done < <("$WORK_DIR/homey" token list --db "$DB_PATH" | awk '$2 == "dev" { print $1 }')

TOKEN="$("$WORK_DIR/homey" token create --name dev --scope read,write --db "$DB_PATH" |
	grep -o 'homey_[A-Za-z0-9_-]*' | head -1)"

if [[ -z "$TOKEN" ]]; then
	echo "error: could not create an API token" >&2
	exit 1
fi

if [[ ! -d web/node_modules ]]; then
	echo "Installing UI dependencies…"
	(cd web && npm install)
fi

echo
echo "──────────────────────────────────────────────────────────────"
echo " UI:    http://localhost:$WEB_PORT"
echo " Token: $TOKEN"
echo
echo " Paste the token in Settings → API token, then Test connection."
echo " Ctrl-C stops the API and the dev server."
echo "──────────────────────────────────────────────────────────────"
echo

(cd web && npm run dev -- --port "$WEB_PORT" --strictPort) &
WEB_PID=$!

wait
