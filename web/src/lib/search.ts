// The terms a query is made of, and the segments the UI marks. Searching
// itself is the server's job (the REST API, the web UI and the MCP server all
// present the same ranked candidates); highlighting stays here, because it is
// a rendering concern over text the page already has. Pure functions over
// plain data, no reactive state.

/** A piece of text, flagged when it is part of a query match. */
export type Segment = { text: string; match: boolean };

/** "  Drill  bits " → ["drill", "bits"]: every term must match. */
export function queryTerms(query: string): string[] {
	return query
		.trim()
		.toLowerCase()
		.split(/\s+/)
		.filter((term) => term.length > 0);
}

/**
 * Splits `text` into plain and matched segments, so the UI can mark matches
 * without `{@html}`. Overlapping terms keep the longest match.
 */
export function highlightSegments(text: string, terms: string[]): Segment[] {
	const needles = terms.filter((term) => term.length > 0);
	if (text.length === 0 || needles.length === 0) {
		return [{ text, match: false }];
	}
	const lower = text.toLowerCase();
	const segments: Segment[] = [];
	let cursor = 0;
	while (cursor < text.length) {
		let start = -1;
		let length = 0;
		for (const needle of needles) {
			const at = lower.indexOf(needle, cursor);
			if (at === -1) continue;
			if (start === -1 || at < start || (at === start && needle.length > length)) {
				start = at;
				length = needle.length;
			}
		}
		if (start === -1) break;
		if (start > cursor) {
			segments.push({ text: text.slice(cursor, start), match: false });
		}
		segments.push({ text: text.slice(start, start + length), match: true });
		cursor = start + length;
	}
	if (cursor < text.length) {
		segments.push({ text: text.slice(cursor), match: false });
	}
	return segments;
}
