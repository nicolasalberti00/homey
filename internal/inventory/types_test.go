package inventory

import "testing"

func TestLocationConstructorsAndAccessors(t *testing.T) {
	room := RoomLocation(7)
	if !room.IsRoom() || room.IsContainer() {
		t.Fatalf("RoomLocation kind = %v", room.Kind)
	}
	if id, ok := room.RoomID(); !ok || id != 7 {
		t.Fatalf("RoomID() = %d, %t; want 7, true", id, ok)
	}
	if _, ok := room.ContainerID(); ok {
		t.Fatal("ContainerID() should be false for a room location")
	}

	container := ContainerLocation(9)
	if container.IsRoom() || !container.IsContainer() {
		t.Fatalf("ContainerLocation kind = %v", container.Kind)
	}
	if id, ok := container.ContainerID(); !ok || id != 9 {
		t.Fatalf("ContainerID() = %d, %t; want 9, true", id, ok)
	}
	if _, ok := container.RoomID(); ok {
		t.Fatal("RoomID() should be false for a container location")
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	all := []error{ErrNotFound, ErrValidation, ErrConflict, ErrCycle}
	for i, a := range all {
		for j, b := range all {
			if i != j && a == b {
				t.Fatalf("sentinel errors %d and %d are the same value", i, j)
			}
		}
	}
}
