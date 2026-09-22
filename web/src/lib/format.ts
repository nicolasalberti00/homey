// Display formatting helpers (pure, non-reactive).

/** Formats an API timestamp for display. */
export function formatTimestamp(value: string): string {
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
