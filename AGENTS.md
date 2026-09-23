# AGENTS.md

## Project

homey — self-hosted home inventory: Go + SQLite core, REST API, MCP server,
Web UI. See `README.md` for the overview, configuration and roadmap.

## Development workflow

- `main` is always green and releasable; never commit or push directly to it.
- One unit of work (typically a roadmap step) per branch, named `feat/…`, `fix/…`,
  `chore/…`, `ci/…` or `docs/…` (e.g. `feat/phase-2-domain-types`).
- When the work is complete (`gofmt`, `go vet` and `go test ./...` green, plus the
  75% coverage floor: `go test -coverpkg=./internal/... -coverprofile=coverage.out ./... && go run ./cmd/coverage`), push the
  branch and open a pull request with `gh pr create` describing the step and its
  exit criteria.
- Keep PRs small and focused; leave them for review — do not merge into `main`
  unless explicitly asked.

## Conventions

- SQL statements are constant strings with `?` placeholders, and user-provided
  values are always passed as parameters. Never build SQL with concatenation or
  `fmt.Sprintf`, and never let user input reach SQL identifiers (table/column
  names): the guard test in `internal/storage` fails the build otherwise.

## Agent skills

### Issue tracker

Issues and specs for this repo live as markdown files under `.scratch/<feature-slug>/` (local markdown tracker). See `docs/agents/issue-tracker.md`.

### Triage labels

Five canonical triage roles; label strings equal role names (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `CONTEXT.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.
