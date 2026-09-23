// Path and label helpers for locations (Step 4.4+): pure functions over the
// loaded lists, no reactive state, so they live in a plain module.

import type { Container, LocationRef, Room } from '$lib/api/client';

export type InventoryIndex = {
	roomName(id: number): string;
	/** "Garage > Toolbox > Drawer 1" for a container, or "Garage" for a room. */
	containerPath(id: number): string;
	locationLabel(location: LocationRef): string;
	/** The room an item location sits in, or undefined when unknown. */
	locationRoomId(location: LocationRef): number | undefined;
	/** Ancestors first, the container itself last; empty when unknown. */
	containerChain(id: number): Container[];
	/** The container itself plus every descendant (for move destinations). */
	subtreeIds(id: number): Set<number>;
};

/** Builds lookup maps once so lists and search can resolve names cheaply. */
export function buildIndex(roomList: Room[], containerList: Container[]): InventoryIndex {
	const roomNames = new Map(roomList.map((room) => [room.id, room.name]));
	const byId = new Map(containerList.map((container) => [container.id, container]));
	const childrenOf = new Map<number, Container[]>();
	for (const container of containerList) {
		if (container.parent_id === undefined) continue;
		const siblings = childrenOf.get(container.parent_id);
		if (siblings) siblings.push(container);
		else childrenOf.set(container.parent_id, [container]);
	}

	function containerPath(id: number): string {
		const parts = containerChain(id).map((container) => container.name);
		const roomId = byId.get(id)?.room_id;
		const room = roomId === undefined ? undefined : roomNames.get(roomId);
		return room === undefined ? parts.join(' > ') : [room, ...parts].join(' > ');
	}

	function containerChain(id: number): Container[] {
		const chain: Container[] = [];
		const seen = new Set<number>();
		let current = byId.get(id);
		while (current && !seen.has(current.id)) {
			seen.add(current.id);
			chain.unshift(current);
			current = current.parent_id === undefined ? undefined : byId.get(current.parent_id);
		}
		return chain;
	}

	function subtreeIds(id: number): Set<number> {
		const ids = new Set<number>();
		const stack = [id];
		while (stack.length > 0) {
			const current = stack.pop();
			if (current === undefined || ids.has(current)) continue;
			ids.add(current);
			for (const child of childrenOf.get(current) ?? []) stack.push(child.id);
		}
		return ids;
	}

	function locationLabel(location: LocationRef): string {
		if (location.kind === 'container') {
			return containerPath(location.id);
		}
		return roomNames.get(location.id) ?? `room ${location.id}`;
	}

	function locationRoomId(location: LocationRef): number | undefined {
		if (location.kind === 'room') {
			return roomNames.has(location.id) ? location.id : undefined;
		}
		return byId.get(location.id)?.room_id;
	}

	return {
		roomName: (id) => roomNames.get(id) ?? `room ${id}`,
		containerPath,
		containerChain,
		subtreeIds,
		locationLabel,
		locationRoomId
	};
}
