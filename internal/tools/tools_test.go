package tools

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

// testTool builds a tool whose handler echoes its input, so the tests can
// exercise the registry without an inventory.
type echoInput struct {
	Word string `json:"word" jsonschema:"the word to echo"`
	Size int    `json:"size" jsonschema:"how many times to repeat it"`
}

type echoOutput struct {
	Echo string `json:"echo"`
}

func echoTool(t *testing.T, mutate func(*Definition[echoInput, echoOutput])) Tool {
	t.Helper()
	definition := Definition[echoInput, echoOutput]{
		Name:        "echo",
		Description: "Echo a word a number of times.",
		Permission:  PermissionRead,
		Handler: func(_ context.Context, input echoInput) (echoOutput, error) {
			return echoOutput{Echo: strings.Repeat(input.Word, input.Size)}, nil
		},
	}
	if mutate != nil {
		mutate(&definition)
	}
	tool, err := New(definition)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tool
}

func TestCallValidatesInputAndOutput(t *testing.T) {
	tool := echoTool(t, nil)

	got, err := tool.Call(t.Context(), json.RawMessage(`{"word":"ab","size":2}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(got) != `{"echo":"abab"}` {
		t.Fatalf("Call = %s, want the echoed word", got)
	}

	// A wrong type is refused before the handler runs, with a message that
	// names the tool.
	_, err = tool.Call(t.Context(), json.RawMessage(`{"word":7,"size":2}`))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Call with a bad argument = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "echo") {
		t.Fatalf("error = %v, want the tool name", err)
	}

	// A missing required property is caught by the schema, not by the decoder.
	if _, err := tool.Call(t.Context(), json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Call without a required argument = %v, want ErrInvalidInput", err)
	}

	// Malformed JSON is an input error too.
	if _, err := tool.Call(t.Context(), json.RawMessage(`{`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Call with malformed JSON = %v, want ErrInvalidInput", err)
	}
}

func TestCallRefusesAnOutputThatCannotBeSerialized(t *testing.T) {
	// A handler whose result JSON cannot carry is a bug in the tool: the
	// registry notices instead of handing a broken payload to a caller.
	tool, err := New(Definition[echoInput, floatOutput]{
		Name:        "bad-output",
		Description: "Returns a value JSON cannot represent.",
		Permission:  PermissionRead,
		Handler:     func(context.Context, echoInput) (floatOutput, error) { return floatOutput{Ratio: math.NaN()}, nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := tool.Call(t.Context(), json.RawMessage(`{"word":"a","size":1}`)); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Call = %v, want ErrInvalidOutput", err)
	}
}

// floatOutput carries a value JSON refuses to encode when it is NaN.
type floatOutput struct {
	Ratio float64 `json:"ratio"`
}

func TestNewRefusesIncompleteDefinitions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Definition[echoInput, echoOutput])
	}{
		{"no name", func(d *Definition[echoInput, echoOutput]) { d.Name = "" }},
		{"no description", func(d *Definition[echoInput, echoOutput]) { d.Description = "" }},
		{"no handler", func(d *Definition[echoInput, echoOutput]) { d.Handler = nil }},
		{"unknown permission", func(d *Definition[echoInput, echoOutput]) { d.Permission = "admin" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := Definition[echoInput, echoOutput]{
				Name:        "echo",
				Description: "Echo.",
				Permission:  PermissionRead,
				Handler:     func(context.Context, echoInput) (echoOutput, error) { return echoOutput{}, nil },
			}
			tc.mutate(&definition)
			if _, err := New(definition); err == nil {
				t.Fatal("New accepted an incomplete definition")
			}
		})
	}
}

func TestSchemasComeFromTheGoTypes(t *testing.T) {
	tool := echoTool(t, nil)

	input := tool.InputSchema()
	if input == nil || input.Type != "object" {
		t.Fatalf("input schema = %+v, want an object", input)
	}
	if _, found := input.Properties["word"]; !found {
		t.Fatalf("input schema has no word property: %+v", input.Properties)
	}
	if got := input.Properties["word"].Description; got == "" {
		t.Fatal("the word property has no description: the jsonschema tag was dropped")
	}
	if output := tool.OutputSchema(); output == nil || output.Type != "object" {
		t.Fatalf("output schema = %+v, want an object", output)
	}
}

func TestRegistryRefusesDuplicatesAndKeepsOrder(t *testing.T) {
	first := echoTool(t, nil)
	second := echoTool(t, func(d *Definition[echoInput, echoOutput]) { d.Name = "another" })

	registry, err := NewRegistry(second, first)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	names := make([]string, 0, 2)
	for _, tool := range registry.List() {
		names = append(names, tool.Name())
	}
	if names[0] != "another" || names[1] != "echo" {
		t.Fatalf("List = %v, want the tools ordered by name", names)
	}
	if _, found := registry.Lookup("echo"); !found {
		t.Fatal("Lookup did not find a registered tool")
	}
	if _, found := registry.Lookup("absent"); found {
		t.Fatal("Lookup found a tool that was never registered")
	}

	if _, err := NewRegistry(first, first); err == nil {
		t.Fatal("NewRegistry accepted the same name twice")
	}
	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("NewRegistry accepted a nil tool")
	}
}
