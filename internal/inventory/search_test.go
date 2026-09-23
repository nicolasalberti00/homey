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
		{"Città", []string{"città"}},
		{"50%", []string{"50%"}},
	}

	for _, tc := range cases {
		if got := SearchTerms(tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("SearchTerms(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}
