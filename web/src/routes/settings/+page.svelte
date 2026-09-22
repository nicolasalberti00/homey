<script lang="ts">
	import { ApiError, testConnection, type Problem } from '$lib/api/client';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { getSettings, saveSettings, type Settings } from '$lib/settings.svelte';

	let form = $state<Settings>({ ...getSettings() });
	let saved = $state(false);
	let testing = $state(false);
	let result = $state('');
	let problem = $state<Problem | null>(null);

	function save() {
		saveSettings(form);
		form = { ...getSettings() };
		saved = true;
	}

	async function test() {
		testing = true;
		saved = false;
		result = '';
		problem = null;
		try {
			result = await testConnection(form);
		} catch (error) {
			problem =
				error instanceof ApiError
					? error.problem
					: { title: 'Connection failed', status: 0, detail: String(error) };
		} finally {
			testing = false;
		}
	}
</script>

<svelte:head><title>Settings · homey</title></svelte:head>

<h1>Settings</h1>
<p class="muted">Stored in this browser only (localStorage); nothing is sent anywhere else.</p>

<form
	class="card"
	onsubmit={(event) => {
		event.preventDefault();
		save();
	}}
>
	<div class="field">
		<label for="api-url">API URL</label>
		<input
			id="api-url"
			bind:value={form.apiUrl}
			placeholder="Same origin"
			autocomplete="url"
			spellcheck="false"
			aria-describedby="api-url-hint"
		/>
		<p id="api-url-hint" class="muted">Leave empty when this page is served by homey itself.</p>
	</div>

	<div class="field">
		<label for="api-token">API token</label>
		<input
			id="api-token"
			type="password"
			bind:value={form.token}
			autocomplete="off"
			spellcheck="false"
			aria-describedby="api-token-hint"
		/>
		<p id="api-token-hint" class="muted">
			Create one with <code>homey token create --name web --scope read,write</code>.
		</p>
	</div>

	<div class="actions">
		<button class="btn primary" type="submit">Save</button>
		<button class="btn" type="button" onclick={test} disabled={testing}>Test connection</button>
		{#if saved}
			<span class="ok" role="status">Saved.</span>
		{/if}
	</div>
</form>

{#if testing}
	<div class="spaced">
		<Spinner label="Testing the connection…" />
	</div>
{/if}

{#if result}
	<div class="card spaced" role="status">
		{result}
	</div>
{/if}

{#if problem}
	<div class="spaced">
		<ProblemPanel title="Connection failed" {problem} />
	</div>
{/if}

<style>
	.field {
		margin-bottom: 1rem;
	}

	.field p {
		margin: 0.35rem 0 0;
		font-size: 0.9rem;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}

	.ok {
		color: var(--ok);
	}

	.spaced {
		margin-top: 1rem;
	}
</style>
