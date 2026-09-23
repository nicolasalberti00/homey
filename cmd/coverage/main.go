// Command coverage summarises a Go coverage profile and enforces the minimum
// this repository expects.
//
// It exists because the obvious numbers mislead in two ways:
//
//   - `go test -cover ./...` reports each package against its *own* tests, so
//     coverage a package gets from other packages' tests is invisible (the
//     contract suite in internal/inventory/inventorytest drives
//     internal/storage, the server tests drive internal/search);
//   - `go test -coverpkg` writes one line per block *per test binary*, so the
//     same block appears many times: summing them reports a fraction of the
//     real coverage (15% instead of 93% in this repository).
//
// cover.ParseProfiles parses the profile and merges the samples of a block —
// the numbers below therefore agree with `go tool cover -func`, which uses the
// same package. What that tool does not print is a per-package summary, and
// that is all this command adds: it groups the parsed profiles by package,
// prints the coverage of each one, lowest first, and fails below -min.
//
// Usage:
//
//	go test -coverpkg=./internal/...,./mcp/... -coverprofile=coverage.out ./...
//	go run ./cmd/coverage -profile coverage.out
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"golang.org/x/tools/cover"
)

func main() {
	profile := flag.String("profile", "coverage.out", "profile written by go test -coverprofile")
	minimum := flag.Float64("min", 75, "minimum total statement coverage, in percent")
	flag.Parse()

	report, err := load(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "coverage:", err)
		os.Exit(1)
	}
	if !report.write(os.Stdout, *minimum) {
		os.Exit(1)
	}
}

// packageCoverage is the statement coverage of one package.
type packageCoverage struct {
	pkg        string
	statements int
	covered    int
}

// percent is the covered share of the package statements, 0 when it has none.
func (p packageCoverage) percent() float64 {
	if p.statements == 0 {
		return 0
	}
	return 100 * float64(p.covered) / float64(p.statements)
}

// coverageReport is the coverage of every package in a profile.
type coverageReport []packageCoverage

// total sums the report into one line.
func (r coverageReport) total() packageCoverage {
	var total packageCoverage
	for _, pkg := range r {
		total.statements += pkg.statements
		total.covered += pkg.covered
	}
	return total
}

// write prints the report and reports whether the total reaches minimum.
func (r coverageReport) write(w io.Writer, minimum float64) bool {
	total := r.total()
	fmt.Fprintln(w, "coverage by package (statements), lowest first:")
	for _, pkg := range r {
		fmt.Fprintf(w, "  %s  %.1f%%  (%d/%d)\n", pkg.pkg, pkg.percent(), pkg.covered, pkg.statements)
	}
	status := "OK"
	if total.percent() < minimum {
		status = "BELOW MINIMUM"
	}
	fmt.Fprintf(w, "total: %.1f%% (%d/%d statements), minimum %.1f%%: %s\n",
		total.percent(), total.covered, total.statements, minimum, status)
	return status == "OK"
}

// load parses a coverage profile and groups it by package.
func load(profile string) (coverageReport, error) {
	profiles, err := cover.ParseProfiles(profile)
	if err != nil {
		return nil, err
	}
	return summarise(profiles), nil
}

// summarise sums the parsed profiles per package, ordered from the lowest
// coverage to the highest. Each profile covers one file, and the package is
// the directory that holds it.
func summarise(profiles []*cover.Profile) coverageReport {
	totals := make(map[string]*packageCoverage)
	for _, profile := range profiles {
		pkg := path.Dir(profile.FileName)
		total, ok := totals[pkg]
		if !ok {
			total = &packageCoverage{pkg: pkg}
			totals[pkg] = total
		}
		for _, block := range profile.Blocks {
			total.statements += block.NumStmt
			if block.Count > 0 {
				total.covered += block.NumStmt
			}
		}
	}

	report := make(coverageReport, 0, len(totals))
	for _, total := range totals {
		report = append(report, *total)
	}
	sort.Slice(report, func(i, j int) bool {
		if report[i].percent() != report[j].percent() {
			return report[i].percent() < report[j].percent()
		}
		return report[i].pkg < report[j].pkg
	})
	return report
}
