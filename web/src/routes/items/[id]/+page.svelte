<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { apiFetch, toProblem, type Item, type LocationRef, type Problem } from '$lib/api/client';
	import ConfirmButton from '$lib/components/ConfirmButton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import ItemForm, { type ItemDraft } from '$lib/components/ItemForm.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory, refreshInventory } from '$lib/inventory.svelte';
	import { buildIndex } from '$lib/paths';
	import { formatTimestamp } from '$lib/format';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());
	const index = $derived(buildIndex(inv.rooms, inv.containers));
	const itemId = $derived(Number(page.params.id));
	const item = $derived(inv.items.find((candidate) => candidate.id === itemId));
	const locationContainer = $derived(
		item?.location.kind === 'container'
			? inv.containers.find((candidate) => candidate.id === item.location.id)
			: undefined
	);
	const locationRoom = $derived(
		inv.rooms.find((candidate) =>
			item?.location.kind === 'room'
				? candidate.id === item.location.id
				: candidate.id === locationContainer?.room_id
		)
	);
	const locationChain = $derived(
		item?.location.kind === 'container' ? index.containerChain(item.location.id) : []
	);

	// Move destinations: every room or container. The API moves the item only;
	// no subtree or cycle concerns, unlike containers.
	const destinationGroups = $derived.by(() => {
		if (!item) return [];
		const groups = [
			{
				label: 'Rooms',
				options: inv.rooms.map((candidate) => ({
					value: `room:${candidate.id}`,
					label: candidate.name
				}))
			},
			{
				label: 'Containers',
				options: inv.containers.map((candidate) => ({
					value: `container:${candidate.id}`,
					label: index.containerPath(candidate.id)
				}))
			}
		];
		return groups.filter((group) => group.options.length > 0);
	});

	let problem = $state<Problem | null>(null);

	let editing = $state(false);
	let saving = $state(false);
	let saved = $state(false);

	let destination = $state('');
	let moving = $state(false);
	let moved = $state(false);

	function startEdit() {
		saved = false;
		problem = null;
		editing = true;
	}

	async function save(draft: ItemDraft) {
		saving = true;
		problem = null;
		try {
			await apiFetch<Item>(`/api/v1/items/${itemId}`, { method: 'PATCH', body: draft });
			await refreshInventory();
			saved = true;
			editing = false;
		} catch (error) {
			problem = toProblem(error, 'Could not save the item');
		} finally {
			saving = false;
		}
	}

	async function move(event: SubmitEvent) {
		event.preventDefault();
		if (!item || destination === '') return;
		moving = true;
		problem = null;
		moved = false;
		try {
			const [kind, rawId] = destination.split(':');
			const ref: LocationRef =
				kind === 'room'
					? { kind: 'room', id: Number(rawId) }
					: { kind: 'container', id: Number(rawId) };
			await apiFetch<Item>(`/api/v1/items/${itemId}/move`, {
				method: 'POST',
				body: { destination: ref }
			});
			await refreshInventory();
			destination = '';
			moved = true;
		} catch (error) {
			problem = toProblem(error, 'Could not move the item');
		} finally {
			moving = false;
		}
	}

	async function remove() {
		problem = null;
		const location = item?.location;
		try {
			await apiFetch<void>(`/api/v1/items/${itemId}`, { method: 'DELETE' });
			await refreshInventory();
			if (location?.kind === 'container') {
				await goto(resolve('/containers/[id]', { id: String(location.id) }));
			} else if (location) {
				await goto(resolve('/rooms/[id]', { id: String(location.id) }));
			} else {
				await goto(resolve('/rooms'));
			}
		} catch (error) {
			problem = toProblem(error, 'Could not delete the item');
		}
	}
</script>

<svelte:head><title>{item ? `${item.name} · homey` : 'Item · homey'}</title></svelte:head>

