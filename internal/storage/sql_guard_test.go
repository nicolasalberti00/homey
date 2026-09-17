package storage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sqlStatementMethods are the database/sql methods whose SQL statement
// argument must never be built dynamically.
var sqlStatementMethods = map[string]bool{
	"Exec": true, "ExecContext": true,
	"Query": true, "QueryContext": true,
	"QueryRow": true, "QueryRowContext": true,
	"Prepare": true, "PrepareContext": true,
}

// TestSQLStatementsAreParameterized enforces the SQL safety convention of
// this repository: statements are constant strings with ? placeholders and
// every user-provided value is passed as a parameter. Building a statement
// with + or fmt.Sprintf is a SQL injection waiting to happen, so it fails
// the build here instead of reaching production.
//
// The check parses every Go file in the repository, not just this package,
// so it also guards the REST, MCP and future adapter layers.
func TestSQLStatementsAreParameterized(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("finding repository root: %v", err)
	}

	var violations []string
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".scratch", "data", "testdata", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !sqlStatementMethods[sel.Sel.Name] {
				return true
			}
			if arg := sqlStatementArgument(call, sel.Sel.Name); arg != nil && buildsString(arg) {
				pos := fset.Position(call.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: %s() receives a statement built with + or fmt.Sprintf",
					rel, pos.Line, sel.Sel.Name,
				))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scanning sources: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("SQL statements must be constant strings with ? placeholders and values passed as parameters:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// sqlStatementArgument returns the expression used as the SQL statement for
// the given database/sql method call, or nil when the call shape is unknown.
func sqlStatementArgument(call *ast.CallExpr, method string) ast.Expr {
	firstArg := 0
	switch method {
	case "ExecContext", "QueryContext", "QueryRowContext", "PrepareContext":
		firstArg = 1 // the context is the first argument
	}
	if len(call.Args) <= firstArg {
		return nil
	}
	return call.Args[firstArg]
}

// buildsString reports whether expr is a string concatenation or a
// fmt.Sprintf call.
func buildsString(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		return e.Op == token.ADD
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Sprintf" {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "fmt"
	}
	return false
}

// repoRoot walks up from the current directory until it finds go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// TestHostileNamesRoundTripAsData proves the other half of the guarantee:
// room, container and item names may contain anything (quotes, semicolons,
// SQL keywords) because values travel as bound parameters. They come back
// verbatim and the schema is untouched.
func TestHostileNamesRoundTripAsData(t *testing.T) {
	db := openDB(t, mustMigrate(t))

	hostile := []string{
		`Garage'); DROP TABLE items; --`,
		`Robert'); DROP TABLE rooms;--`,
		`double " quote, backslash \ and percent %`,
		"newline\nand; semicolon; -- comment",
	}
	for _, name := range hostile {
		res, err := db.Exec(`INSERT INTO rooms (name) VALUES (?)`, name)
		if err != nil {
			t.Fatalf("insert %q: %v", name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("LastInsertId: %v", err)
		}
		var got string
		if err := db.QueryRow(`SELECT name FROM rooms WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("select %q: %v", name, err)
		}
		if got != name {
			t.Fatalf("round-trip mismatch: got %q, want %q", got, name)
		}
	}

	var tables int
	err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('rooms', 'containers', 'items')`,
	).Scan(&tables)
	if err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables != 3 {
		t.Fatalf("schema damaged by hostile names: %d/3 tables present", tables)
	}
}
