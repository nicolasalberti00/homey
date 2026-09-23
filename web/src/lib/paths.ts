// Path and label helpers for locations (Step 4.4+): pure functions over the
// loaded lists, no reactive state, so they live in a plain module.

import type { Container, LocationRef, Room } from '$lib/api/client';

export type InventoryIndex = {
	roomName(id: number): string;
	/** "Garage > Toolbox > Drawer 1" for a container, or "Garage" for a room. */
	containerPath(id: number): string;
	locationLabel(location: LocationRef): string;
};

/** Builds lookup maps once so lists and search can resolve names cheaply. */
export function buildIndex(roomList: Room[], containerList: Container[]): InventoryIndex {
	const roomNames = new Map(roomList.map((room) => [room.id, room.name]));
	const byId = new Map(containerList.map((container) => [container.id, container]));

	function containerPath(id: number): string {
		const parts: string[] = [];
		const seen = new Set<number>();
		let current = byId.get(id);
		const roomId = current?.room_id;
		while (current && !seen.has(current.id)) {
			seen.add(current.id);
			parts.unshift(current.name);
			current = current.parent_id ? byId.get(current.parent_id) : undefined;
		}
		const room = roomId === undefined ? undefined : roomNames.get(roomId);
		return room === undefined ? parts.join(' > ') : [room, ...parts].join(' > ');
	}

	function locationLabel(location: LocationRef): string {
		if (location.kind === 'container') {
			return containerPath(location.id);
		}
		return roomNames.get(location.id) ?? `room ${location.id}`;
	}

	return {
		roomName: (id) => roomNames.get(id) ?? `room ${id}`,
		containerPath,
		locationLabel
	};
}
