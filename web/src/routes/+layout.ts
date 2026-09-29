// Single-page application: no SSR. The Go server embeds the static build and
// serves `index.html` for client-side routes.
export const ssr = false;
export const prerender = false;
