package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRejectsUnknownLevel(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "verbose", "text"); err == nil {
		t.Fatal("expected error for unknown level")
	}
}

func TestNewRejectsUnknownFormat(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "info", "xml"); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, "info", "json")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("hello", "key", "value")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) || !strings.Contains(out, `"key":"value"`) {
		t.Fatalf("unexpected JSON log output: %s", out)
	}
}

func TestLevelFiltersLowerSeverities(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, "warn", "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("should not appear")
	if buf.Len() != 0 {
		t.Fatalf("info should be filtered at warn level, got: %s", buf.String())
	}
	logger.Warn("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Fatalf("warn should be logged, got: %s", buf.String())
	}
}
