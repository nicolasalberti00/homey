package inventory

import (
	"errors"
	"strings"
	"testing"
)

func validRoom() Room {
	return Room{ID: 1, Name: "Garage", Description: "Cars and tools"}
}

func TestRoomValidateAcceptsValidRoom(t *testing.T) {
	r := validRoom()
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestRoomValidateRejectsEmptyName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		r := validRoom()
		r.Name = name
		err := r.Validate()
		if err == nil {
			t.Fatalf("name %q should be rejected", name)
		}
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("error should match ErrValidation, got %v", err)
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("error should be a *ValidationError, got %T", err)
		}
		if ve.Problems[0].Field != "name" {
			t.Fatalf("problem field = %q, want name", ve.Problems[0].Field)
		}
	}
}

func TestRoomValidateRejectsLongName(t *testing.T) {
	r := validRoom()
	r.Name = strings.Repeat("a", MaxNameLen+1)
	assertProblem(t, r.Validate(), "name")
}

func TestRoomValidateAcceptsNameAtLimit(t *testing.T) {
	r := validRoom()
	r.Name = strings.Repeat("a", MaxNameLen)
	if err := r.Validate(); err != nil {
		t.Fatalf("name at the limit should be accepted: %v", err)
	}
}

func TestValidateCollectsMultipleProblems(t *testing.T) {
	r := validRoom()
	r.Name = "  "
	r.Description = strings.Repeat("d", MaxDescriptionLen+1)
	err := r.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error should be a *ValidationError, got %v", err)
	}
	if len(ve.Problems) != 2 {
		t.Fatalf("problems = %d, want 2 (%v)", len(ve.Problems), ve.Problems)
	}
}

func TestNormalizeTrimsTextFields(t *testing.T) {
	r := Room{Name: "  Garage  ", Description: "  cars and tools  "}
	r.Normalize()
	if r.Name != "Garage" || r.Description != "cars and tools" {
		t.Fatalf("Normalize = %q/%q", r.Name, r.Description)
	}

	i := Item{Name: " drill ", Description: " cordless ", Notes: " warranty 2y "}
	i.Normalize()
	if i.Name != "drill" || i.Description != "cordless" || i.Notes != "warranty 2y" {
		t.Fatalf("Normalize = %q/%q/%q", i.Name, i.Description, i.Notes)
	}
}

func TestContainerValidate(t *testing.T) {
	t.Run("valid root container", func(t *testing.T) {
		c := Container{ID: 1, RoomID: 1, Name: "Toolbox"}
		if err := c.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("valid nested container", func(t *testing.T) {
		parent := ContainerID(1)
		c := Container{ID: 2, RoomID: 1, ParentID: &parent, Name: "Drawer 1"}
		if err := c.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("missing room", func(t *testing.T) {
		c := Container{Name: "Toolbox"}
		assertProblem(t, c.Validate(), "room_id")
	})

	t.Run("self parent", func(t *testing.T) {
		id := ContainerID(1)
		c := Container{ID: 1, RoomID: 1, Name: "Toolbox", ParentID: &id}
		assertProblem(t, c.Validate(), "parent_id")
	})

	t.Run("invalid parent id", func(t *testing.T) {
		bad := ContainerID(-1)
		c := Container{ID: 1, RoomID: 1, Name: "Toolbox", ParentID: &bad}
		assertProblem(t, c.Validate(), "parent_id")
	})
}

func TestItemValidate(t *testing.T) {
	t.Run("valid item in room", func(t *testing.T) {
		i := Item{ID: 1, Name: "Drill", Quantity: 1, Location: RoomLocation(1)}
		if err := i.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("valid item in container", func(t *testing.T) {
		i := Item{ID: 1, Name: "Screwdriver", Quantity: 3, Location: ContainerLocation(5)}
		if err := i.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("zero quantity is allowed", func(t *testing.T) {
		i := Item{Name: "Cups", Quantity: 0, Location: RoomLocation(1)}
		if err := i.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("negative quantity", func(t *testing.T) {
		i := Item{Name: "Drill", Quantity: -1, Location: RoomLocation(1)}
		assertProblem(t, i.Validate(), "quantity")
	})

	t.Run("quantity above maximum", func(t *testing.T) {
		i := Item{Name: "Screws", Quantity: MaxQuantity + 1, Location: ContainerLocation(1)}
		assertProblem(t, i.Validate(), "quantity")
	})

	t.Run("missing location kind", func(t *testing.T) {
		i := Item{Name: "Drill", Quantity: 1}
		assertProblem(t, i.Validate(), "location.kind")
	})

	t.Run("missing location id", func(t *testing.T) {
		i := Item{Name: "Drill", Quantity: 1, Location: Location{Kind: LocationRoom}}
		assertProblem(t, i.Validate(), "location.id")
	})

	t.Run("long notes", func(t *testing.T) {
		i := Item{Name: "Drill", Quantity: 1, Location: RoomLocation(1), Notes: strings.Repeat("n", MaxNotesLen+1)}
		assertProblem(t, i.Validate(), "notes")
	})
}

func TestLocationValidate(t *testing.T) {
	if err := RoomLocation(3).Validate(); err != nil {
		t.Fatalf("room location should be valid: %v", err)
	}
	if err := ContainerLocation(3).Validate(); err != nil {
		t.Fatalf("container location should be valid: %v", err)
	}
	assertProblem(t, (Location{}).Validate(), "location.kind")
	assertProblem(t, (Location{Kind: LocationRoom, ID: 0}).Validate(), "location.id")
}

func TestValidationErrorMessage(t *testing.T) {
	err := (&ValidationError{Problems: []FieldProblem{{Field: "name", Problem: "must not be empty"}}}).Error()
	if !strings.Contains(err, "name") || !strings.Contains(err, "must not be empty") {
		t.Fatalf("unexpected message: %q", err)
	}
}

// assertProblem checks that err matches ErrValidation and reports a problem
// for the given field.
func assertProblem(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a validation error for %q", field)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error should match ErrValidation, got %v", err)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error should be a *ValidationError, got %T", err)
	}
	for _, p := range ve.Problems {
		if p.Field == field {
			return
		}
	}
	t.Fatalf("no problem for field %q in %v", field, ve.Problems)
}
