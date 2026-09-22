// Shared inventory state (Step 4.4+): rooms, containers and items are loaded
// once and reused by the dashboard, rooms and (later) container/item screens.
//
// For a home inventory the three list endpoints return the whole dataset
// (GET /containers and GET /items without filters list everything), so the UI
// can build paths and counts client-side without N+1 requests.

import {
	ApiError,
	apiFetch,
	type Container,
	type Item,
	type Problem,
	type Room
} from '$lib/api/client';

type LoadStatus = 'idle' | 'loading' | 'ready' | 'error';

// API responses are only ever reassigned, never mutated: `$state.raw` skips
// the deep-proxy overhead (see svelte-core-bestpractices).
let rooms = $state.raw<Room[]>([]);
let containers = $state.raw<Container[]>([]);
let items = $state.raw<Item[]>([]);
let status = $state<LoadStatus>('idle');
let problem = $state<Problem | null>(null);

/** Current snapshot; call it from templates/components to stay reactive. */
export function inventory() {
	return { rooms, containers, items, status, problem };
}

export async function refreshInventory(): Promise<void> {
	status = 'loading';
	problem = null;
	try {
		const [loadedRooms, loadedContainers, loadedItems] = await Promise.all([
			apiFetch<Room[] | null>('/api/v1/rooms'),
			apiFetch<Container[] | null>('/api/v1/containers'),
			apiFetch<Item[] | null>('/api/v1/items')
		]);
		rooms = loadedRooms ?? [];
		containers = loadedContainers ?? [];
		items = loadedItems ?? [];
		status = 'ready';
	} catch (error) {
		problem =
			error instanceof ApiError
				? error.problem
				: { title: 'Could not load the inventory', status: 0, detail: String(error) };
		status = 'error';
	}
}

/** Loads once per session; safe to call from an effect on every page. */
export async function ensureLoaded(): Promise<void> {
	if (status === 'idle') {
		await refreshInventory();
	}
}
