package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func containerPathID(id int64) string {
	return "/containers/" + strconv.FormatInt(id, 10)
}

func TestContainerLifecycle(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")

	// Create a root container.
	rec := api.Post("/containers", bearer.Write, ContainerInput{Name: "  Toolbox  ", Description: "red", RoomID: room.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var toolbox ContainerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &toolbox); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if toolbox.Name != "Toolbox" || toolbox.RoomID != room.ID || toolbox.ParentID != nil {
		t.Fatalf("created = %+v", toolbox)
	}

	// Create a nested container.
	parent := toolbox.ID
	rec = api.Post("/containers", bearer.Write, ContainerInput{Name: "Drawer 1", RoomID: room.ID, ParentID: &parent})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var drawer ContainerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &drawer); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// List by room returns both, including nested ones.
	rec = api.Get("/containers?room_id="+strconv.FormatInt(room.ID, 10), bearer.Write)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var list []ContainerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %+v, want 2 containers", list)
	}

	// Detail carries the computed path.
	rec = api.Get(containerPathID(drawer.ID), bearer.Write)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var detail ContainerDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decoding detail: %v", err)
	}
	if detail.Path != "Garage > Toolbox > Drawer 1" {
		t.Fatalf("path = %q", detail.Path)
	}

	// Patch the name.
	name := "Drawer 2"
	rec = api.Patch(containerPathID(drawer.ID), bearer.Write, ContainerPatch{Name: &name})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Delete a leaf.
	rec = api.Delete(containerPathID(drawer.ID), bearer.Write)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
}

func TestContainerErrors(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	kitchen := createRoomViaAPI(t, api, bearer.Write, "Kitchen")

	rec := api.Post("/containers", bearer.Write, ContainerInput{Name: "Toolbox", RoomID: room.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	// Duplicate name within the same room: 409.
	rec = api.Post("/containers", bearer.Write, ContainerInput{Name: "toolbox", RoomID: room.ID})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}

	// Unknown room: 404.
	rec = api.Post("/containers", bearer.Write, ContainerInput{Name: "Box", RoomID: 4242})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// Parent in another room: 422 from the core.
	cupboard := createContainerViaAPI(t, api, bearer.Write, kitchen.ID, nil, "Cupboard")
	rec = api.Post("/containers", bearer.Write, ContainerInput{Name: "Box", RoomID: room.ID, ParentID: &cupboard.ID})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	_, _, _, locations := decodeProblem(t, rec)
	if len(locations) == 0 || locations[0] != "body.parent_id" {
		t.Fatalf("locations = %v, want body.parent_id", locations)
	}

	// Unknown container: 404 on every operation.
	rec = api.Get(containerPathID(4242), bearer.Write)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	name := "Box"
	rec = api.Patch(containerPathID(4242), bearer.Write, ContainerPatch{Name: &name})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	rec = api.Delete(containerPathID(4242), bearer.Write)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	rec = api.Post(containerPathID(4242)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: room.ID}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// Listing the containers of an unknown room: 404.
	rec = api.Get("/containers?room_id=4242", bearer.Write)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestContainerMoveViaAPI(t *testing.T) {
	api, bearer := newTestAPI(t)

	garage := createRoomViaAPI(t, api, bearer.Write, "Garage")
	kitchen := createRoomViaAPI(t, api, bearer.Write, "Kitchen")
	toolbox := createContainerViaAPI(t, api, bearer.Write, garage.ID, nil, "Toolbox")
	drawer := createContainerViaAPI(t, api, bearer.Write, garage.ID, &toolbox.ID, "Drawer")

	// Moving the subtree to another room: every descendant follows.
	rec := api.Post(containerPathID(toolbox.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: kitchen.ID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var moved ContainerDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &moved); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if moved.Path != "Kitchen > Toolbox" {
		t.Fatalf("path after move = %q", moved.Path)
	}

	rec = api.Get(containerPathID(drawer.ID), bearer.Write)
	var gotDrawer ContainerDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &gotDrawer); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if gotDrawer.RoomID != kitchen.ID {
		t.Fatalf("drawer room after move = %d, want %d", gotDrawer.RoomID, kitchen.ID)
	}

	// Moving under a descendant is a cycle: 409.
	rec = api.Post(containerPathID(toolbox.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "container", ID: drawer.ID}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	status, title, detail, _ := decodeProblem(t, rec)
	if status != http.StatusConflict || title != "Conflict" || detail == "" {
		t.Fatalf("problem = %d/%q/%q", status, title, detail)
	}

	// Unknown destination: 404.
	rec = api.Post(containerPathID(toolbox.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: 4242}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// Name collision in the destination room: 409.
	box := createContainerViaAPI(t, api, bearer.Write, garage.ID, nil, "Box")
	createContainerViaAPI(t, api, bearer.Write, kitchen.ID, nil, "Box")
	rec = api.Post(containerPathID(box.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: kitchen.ID}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
}

// createRoomViaAPI is a test helper creating a room through the API.
func createRoomViaAPI(t *testing.T, api humatest.TestAPI, bearer string, name string) RoomResponse {
	t.Helper()
	rec := api.Post("/rooms", bearer, RoomInput{Name: name})
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating room %q: status = %d, body: %s", name, rec.Code, rec.Body.String())
	}
	var room RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &room); err != nil {
		t.Fatalf("decoding room: %v", err)
	}
	return room
}

func createContainerViaAPI(t *testing.T, api humatest.TestAPI, bearer string, roomID int64, parent *int64, name string) ContainerResponse {
	t.Helper()
	rec := api.Post("/containers", bearer, ContainerInput{Name: name, RoomID: roomID, ParentID: parent})
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating container %q: status = %d, body: %s", name, rec.Code, rec.Body.String())
	}
	var container ContainerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &container); err != nil {
		t.Fatalf("decoding container: %v", err)
	}
	return container
}
