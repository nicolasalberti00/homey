<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AddButton from '$lib/components/AddButton.svelte';
	import { page } from '$app/state';
	import {
		apiFetch,
		toProblem,
		type Container,
		type Item,
		type Problem,
		type Room
	} from '$lib/api/client';
	import ConfirmButton from '$lib/components/ConfirmButton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import ItemForm, { type ItemDraft } from '$lib/components/ItemForm.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory, refreshInventory } from '$lib/inventory.svelte';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());
	const roomId = $derived(Number(page.params.id));
	const room = $derived(inv.rooms.find((candidate) => candidate.id === roomId));
	const roomContainers = $derived(
		inv.containers.filter(
			(container) => container.room_id === roomId && container.parent_id === undefined
		)
	);
	const directItems = $derived(
		inv.items.filter((item) => item.location.kind === 'room' && item.location.id === roomId)
	);

	let editing = $state(false);
	let name = $state('');
	let description = $state('');
	let saving = $state(false);
	let saved = $state(false);
	let problem = $state<Problem | null>(null);

	let containerName = $state('');
	let containerDescription = $state('');
	let creatingContainer = $state(false);
	let creatingItem = $state(false);
	let showCreateContainer = $state(false);
	let showCreateItem = $state(false);

	// The edit form is seeded in the event handler, not in an effect.
	function startEdit() {
		if (!room) return;
		name = room.name;
		description = room.description;
		saved = false;
		problem = null;
		editing = true;
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		problem = null;
		try {
			await apiFetch<Room>(`/api/v1/rooms/${roomId}`, {
				method: 'PATCH',
				body: { name: name.trim(), description: description.trim() }
			});
			await refreshInventory();
			saved = true;
			editing = false;
		} catch (error) {
			problem = toProblem(error, 'Could not save the room');
		} finally {
			saving = false;
		}
	}

	async function createContainer(event: SubmitEvent) {
		event.preventDefault();
		if (!room) return;
		creatingContainer = true;
		problem = null;
		try {
			await apiFetch<Container>('/api/v1/containers', {
				method: 'POST',
				body: {
					name: containerName.trim(),
					description: containerDescription.trim(),
					room_id: roomId
				}
			});
			containerName = '';
			containerDescription = '';
			showCreateContainer = false;
			await refreshInventory();
		} catch (error) {
			problem = toProblem(error, 'Could not create the container');
		} finally {
			creatingContainer = false;
		}
	}

	async function createItem(draft: ItemDraft) {
		creatingItem = true;
		problem = null;
		try {
			await apiFetch<Item>('/api/v1/items', {
				method: 'POST',
				body: { ...draft, location: { kind: 'room', id: roomId } }
			});
			showCreateItem = false;
			await refreshInventory();
		} catch (error) {
			problem = toProblem(error, 'Could not create the item');
		} finally {
			creatingItem = false;
		}
	}

	async function remove() {
		problem = null;
		try {
			await apiFetch<void>(`/api/v1/rooms/${roomId}`, { method: 'DELETE' });
			await refreshInventory();
			await goto(resolve('/rooms'));
		} catch (error) {
			problem = toProblem(error, 'Could not delete the room');
		}
	}
</script>

<svelte:head><title>{room ? `${room.name} · homey` : 'Room · homey'}</title></svelte:head>

<p class="back"><a href={resolve('/rooms')}>← All rooms</a></p>

{#if inv.status === 'loading' || inv.status === 'idle'}
	<Spinner label="Loading the room…" />
{:else if !room}
	<EmptyState title="Room not found" hint="It may have been deleted. Go back to the rooms list." />
{:else}
	<h1>{room.name}</h1>

	<section class="card" aria-labelledby="details-heading">
		<h2 id="details-heading">Details</h2>
		{#if editing}
			<form onsubmit={save}>
				<div class="field">
					<label for="room-name">Name</label>
					<input id="room-name" bind:value={name} required maxlength="120" autocomplete="off" />
				</div>
				<div class="field">
					<label for="room-description">Description</label>
					<input
						id="room-description"
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
			<p>{room.description || 'No description.'}</p>
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

	<section aria-labelledby="containers-heading">
		<div class="section-head">
			<h2 id="containers-heading">Containers</h2>
			<AddButton
				label="New container"
				expanded={showCreateContainer}
				controls="create-container"
				onToggle={() => (showCreateContainer = !showCreateContainer)}
			/>
		</div>
		{#if showCreateContainer}
			<form id="create-container" class="card create" onsubmit={createContainer}>
				<h3>New container</h3>
				<div class="field">
					<label for="container-name">Name</label>
					<input
						id="container-name"
						bind:value={containerName}
						required
						maxlength="120"
						autocomplete="off"
					/>
				</div>
				<div class="field">
					<label for="container-description">
						Description <span class="muted">(optional)</span>
					</label>
					<input
						id="container-description"
						bind:value={containerDescription}
						maxlength="2000"
						autocomplete="off"
					/>
				</div>
				<div class="actions">
					<button class="btn primary" type="submit" disabled={creatingContainer}>
						{creatingContainer ? 'Creating…' : 'Create container'}
					</button>
					<button class="btn" type="button" onclick={() => (showCreateContainer = false)}
						>Cancel</button
					>
				</div>
			</form>
		{/if}
		{#if roomContainers.length === 0}
			<EmptyState
				title="No containers in this room"
				hint="Add the first one with + next to the title."
			/>
		{:else}
			<ul class="list">
				{#each roomContainers as container (container.id)}
					<li class="card">
						<p class="name">
							<a href={resolve('/containers/[id]', { id: String(container.id) })}
								>{container.name}</a
							>
						</p>
						{#if container.description}
							<p class="muted">{container.description}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<section aria-labelledby="items-heading">
		<div class="section-head">
			<h2 id="items-heading">Items in the room</h2>
			<AddButton
				label="New item"
				expanded={showCreateItem}
				controls="create-item"
				onToggle={() => (showCreateItem = !showCreateItem)}
			/>
		</div>
		{#if showCreateItem}
			<div id="create-item" class="card create">
				<h3>New item</h3>
				<ItemForm
					saving={creatingItem}
					onSubmit={createItem}
					onCancel={() => (showCreateItem = false)}
				/>
			</div>
		{/if}
		{#if directItems.length === 0}
			<EmptyState
				title="No items directly in this room"
				hint="Add the first one with + next to the title, or add items inside a container."
			/>
		{:else}
			<ul class="list">
				{#each directItems as item (item.id)}
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
		<h2 id="danger-heading">Delete this room</h2>
		<p class="muted">
			Only empty rooms can be deleted: move or remove the containers and items first.
		</p>
		<ConfirmButton label="Delete room" confirmLabel="Delete for good?" onConfirm={remove} />
	</section>
{/if}

<style>
	.back {
		margin: 0 0 0.5rem;
	}

	.create {
		margin-top: 1rem;
	}

	.create h3 {
		margin: 0 0 0.75rem;
		font-size: 1rem;
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
