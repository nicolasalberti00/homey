# homey

Self-hosted home inventory: keep track of objects by room and nested
containers, manage them from a web UI, search them, and let an LLM drive it
through MCP. One binary, SQLite inside, no cloud account, no external
database, no mandatory AI provider.

## Quickstart

### Docker

```bash
mkdir -p data && sudo chown 1000:1000 data # the container writes the database as uid 1000
docker compose up -d
curl -s http://localhost:8080/healthz
```

Create a token, then open <http://localhost:8080> and paste it in
**Settings** (the UI keeps it in that browser's `localStorage` and sends it as
`Authorization: Bearer …` on every request):

```bash
docker compose exec homey homey token create --name web --scope read,write
```

Data lives in `./data` (SQLite). The container runs as uid 1000 and that
directory must be writable by it: the setup line above takes care of it, and
it applies on Linux hosts and on macOS alike (Docker Desktop enforces the
host's file permissions through its file sharing).

To build for a specific architecture (amd64/arm64):

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t homey:dev .
```

### From source

```bash
./scripts/embed-ui.sh    # build the SPA into the binary (optional, see below)
go run ./cmd/server serve
go run ./cmd/server token create --name web --scope read,write
```

Migrations run at start-up. Without the embedded UI the server answers `/`
with a 404 and a hint rather than a blank page.

## Configuration

Every setting can be provided as an environment variable and as a flag;
flags take precedence.

| Env var              | Flag             | Default               | Description                          |
| -------------------- | ---------------- | --------------------- | ------------------------------------ |
| `HOMEY_LISTEN`       | `--listen`       | `:8080`               | HTTP listen address                  |
| `HOMEY_DATA_DIR`     | `--data-dir`     | `./data`              | Directory for persistent data        |
| `HOMEY_DB_PATH`      | `--db`           | `<data-dir>/homey.db` | SQLite database file                 |
| `HOMEY_LOG_LEVEL`    | `--log-level`    | `info`                | debug, info, warn, error             |
| `HOMEY_LOG_FORMAT`   | `--log-format`   | `text`                | text or json                         |
| `HOMEY_CORS_ORIGINS` | `--cors-origins` | *(empty)*             | Comma-separated allowed CORS origins |
| `HOMEY_MCP_ENABLED`  | `--mcp`          | `true`                | Serve the MCP endpoint at `/mcp`     |
| `HOMEY_RATE_LIMIT_WRITES` | `--rate-limit-writes` | `60`            | Mutating requests per IP per minute (0 disables) |
| `HOMEY_RATE_LIMIT_AUTH_FAILURES` | `--rate-limit-auth-failures` | `10` | Failed authentication attempts per IP per minute (0 disables) |

## API

Every operation under `/api/v1/` requires `Authorization: Bearer <token>`;
only the spec, the docs UI and the schemas are public. Errors are RFC 9457
problem documents (`application/problem+json`) carrying `status`, `detail`
and — for validation failures — one entry per field, so a client knows which
field to fix.

Every response carries defensive headers (`nosniff`, frame deny, referrer
policy, permissions policy). CORS is same-origin by default;
`HOMEY_CORS_ORIGINS` lets browser clients from other origins through.

The contract is generated from the code, never written by hand, and served at
`GET /api/v1/openapi.json` (and rendered at `GET /api/v1/docs`); the committed
artifact [`api/openapi.yaml`](api/openapi.yaml) is what clients generate from.

- Rooms — `GET/POST /api/v1/rooms`, `GET/PATCH/DELETE /api/v1/rooms/{id}`
- Containers — `GET/POST /api/v1/containers` (filter `room_id`),
  `GET/PATCH/DELETE /api/v1/containers/{id}`,
  `POST /api/v1/containers/{id}/move`
- Items — `GET/POST /api/v1/items` (filters `room_id`, `container_id`),
  `GET/PATCH/DELETE /api/v1/items/{id}`, `POST /api/v1/items/{id}/move`
- Search — `GET /api/v1/search?q=…` (see [Search](#search))
- Backup — `GET /api/v1/export` and `POST /api/v1/import` (see [Backup](#backup))
- Health — `GET /healthz`, `GET /readyz`

Regenerate the contract after changing any operation:

```bash
go run ./cmd/openapi > api/openapi.yaml
```

### API tokens

Tokens are stored hashed (SHA-256) and shown once, at creation:

```bash
go run ./cmd/server token create --name "curl" --scope read,write
go run ./cmd/server token list
go run ./cmd/server token revoke <id>
```

`--scope read` allows only reads; `--scope read,write` also allows mutations.
Without a token the API answers `401`; a read-only token gets `403` on a
mutation. `--destructive-confirmation=bypass` marks a token that may run
destructive tools without the extra confirmation step (see
[MCP](#mcp)); the bypass is recorded in the event log.

Repeated failures from one address are answered with `429`
(`HOMEY_RATE_LIMIT_AUTH_FAILURES`), and mutations are rate limited the same
way (`HOMEY_RATE_LIMIT_WRITES`). Both are keyed by the direct peer address:
behind a reverse proxy every client shares the proxy's bucket, so tune them
there, or disable them and let the proxy rate limit.

## Search

`GET /api/v1/search?q=…` is the single search behind the REST API, the web UI
and the MCP server: all three present the same candidates.

**Matching.** The query is split on whitespace and **every term must match**,
in any order. A term matches when it appears anywhere in the item's name,
description, aliases or tags — as a plain substring of *folded* text, so `rapa`
finds `Trapano` and `caffe`, `caffè` and `CAFFÈ` all find `Caffè`. Notes are
not searched, `%` and `_` are literal characters rather than wildcards, and a
blank query matches nothing. `q` is required and holds 1–200 characters.

**Ranking.** Best match first: the field decides (name > alias > tag >
description), then how closely it matched (exact > prefix > partial). A partial
match in the name therefore beats an exact tag. Candidates of equal rank keep
the listing order (by name).

**Results.** Every candidate carries the item, the path of its location
(`Garage > Toolbox > Drawer 1`) and why it matched (`match.field` and
`match.kind`) — which is what makes an ambiguous query answerable without a
second lookup.

**Known limits.** Folding is not transliteration: case and diacritics go, but a
letter without a decomposition keeps its identity, so `søren` does not match
`soren` and `Straße` does not match `strasse`. The query is a scan of the
inventory; `TestSearchPerformanceOnLargeInventory` measures it on 10,000 items.

## MCP

homey speaks the Model Context Protocol over **Streamable HTTP** at `/mcp`, so
an MCP host drives the inventory through tools instead of raw HTTP. It is on
by default (`HOMEY_MCP_ENABLED=false` turns it off) and sits behind the same
bearer tokens as the API.

Fifteen tools: four readers (`search_inventory`, `get_item`, `list_location`,
`count_items`) open to any valid token, eleven writers that need the `write`
scope — the items (`add_item`, `update_item`, `move_item`, `delete_item`) and
the places (`add_room`, `update_room`, `delete_room`, `add_container`,
`update_container`, `move_container`, `delete_container`).
Places are named the way a person names them (`"Garage > Toolbox"`); a name
that fits several places comes back as a clarification instead of a guess; and
the deletions (`delete_item`, `delete_room`, `delete_container`) ask before
they remove anything, with every bypass recorded.

**[docs/mcp.md](docs/mcp.md)** has the client recipes (Cursor, Claude Code,
Claude Desktop, claude.ai connectors, ChatGPT, Ollama), the tool reference,
the confirmation flow and a troubleshooting table.

```bash
curl -sS -X POST http://127.0.0.1:8080/mcp \
  -H "Authorization: Bearer $HOMEY_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
        "protocolVersion":"2025-06-18","capabilities":{},
        "clientInfo":{"name":"curl","version":"0"}}}'
