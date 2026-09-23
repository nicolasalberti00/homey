package inventory

import "strings"

// Search matches a free-text query against the items of the inventory. The
// rules live here, next to the ports, because every adapter and every caller
// (the REST API, the MCP server, the web UI) has to agree on them:
//
//   - a query is split into terms on whitespace and lowercased;
//   - an item matches when *every* term matches at least one of its fields:
//     the name, the description, an alias or a tag;
//   - matching is a case-insensitive substring test, so "trap" finds
//     "Trapano";
//   - a query with no terms (empty, or whitespace only) matches nothing.

// SearchTerms splits a query into the terms every candidate must match. It
// returns an empty slice when the query carries no term.
func SearchTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	if len(fields) == 0 {
		return []string{}
	}
	return fields
}
