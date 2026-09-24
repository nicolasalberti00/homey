package tools

import (
	"context"
	"errors"
	"fmt"
)

// ErrPermission reports a caller that may not run a tool: a read-only identity
// trying to change the inventory.
var ErrPermission = errors.New("permission denied")

// authorize checks the caller in ctx against what a tool requires. Reading runs
// for anyone; writing needs a caller that was granted the write scope. A context
// without a caller cannot prove it may write, so writing is refused: an adapter
// that forgets to say who is calling cannot change the inventory by accident.
func authorize(ctx context.Context, tool string, permission Permission) error {
	if permission != PermissionWrite {
		return nil
	}
	caller, found := CallerFrom(ctx)
	if !found {
		return fmt.Errorf("%w: %s changes the inventory, and no authenticated caller was provided", ErrPermission, tool)
	}
	if !caller.CanWrite {
		return fmt.Errorf("%w: %s changes the inventory, and caller %q may only read", ErrPermission, tool, caller.Name)
	}
	return nil
}
