package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/inventory"
)

// eventTypes collects the types of an event history, in the order they were
// read.
func eventTypes(history []events.Event) []string {
	got := make([]string, len(history))
	for index, event := range history {
		got[index] = event.Type
	}
	return got
}

// deleteAs runs delete_item with the given context: the caller identity travels
// there, so the bypass tests can pretend to be a trusted token.
func (f fixture) deleteAs(t *testing.T, ctx context.Context, input string) (DeleteItemOutput, error) {
	t.Helper()
	tool, found := f.registry.Lookup("delete_item")
	if !found {
		t.Fatal("delete_item is not registered")
	}
	output, err := tool.Call(ctx, json.RawMessage(input))
	if err != nil {
		return DeleteItemOutput{}, err
	}
	var answer DeleteItemOutput
	if err := json.Unmarshal(output, &answer); err != nil {
		t.Fatalf("decoding the answer of delete_item: %v", err)
	}
	return answer, nil
}

// TestDeleteItemAsksBeforeDeleting is the default the step is built around: a
// delete never happens on the first call, it is proposed with a token.
func TestDeleteItemAsksBeforeDeleting(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["bits"]))

	proposal, err := f.deleteAs(t, writerContext(t), `{"id":`+id+`}`)
	if err != nil {
		t.Fatalf("delete_item: %v", err)
	}
	if proposal.Deleted {
		t.Fatal("the item was deleted before anyone confirmed")
	}
	if proposal.Confirmation != "pending" {
		t.Fatalf("confirmation = %q, want pending", proposal.Confirmation)
	}
	if proposal.ConfirmationToken == "" {
		t.Fatal("the proposal came without a confirmation token")
	}
	if proposal.ExpiresInSeconds <= 0 {
		t.Fatalf("expires_in_seconds = %d, want a positive lifetime", proposal.ExpiresInSeconds)
	}
	// The proposal says what would go, so a caller can show it and ask.
	if proposal.Item.Name != "Punte" || proposal.Item.Location != "Garage > Toolbox" {
		t.Fatalf("proposal item = %+v, want the bits in the toolbox", proposal.Item)
	}

	// Nothing was removed.
	var stored ItemView
	f.decode(t, "get_item", `{"id":`+id+`}`, &stored)
	if stored.Name != "Punte" {
		t.Fatalf("stored = %+v, want the item still there", stored)
	}
}

// TestDeleteItemCarriesOutAConfirmedDeletion is the second half of the
// two-step flow: the token from the proposal deletes the item.
func TestDeleteItemCarriesOutAConfirmedDeletion(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["moka"]))

	proposal, err := f.deleteAs(t, writerContext(t), `{"id":`+id+`}`)
	if err != nil {
		t.Fatalf("delete_item: %v", err)
	}
	done, err := f.deleteAs(t, writerContext(t), `{"id":`+id+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`)
	if err != nil {
		t.Fatalf("delete_item with the token: %v", err)
	}
	if !done.Deleted {
		t.Fatal("the confirmed deletion did not happen")
	}
	if done.Confirmation != "confirmed" {
		t.Fatalf("confirmation = %q, want confirmed", done.Confirmation)
	}
	if done.ConfirmationToken != "" {
		t.Fatal("a completed deletion handed back a token")
	}

	// The item is gone from storage, not just hidden.
	tool, _ := f.registry.Lookup("get_item")
	if _, err := tool.Call(t.Context(), json.RawMessage(`{"id":`+id+`}`)); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("reading the deleted item = %v, want ErrNotFound", err)
	}
	// And the deletion is on the audit trail, authorised by confirmation.
	story := f.story(t, t.Context(), events.EntityItem, int64(f.ids["moka"]))
	if got := eventTypes(story); !slices.Contains(got, events.ItemDeleted) {
		t.Fatalf("event types = %v, want the deletion recorded", got)
	}
	last := story[len(story)-1]
	if last.Type != events.ItemDeleted {
		t.Fatalf("last event = %+v, want the deletion", last)
	}
	if last.Tool != "delete_item" || last.Confirmation != "confirmed" {
		t.Fatalf("event = %+v, want a confirmed delete", last)
	}
	if last.Actor != "test" {
		t.Fatalf("event = %+v, want the caller recorded as the actor", last)
	}
}

// TestDeleteItemRefusesAnUnusableConfirmation collects the ways a token must
// not work: wrong action, wrong item, expired. None of them may delete.
func TestDeleteItemRefusesAnUnusableConfirmation(t *testing.T) {
	f := newFixture(t)
	bits := itoa(int64(f.ids["bits"]))

	proposal, err := f.deleteAs(t, writerContext(t), `{"id":`+bits+`}`)
	if err != nil {
		t.Fatalf("delete_item: %v", err)
	}

	// A token nobody issued.
	if _, err := f.deleteAs(t, writerContext(t), `{"id":`+bits+`,"confirmation_token":"made-up"}`); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("an invented token = %v, want ErrConfirmation", err)
	}
	// A token issued for another item.
	moka := itoa(int64(f.ids["moka"]))
	_, err = f.deleteAs(t, writerContext(t), `{"id":`+moka+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`)
	if !errors.Is(err, ErrConfirmation) {
		t.Fatalf("a token for another item = %v, want ErrConfirmation", err)
	}
	if !CallerError(err) {
		t.Fatal("a bad confirmation is not reported as something the caller can act on")
	}
	// A token that has run out of time.
	f.confirmer.now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, err := f.deleteAs(t, writerContext(t), `{"id":`+bits+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("an expired token = %v, want ErrConfirmation", err)
	}

	// Through it all, both items are untouched.
	for _, id := range []string{bits, moka} {
		var stored ItemView
		f.decode(t, "get_item", `{"id":`+id+`}`, &stored)
	}
	// Refused confirmations are not deletions, so they leave no event.
	for _, id := range []inventory.ItemID{f.ids["bits"], f.ids["moka"]} {
		story := f.story(t, t.Context(), events.EntityItem, int64(id))
		if got := eventTypes(story); slices.Contains(got, events.ItemDeleted) {
			t.Fatalf("event types of item %d = %v, want nothing for refused confirmations", id, got)
		}
	}
}

