<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		apiFetch,
		toProblem,
		type Item,
		type LocationRef,
		type Problem,
		type SearchMatch,
		type SearchResult
	} from '$lib/api/client';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Highlight from '$lib/components/Highlight.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory } from '$lib/inventory.svelte';
	import { buildIndex } from '$lib/paths';
	import { queryTerms } from '$lib/search';
	import { formatTimestamp } from '$lib/format';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());
	const index = $derived(buildIndex(inv.rooms, inv.containers));

	const recent = $derived(
		[...inv.items].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 5)
	);

	// The search is the server's: the dashboard sends the query to the search
	// endpoint and shows the ranked candidates, so the UI, the API and the MCP
	// server answer the same way. The room and tag filters narrow those
	// candidates here, because the endpoint takes a query only.
	const RESULT_LIMIT = 20;
	const MIN_QUERY_LENGTH = 2;
	// Long enough not to send a request per keystroke, short enough to feel
	// immediate.
	const SEARCH_DEBOUNCE_MS = 200;

	let query = $state('');
	let roomFilter = $state('');
	let tagFilter = $state('');

	let candidates = $state<SearchResult[]>([]);
	let searchProblem = $state<Problem | null>(null);
	let loading = $state(false);
	let controller: AbortController | undefined;

	// A candidate as the page shows it: the server's path, and why the item
	// matched when a query produced it.
	type Candidate = { item: Item; path: string; match?: SearchMatch };

	const terms = $derived(queryTerms(query));
	const selectedRoom = $derived(roomFilter === '' ? undefined : Number(roomFilter));
	const searching = $derived(query.trim().length >= MIN_QUERY_LENGTH);
	const filtering = $derived(roomFilter !== '' || tagFilter !== '');
	const active = $derived(searching || filtering);
	// Without a query the filters browse what is already loaded, name-ordered:
	// the list this section showed before the search endpoint existed.
	const browsed: Candidate[] = $derived(
		inv.items
			.filter(inRoom)
			.filter(hasTag)
			.sort((a, b) => a.name.localeCompare(b.name))
			.map((item) => ({ item, path: index.locationLabel(item.location) }))
	);
	const matched: Candidate[] = $derived(
		searching
			? candidates.filter((candidate) => inRoom(candidate.item) && hasTag(candidate.item))
			: browsed
	);
	const results = $derived(matched.slice(0, RESULT_LIMIT));

	// One request per pause in typing, and a newer query always wins over an
	// older response. The query is the only thing that starts a request, so it
	// starts from the input event rather than an effect.
	let debounce: ReturnType<typeof setTimeout> | undefined;

	function onQueryInput(event: Event): void {
		query = (event.currentTarget as HTMLInputElement).value;
		clearTimeout(debounce);
		const text = query.trim();
		if (text.length < MIN_QUERY_LENGTH) {
			controller?.abort();
			candidates = [];
			searchProblem = null;
			loading = false;
			return;
		}
		debounce = setTimeout(() => void runSearch(text), SEARCH_DEBOUNCE_MS);
	}

	async function runSearch(text: string): Promise<void> {
		controller?.abort();
		const current = new AbortController();
		controller = current;
		loading = true;
		try {
			const found = await apiFetch<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(text)}`, {
				signal: current.signal
			});
			if (current.signal.aborted) return;
			candidates = found;
			searchProblem = null;
		} catch (error) {
			if (current.signal.aborted) return;
			candidates = [];
			searchProblem = toProblem(error, 'Search failed');
		} finally {
			if (controller === current) loading = false;
		}
	}

	const rooms = $derived([...inv.rooms].sort((a, b) => a.name.localeCompare(b.name)));
	const tags = $derived(
		[...new Set(inv.items.flatMap((item) => item.tags ?? []))].sort((a, b) => a.localeCompare(b))
	);

	function clearFilters(): void {
		roomFilter = '';
		tagFilter = '';
	}

	// The endpoint takes a query only, so the filters narrow the candidates the
	// page already has.
	function inRoom(item: Item): boolean {
		return selectedRoom === undefined || index.locationRoomId(item.location) === selectedRoom;
	}

	function hasTag(item: Item): boolean {
		return tagFilter === '' || (item.tags ?? []).includes(tagFilter);
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
				value={query}
				oninput={onQueryInput}
				placeholder="drill, screws, camping…"
				autocomplete="off"
				aria-describedby="search-hint"
			/>
			<p id="search-hint" class="muted">
				Every term must match the name, description, aliases or tags; the best matches come first.
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

		{#if active && searchProblem}
			<ProblemPanel title="Search failed" problem={searchProblem} />
		{/if}

		{#if active && !searchProblem}
			<p role="status">
				{#if loading && matched.length === 0}
					Searching…
				{:else}
					{matched.length}
					{matched.length === 1 ? 'result' : 'results'}
					{#if matched.length > results.length}
						<span class="muted">(showing the first {results.length})</span>
					{/if}
				{/if}
			</p>
			{#if results.length === 0 && !loading}
				<EmptyState
					title={terms.length > 0
						? `Nothing matches “${query.trim()}”`
						: 'Nothing matches this selection'}
					hint="Try a shorter term, or clear the room and tag filters."
				/>
			{:else if results.length > 0}
				<ul class="results">
					{#each results as { item, path, match } (item.id)}
						<li class="card">
							<p class="name">
								<a href={resolve('/items/[id]', { id: String(item.id) })}
									><Highlight text={item.name} {terms} /></a
								>
								{#if item.quantity > 1}<span class="qty">×{item.quantity}</span>{/if}
								{#if match}
									<span class="match" title={`${match.kind} match in the ${match.field}`}>
										<span class="sr-only">Matched </span>{match.field}
									</span>
								{/if}
							</p>
							<p class="muted location">
								<!-- locationHref() picks the route and resolves it internally. -->
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
								<a href={locationHref(item.location)}
									>{path || index.locationLabel(item.location)}</a
								>
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
							{#if item.aliases?.length}
								<ul class="tags aliases" aria-label="Aliases">
									{#each item.aliases as alias (alias)}
										<li><Highlight text={alias} {terms} /></li>
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

	/* Why the server ranked this candidate: the field the query matched. */
	.match {
		border: 1px solid var(--border);
		border-radius: 999px;
		color: var(--muted);
		font-size: 0.75rem;
		font-weight: 400;
		margin-left: 0.35rem;
		padding: 0 0.5rem;
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

	/* Aliases are chips too, but dashed: they are the other names of the item. */
	.tags.aliases li {
		background: none;
		border: 1px dashed var(--border);
	}
</style>
