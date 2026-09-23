package inventory

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Search matches a free-text query against the items of the inventory. The
// rules live here, next to the ports, because every adapter and every caller
// (the REST API, the MCP server, the web UI) has to agree on them:
//
//   - a query is split into terms on whitespace and folded;
//   - an item matches when *every* term matches at least one of its fields:
//     the name, the description, an alias or a tag;
//   - matching is a substring test over folded text, so "trap" finds
//     "Trapano", "caffe" finds "Caffè" and "CAFFÉ" finds "caffè";
//   - a query with no terms (empty, or whitespace only) matches nothing.

// SearchTerms splits a query into the terms every candidate must match, each
// in the folded form the adapters compare with (see FoldText). It returns an
// empty slice when the query carries no term.
func SearchTerms(query string) []string {
	fields := strings.Fields(FoldText(query))
	if len(fields) == 0 {
		return []string{}
	}
	return fields
}

// FoldText reduces text to the form search compares: lower case and without
// diacritics, so "Caffè", "caffé" and "CAFFE" all fold to "caffe" and the
// accent a user types — or forgets — never decides whether an item is found.
//
// Folding is not transliteration: a letter that does not decompose keeps its
// identity, so "søren" does not fold to "soren".
func FoldText(text string) string {
	if isASCII(text) {
		// Most names never leave ASCII, and there is nothing to strip.
		return strings.ToLower(text)
	}
	return strings.ToLower(stripMarks(norm.NFD.String(text)))
}

// stripMarks drops the combining marks a decomposition leaves behind.
func stripMarks(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, text)
}

// isASCII reports whether every byte of text is ASCII.
func isASCII(text string) bool {
	for index := range len(text) {
		if text[index] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
