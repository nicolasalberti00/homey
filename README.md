# homey

Self-hosted, platform-independent home inventory: keep track of objects by
room and location, manage them from a Web UI and interact with them through
LLMs via MCP. No cloud account, no external database, no mandatory AI
provider.

> **Status: Phase 6 (MCP) in progress.** The REST API, Web UI and search are
> complete, and the MCP endpoint serves read and write tools — including
> deletion guarded by a confirmation step and an explicit, audited bypass.
> Remaining: MCP permissions, ambiguity handling, hardening (Phase 7).

## Quickstart (Docker)

```bash
docker compose up -d
curl http://localhost:8080/healthz
```

Data lives in `./data` (SQLite). On Linux hosts make sure the directory is
writable by uid 1000 (the container user):

```bash
mkdir -p data && sudo chown 1000:1000 data
```

To build for a specific architecture
(amd64/arm64):

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t homey:dev .
```

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
| `HOMEY_MCP_ENABLED`  | `--mcp`          | `true`                | Enable the MCP endpoint (Phase 6)    |
| `HOMEY_RATE_LIMIT_WRITES` | `--rate-limit-writes` | `60`            | Mutating `/api/v1` requests per IP per minute (0 disables) |
| `HOMEY_RATE_LIMIT_AUTH_FAILURES` | `--rate-limit-auth-failures` | `10` | Failed authentication attempts per IP per minute (0 disables) |

## Endpoints (so far)

- `GET /healthz` — liveness
- `GET /readyz` — readiness (pings the database)

The REST API lives under `/api/v1/` and is documented by the generated
contract in [`api/openapi.yaml`](api/openapi.yaml):

- Rooms — `GET/POST /api/v1/rooms`, `GET/PATCH/DELETE /api/v1/rooms/{id}`
- Containers — `GET/POST /api/v1/containers` (filter `room_id`),
  `GET/PATCH/DELETE /api/v1/containers/{id}`,
  `POST /api/v1/containers/{id}/move`
- Items — `GET/POST /api/v1/items` (filters `room_id`, `container_id`),
  `GET/PATCH/DELETE /api/v1/items/{id}`, `POST /api/v1/items/{id}/move`
- Search — `GET /api/v1/search?q=…` matches every term against item names,
  descriptions, aliases and tags, and returns the candidates best match first,
  each with the path of its location
- Public — `GET /api/v1/openapi.json` and `GET /api/v1/docs` (Stoplight docs UI)

Every response carries defensive security headers (`nosniff`, frame deny,
referrer policy). CORS is same-origin by default; configure
`HOMEY_CORS_ORIGINS` to allow browser clients from other origins.

Regenerate the contract after changing any operation:

```bash
go run ./cmd/openapi > api/openapi.yaml
```

## Search

`GET /api/v1/search?q=…` is the single search behind the REST API, the web UI
and the MCP server: all three present the same candidates.

**Matching.** The query is split on whitespace and **every term must match**, in
any order. A term matches when it appears anywhere in the item's name,
description, aliases or tags — as a plain substring of *folded* text, so `rapa`
finds `Trapano` and `caffe`, `caffè` and `CAFFÈ` all find `Caffè`. Notes are not
searched, `%` and `_` are literal characters rather than wildcards, and a blank
query matches nothing. `q` is required and holds 1–200 characters.

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

homey speaks the Model Context Protocol over the **Streamable HTTP** transport
at `/mcp`, so an MCP host (Claude, or any other client) can drive the inventory
through tools instead of raw HTTP. It is **on by default** and switched off with
`HOMEY_MCP_ENABLED=false` or `--mcp=false`.

The endpoint sits behind the same bearer tokens as the REST API: configure the
client with a token (`Authorization: Bearer …`), and a request without one is
answered with `401` and a `WWW-Authenticate` challenge.

The tools come from one registry, and today they are these.

Reading:

| Tool | What it answers |
| --- | --- |
| `search_inventory` | where something is, by name, alias, tag or description: every candidate carries its location path, quantity and tags |
| `get_item` | everything stored about one item, by id |
| `list_location` | what sits directly in a room or a container; without one, the rooms of the home |
| `count_items` | how many items match, optionally filtered by tag or place (a place counts everything inside it, containers included) |

Writing:

| Tool | What it does |
| --- | --- |
| `add_item` | records a new item in a room or container that already exists, and answers with its id |
| `update_item` | changes the fields it is given — name, description, quantity, tags, aliases, notes — and leaves the others alone |
| `move_item` | puts an item in another room or container, leaving everything else about it alone |
| `delete_item` | removes an item for good, after a confirmation step (see below) |

Each tool is a definition plus a handler that calls the core, and its JSON
schemas are inferred from the Go types it is defined with — what a client sees
and what the code accepts cannot drift apart. Inputs and outputs are validated
in the registry, so the same tool can be served to another adapter without
rewriting it.

### Permissions

Every tool declares whether it reads or writes, and the registry enforces that
before the handler runs: reads are open to any valid token, writes need one
with the `write` scope. A **read-only token** (`--scope read`) is the client for
exploration: it sees the whole tool list, but calling a write tool answers with
a permission error and changes nothing. A context that never said who is
calling is refused for writes too, so an adapter cannot change the inventory by
forgetting to authenticate.

Places are named the way a person names them: `"Garage"` or
`"Garage > Toolbox > Cassetto 1"`, matched ignoring case and accents, with a
bare name accepted when only one place carries it. A name that fits several
places fails with the candidates listed, so a model can ask which one was
meant instead of guessing; the same goes for an item that does not exist.

### Deleting asks first

`delete_item` deletes for good, and never silently. By default it asks:

1. A call with just an `id` answers `confirmation: "pending"`, the item that
   would go, and a short-lived `confirmation_token` — **nothing is removed**.
2. Calling it again with that token in `confirmation_token` carries the
   deletion out and answers `confirmation: "confirmed"`.

The token lasts two minutes and is bound to that one action: it cannot be
replayed or spent on a different item. A caller that has already confirmed in
the same turn passes `confirm: true` to delete at once. A token created with
`--destructive-confirmation=bypass` deletes without the extra step on every
call. Both bypasses are recorded as `confirmation: "bypassed"` in the audit
log, so a trusted automation is never invisible.

```bash
curl -sS -X POST http://127.0.0.1:8080/mcp \
  -H "Authorization: Bearer $HOMEY_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
        "protocolVersion":"2025-06-18","capabilities":{},
        "clientInfo":{"name":"curl","version":"0"}}}'
