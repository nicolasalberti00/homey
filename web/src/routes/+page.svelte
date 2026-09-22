<script lang="ts">
	// Scaffold page (Step 4.1): proves the dev server and the API proxy work.
	// The real shell and dashboard arrive in Steps 4.2–4.4.

	type Status = 'checking' | 'online' | 'offline';

	let status = $state<Status>('checking');

	async function ping() {
		status = 'checking';
		try {
			const res = await fetch('/healthz');
			status = res.ok ? 'online' : 'offline';
		} catch {
			status = 'offline';
		}
	}

	ping();
</script>

<main>
	<h1>homey</h1>
	<p class="subtitle">Web UI scaffold — Phase 4, Step 4.1</p>
	<p class="status">
		API:
		<span class="pill {status}">{status}</span>
	</p>
</main>

<style>
	main {
		font-family:
			system-ui,
			-apple-system,
			sans-serif;
		max-width: 40rem;
		margin: 4rem auto;
		padding: 0 1.5rem;
	}
	h1 {
		margin-bottom: 0.25rem;
	}
	.subtitle {
		color: #6b7280;
		margin-top: 0;
	}
	.pill {
		border-radius: 999px;
		padding: 0.15rem 0.6rem;
		font-size: 0.85rem;
	}
	.pill.online {
		background: #dcfce7;
		color: #166534;
	}
	.pill.offline {
		background: #fee2e2;
		color: #991b1b;
	}
	.pill.checking {
		background: #e5e7eb;
		color: #374151;
	}
</style>
