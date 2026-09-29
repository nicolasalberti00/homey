# AGENTS.md

## Project

homey — self-hosted home inventory: Go + SQLite core, REST API, MCP server,
Web UI. `README.md` is the overview and the configuration reference; the
user-facing guides live in `docs/` (`mcp.md` for MCP clients,
`security-review.md` for the security checklist) and in `deployment/docker/`
for the compose examples.

## Development workflow

- `main` is always green and releasable; never commit or push directly to it.
- One unit of work per branch, named `feat/…`, `fix/…`, `chore/…`, `ci/…` or
  `docs/…` (e.g. `feat/events-audit`).
- When the work is complete (`gofmt`, `go vet` and `go test ./...` green, plus the
  75% coverage floor: `go test -coverpkg=./internal/...,./mcp/... -coverprofile=coverage.out ./... && go run ./cmd/coverage`), push the
  branch and open a pull request with `gh pr create` describing what changed
  and how to verify it.
- Keep PRs small and focused; leave them for review — do not merge into `main`
  unless explicitly asked.
- Write pull request titles and descriptions in English, like the rest of the
  repository, regardless of the language the session is held in.

## Conventions

- SQL statements are constant strings with `?` placeholders, and user-provided
  values are always passed as parameters. Never build SQL with concatenation or
  `fmt.Sprintf`, and never let user input reach SQL identifiers (table/column
  names): the guard test in `internal/storage` fails the build otherwise.

## Agent skills

### Issue tracker

Two places, two jobs. The plan of record is the Obsidian vault
(`~/Library/Mobile Documents/com~apple~CloudDocs/NAVault/projects/homey/`):
`Homey Roadmap.md`, `Homey Kanban.md` and the session notes. Specs and tickets
that a skill publishes live under `.scratch/<feature-slug>/` (local markdown
tracker). See `docs/agents/issue-tracker.md`.

### Triage labels

Five canonical triage roles; label strings equal role names (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `CONTEXT.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.
