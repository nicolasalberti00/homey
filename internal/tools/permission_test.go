package tools

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestWriteToolsNeedAWriteCaller is the guard Step 6.6 hangs on: a tool that
// changes the inventory runs only for a caller granted the write scope. A
// read-only identity — and a context that never said who is calling — is
// refused before anything is read or written.
func TestWriteToolsNeedAWriteCaller(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["bits"]))
	cases := []struct {
		name  string
		input string
	}{
		{"add_item", `{"name":"Chiave inglese","location":"Garage"}`},
		{"update_item", `{"id":` + id + `,"quantity":2}`},
		{"move_item", `{"id":` + id + `,"destination":"Garage"}`},
		{"delete_item", `{"id":` + id + `}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool, found := f.registry.Lookup(tc.name)
			if !found {
				t.Fatalf("%s is not registered", tc.name)
			}

			// No caller at all: nothing proves this may write.
			if _, err := tool.Call(t.Context(), json.RawMessage(tc.input)); !errors.Is(err, ErrPermission) {
				t.Fatalf("without a caller = %v, want ErrPermission", err)
			}
			// A read-only caller is refused, and the message says why.
			ctx := WithCaller(t.Context(), Caller{Name: "explorer"})
			_, err := tool.Call(ctx, json.RawMessage(tc.input))
			if !errors.Is(err, ErrPermission) {
				t.Fatalf("as a read-only caller = %v, want ErrPermission", err)
			}
			if !CallerError(err) {
				t.Fatal("a refusal is not reported as something the caller can act on")
			}
			if !strings.Contains(err.Error(), tc.name) || !strings.Contains(err.Error(), "explorer") {
				t.Fatalf("error = %q, want the tool and the caller named", err)
			}
		})
	}

	// Through all those refusals, nothing changed: the bits are still there.
	var stored ItemView
	f.decode(t, "get_item", `{"id":`+id+`}`, &stored)
	if stored.Quantity != 12 {
		t.Fatalf("the inventory changed during a refused write: quantity = %d", stored.Quantity)
	}
}

// TestWriteToolsRunForAWriteCaller checks the other side: a caller with the
// write scope is allowed, so the check does not lock everyone out.
func TestWriteToolsRunForAWriteCaller(t *testing.T) {
	f := newFixture(t)
	tool, _ := f.registry.Lookup("add_item")

	ctx := WithCaller(t.Context(), Caller{Name: "editor", CanWrite: true})
	if _, err := tool.Call(ctx, json.RawMessage(`{"name":"Chiave inglese","location":"Garage"}`)); err != nil {
		t.Fatalf("a write caller was refused: %v", err)
	}
}

// TestReadToolsRunForAReadOnlyCaller checks reads are the baseline: they run
// for a read-only identity, and for an in-process caller that said nothing.
func TestReadToolsRunForAReadOnlyCaller(t *testing.T) {
	f := newFixture(t)
	tool, _ := f.registry.Lookup("count_items")

	ctx := WithCaller(t.Context(), Caller{Name: "explorer"})
	if _, err := tool.Call(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("a read-only caller could not read: %v", err)
	}
	if _, err := tool.Call(t.Context(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("a caller that said nothing could not read: %v", err)
	}
}

// TestPermissionCoversEveryTool checks no tool escapes the rule: a tool that
// writes must be marked write, and a tool marked read must not change anything.
func TestPermissionCoversEveryTool(t *testing.T) {
	f := newFixture(t)
	writers := map[string]bool{"add_item": true, "update_item": true, "move_item": true, "delete_item": true}
	readers := map[string]bool{"search_inventory": true, "get_item": true, "list_location": true, "count_items": true}

	for _, tool := range f.registry.List() {
		switch {
		case readers[tool.Name()]:
			if tool.Permission() != PermissionRead {
				t.Fatalf("%s needs %s, want read", tool.Name(), tool.Permission())
			}
		case writers[tool.Name()]:
			if tool.Permission() != PermissionWrite {
				t.Fatalf("%s needs %s, want write", tool.Name(), tool.Permission())
			}
		default:
			t.Fatalf("%s is not classified: every tool must declare read or write", tool.Name())
		}
	}
}
