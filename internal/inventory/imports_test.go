package inventory_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCoreHasNoAdapterOrTransportImports keeps the dependency pointing one
// way: adapters (internal/storage) and transports (HTTP, MCP) import the
// core, never the reverse. The core stays testable and portable because it
// defines the ports and does not know how they are implemented.
func TestCoreHasNoAdapterOrTransportImports(t *testing.T) {
	forbidden := map[string]string{
		"github.com/nicolasalberti00/homey/internal/storage": "adapter",
		"database/sql": "database driver interface",
		"net/http":     "transport",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if kind, ok := forbidden[importPath]; ok {
				t.Errorf("%s imports %s (%s): the core must stay independent of adapters and transports", path, importPath, kind)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scanning inventory sources: %v", err)
	}
}
