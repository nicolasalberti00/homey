<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Problem } from '$lib/api/client';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { getSettings } from '$lib/settings.svelte';

	// Placeholder dashboard (Step 4.2–4.3): the real one — counts, recent
	// items, global search — arrives in Step 4.4.

	type Status = 'loading' | 'ok' | 'error';

	let status = $state<Status>('loading');
	let problem = $state<Problem | null>(null);

	async function ping() {
		status = 'loading';
		problem = null;
		try {
			const response = await fetch(`${getSettings().apiUrl}/healthz`);
			if (!response.ok) {
				throw new Error(`health check answered HTTP ${response.status}`);
			}
			status = 'ok';
		} catch (error) {
			problem = { title: 'API unreachable', status: 0, detail: String(error) };
			status = 'error';
		}
	}

	ping();
</script>

<svelte:head><title>Dashboard · homey</title></svelte:head>

<h1>Dashboard</h1>

<div class="card">
	<h2>API status</h2>
	{#if status === 'loading'}
		<Spinner label="Checking the API…" />
	{:else if status === 'ok'}
		<p role="status">Connected.</p>
	{:else if problem}
		<ProblemPanel {problem} />
	{/if}
</div>

<div class="card">
	<h2>Getting started</h2>
	<ol>
		<li>Create a token: <code>homey token create --name web --scope read,write</code></li>
		<li>Paste it in <a href={resolve('/settings')}>Settings</a> and test the connection.</li>
	</ol>
	<p class="muted">Rooms, containers and items screens arrive in the next steps.</p>
</div>

<style>
	.card + .card {
		margin-top: 1rem;
	}
</style>
