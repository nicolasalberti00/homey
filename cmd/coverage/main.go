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
// This tool reads the profile, counts every block once — covered when any
// binary covered it — and prints the coverage of each package, lowest first,
// followed by the total. It exits non-zero when the total is below -min.
//
// Usage:
//
//	go test -coverpkg=./internal/... -coverprofile=coverage.out ./...
//	go run ./cmd/coverage -profile coverage.out
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

func main() {
	profile := flag.String("profile", "coverage.out", "profile written by go test -coverprofile")
	minimum := flag.Float64("min", 75, "minimum total statement coverage, in percent")
	flag.Parse()

	report, err := summarise(*profile)
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

// summarise reads a profile and returns the coverage of every package in it,
// ordered from the lowest coverage to the highest.
func summarise(profile string) (coverageReport, error) {
	file, err := os.Open(profile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// One entry per block, however many test binaries reported it.
	type block struct {
		packageOf  string
		statements int
		covered    bool
	}
	blocks := make(map[string]block)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// <file>:<start>,<end> <statements> <count>
		key, rest, found := strings.Cut(line, " ")
		if !found {
			return nil, fmt.Errorf("malformed profile line %q", line)
		}
		fields := strings.Fields(rest)
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed profile line %q", line)
		}
		statements, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("malformed statement count in %q: %w", line, err)
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("malformed count in %q: %w", line, err)
		}

		previous := blocks[key]
		blocks[key] = block{
			packageOf:  packageOf(key),
			statements: statements,
			covered:    count > 0 || previous.covered,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	totals := make(map[string]*packageCoverage)
	for _, b := range blocks {
		total, ok := totals[b.packageOf]
		if !ok {
			total = &packageCoverage{pkg: b.packageOf}
			totals[b.packageOf] = total
		}
		total.statements += b.statements
		if b.covered {
			total.covered += b.statements
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
	return report, nil
}

// packageOf turns the file of a profile block into the package that holds it.
func packageOf(key string) string {
	file, _, _ := strings.Cut(key, ":")
	return path.Dir(file)
}
