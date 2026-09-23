<script lang="ts">
	import { resolve } from '$app/paths';
	import type { LocationRef } from '$lib/api/client';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Highlight from '$lib/components/Highlight.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory } from '$lib/inventory.svelte';
	import { buildIndex } from '$lib/paths';
	import { matchesTerms, queryTerms } from '$lib/search';
	import { formatTimestamp } from '$lib/format';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());
	const index = $derived(buildIndex(inv.rooms, inv.containers));

	const recent = $derived(
		[...inv.items].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 5)
	);

	// Client-side search until the search endpoint lands in Phase 5: it looks
	// at the same fields the API will (name, description, tags) and narrows
	// the results with simple room and tag filters.
	const RESULT_LIMIT = 20;

	let query = $state('');
	let roomFilter = $state('');
	let tagFilter = $state('');

	const terms = $derived(queryTerms(query));
	const selectedRoom = $derived(roomFilter === '' ? undefined : Number(roomFilter));
	const searching = $derived(query.trim().length >= 2 || roomFilter !== '' || tagFilter !== '');
	const matches = $derived(
		searching
			? inv.items
					.filter((item) => matchesTerms(item, terms))
					.filter(
						(item) =>
							selectedRoom === undefined || index.locationRoomId(item.location) === selectedRoom
					)
					.filter((item) => tagFilter === '' || (item.tags ?? []).includes(tagFilter))
					.sort((a, b) => a.name.localeCompare(b.name))
			: []
	);
	const results = $derived(matches.slice(0, RESULT_LIMIT));

	const rooms = $derived([...inv.rooms].sort((a, b) => a.name.localeCompare(b.name)));
	const tags = $derived(
		[...new Set(inv.items.flatMap((item) => item.tags ?? []))].sort((a, b) => a.localeCompare(b))
	);

	function clearFilters(): void {
		roomFilter = '';
		tagFilter = '';
	}

	function locationHref(location: LocationRef): string {
		return location.kind === 'room'
			? resolve('/rooms/[id]', { id: String(location.id) })
			: resolve('/containers/[id]', { id: String(location.id) });
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
				Every term must match the name, description or tags. Server-side ranking arrives in Phase 5.
			</p>
		</div>

		<div class="filters">
			<div class="field">
				<label for="filter-room">Room</label>
				<select id="filter-room" bind:value={roomFilter}>
					<option value="">All rooms</option>
					{#each rooms as room (room.id)}
						<option value={String(room.id)}>{room.name}</option>
					{/each}
				</select>
			</div>
			<div class="field">
				<label for="filter-tag">Tag</label>
				<select id="filter-tag" bind:value={tagFilter}>
					<option value="">All tags</option>
					{#each tags as tag (tag)}
						<option value={tag}>{tag}</option>
					{/each}
				</select>
			</div>
			{#if roomFilter !== '' || tagFilter !== ''}
				<button class="btn" type="button" onclick={clearFilters}>Clear filters</button>
			{/if}
		</div>

		{#if searching}
			<p role="status">
				{matches.length}
				{matches.length === 1 ? 'result' : 'results'}
				{#if matches.length > results.length}
					<span class="muted">(showing the first {results.length})</span>
				{/if}
			</p>
			{#if results.length === 0}
				<EmptyState
					title={terms.length > 0
						? `Nothing matches “${query.trim()}”`
						: 'Nothing matches this selection'}
					hint="Try a shorter term, or clear the room and tag filters."
				/>
			{:else}
				<ul class="results">
					{#each results as item (item.id)}
						<li class="card">
							<p class="name">
								<a href={resolve('/items/[id]', { id: String(item.id) })}
									><Highlight text={item.name} {terms} /></a
								>
								{#if item.quantity > 1}<span class="qty">×{item.quantity}</span>{/if}
							</p>
							<p class="muted location">
								<!-- locationHref() picks the route and resolves it internally. -->
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
								<a href={locationHref(item.location)}>{index.locationLabel(item.location)}</a>
							</p>
							{#if item.description}
								<p class="desc"><Highlight text={item.description} {terms} /></p>
							{/if}
							{#if item.tags?.length}
								<ul class="tags" aria-label="Tags">
									{#each item.tags as tag (tag)}
										<li><Highlight text={tag} {terms} /></li>
									{/each}
								</ul>
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
						<p class="name">
							<a href={resolve('/items/[id]', { id: String(item.id) })}>{item.name}</a>
						</p>
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

	.field p {
		margin: 0.35rem 0 0;
		font-size: 0.9rem;
	}

	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
		gap: 0.75rem;
		align-items: end;
		margin-top: 0.75rem;
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

	.tags {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		list-style: none;
		margin: 0.4rem 0 0;
		padding: 0;
	}

	.tags li {
		background: var(--surface-2);
		border-radius: 999px;
		font-size: 0.8rem;
		padding: 0.1rem 0.55rem;
	}
</style>
