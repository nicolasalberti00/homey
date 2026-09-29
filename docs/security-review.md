# Security review

Step 7.2 of the roadmap: the security requirements of the spec (§15) worked
through one by one against the code, on 2026-09-29, against `main` at `0a5614e`.
Every requirement below names where it is implemented and the test that would
fail if it stopped being true. The review produced three fixes (F1–F3) and two
new guards (F4–F5); the two open items (F6–F7) are accepted with a reason.

The scope is homey as it ships: the REST API, the MCP endpoint, the embedded
web UI, the CLI and the SQLite database. Deployment concerns (TLS, backups) are
called out where they matter and are finished in Steps 7.3–7.4.

## Checklist

| Requirement (spec §15) | Verdict | Where and how it is guarded |
| --- | --- | --- |
| No direct database exposure | ✅ | The only database is a local SQLite file (`internal/storage`); there is no network listener for it. The surfaces are the HTTP API, `/mcp`, the SPA and the CLI. The file is created `0600` (F1/F3 below). |
| Authentication for API access | ✅ | Bearer tokens, stored as `sha256:`-prefixed hashes and compared in constant time (`internal/auth`), read and write scopes, `401` + `WWW-Authenticate` when missing, `403` when the scope is not enough. `/mcp` requires a token for every request (`mcpAuth`). Covered by `internal/auth/auth_test.go`, `internal/server/hardening_test.go`, `internal/server/mcp_permissions_test.go`. |
| Input validation | ✅ | Every Huma operation validates through its schema (lengths, enums, ranges, formats) and the core re-validates with `inventory.Validate`/`Normalize` — the domain is the authority, the schema is the courtesy. MCP arguments are validated against the tool's JSON schema *before* the handler runs (`internal/tools.Call`). Bodies are capped at 4 MiB (**F1**). |
| Parameterized SQL | ✅ | Statements are constant strings with `?` placeholders; values always travel as parameters. `TestSQLStatementsAreParameterized` parses every Go file in the repository and fails the build on `+`/`fmt.Sprintf` statements, and `TestHostileNamesRoundTripAsData` proves hostile names are data, not SQL. |
| Least-privilege permissions | ✅ | REST: `read` vs `read,write` scopes, enforced by `bearerAuth`. MCP: `Caller.CanWrite` is checked in the registry *before* validation or the handler, and a context with no caller is refused for writes (fail closed, Step 6.6). Destructive tools additionally need a confirmation or an explicit per-token bypass (Step 6.5). The database file is `0600` (**F3**). |
| Rate limiting where appropriate | ✅ (fixed) | Mutating requests per IP (default 60/min) and failed authentications per IP (default 10/min), both disable with `0`, both answer `429` + `Retry-After`. **F2**: the limit used to cover `/api/v1/` only, leaving the endpoint an agent drives mutations through unlimited — `POST /mcp` counts now. |
| Configurable CORS | ✅ | `HOMEY_CORS_ORIGINS`; off by default (same-origin only), exact-origin matching, `Vary: Origin`, preflight answered with the allowed methods and headers. `TestCORSIsOffByDefault`, `TestCORSAllowsConfiguredOrigins`, `TestCORSPreflight`. |
| Secure HTTP headers | ✅ (CSP deferred, F6) | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `Permissions-Policy` on every response — `TestSecurityHeadersOnEveryResponse`. |
| HTTPS for public deployments | ✅ (documented, F7) | homey speaks plain HTTP and expects TLS at the reverse proxy; a token must never cross a public network without it. The compose example with a reverse proxy and HTTPS lands in Step 7.4. |
| Audit destructive LLM operations | ✅ | Every mutation writes an `events` row in the same transaction (Step 7.1): `actor` = the token's name, `tool` = the MCP tool, `confirmation` = `confirmed`/`bypassed`. `TestMCPDeleteIsAudited`, `TestRESTMutationsAreRecorded`, `internal/storage/events_test.go`. |
| Explicit confirmation for destructive actions | ✅ | `delete_item` asks first: a proposal returns a single-use token bound to tool and item, valid 2 minutes. The two bypasses (`confirm: true` per call, per-token policy) are both recorded as `bypassed`. `internal/tools/delete_test.go`, `internal/tools/confirm_test.go`. |
| LLMs never receive unrestricted database access | ✅ | `/mcp` exposes exactly the eight registry tools; there is no SQL tool, no file tool and no way to widen the set from a request. `HOMEY_MCP_ENABLED=false` turns the endpoint off entirely. `mcp/server/tools_test.go` pins the tool list and schemas. |