// TestDeleteItemBypassesWithConfirm is the per-call bypass: a caller that has
// already confirmed passes confirm=true and the deletion happens at once.
func TestDeleteItemBypassesWithConfirm(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["bits"]))

	done, err := f.deleteAs(t, writerContext(t), `{"id":`+id+`,"confirm":true}`)
	if err != nil {
		t.Fatalf("delete_item: %v", err)
	}
	if !done.Deleted {
		t.Fatal("confirm=true did not delete")
	}
	if done.Confirmation != "bypassed" {
		t.Fatalf("confirmation = %q, want bypassed", done.Confirmation)
	}
	if done.ConfirmationToken != "" {
		t.Fatal("a bypass handed back a token")
	}

	// And the bypass is on the audit trail, written exactly as it was called.
	story := f.story(t, t.Context(), events.EntityItem, int64(f.ids["bits"]))
	last := story[len(story)-1]
	if last.Type != events.ItemDeleted || last.Confirmation != "bypassed" {
		t.Fatalf("last event = %+v, want a bypassed delete", last)
	}
}

// TestDeleteItemBypassesForATrustedCaller is the per-client bypass: an identity
// the transport marked as trusted deletes without the extra step.
func TestDeleteItemBypassesForATrustedCaller(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["bits"]))

	ctx := WithCaller(t.Context(), Caller{Name: "automation", CanWrite: true, BypassConfirmation: true})
	done, err := f.deleteAs(t, ctx, `{"id":`+id+`}`)
	if err != nil {
		t.Fatalf("delete_item: %v", err)
	}
	if !done.Deleted || done.Confirmation != "bypassed" {
		t.Fatalf("trusted caller got %+v, want a bypassed deletion", done)
	}

	story := f.story(t, t.Context(), events.EntityItem, int64(f.ids["bits"]))
	last := story[len(story)-1]
	if last.Type != events.ItemDeleted || last.Actor != "automation" || last.Confirmation != "bypassed" {
		t.Fatalf("event = %+v, want the trusted caller recorded as bypassed", last)
	}
}

// TestUntrustedCallerStillAsks checks the bypass is not the default: a caller
// that is not trusted goes through the confirmation step like everyone else.
func TestUntrustedCallerStillAsks(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["bits"]))

	ctx := WithCaller(t.Context(), Caller{Name: "assistant", CanWrite: true})
	proposal, err := f.deleteAs(t, ctx, `{"id":`+id+`}`)
	if err != nil {
		t.Fatalf("delete_item: %v", err)
	}
	if proposal.Deleted || proposal.Confirmation != "pending" {
		t.Fatalf("an untrusted caller got %+v, want a pending confirmation", proposal)
	}
	story := f.story(t, t.Context(), events.EntityItem, int64(f.ids["bits"]))
	if got := eventTypes(story); slices.Contains(got, events.ItemDeleted) {
		t.Fatalf("event types = %v, want no deletion for a proposal", got)
	}
}

// TestDeleteItemReportsAMissingItem: asking to delete something that is not
// there is an answer the caller can act on, not a server failure.
func TestDeleteItemReportsAMissingItem(t *testing.T) {
	f := newFixture(t)

	_, err := f.deleteAs(t, writerContext(t), `{"id":4242}`)
	if !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("delete_item on a missing item = %v, want ErrNotFound", err)
	}
	if !CallerError(err) {
		t.Fatal("a missing item is not reported as something the caller can act on")
	}
}

// TestDeleteItemIsADestructiveWrite checks the annotations the confirmation
// flow hangs off: deleting is a destructive write that asks first.
func TestDeleteItemIsADestructiveWrite(t *testing.T) {
	f := newFixture(t)
	tool, found := f.registry.Lookup("delete_item")
	if !found {
		t.Fatal("delete_item is not registered")
	}
	if tool.Permission() != PermissionWrite {
		t.Fatalf("delete_item needs %s, want write", tool.Permission())
	}
	if !tool.Destructive() {
		t.Fatal("delete_item is not marked destructive")
	}
	if !tool.RequiresConfirmation() {
		t.Fatal("delete_item does not ask for confirmation")
	}
}

// quote renders a string as a JSON string for building tool inputs.
func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
