package inventory

import (
	"slices"
	"testing"
)

func TestSearchTerms(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{}},
		{"   ", []string{}},
		{"\t\n", []string{}},
		{"Drill", []string{"drill"}},
		{"  Bosch   Drill ", []string{"bosch", "drill"}},
		{"Città", []string{"citta"}},
		{"50%", []string{"50%"}},
	}

	for _, tc := range cases {
		if got := SearchTerms(tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("SearchTerms(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestFoldText(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"already folded", "caffe", "caffe"},
		{"plain name", "Trapano", "trapano"},
		{"grave accent", "Caffè", "caffe"},
		{"acute accent", "caffé", "caffe"},
		{"upper case accents", "CAFFÈ", "caffe"},
		{"accent inside a word", "Crème brûlée", "creme brulee"},
		{"ring and umlaut", "Ångström", "angstrom"},
		{"cedilla and tilde", "Façade · Niño", "facade · nino"},
		{"decomposed input", "caffe\u0300", "caffe"},
		{"empty", "", ""},
		// Folding removes diacritics, it does not transliterate: a letter
		// without a decomposition keeps its identity.
		{"slashed o", "søren", "søren"},
		{"sharp s", "Straße", "straße"},
		{"ae ligature", "Encyclopædia", "encyclopædia"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FoldText(tc.text); got != tc.want {
				t.Fatalf("FoldText(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestFoldTextIsIdempotent(t *testing.T) {
	// The query is folded once and the stored text once per row; folding a
	// folded string has to be a no-op, or the two sides would drift.
	for _, text := range []string{"Caffè", "CAFFÈ", "Crème brûlée", "Trapano", "søren", ""} {
		once := FoldText(text)
		if twice := FoldText(once); twice != once {
			t.Errorf("FoldText(FoldText(%q)) = %q, want %q", text, twice, once)
		}
	}
}

func BenchmarkFoldText(b *testing.B) {
	for _, tc := range []struct{ name, text string }{
		{"ascii", "Bosch drill 0042"},
		{"accented", "Caffè"},
		{"accented sentence", "Crème brûlée con panna"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for range b.N {
				FoldText(tc.text)
			}
		})
	}
}