```

## API tokens

Every `/api/v1/` operation requires a bearer token; only the spec, the docs
UI and the schemas are public. Tokens are stored hashed (SHA-256) and shown
once at creation:

```bash
go run ./cmd/server token create --name "curl" --scope read,write
go run ./cmd/server token list
go run ./cmd/server token revoke <id>
```

`--scope read` allows only reads; `--scope read,write` also allows mutations.
`--destructive-confirmation=bypass` marks a token that may run destructive
tools (currently `delete_item`) without the confirmation step; the bypass is
recorded in the audit log. Without a token the API answers `401`;
a read-only token gets `403` on mutating operations. The MCP endpoint answers
the same way: a read-only token can connect and explore the inventory, but
every write tool is refused with a permission error. Repeated failed
authentications from one address are answered with `429` (see
`HOMEY_RATE_LIMIT_AUTH_FAILURES`); mutating requests are rate limited the
same way. Limits are keyed by the direct peer address — behind a reverse
proxy they apply to the proxy, so tune them accordingly.

## Web UI (development)

The UI in [`web/`](web/) is a Svelte 5 + TypeScript app (SvelteKit, static
build). The dev server proxies `/api`, `/healthz` and `/readyz` to a local Go
server:

```bash
go run ./cmd/server serve          # terminal 1
cd web && npm install && npm run dev   # terminal 2 → http://localhost:5173
```

`HOMEY_API_PROXY` overrides the proxy target. Lint and type checks:
`npm run lint`, `npm run check` (both required in CI). API types are
generated from the contract: `npm run api:generate` in `web/`.

### Single binary

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
JSON problem documents. A build without the UI — a plain `go build` in a fresh
clone — answers 404 with a hint instead of a blank page.

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
minimum. The profile is cross-package (`-coverpkg`), so a package counts the
coverage it gets from other packages' tests. It reads the profile with
`golang.org/x/tools/cover`, the parser behind `go tool cover`, which merges the
samples of a block — `-coverpkg` reports each block once per test binary — so
the total agrees with `go tool cover -func`.

Migrations run automatically at start-up. Operational helpers:

```bash
go run ./cmd/server migrate version
go run ./cmd/server migrate up
go run ./cmd/server migrate down
go run ./cmd/server migrate force <version>
```

`force` is the escape hatch for a dirty migration state (for example
`force -1` resets the version).

## Roadmap

1. Foundations — repo, configuration, SQLite, migrations, Docker ✅
2. Inventory core — rooms, containers, items, move, validation ✅
3. REST API — OpenAPI, auth, tests ✅
4. Web UI — dashboard, rooms, containers, items, search, themes ✅
5. Search — aliases, tags, ranking, ambiguity handling
6. MCP — tool registry, read/write tools, confirmation flow
7. Hardening — audit events, security review, export/import, docs

## License

MIT — see [LICENSE](LICENSE).
