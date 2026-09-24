package tools

import (
	"errors"
	"fmt"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Clarification is what a tool asks for when a name fits several places and it
// will not guess. The tool answers it instead of acting, so a caller has to
// choose — it can present the candidates and ask the person, then call again
// with a full path. The choice is never taken here.
type Clarification struct {
	// Argument is the tool argument that was ambiguous: "location" or
	// "destination".
	Argument string `json:"argument" jsonschema:"the argument that needs a choice, such as location or destination"`
	// Name is the name the caller gave, which more than one place carries.
	Name string `json:"name" jsonschema:"the name the caller gave, which more than one place carries"`
	// Question is what to ask the person before trying again.
	Question string `json:"question" jsonschema:"the question to ask the person, so the caller can repeat the call with one of the candidates"`
	// Candidates are the full paths that match, in listing order; the caller
	// must choose one.
	Candidates []string `json:"candidates" jsonschema:"the full paths that match, one of which the caller must choose"`
}

// clarify turns the core's ambiguity into a clarification. It returns false
// when err is about something else, so the caller keeps reporting that as an
// error.
func clarify(argument, name string, err error) (*Clarification, bool) {
	var ambiguous *inventory.AmbiguousError
	if !errors.As(err, &ambiguous) {
		return nil, false
	}
	return &Clarification{
		Argument:   argument,
		Name:       ambiguous.Name,
		Question:   fmt.Sprintf("More than one place is called %q. Which one did you mean?", ambiguous.Name),
		Candidates: ambiguous.Candidates,
	}, true
}
