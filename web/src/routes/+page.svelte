<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Item } from '$lib/api/client';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory } from '$lib/inventory.svelte';
	import { buildIndex } from '$lib/paths';
	import { formatTimestamp } from '$lib/format';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());
	const index = $derived(buildIndex(inv.rooms, inv.containers));

	const recent = $derived(
		[...inv.items].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 5)
	);

	let query = $state('');
	const trimmedQuery = $derived(query.trim().toLowerCase());
	const results = $derived(search(inv.items, trimmedQuery));

	// Client-side filtering until the search endpoint lands in Phase 5; it
	// searches the same fields the API will (name, description, tags).
	function search(items: Item[], term: string): Item[] {
		if (term.length < 2) return [];
		return items
			.filter((item) => {
				const haystack = [item.name, item.description, ...(item.tags ?? [])]
					.join('\n')
					.toLowerCase();
				return haystack.includes(term);
			})
			.slice(0, 20);
	}
</script>

<svelte:head><title>Dashboard · homey</title></svelte:head>

<h1>Dashboard</h1>

{#if inv.status === 'loading' || inv.status === 'idle'}
	<Spinner label="Loading the inventory…" />
{:else if inv.status === 'error'}
	{#if inv.problem}
		<ProblemPanel title="Could not load the inventory" problem={inv.problem} />
	{/if}
{:else}
	<ul class="stats">
		<li class="card">
			<span class="count">{inv.rooms.length}</span>
			<span class="label">Rooms</span>
		</li>
		<li class="card">
			<span class="count">{inv.containers.length}</span>
			<span class="label">Containers</span>
		</li>
		<li class="card">
			<span class="count">{inv.items.length}</span>
			<span class="label">Items</span>
		</li>
	</ul>

	<section aria-labelledby="search-heading">
		<h2 id="search-heading">Search</h2>
		<div class="field">
			<label for="search">Find an item</label>
			<input
				id="search"
				type="search"
				bind:value={query}
				placeholder="drill, screws, camping…"
				autocomplete="off"
				aria-describedby="search-hint"
			/>
			<p id="search-hint" class="muted">
				Searches name, description and tags. The server-side search arrives in Phase 5.
			</p>
		</div>

		{#if trimmedQuery.length >= 2}
			<p role="status">
				{results.length}
				{results.length === 1 ? 'result' : 'results'}
			</p>
			{#if results.length === 0}
				<EmptyState title="Nothing matches “{query.trim()}”" hint="Try a shorter term." />
			{:else}
				<ul class="results">
					{#each results as item (item.id)}
						<li class="card">
							<p class="name">
								{item.name}
								{#if item.quantity > 1}<span class="qty">×{item.quantity}</span>{/if}
							</p>
							<p class="muted location">{index.locationLabel(item.location)}</p>
							{#if item.description}
								<p class="desc">{item.description}</p>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</section>

	<section aria-labelledby="recent-heading">
		<h2 id="recent-heading">Recently updated</h2>
		{#if recent.length === 0}
			<EmptyState
				title="No items yet"
				hint="Create a room and add items to see them here. Rooms live under Rooms."
			/>
		{:else}
			<ul class="results">
				{#each recent as item (item.id)}
					<li class="card">
						<p class="name">{item.name}</p>
						<p class="muted location">
							{index.locationLabel(item.location)} · {formatTimestamp(item.updated_at)}
						</p>
					</li>
				{/each}
			</ul>
		{/if}
		<p class="muted">
			Manage locations under <a href={resolve('/rooms')}>Rooms</a>.
		</p>
	</section>
{/if}

<style>
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
		gap: 1rem;
		list-style: none;
		margin: 0 0 1.5rem;
		padding: 0;
	}

	.count {
		display: block;
		font-size: 1.8rem;
		font-weight: 700;
	}

	.label {
		color: var(--muted);
	}

	section + section {
		margin-top: 1.5rem;
	}

	.field {
		max-width: 32rem;
	}

	.field p {
		margin: 0.35rem 0 0;
		font-size: 0.9rem;
	}

	.results {
		list-style: none;
		margin: 0.75rem 0 0;
		padding: 0;
		display: grid;
		gap: 0.6rem;
	}

	.name {
		margin: 0;
		font-weight: 600;
	}

	.qty {
		color: var(--muted);
		font-weight: 400;
	}

	.location,
	.desc {
		margin: 0.15rem 0 0;
		font-size: 0.9rem;
	}
</style>
