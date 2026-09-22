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
npm run check    # svelte-check (TypeScript diagnostics)
npm run build    # static build into build/
```

All three run in CI (`web checks` job).

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
├── lib/               shared code (`$lib` alias)
│   └── components/    reusable components
├── app.html           HTML shell
└── app.d.ts           ambient types
```