```

## Events

Every mutation writes a row to the `events` table **in the same transaction as
the change it describes**, so the history is complete by construction:
`room.created`, `container.created|updated|deleted|moved` and
`item.created|updated|deleted|moved`, each with the entity, its state as JSON
and when it happened. Nothing mutates without leaving a trace, and a mutation
that failed leaves nothing behind.

The actor travels with it: the token's name, the MCP tool that carried the
mutation out, and — when the tool had to ask — `confirmation="confirmed"` or
`"bypassed"`. *"Who deleted this, with which tool, and was it confirmed?"* is
answered by the table alone. The payload holds what a replay would need:
`created` and `updated` carry the entity as it now stands, `deleted` the
entity as it was, and `moved` the two places. The log is read from Go
(`events.Recent`, `events.ForEntity`); there is no history endpoint yet.

## Backup

Two ways to keep the inventory safe, and they answer different questions: a
copy of the database restores *exactly* what was there, an export restores
*what it means* on any instance.

**Copy the file.** The database is `data/homey.db` (see Configuration) with
its `-wal` and `-shm` beside it. While the server runs, copy it with SQLite's
own command rather than `cp`, so the write-ahead log is folded in:

```bash
sqlite3 data/homey.db ".backup backup/homey-$(date +%F).db"
```

With the server stopped, `cp data/homey.db backup/` is enough: a clean
shutdown checkpoints the log away. The file is `0600`; keep it that way.

**Export the document.** `GET /export` writes the whole inventory as JSON —
rooms, containers (parents first), items with their tags, aliases and places —
naming everything instead of numbering it, so the same file means the same
thing on any instance:

```bash
curl -H "Authorization: Bearer $HOMEY_TOKEN" http://127.0.0.1:8080/api/v1/export > homey.json
```

**Import it back.** `POST /import` applies a document, and it is built to be
run more than once:

- the whole document is validated **before anything is written**, and a
  problem points at the entry it came from (`body.items[3].quantity`);
- every entity is matched **by name within its scope** — the same scope the
  uniqueness rules use — so a second import of the same document reports
  everything `unchanged` and writes nothing;
- entities are **never deleted**: a document that does not mention an
  instance's item leaves it alone. What it does mention is authoritative.

```bash
curl -X POST -H "Authorization: Bearer $HOMEY_TOKEN" -H "Content-Type: application/json" \
  --data @homey.json http://127.0.0.1:8080/api/v1/import
