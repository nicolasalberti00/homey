// "Remember where I was": the last page visited per sidebar section, so a
// sidebar click resumes it instead of always landing on the section root.
// sessionStorage keeps this per tab — it is session state, not a preference —
// and clicking the already active section still goes to the root, so there is
// always a one-click way out.

export type LastLocation = {
	/** Pathname (+ search) of the last visited page of the section. */
	path: string;
	/** Scroll offset to restore when resuming that page. */
	y: number;
};

const STORAGE_KEY = 'homey:last-location';

function read(): Record<string, LastLocation> {
	try {
		const raw = sessionStorage.getItem(STORAGE_KEY);
		if (!raw) return {};
		const parsed = JSON.parse(raw) as Record<string, unknown>;
		const locations: Record<string, LastLocation> = {};
		for (const [section, value] of Object.entries(parsed)) {
			if (typeof value !== 'object' || value === null) continue;
			const entry = value as { path?: unknown; y?: unknown };
			if (typeof entry.path === 'string' && typeof entry.y === 'number') {
				locations[section] = { path: entry.path, y: entry.y };
			}
		}
		return locations;
	} catch {
		// Unavailable or corrupt storage: remembering is best-effort.
		return {};
	}
}

export function getLastLocation(section: string): LastLocation | undefined {
	return read()[section];
}

export function rememberLocation(section: string, path: string, y: number): void {
	const locations = read();
	locations[section] = { path, y: Math.max(0, Math.round(y)) };
	try {
		sessionStorage.setItem(STORAGE_KEY, JSON.stringify(locations));
	} catch {
		// Private mode or full storage: the UI keeps working without memory.
	}
}