{#if inv.status === 'loading' || inv.status === 'idle'}
	<Spinner label="Loading the item…" />
{:else if !item}
	<EmptyState title="Item not found" hint="It may have been deleted. Go back to the rooms list." />
	<p class="spaced"><a href={resolve('/rooms')}>← All rooms</a></p>
{:else}
	<nav class="crumbs" aria-label="Breadcrumb">
		<ol>
			<li><a href={resolve('/rooms')}>Rooms</a></li>
			{#if locationRoom}
				<li>
					<a href={resolve('/rooms/[id]', { id: String(locationRoom.id) })}>{locationRoom.name}</a>
				</li>
			{/if}
			{#each locationChain as ancestor (ancestor.id)}
				<li>
					<a href={resolve('/containers/[id]', { id: String(ancestor.id) })}>{ancestor.name}</a>
				</li>
			{/each}
			<li><span aria-current="page">{item.name}</span></li>
		</ol>
	</nav>

	<h1>
		{item.name}
		{#if item.quantity > 1}<span class="muted">×{item.quantity}</span>{/if}
	</h1>

	<section class="card" aria-labelledby="details-heading">
		<h2 id="details-heading">Details</h2>
		{#if editing}
			<ItemForm
				initial={item}
				{saving}
				submitLabel="Save"
				onSubmit={save}
				onCancel={() => (editing = false)}
			/>
		{:else}
			<p>{item.description || 'No description.'}</p>
			{#if item.tags?.length}
				<ul class="tags" aria-label="Tags">
					{#each item.tags as tag (tag)}
						<li>{tag}</li>
					{/each}
				</ul>
			{/if}
			{#if item.notes}
				<p class="notes">Notes: {item.notes}</p>
			{/if}
			<p class="muted path">
				Quantity: {item.quantity} · Location: {index.locationLabel(item.location)}
			</p>
			<p class="muted path">
				Updated {formatTimestamp(item.updated_at)} · created {formatTimestamp(item.created_at)}
			</p>
			<div class="actions">
				<button class="btn" type="button" onclick={startEdit}>Edit</button>
				{#if saved}<span class="ok" role="status">Saved.</span>{/if}
			</div>
		{/if}
	</section>

	{#if problem}
		<div class="spaced">
			<ProblemPanel title="The operation failed" {problem} />
		</div>
	{/if}

	<section class="card" aria-labelledby="move-heading">
		<h2 id="move-heading">Move this item</h2>
		<form onsubmit={move}>
			<div class="field">
				<label for="move-destination">New location</label>
				<select id="move-destination" bind:value={destination} required>
					<option value="" disabled>Choose a destination…</option>
					{#each destinationGroups as group (group.label)}
						<optgroup label={group.label}>
							{#each group.options as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</optgroup>
					{/each}
				</select>
			</div>
			<div class="actions">
				<button class="btn" type="submit" disabled={moving || destination === ''}>
					{moving ? 'Moving…' : 'Move'}
				</button>
				{#if moved}<span class="ok" role="status">Moved.</span>{/if}
			</div>
		</form>
	</section>

	<section class="card danger-zone" aria-labelledby="danger-heading">
		<h2 id="danger-heading">Delete this item</h2>
		<p class="muted">Move it to the bin or remove it for good — there is no undo for now.</p>
		<ConfirmButton label="Delete item" confirmLabel="Delete for good?" onConfirm={remove} />
	</section>
{/if}

<style>
	.crumbs {
		margin: 0 0 0.5rem;
	}

	.crumbs ol {
		display: flex;
		flex-wrap: wrap;
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.crumbs li + li::before {
		color: var(--muted);
		content: '›';
		margin: 0 0.4rem;
	}

	.crumbs [aria-current='page'] {
		color: var(--muted);
	}

	h1 .muted {
		font-size: 1rem;
		font-weight: 400;
	}

	section {
		margin-top: 1.5rem;
	}

	.field {
		margin-bottom: 0.75rem;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.ok {
		color: var(--ok);
	}

	.spaced {
		margin-top: 1rem;
	}

	.path,
	.notes {
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

	.danger-zone {
		border-color: var(--danger);
	}
</style>
