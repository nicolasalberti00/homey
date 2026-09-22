# homey Web UI

Svelte 5 + TypeScript application built with [SvelteKit](https://svelte.dev/docs/kit)
in static single-page mode: the Go server embeds the production build and
serves it, so deployment stays a single container (Phase 4, Step 4.9).

## Development

```bash
npm install
npm run dev      # http://localhost:5173
```

The dev server proxies API and health routes to a local Go server
(`go run ./cmd/server serve`), so the browser only ever sees one origin.
`HOMEY_API_PROXY` overrides the target (default `http://127.0.0.1:8080`):

```bash
HOMEY_API_PROXY=http://127.0.0.1:9000 npm run dev
```

## Checks

```bash
npm run lint     # eslint (flat config) + prettier --check
npm run check    # svelte-check --fail-on-warnings (TypeScript + compiler warnings)
npm run build    # static build into build/
npm run api:generate  # regenerate src/lib/api/schema.d.ts from ../api/openapi.yaml
```

All of them run in CI (`web checks` job), including a drift check that
regenerates the API types and fails if `schema.d.ts` is stale.

## Accessibility

Accessibility is enforced, not aspirational:

- `svelte-check --fail-on-warnings` makes **Svelte compiler a11y warnings
  fail CI** (missing labels, non-interactive elements with handlers, ARIA
  misuse, …);
- semantic landmarks (`aside`, `nav`, `main`), a skip-to-content link,
  `aria-current` on the active nav item, visible `:focus-visible` outlines,
  labelled form controls with described-by hints, and `role="status"` /
  `role="alert"` live regions for async results;
- colors are AA-contrast pairs defined once with `light-dark()` and follow
  `color-scheme`, so light/dark/system all pass together;
- pages are verified with axe-core (0 violations on dashboard and settings
  in both themes) during development.

## Security rule: no `{@html}`

Formatting of user-provided text never goes through `{@html}`: user text is
data, escaping is the renderer's job. This mirrors the API's structural
approach to SQL injection (parameterised queries, no input blacklists) and is
enforced by the `svelte/no-at-html-tags` ESLint rule (`error` in
`eslint.config.js`).

## Structure

```text
src/
├── routes/            SvelteKit routes (pages and layouts)
│   └── settings/      connection settings page
├── lib/               shared code (`$lib` alias)
│   ├── api/           typed client + schema.d.ts generated from OpenAPI
│   ├── components/    reusable components
│   ├── settings.svelte.ts  API URL + token, persisted in localStorage
│   └── theme.svelte.ts     light/dark/system theme, persisted
├── app.css            design tokens (light-dark()) and base styles
├── app.html           HTML shell (pre-paint theme bootstrap)
└── app.d.ts           ambient types
```
