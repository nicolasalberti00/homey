package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProfile stores a profile in the test's temporary directory.
func writeProfile(t *testing.T, content string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("writing profile: %v", err)
	}
	return file
}

func TestSummariseCountsEachBlockOnce(t *testing.T) {
	// The same block is reported by two test binaries: one covered it, the
	// other did not. It must count once, and as covered — summing the lines
	// would report a fraction of the real coverage.
	profile := writeProfile(t, `mode: set
example.com/app/a/a.go:1.1,2.2 3 1
example.com/app/a/a.go:1.1,2.2 3 0
example.com/app/a/a.go:3.1,4.2 2 0
example.com/app/b/b.go:1.1,2.2 5 0
`)

	report, err := summarise(profile)
	if err != nil {
		t.Fatalf("summarise: %v", err)
	}
	if len(report) != 2 {
		t.Fatalf("report = %+v, want two packages", report)
	}

	// Lowest first: b (0%) before a (60%: 3 of 5 statements).
	if report[0].pkg != "example.com/app/b" || report[0].percent() != 0 {
		t.Fatalf("report[0] = %+v, want example.com/app/b at 0%%", report[0])
	}
	if report[1].pkg != "example.com/app/a" || report[1].statements != 5 || report[1].covered != 3 {
		t.Fatalf("report[1] = %+v, want example.com/app/a with 3/5 statements", report[1])
	}

	total := report.total()
	if total.statements != 10 || total.covered != 3 {
		t.Fatalf("total = %+v, want 3/10 statements", total)
	}
}

func TestWriteEnforcesTheMinimum(t *testing.T) {
	report := coverageReport{
		{pkg: "example.com/app/a", statements: 3, covered: 3},
		{pkg: "example.com/app/b", statements: 1, covered: 0},
	}

	var out bytes.Buffer
	if !report.write(&out, 75) {
		t.Fatalf("write reported a failure at 75%% for 3/4 statements:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "total: 75.0%") || !strings.Contains(out.String(), "OK") {
		t.Fatalf("write output = %q, want the total and OK", out.String())
	}

	out.Reset()
	if report.write(&out, 80) {
		t.Fatalf("write reported success at 80%% for 3/4 statements:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "BELOW MINIMUM") {
		t.Fatalf("write output = %q, want BELOW MINIMUM", out.String())
	}
}

func TestSummariseRejectsMalformedProfiles(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"missing counts", "mode: set\nexample.com/app/a/a.go:1.1,2.2\n"},
		{"statement count is not a number", "mode: set\nexample.com/app/a/a.go:1.1,2.2 three 1\n"},
		{"count is not a number", "mode: set\nexample.com/app/a/a.go:1.1,2.2 3 one\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := summarise(writeProfile(t, tc.content)); err == nil {
				t.Fatal("summarise accepted a malformed profile")
			}
		})
	}
}

func TestSummariseRejectsMissingProfile(t *testing.T) {
	if _, err := summarise(filepath.Join(t.TempDir(), "absent.out")); err == nil {
		t.Fatal("summarise accepted a missing profile")
	}
}
