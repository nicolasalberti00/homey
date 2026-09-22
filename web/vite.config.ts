import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// In development the UI talks to a local Go server: API and health routes are
// proxied so the browser only ever sees one origin. HOMEY_API_PROXY overrides
// the target.
const target = process.env.HOMEY_API_PROXY ?? 'http://127.0.0.1:8080';

export default defineConfig({
	plugins: [sveltekit()],
	// Keep the terminal output readable: dev.sh prints the API token after
	// the dev server is up.
	clearScreen: false,
	server: {
		proxy: {
			'/api': { target, changeOrigin: true },
			'/healthz': { target, changeOrigin: true },
			'/readyz': { target, changeOrigin: true }
		}
	}
});