```

The answer counts what happened per kind of entity: `created`, `updated` and
`unchanged`. Both directions go through the event log like any other mutation,
and both need a token with the right scope (export `read`, import `write`).
The document is deliberately **not** an MCP tool: a language model neither
needs a dump of the whole inventory nor a bulk way to rewrite it.

## Web UI

The UI in [`web/`](web/) is a Svelte 5 + TypeScript app (SvelteKit, static
build). The dev server proxies `/api`, `/healthz` and `/readyz` to a local Go
server:

```bash
go run ./cmd/server serve              # terminal 1
cd web && npm install && npm run dev   # terminal 2 → http://localhost:5173
```

`HOMEY_API_PROXY` overrides the proxy target. Lint and type checks:
`npm run lint`, `npm run check` (both required in CI). API types are
generated from the contract: `npm run api:generate` in `web/`.

The production build is embedded in the server, so deployment is one binary —
no static file server next to it:

```bash
./scripts/embed-ui.sh        # builds the SPA into internal/webui/dist
go build ./cmd/server        # self-contained binary, UI included
```

The Docker image does the same in two stages (`node` builds the SPA, `golang`
embeds it). The server answers `/` and any client-side route with the shell,
serves `/_app/immutable/**` with a one-year immutable cache (the filenames are
content-hashed), revalidates the shell, and keeps unknown `/api/` paths as
JSON problem documents.

## Security

The security requirements of the spec are worked through against the code in
[`docs/security-review.md`](docs/security-review.md): authentication, scopes,
input validation, parameterized SQL, rate limiting, CORS, headers, output
escaping and the audit of destructive LLM actions — each with the test that
keeps it true. Two findings are worth knowing in operation:

- The database file is created `0600`, so the inventory and the token hashes
  are the owner's alone; a file that already exists keeps its mode, and
  `chmod 600 data/homey.db` tightens an old one.
- homey speaks plain HTTP and expects TLS at the reverse proxy: a token must
  never cross a public network without it. A working example with Caddy is in
  [`deployment/docker`](deployment/docker/README.md).

## Development

```bash
go build ./...
go test ./...
go run ./cmd/server serve --listen 127.0.0.1:8080
```

Coverage is part of CI and has a floor of 75% over `internal/...`:

```bash
go test -coverpkg=./internal/...,./mcp/... -coverprofile=coverage.out ./...
go run ./cmd/coverage -profile coverage.out
```

`cmd/coverage` prints the coverage of every package and fails below the
minimum; the profile is cross-package (`-coverpkg`), so a package counts the
coverage it gets from other packages' tests.

Operational helpers — migrations run at start-up, these are the manual ones:

```bash
go run ./cmd/server migrate version
go run ./cmd/server migrate up
go run ./cmd/server migrate down
go run ./cmd/server migrate force <version>
```

`force` is the escape hatch for a dirty migration state (for example
`force -1` resets the version).

## License

MIT — see [LICENSE](LICENSE).
