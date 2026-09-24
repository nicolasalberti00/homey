package inventory

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors returned by the inventory core. Callers match them with
// errors.Is; the REST and MCP layers map them onto their own error models.
var (
	// ErrNotFound reports that the requested entity does not exist.
	ErrNotFound = errors.New("inventory: not found")
	// ErrValidation reports invalid input. Errors matching ErrValidation
	// carry a *ValidationError with field-level details.
	ErrValidation = errors.New("inventory: validation failed")
	// ErrConflict reports that an operation conflicts with the current state,
	// for example a duplicate name.
	ErrConflict = errors.New("inventory: conflict")
	// ErrCycle reports that an operation would create a container cycle.
	ErrCycle = errors.New("inventory: cycle")
	// ErrAmbiguous reports that a name matches more than one entity. Errors
	// matching ErrAmbiguous carry an *AmbiguousError listing the candidates.
	ErrAmbiguous = errors.New("inventory: ambiguous")
)

// AmbiguousError lists the candidates a name could mean, so a caller can ask
// which one was meant. It matches ErrAmbiguous with errors.Is.
type AmbiguousError struct {
	// Name is what the caller asked for, as it was given.
	Name string
	// Candidates are the paths of the entities that match it, in listing
	// order.
	Candidates []string
}

// Error implements the error interface.
func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("inventory: %q matches %d places: %s", e.Name, len(e.Candidates), strings.Join(e.Candidates, "; "))
}

// Is reports that an AmbiguousError matches ErrAmbiguous.
func (e *AmbiguousError) Is(target error) bool {
	return target == ErrAmbiguous
}

// FieldProblem is one field-level validation problem.
type FieldProblem struct {
	// Field is the dot-separated path of the offending field, e.g. "name" or
	// "location.id".
	Field string
	// Problem describes what is wrong, e.g. "must not be empty".
	Problem string
}

// ValidationError collects every field-level problem found while validating
// an entity. It matches ErrValidation with errors.Is.
type ValidationError struct {
	Problems []FieldProblem
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Field + " " + p.Problem
	}
	return "inventory: validation failed: " + strings.Join(parts, "; ")
}

// Is reports that a ValidationError matches ErrValidation.
func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// add appends one problem to the error being built.
func (e *ValidationError) add(field, problem string) {
	e.Problems = append(e.Problems, FieldProblem{Field: field, Problem: problem})
}

// orNil returns nil when no problems were collected, otherwise the error
// itself, so validators can simply return v.orNil().
func (e *ValidationError) orNil() error {
	if len(e.Problems) == 0 {
		return nil
	}
	return e
}

// NewValidationError builds a *ValidationError carrying a single field
// problem. Adapters use it to report rule violations only they can detect,
// such as a container parent that lives in another room.
func NewValidationError(field, problem string) error {
	v := &ValidationError{}
	v.add(field, problem)
	return v
}
