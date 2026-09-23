<script lang="ts">
	import { resolve } from '$app/paths';
	import { ApiError, apiFetch, type Problem, type Room } from '$lib/api/client';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import ProblemPanel from '$lib/components/ProblemPanel.svelte';
	import Spinner from '$lib/components/Spinner.svelte';
	import { ensureLoaded, inventory, refreshInventory } from '$lib/inventory.svelte';

	// Single-page app: kick the shared load off once, without an effect.
	void ensureLoaded();

	const inv = $derived(inventory());

	let name = $state('');
	let description = $state('');
	let saving = $state(false);
	let problem = $state<Problem | null>(null);

	async function createRoom(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		problem = null;
		try {
			await apiFetch<Room>('/api/v1/rooms', {
				method: 'POST',
				body: { name: name.trim(), description: description.trim() }
			});
			name = '';
			description = '';
			await refreshInventory();
		} catch (error) {
			problem =
				error instanceof ApiError
					? error.problem
					: { title: 'Could not create the room', status: 0, detail: String(error) };
		} finally {
			saving = false;
		}
	}

	// Counts per room: containers at any depth belong to their room via
	// room_id; items count whether they sit directly in the room or in one of
	// its containers.
	function roomCounts(roomId: number): { containers: number; items: number } {
		const containerIds = new Set(
			inv.containers.filter((container) => container.room_id === roomId).map((c) => c.id)
		);
		const itemCount = inv.items.filter((item) =>
			item.location.kind === 'room'
				? item.location.id === roomId
				: containerIds.has(item.location.id)
		).length;
		return { containers: containerIds.size, items: itemCount };
	}
</script>

<svelte:head><title>Rooms · homey</title></svelte:head>

<h1>Rooms</h1>

<form class="card create" onsubmit={createRoom}>
	<h2>New room</h2>
	<div class="field">
		<label for="room-name">Name</label>
		<input id="room-name" bind:value={name} required maxlength="120" autocomplete="off" />
	</div>
	<div class="field">
		<label for="room-description">Description <span class="muted">(optional)</span></label>
		<input id="room-description" bind:value={description} maxlength="2000" autocomplete="off" />
	</div>
	<button class="btn primary" type="submit" disabled={saving}>
		{saving ? 'Creating…' : 'Create room'}
	</button>
</form>

{#if problem}
	<div class="spaced">
		<ProblemPanel title="Could not create the room" {problem} />
	</div>
{/if}

{#if inv.status === 'loading' || inv.status === 'idle'}
	<div class="spaced"><Spinner label="Loading rooms…" /></div>
{:else if inv.status === 'error'}
	{#if inv.problem}
		<div class="spaced">
			<ProblemPanel title="Could not load the rooms" problem={inv.problem} />
		</div>
	{/if}
{:else if inv.rooms.length === 0}
	<div class="spaced">
		<EmptyState title="No rooms yet" hint="Create the first room with the form above." />
	</div>
{:else}
	<ul class="rooms">
		{#each inv.rooms as room (room.id)}
			{@const counts = roomCounts(room.id)}
			<li class="card">
				<h2 class="room-name">
					<a href={resolve('/rooms/[id]', { id: String(room.id) })}>{room.name}</a>
				</h2>
				{#if room.description}
					<p class="muted">{room.description}</p>
				{/if}
				<p class="muted counts">
					{counts.containers}
					{counts.containers === 1 ? 'container' : 'containers'}
					·
					{counts.items}
					{counts.items === 1 ? 'item' : 'items'}
				</p>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.create {
		margin-bottom: 1.5rem;
	}

	.field {
		margin-bottom: 0.75rem;
	}

	.spaced {
		margin-bottom: 1rem;
	}

	.rooms {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.75rem;
	}

	.room-name {
		margin: 0;
		font-size: 1.1rem;
	}

	.rooms p {
		margin: 0.25rem 0 0;
	}

	.counts {
		font-size: 0.9rem;
	}
</style>
