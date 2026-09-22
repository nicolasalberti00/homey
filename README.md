# homey

Self-hosted, platform-independent home inventory: keep track of objects by
room and location, manage them from a Web UI and interact with them through
LLMs via MCP. No cloud account, no external database, no mandatory AI
provider.

> **Status: Phase 3 (REST API) complete.** The repository ships a
> versioned REST API with a generated OpenAPI 3.1 contract, bearer-token
> auth and rate limiting on a SQLite-backed inventory core (rooms,
> containers, items, tags, moves). Upcoming: Web UI (Phase 4), search
> (Phase 5), MCP (Phase 6).

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
- Search — `GET /api/v1/search`, answers `501` until Phase 5
- Public — `GET /api/v1/openapi.json` and `GET /api/v1/docs` (Stoplight docs UI)

Every response carries defensive security headers (`nosniff`, frame deny,
referrer policy). CORS is same-origin by default; configure
`HOMEY_CORS_ORIGINS` to allow browser clients from other origins.

Regenerate the contract after changing any operation:

```bash
go run ./cmd/openapi > api/openapi.yaml
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
`--destructive-confirmation=bypass` marks a trusted token for the future
two-step destructive flows (Phase 6). Without a token the API answers `401`;
a read-only token gets `403` on mutating operations. Repeated failed
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
`npm run lint`, `npm run check` (both required in CI).

## Development

```bash
go build ./...
go test ./...
go run ./cmd/server serve --listen 127.0.0.1:8080
```

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
4. Web UI — dashboard, rooms, containers, items, search, themes
5. Search — aliases, tags, ranking, ambiguity handling
6. MCP — tool registry, read/write tools, confirmation flow
7. Hardening — audit events, security review, export/import, docs

## License

MIT — see [LICENSE](LICENSE).
