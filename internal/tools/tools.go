// Package tools is the registry of operations homey exposes to an agent: one
// definition per tool, with the JSON schemas inferred from its Go types, the
// permission it needs, and whether it is destructive or must be confirmed.
//
// The registry knows nothing about the protocol that serves it: the MCP
// adapter walks it, and a future voice adapter can call the same definitions.
// Input and output are validated against the schemas here, once, so every
// caller gets the same guarantee and no adapter has to reimplement it.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Permission is what a tool requires of its caller.
type Permission string

const (
	// PermissionRead is for tools that only read the inventory.
	PermissionRead Permission = "read"
	// PermissionWrite is for tools that change it.
	PermissionWrite Permission = "write"
)

// ErrInvalidInput reports arguments that do not match the tool's input schema.
var ErrInvalidInput = errors.New("invalid tool input")

// ErrInvalidOutput reports a result that does not match the tool's output
// schema: a bug in the tool, not in the request.
var ErrInvalidOutput = errors.New("invalid tool output")

// Definition describes one tool. The schemas are deliberately absent: they are
// inferred from In and Out, so they cannot drift from the code that runs.
type Definition[In, Out any] struct {
	// Name is what a caller asks for, in snake_case.
	Name string
	// Description tells an agent when to use the tool. It is the only thing a
	// model sees before deciding, so it names the question the tool answers.
	Description string
	// Permission is what the caller needs to run it.
	Permission Permission
	// Destructive marks a tool whose effect cannot be undone.
	Destructive bool
	// RequiresConfirmation marks a tool that asks before it acts.
	RequiresConfirmation bool
	// Handler is the operation itself: it calls the core, nothing else.
	Handler func(ctx context.Context, input In) (Out, error)
}

// Tool is a registered definition, whatever types it was defined with.
type Tool interface {
	Name() string
	Description() string
	Permission() Permission
	Destructive() bool
	RequiresConfirmation() bool
	InputSchema() *jsonschema.Schema
	OutputSchema() *jsonschema.Schema
	// Call decodes input, checks it against the input schema, runs the
	// handler and checks what comes back. Payloads are JSON so any adapter can
	// carry them over any protocol.
	Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// New turns a definition into a tool, inferring and resolving both schemas
// once, at registration time.
func New[In, Out any](definition Definition[In, Out]) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("tool name is empty")
	}
	if definition.Description == "" {
		return nil, fmt.Errorf("tool %q has no description", definition.Name)
	}
	if definition.Permission != PermissionRead && definition.Permission != PermissionWrite {
		return nil, fmt.Errorf("tool %q has permission %q, want %q or %q",
			definition.Name, definition.Permission, PermissionRead, PermissionWrite)
	}
	if definition.Handler == nil {
		return nil, fmt.Errorf("tool %q has no handler", definition.Name)
	}

	inputSchema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, fmt.Errorf("inferring the input schema of %q: %w", definition.Name, err)
	}
	outputSchema, err := jsonschema.For[Out](nil)
	if err != nil {
		return nil, fmt.Errorf("inferring the output schema of %q: %w", definition.Name, err)
	}
	inputResolved, err := inputSchema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolving the input schema of %q: %w", definition.Name, err)
	}
	outputResolved, err := outputSchema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolving the output schema of %q: %w", definition.Name, err)
	}

	return typedTool[In, Out]{
		definition:     definition,
		inputSchema:    inputSchema,
		outputSchema:   outputSchema,
		inputResolved:  inputResolved,
		outputResolved: outputResolved,
	}, nil
}

// typedTool is the Tool implementation New returns.
type typedTool[In, Out any] struct {
	definition     Definition[In, Out]
	inputSchema    *jsonschema.Schema
	outputSchema   *jsonschema.Schema
	inputResolved  *jsonschema.Resolved
	outputResolved *jsonschema.Resolved
}

func (t typedTool[In, Out]) Name() string           { return t.definition.Name }
func (t typedTool[In, Out]) Description() string    { return t.definition.Description }
func (t typedTool[In, Out]) Permission() Permission { return t.definition.Permission }
func (t typedTool[In, Out]) Destructive() bool      { return t.definition.Destructive }
func (t typedTool[In, Out]) RequiresConfirmation() bool {
	return t.definition.RequiresConfirmation
}
func (t typedTool[In, Out]) InputSchema() *jsonschema.Schema  { return t.inputSchema }
func (t typedTool[In, Out]) OutputSchema() *jsonschema.Schema { return t.outputSchema }

func (t typedTool[In, Out]) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	// The schema validator works on JSON values, not on Go structs, so the
	// arguments are checked in the shape the schema describes and the typed
	// input is decoded from the value that passed.
	value, err := jsonValue(input, t.inputSchema)
	if err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidInput, t.Name(), err)
	}
	if err := t.inputResolved.Validate(value); err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidInput, t.Name(), err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidInput, t.Name(), err)
	}
	var arguments In
	if err := json.Unmarshal(encoded, &arguments); err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidInput, t.Name(), err)
	}

	result, err := t.definition.Handler(ctx, arguments)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidOutput, t.Name(), err)
	}
	returned, err := jsonValue(payload, t.outputSchema)
	if err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidOutput, t.Name(), err)
	}
	if err := t.outputResolved.Validate(returned); err != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrInvalidOutput, t.Name(), err)
	}
	return payload, nil
}

// jsonValue decodes a payload into the plain value a schema validates. An
// empty payload stands for the empty object when the schema describes one, so
// a tool that takes no arguments is called with none.
func jsonValue(payload json.RawMessage, schema *jsonschema.Schema) (any, error) {
	if len(payload) == 0 {
		if schema.Type == "object" {
			return map[string]any{}, nil
		}
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	if value == nil && schema.Type == "object" {
		return map[string]any{}, nil
	}
	return value, nil
}

// CallerError reports whether err is a failure the caller of a tool can act
// on: input that does not fit the schema, an entity that does not exist, a
// name that matches several places, a write that clashes with what is already
// stored, a confirmation that is missing or no longer valid. A transport hands
// these back to whoever made the call instead of reporting a failure of the
// server.
func CallerError(err error) bool {
	return errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrConfirmation) ||
		errors.Is(err, inventory.ErrValidation) ||
		errors.Is(err, inventory.ErrNotFound) ||
		errors.Is(err, inventory.ErrAmbiguous) ||
		errors.Is(err, inventory.ErrConflict)
}

// Registry is the set of tools an adapter serves.
type Registry struct {
	tools  []Tool
	byName map[string]Tool
}

// NewRegistry collects tools, refusing a name registered twice: the lookup an
// adapter does must have one answer.
func NewRegistry(list ...Tool) (*Registry, error) {
	registry := &Registry{byName: make(map[string]Tool, len(list))}
	for _, tool := range list {
		if tool == nil {
			return nil, errors.New("a nil tool was registered")
		}
		name := tool.Name()
		if _, exists := registry.byName[name]; exists {
			return nil, fmt.Errorf("tool %q is registered twice", name)
		}
		registry.byName[name] = tool
		registry.tools = append(registry.tools, tool)
	}
	sort.Slice(registry.tools, func(i, j int) bool { return registry.tools[i].Name() < registry.tools[j].Name() })
	return registry, nil
}

// List returns the tools, ordered by name.
func (r *Registry) List() []Tool { return slices.Clone(r.tools) }

// Lookup finds a tool by name.
func (r *Registry) Lookup(name string) (Tool, bool) {
	tool, found := r.byName[name]
	return tool, found
}
