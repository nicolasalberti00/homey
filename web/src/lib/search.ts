// The terms a query is made of, and the segments the UI marks. Searching
// itself is the server's job (the REST API, the web UI and the MCP server all
// present the same ranked candidates); highlighting stays here, because it is
// a rendering concern over text the page already has. Pure functions over
// plain data, no reactive state.

/** A piece of text, flagged when it is part of a query match. */
export type Segment = { text: string; match: boolean };

/**
 * Folds text the way the server does: lower case, without diacritics, so a
 * query typed as "caffe" highlights "Caffè". Folding keeps one code point per
 * original one for the Latin letters this app stores, which is what lets the
 * segments below be sliced out of the original text.
 */
export function foldText(text: string): string {
	return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}

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
 * without `{@html}`. The comparison happens on folded text — the same form the
 * server matched — while the segments keep the original spelling. Folding here
 * rather than in the terms keeps the highlight right whether the matching ran
 * on the server or, as it did before the search endpoint, in the browser.
 * Overlapping terms keep the longest match.
 */
export function highlightSegments(text: string, terms: string[]): Segment[] {
	const needles = terms.map(foldText).filter((term) => term.length > 0);
	if (text.length === 0 || needles.length === 0) {
		return [{ text, match: false }];
	}
	const lower = foldText(text);
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
