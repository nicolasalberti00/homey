<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		apiFetch,
		toProblem,
		type Container,
		type ContainerDetail,
		type Item,
		type LocationRef,
		type Problem
	} from '$lib/api/client';
	import ConfirmButton from '$lib/components/ConfirmButton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import ItemForm, { type ItemDraft } from '$lib/components/ItemForm.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory, refreshInventory } from '$lib/inventory.svelte';
	import { buildIndex } from '$lib/paths';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());
	const index = $derived(buildIndex(inv.rooms, inv.containers));
	const containerId = $derived(Number(page.params.id));
	const container = $derived(inv.containers.find((candidate) => candidate.id === containerId));
	const room = $derived(inv.rooms.find((candidate) => candidate.id === container?.room_id));
	// Ancestors first, the container itself last: the breadcrumb chain.
	const chain = $derived(index.containerChain(containerId));
	const children = $derived(
		inv.containers.filter((candidate) => candidate.parent_id === containerId)
	);
	const items = $derived(
		inv.items.filter(
			(item) => item.location.kind === 'container' && item.location.id === containerId
		)
	);

	// Move destinations: a room makes the container a root there, a container
	// makes it a child. The container itself and its subtree are excluded —
	// the API rejects cycles, and offering them would only be a trap.
	const destinationGroups = $derived.by(() => {
		if (!container) return [];
		const excluded = index.subtreeIds(containerId);
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
				options: inv.containers
					.filter((candidate) => !excluded.has(candidate.id))
					.map((candidate) => ({
						value: `container:${candidate.id}`,
						label: index.containerPath(candidate.id)
					}))
			}
		];
		return groups.filter((group) => group.options.length > 0);
	});

	let problem = $state<Problem | null>(null);

	// Create a container inside this one.
	let childName = $state('');
	let childDescription = $state('');
	let creating = $state(false);
	let creatingItem = $state(false);

	// Edit this container.
	let editing = $state(false);
	let name = $state('');
	let description = $state('');
	let saving = $state(false);
	let saved = $state(false);

	// Move this container (with its subtree).
	let destination = $state('');
	let moving = $state(false);
	let moved = $state(false);

	// The edit form is seeded in the event handler, not in an effect.
	function startEdit() {
		if (!container) return;
		name = container.name;
		description = container.description;
		saved = false;
		problem = null;
		editing = true;
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		problem = null;
		try {
			await apiFetch<Container>(`/api/v1/containers/${containerId}`, {
				method: 'PATCH',
				body: { name: name.trim(), description: description.trim() }
			});
			await refreshInventory();
			saved = true;
			editing = false;
		} catch (error) {
			problem = toProblem(error, 'Could not save the container');
		} finally {
			saving = false;
		}
	}

	async function createChild(event: SubmitEvent) {
		event.preventDefault();
		if (!container) return;
		creating = true;
		problem = null;
		try {
			await apiFetch<Container>('/api/v1/containers', {
				method: 'POST',
				body: {
					name: childName.trim(),
					description: childDescription.trim(),
					room_id: container.room_id,
					parent_id: container.id
				}
			});
			childName = '';
			childDescription = '';
			await refreshInventory();
		} catch (error) {
			problem = toProblem(error, 'Could not create the container');
		} finally {
			creating = false;
		}
	}

	async function createItem(draft: ItemDraft) {
		if (!container) return;
		creatingItem = true;
		problem = null;
		try {
			await apiFetch<Item>('/api/v1/items', {
				method: 'POST',
				body: { ...draft, location: { kind: 'container', id: container.id } }
			});
			await refreshInventory();
		} catch (error) {
			problem = toProblem(error, 'Could not create the item');
		} finally {
			creatingItem = false;
		}
	}

	async function move(event: SubmitEvent) {
		event.preventDefault();
		if (!container || destination === '') return;
		moving = true;
		problem = null;
		moved = false;
		try {
			const [kind, rawId] = destination.split(':');
			const ref: LocationRef =
				kind === 'room'
					? { kind: 'room', id: Number(rawId) }
					: { kind: 'container', id: Number(rawId) };
			await apiFetch<ContainerDetail>(`/api/v1/containers/${containerId}/move`, {
				method: 'POST',
				body: { destination: ref }
			});
			await refreshInventory();
			destination = '';
			moved = true;
		} catch (error) {
			problem = toProblem(error, 'Could not move the container');
		} finally {
			moving = false;
		}
	}

	async function remove() {
		problem = null;
		const parentId = container?.parent_id;
		const roomId = container?.room_id;
		try {
			await apiFetch<void>(`/api/v1/containers/${containerId}`, { method: 'DELETE' });
			await refreshInventory();
			if (parentId !== undefined) {
				await goto(resolve('/containers/[id]', { id: String(parentId) }));
			} else if (roomId !== undefined) {
				await goto(resolve('/rooms/[id]', { id: String(roomId) }));
			} else {
				await goto(resolve('/rooms'));
			}
		} catch (error) {
			problem = toProblem(error, 'Could not delete the container');
		}
	}
