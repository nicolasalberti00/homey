import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
	preprocess: vitePreprocess(),
	kit: {
		// Static single-page build: the Go server embeds `build/` and serves
		// `index.html` as the fallback for client-side routes (Step 4.9).
		adapter: adapter({ fallback: 'index.html' })
	}
};
