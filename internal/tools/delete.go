package tools

import (
	"context"
	"fmt"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/inventory"
)

// The three ways a delete_item call can end. They are also the value written to
// the audit trail, so the two words mean the same thing wherever they appear.
const (
	confirmationPending   = "pending"
	confirmationConfirmed = "confirmed"
	confirmationBypassed  = "bypassed"
)

// deleteItemTool is the name the confirmation token is bound to.
const deleteItemTool = "delete_item"

// DeleteItemInput is what delete_item takes. By default nothing is removed: the
// call proposes the deletion and answers with a token, and the deletion happens
// when the token comes back. A caller that has already confirmed can say so
// with confirm.
type DeleteItemInput struct {
	ID                inventory.ItemID `json:"id" jsonschema:"the id of the item to delete, as returned by search_inventory"`
	Confirm           bool             `json:"confirm,omitempty" jsonschema:"set true only when the person has already confirmed this exact deletion, to carry it out now without the token step"`
	ConfirmationToken string           `json:"confirmation_token,omitempty" jsonschema:"the token from the earlier proposal; pass it back to carry the deletion out"`
}

// DeleteItemOutput is what delete_item answers: either the deletion happened
// (with how it was authorised) or it is proposed, carrying the token that
// confirms it.
type DeleteItemOutput struct {
	Deleted           bool     `json:"deleted" jsonschema:"true when the item was removed; false when the call proposed a deletion instead"`
	Item              ItemView `json:"item" jsonschema:"the item that was removed, or that confirming would remove"`
	Confirmation      string   `json:"confirmation" jsonschema:"how the deletion stands: pending, confirmed or bypassed"`
	ConfirmationToken string   `json:"confirmation_token,omitempty" jsonschema:"the token to pass back to carry the deletion out; present only while the deletion is pending"`
	ExpiresInSeconds  int      `json:"expires_in_seconds,omitempty" jsonschema:"how long the confirmation token stays valid"`
}

// proposal is a destructive call waiting for an answer: the token that would
// carry it out, and how long it lasts.
type proposal struct {
	token   string
	expires int
}

// authoriseDestructive decides what a destructive call does next, for every
// tool that asks before it acts. It answers with the word to record —
// "bypassed" or "confirmed" — when the caller may act now, and with a
// proposal to answer with when the call must ask first. A token that does not
// fit this exact action on this exact entity comes back as ErrConfirmation:
// one rule, wherever the deletion runs from.
func authoriseDestructive(
	ctx context.Context,
	confirmer *Confirmer,
	tool, noun string,
	entity int64,
	confirm bool,
	token string,
) (string, *proposal, error) {
	caller, _ := CallerFrom(ctx)
	switch {
	case confirm || caller.BypassConfirmation:
		return confirmationBypassed, nil, nil

	case token != "":
		if !confirmer.redeem(tool, token, entity) {
			return "", nil, fmt.Errorf(
				"%w: the token is not valid for deleting %s %d; it may have expired, been used, or belong to another action. Call %s again without a token for a fresh one",
				ErrConfirmation, noun, entity, tool)
		}
		return confirmationConfirmed, nil, nil

	default:
		issued, _, err := confirmer.issue(tool, entity)
		if err != nil {
			return "", nil, fmt.Errorf("issuing a confirmation token: %w", err)
		}
		return confirmationPending, &proposal{token: issued, expires: int(confirmer.ttl.Seconds())}, nil
	}
}

// DeleteItem removes an item for good. It asks first: a call without confirm or
// confirmation_token proposes the deletion and answers with a token, leaving
// the item in place until that token comes back. A caller that has already
// confirmed in the same turn passes confirm=true, and a caller trusted by
// policy skips the step the same way.
func (inv Inventory) DeleteItem(ctx context.Context, input DeleteItemInput) (DeleteItemOutput, error) {
	// Read the item first: the answer says what is going, a missing item is
	// an error the caller can act on, and the path belongs in the proposal.
	item, err := inv.Items.Get(ctx, input.ID)
	if err != nil {
		return DeleteItemOutput{}, err
	}
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return DeleteItemOutput{}, err
	}
	view := itemView(item, index.Path(item.Location))

	mode, pending, err := authoriseDestructive(ctx, inv.confirmer(),
		deleteItemTool, "item", int64(input.ID), input.Confirm, input.ConfirmationToken)
	if err != nil {
		return DeleteItemOutput{}, err
	}
	if pending != nil {
		return DeleteItemOutput{
			Deleted:           false,
			Item:              view,
			Confirmation:      mode,
			ConfirmationToken: pending.token,
			ExpiresInSeconds:  pending.expires,
		}, nil
	}

	// The storage layer writes the event, so the caller and the way the
	// step was authorised travel in the context next to it.
	caller, _ := CallerFrom(ctx)
	if err := inv.Items.Delete(events.WithActor(ctx, events.Actor{
		Name: caller.Name, Tool: deleteItemTool, Confirmation: mode,
	}), input.ID); err != nil {
		return DeleteItemOutput{}, err
	}
	return DeleteItemOutput{Deleted: true, Item: view, Confirmation: mode}, nil
}

// confirmer returns the confirmer to issue and redeem with, falling back to the
// shared one so an Inventory built without one still works.
func (inv Inventory) confirmer() *Confirmer {
	if inv.Confirmations != nil {
		return inv.Confirmations
	}
	return defaultConfirmer
}
