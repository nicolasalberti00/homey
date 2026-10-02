package webui

// Output escaping, from the security review: the built UI
// is what ships, but the sink that would make stored XSS possible lives in the
// sources, so the sources are what gets checked. Svelte escapes everything it
// renders; the only ways out of that are the raw-HTML directive and the DOM
// APIs that take HTML strings. Item names, tags and aliases are user data, so
// neither may appear.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rawHTMLInSvelte is the directive that renders a string as markup.
const rawHTMLInSvelte = "{@html"

// htmlSinks are the JavaScript entry points that interpret their argument as
// HTML rather than as text.
var htmlSinks = []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "dangerouslySetInnerHTML"}

func TestUISourceHasNoRawHTMLInjection(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("finding repository root: %v", err)
	}

	var violations []string
	err = filepath.WalkDir(filepath.Join(root, "web", "src"),
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			source := string(content)
			switch {
			case strings.HasSuffix(path, ".svelte"):
				if strings.Contains(source, rawHTMLInSvelte) {
					violations = append(violations, fmt.Sprintf(
						"%s: renders markup with %s; render the value as text instead",
						rel, rawHTMLInSvelte))
				}
			case strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".js"):
				for _, sink := range htmlSinks {
					if strings.Contains(source, sink) {
						violations = append(violations, fmt.Sprintf(
							"%s: writes HTML with %s; use a text node instead", rel, sink))
					}
				}
			}
			return nil
		})
	if err != nil {
		t.Fatalf("scanning the UI sources: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("the UI must never render data as markup:\n  %s", strings.Join(violations, "\n  "))
	}
}

// repoRoot walks up from the test's package directory until it finds go.mod.
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