## Output escaping (roadmap item)

Svelte escapes every value it renders, and the sources contain no `{@html}` and
no DOM API that takes an HTML string. Item names, tags and aliases are user
data, so that is the invariant that matters: **`TestUISourceHasNoRawHTMLInjection`**
(`internal/webui`) walks `web/src` and fails the build on the raw-HTML directive
or on `innerHTML`/`outerHTML`/`insertAdjacentHTML`/`dangerouslySetInnerHTML`.
Any future adapter that renders HTML (the voice adapter, an email report) needs
the same kind of guard at its own sink.

## Token handling and logs (roadmap item)

- Tokens are generated with `crypto/rand`, shown **once** at creation, and
  stored only as `sha256:<hex>` (versioned, so the algorithm can change later).
- Comparison is `subtle.ConstantTimeCompare`.
- The request logger writes method, path, status and duration — never a query
  string, a header or a body. Auth failures log the reason and the peer, not
  what was presented. The `events` table records the token's **name**, never
  its value.
- **`TestLogsNeverCarryAToken`** (new) makes a successful write with a valid
  token and a rejected one with a wrong token, and fails if either string
  appears in the captured log output.

## Findings

| # | Finding | Status |
| --- | --- | --- |
| F1 | No cap on request bodies: a client could stream an arbitrarily large body into memory before validation saw it. | **Fixed** — `bodyLimit` middleware refuses an announced length over 4 MiB with `413` and cuts a streamed body with `http.MaxBytesReader`; the write limit counts the attempt first. `TestRequestBodyIsCapped`. |
| F2 | The write rate limit matched `/api/v1/` only, so `POST /mcp` — where an agent performs mutations — was unlimited. | **Fixed** — `/mcp` counts. The endpoint cannot tell a read from a write without parsing the stream, so every POST counts; tune `HOMEY_RATE_LIMIT_WRITES` (or set `0`) for a read-heavy agent session. `TestMCPPostsCountTowardTheWriteLimit`. |
| F3 | A fresh database was created with the default umask (`0644`), leaving the inventory and the token hashes readable by every account on the host. | **Fixed** — `storage.Open` creates the file `0600` before SQLite touches it, and the WAL/SHM files inherit that mode. A file that already exists keeps the mode it has, so a widened file for a backup tool is not silently undone: run `chmod 600 data/homey.db` to tighten an existing install. `TestDatabaseFileIsCreatedOwnerOnly`, `TestAnExistingDatabaseKeepsItsMode`. |
| F4 | No guard that a token never reaches a log line. | **Guarded** — `TestLogsNeverCarryAToken`. |
| F5 | No guard that the UI keeps rendering data as text. | **Guarded** — `TestUISourceHasNoRawHTMLInjection`. |
| F6 | No `Content-Security-Policy`. | **Accepted for now, follow-up.** The SPA carries a small inline theme bootstrap in `index.html`, so a strict policy needs a hash maintained with the build; there is no raw-HTML sink today (F5), which is what CSP would be protecting against. Add it together with the UI test suite (post-MVP). |
| F7 | HTTPS is not terminated by homey itself. | **Accepted, documented in 7.4.** Terminating TLS at the reverse proxy is the standard split for a self-hosted service; the compose example ships with Step 7.4. |

## Known limits (accepted, documented)

- **Rate-limit keying is the direct peer address.** Behind a reverse proxy every
  client shares the proxy's bucket, so tune `HOMEY_RATE_LIMIT_WRITES` there (or
  disable it and let the proxy rate limit). This is noted next to the code in
  `internal/server/middleware.go`.
- **The data directory keeps the system default mode** (`0755` as created);
  only the database file is tightened to `0600`.
- **No CSP yet** (F6), **no TLS termination** (F7), **no dependency
  vulnerability scan** — the last one belongs to the CI hardening discussion in
  the README's development section and is worth adding when the release
  pipeline exists.

## Re-running the review

The checklist is the spec §15 list; the guards are ordinary Go tests, so
`go test ./...` re-runs the whole review on every change:

```sh
go test ./internal/storage/ ./internal/server/ ./internal/webui/
```