</script>

<svelte:head>
	<title>{container ? `${container.name} · homey` : 'Container · homey'}</title>
</svelte:head>

{#if inv.status === 'loading' || inv.status === 'idle'}
	<Spinner label="Loading the container…" />
{:else if !container}
	<EmptyState
		title="Container not found"
		hint="It may have been deleted. Go back to the rooms list."
	/>
	<p class="spaced"><a href={resolve('/rooms')}>← All rooms</a></p>
{:else}
	<nav class="crumbs" aria-label="Breadcrumb">
		<ol>
			<li><a href={resolve('/rooms')}>Rooms</a></li>
			{#if room}
				<li><a href={resolve('/rooms/[id]', { id: String(room.id) })}>{room.name}</a></li>
			{/if}
			{#each chain as ancestor, i (ancestor.id)}
				<li>
					{#if i === chain.length - 1}
						<span aria-current="page">{ancestor.name}</span>
					{:else}
						<a href={resolve('/containers/[id]', { id: String(ancestor.id) })}>
							{ancestor.name}
						</a>
					{/if}
				</li>
			{/each}
		</ol>
	</nav>

	<h1>{container.name}</h1>

	<section class="card" aria-labelledby="details-heading">
		<h2 id="details-heading">Details</h2>
		{#if editing}
			<form onsubmit={save}>
				<div class="field">
					<label for="container-name">Name</label>
					<input
						id="container-name"
						bind:value={name}
						required
						maxlength="120"
						autocomplete="off"
					/>
				</div>
				<div class="field">
					<label for="container-description">Description</label>
					<input
						id="container-description"
						bind:value={description}
						maxlength="2000"
						autocomplete="off"
					/>
				</div>
				<div class="actions">
					<button class="btn primary" type="submit" disabled={saving}>
						{saving ? 'Saving…' : 'Save'}
					</button>
					<button class="btn" type="button" onclick={() => (editing = false)}>Cancel</button>
				</div>
			</form>
		{:else}
			<p>{container.description || 'No description.'}</p>
			<p class="muted path">Location: {index.containerPath(container.id)}</p>
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
		<h2 id="move-heading">Move this container</h2>
		<p class="muted">The whole subtree moves with it.</p>
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

	<section aria-labelledby="children-heading">
		<h2 id="children-heading">Containers inside</h2>
		<form class="card create" onsubmit={createChild}>
			<h3>New container</h3>
			<div class="field">
				<label for="child-name">Name</label>
				<input id="child-name" bind:value={childName} required maxlength="120" autocomplete="off" />
			</div>
			<div class="field">
				<label for="child-description">
					Description <span class="muted">(optional)</span>
				</label>
				<input
					id="child-description"
					bind:value={childDescription}
					maxlength="2000"
					autocomplete="off"
				/>
			</div>
			<button class="btn primary" type="submit" disabled={creating}>
				{creating ? 'Creating…' : 'Create container'}
			</button>
		</form>
		{#if children.length === 0}
			<EmptyState title="No containers inside" hint="Create one with the form above." />
		{:else}
			<ul class="list">
				{#each children as child (child.id)}
					<li class="card">
						<p class="name">
							<a href={resolve('/containers/[id]', { id: String(child.id) })}>{child.name}</a>
						</p>
						{#if child.description}
							<p class="muted">{child.description}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<section aria-labelledby="items-heading">
		<h2 id="items-heading">Items inside</h2>
		<div class="card create">
			<h3>New item</h3>
			<ItemForm saving={creatingItem} onSubmit={createItem} />
		</div>
		{#if items.length === 0}
			<EmptyState title="No items here" hint="Create the first item with the form above." />
		{:else}
			<ul class="list">
				{#each items as item (item.id)}
					<li class="card">
						<p class="name">
							<a href={resolve('/items/[id]', { id: String(item.id) })}>{item.name}</a>
							{#if item.quantity > 1}<span class="muted">×{item.quantity}</span>{/if}
						</p>
						{#if item.description}
							<p class="muted">{item.description}</p>
						{/if}
						{#if item.tags?.length}
							<ul class="tags" aria-label="Tags">
								{#each item.tags as tag (tag)}
									<li>{tag}</li>
								{/each}
							</ul>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<section class="card danger-zone" aria-labelledby="danger-heading">
		<h2 id="danger-heading">Delete this container</h2>
		<p class="muted">
			Only empty containers can be deleted: move or remove the containers and items inside first.
		</p>
		<ConfirmButton label="Delete container" confirmLabel="Delete for good?" onConfirm={remove} />
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

	section {
		margin-top: 1.5rem;
	}

	.create {
		max-width: 32rem;
		margin-bottom: 1.5rem;
	}

	.create h3 {
		margin: 0 0 0.75rem;
		font-size: 1rem;
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

	.path {
		font-size: 0.9rem;
	}

	.list {
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

	.list p {
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

	.danger-zone {
		border-color: var(--danger);
	}
</style>
