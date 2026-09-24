package server

// Step 6.8 — MCP integration test in-process: a real client over the SDK's
// in-memory transport, against a real registry over a real database. No HTTP
// and no auth sit between the caller and the tools, so this is where the
// deterministic scenarios of the spec §17 live, together with the permission
// and schema checks.

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicolasalberti00/homey/internal/inventory"
	"github.com/nicolasalberti00/homey/internal/storage"
	"github.com/nicolasalberti00/homey/internal/tools"
)

// inventorySession is an MCP client connected to a server that serves the
// inventory tools of one temporary database: the smallest real MCP stack there
// is, without an HTTP hop.
type inventorySession struct {
	session *mcp.ClientSession
	repos   storage.Repos
	ids     map[string]inventory.ItemID
}

// newInventorySession builds the in-process session. The caller decides who the
// tools think is calling: write-capable for the scenarios, read-only or absent
// for the permission tests.
func newInventorySession(t *testing.T, caller *tools.Caller) *inventorySession {
	t.Helper()
	dbPath := t.TempDir() + "/homey.db"
	if err := storage.MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repos := storage.NewRepos(db)
	ctx := t.Context()
	if caller != nil {
		ctx = tools.WithCaller(ctx, *caller)
	}

	garage := inventory.Room{Name: "Garage"}
	kitchen := inventory.Room{Name: "Cucina"}
	for _, room := range []*inventory.Room{&garage, &kitchen} {
		if err := repos.Rooms.Create(ctx, room); err != nil {
			t.Fatalf("creating room: %v", err)
		}
	}
	garageToolbox := inventory.Container{RoomID: garage.ID, Name: "Toolbox"}
	if err := repos.Containers.Create(ctx, &garageToolbox); err != nil {
		t.Fatalf("creating the garage toolbox: %v", err)
	}
	drawer := inventory.Container{RoomID: garage.ID, ParentID: &garageToolbox.ID, Name: "Cassetto 1"}
	if err := repos.Containers.Create(ctx, &drawer); err != nil {
		t.Fatalf("creating the drawer: %v", err)
	}
	// A second toolbox, as the spec §11 ambiguity example asks for.
	kitchenToolbox := inventory.Container{RoomID: kitchen.ID, Name: "Toolbox"}
	if err := repos.Containers.Create(ctx, &kitchenToolbox); err != nil {
		t.Fatalf("creating the kitchen toolbox: %v", err)
	}

	items := []struct {
		slug string
		item inventory.Item
	}{
		{"bosch drill", inventory.Item{Name: "Bosch drill", Quantity: 1, Location: inventory.RoomLocation(garage.ID)}},
		{"makita drill", inventory.Item{Name: "Makita drill", Quantity: 1, Location: inventory.RoomLocation(kitchen.ID)}},
		{"cordless drill", inventory.Item{Name: "Cordless drill", Quantity: 1, Location: inventory.RoomLocation(kitchen.ID)}},
		{"broken cable", inventory.Item{
			Name: "Broken HDMI cable", Quantity: 1,
			Location: inventory.ContainerLocation(drawer.ID),
		}},
	}
	ids := make(map[string]inventory.ItemID, len(items))
	for _, entry := range items {
		item := entry.item
		if err := repos.Items.Create(ctx, &item); err != nil {
			t.Fatalf("creating %s: %v", entry.slug, err)
		}
		ids[entry.slug] = item.ID
	}

	list, err := (tools.Inventory{
		Rooms:      repos.Rooms,
		Containers: repos.Containers,
		Items:      repos.Items,
	}).Tools()
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	registry, err := tools.NewRegistry(list...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(nil, registry).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting the server: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "homey-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting the client: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	return &inventorySession{session: session, repos: repos, ids: ids}
}

// call runs one tool and returns its structured answer.
func (s *inventorySession) call(t *testing.T, name string, arguments map[string]any) map[string]any {
	t.Helper()
	result, err := s.session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %s", name, textOf(result))
	}
	content, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("%s structured content = %#v, want an object", name, result.StructuredContent)
	}
	return content
}

// callFailed runs a tool that is expected to be refused, and returns what a
// model would read.
func (s *inventorySession) callFailed(t *testing.T, name string, arguments map[string]any) string {
	t.Helper()
	result, err := s.session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	if !result.IsError {
		t.Fatalf("%s was accepted, want a refusal", name)
	}
	return textOf(result)
}

// callRead runs a tool that is expected to be refused by the core itself, so
// the result is an error result carrying the inventory's answer.
func (s *inventorySession) callRead(t *testing.T, name string, arguments map[string]any) string {
	t.Helper()
	result, err := s.session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	return textOf(result)
}

// textOf is the message a result carries, which is what a model reads.
func textOf(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if typed, ok := content.(*mcp.TextContent); ok {
			text.WriteString(typed.Text)
		}
	}
	return text.String()
}

// TestWhereIsTheDrill is the spec §17 case: a question about a place is
// answered by search_inventory, and a name shared by several drills comes back
// as candidates, one of which the caller must pick.
func TestWhereIsTheDrill(t *testing.T) {
	s := newInventorySession(t, &tools.Caller{Name: "test", CanWrite: true})

	answer := s.call(t, "search_inventory", map[string]any{"query": "drill"})
	raw, _ := answer["results"].([]any)
	if len(raw) != 3 {
		t.Fatalf("results = %#v, want three drills", answer["results"])
	}
	locations := make([]string, len(raw))
	for index, entry := range raw {
		candidate, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want an object", index, entry)
		}
		match, _ := candidate["match"].(map[string]any)
		if match["field"] != "name" {
			t.Fatalf("match = %#v, want the name field", match)
		}
		locations[index], _ = candidate["location"].(string)
	}
	slices.Sort(locations)
	if !slices.Equal(locations, []string{"Cucina", "Cucina", "Garage"}) {
		t.Fatalf("locations = %v, want the two drills in the kitchen and one in the garage", locations)
	}
}

// TestMoveTheDrillToTheGarage is the spec §17 move case: the model finds the
// drill, then names the destination the way the spec does. On an ambiguous
// destination the tool asks instead of guessing.
func TestMoveTheDrillToTheGarage(t *testing.T) {
	s := newInventorySession(t, &tools.Caller{Name: "test", CanWrite: true})

	// "Which drill?" is answered by the candidates' ids.
	boschID := s.call(t, "search_inventory", map[string]any{"query": "bosch"})
	raw, _ := boschID["results"].([]any)
	if len(raw) != 1 {
		t.Fatalf("searching \"bosch\" = %#v, want only the Bosch drill", boschID["results"])
	}
	first, _ := raw[0].(map[string]any)
	id, _ := first["id"].(float64)

	// The destination "Toolbox" names two places: ask, do not move.
	ambiguous := s.call(t, "move_item", map[string]any{"id": id, "destination": "Toolbox"})
	clarification, ok := ambiguous["clarification"].(map[string]any)
	if !ok {
		t.Fatalf("clarification = %#v, want the candidates", ambiguous["clarification"])
	}
	if clarification["name"] != "Toolbox" {
		t.Fatalf("clarification name = %#v, want Toolbox", clarification["name"])
	}
	raw, _ = clarification["candidates"].([]any)
	if len(raw) != 2 {
		t.Fatalf("candidates = %#v, want both toolboxes", clarification["candidates"])
	}
	if _, hasItem := ambiguous["item"]; hasItem {
		t.Fatalf("move_item acted on an ambiguous destination: %#v", ambiguous)
	}
	// The drill is still where it was.
	still := s.call(t, "get_item", map[string]any{"id": id})
	if still["location"] != "Garage" {
		t.Fatalf("location = %#v, want the drill unmoved", still["location"])
	}

	// The full path carries the move through.
	moved := s.call(t, "move_item", map[string]any{"id": id, "destination": "Garage > Toolbox"})
	item, ok := moved["item"].(map[string]any)
	if !ok || item["location"] != "Garage > Toolbox" {
		t.Fatalf("moved to %#v, want the toolbox", moved["item"])
	}
}

// TestDeleteTheBrokenCable is the spec §17 delete case: the deletion is
// proposed first, and only a confirmed second call removes the item.
func TestDeleteTheBrokenCable(t *testing.T) {
	s := newInventorySession(t, &tools.Caller{Name: "test", CanWrite: true})

	found := s.call(t, "search_inventory", map[string]any{"query": "cable"})
	raw, _ := found["results"].([]any)
	if len(raw) != 1 {
		t.Fatalf("searching \"cable\" = %#v, want the broken HDMI cable", found["results"])
	}
	first, _ := raw[0].(map[string]any)
	id, _ := first["id"].(float64)

	proposal := s.call(t, "delete_item", map[string]any{"id": id})
	if proposal["deleted"] == true {
		t.Fatal("delete_item removed the item before it was confirmed")
	}
	if proposal["confirmation"] != "pending" {
		t.Fatalf("confirmation = %#v, want pending", proposal["confirmation"])
	}
	token, _ := proposal["confirmation_token"].(string)
	if token == "" {
		t.Fatal("the proposal came without a confirmation token")
	}
	// The cable is still in the drawer.
	whole := s.call(t, "get_item", map[string]any{"id": id})
	if whole["name"] != "Broken HDMI cable" {
		t.Fatalf("item = %#v, want the cable still there", whole)
	}

	done := s.call(t, "delete_item", map[string]any{"id": id, "confirmation_token": token})
	if done["deleted"] != true || done["confirmation"] != "confirmed" {
		t.Fatalf("confirmed deletion = %#v, want deleted", done)
	}
	// The cable is gone: reading it is an error the model reads.
	answer := s.callRead(t, "get_item", map[string]any{"id": id})
	if !strings.Contains(answer, "not found") {
		t.Fatalf("reading the deleted item = %q, want not found", answer)
	}
}

// TestMCPInProcessRefusesWritesWithoutPermission checks the permission rule
// holds without HTTP: a read-only caller can explore, but nothing it can change.
func TestMCPInProcessRefusesWritesWithoutPermission(t *testing.T) {
	reader := newInventorySession(t, &tools.Caller{Name: "explorer"})

	answer := reader.call(t, "count_items", nil)
	if answer["count"] != float64(4) {
		t.Fatalf("count = %#v, want the four items", answer["count"])
	}

	message := reader.callFailed(t, "add_item", map[string]any{"name": "Martello", "location": "Garage"})
	if !strings.Contains(message, "permission") {
		t.Fatalf("add_item = %q, want a permission error", message)
	}
	reader.callFailed(t, "delete_item", map[string]any{"id": float64(reader.ids["broken cable"]), "confirm": true})

	// Nothing changed.
	answer = reader.call(t, "count_items", nil)
	if answer["count"] != float64(4) {
		t.Fatalf("count = %#v, want the inventory untouched", answer["count"])
	}
}

// TestMCPInProcessRefusesWritesWithoutACaller checks the stricter half of the
// permission rule: an adapter that never says who is calling cannot write.
func TestMCPInProcessRefusesWritesWithoutACaller(t *testing.T) {
	s := newInventorySession(t, nil)

	message := s.callFailed(t, "add_item", map[string]any{"name": "Martello", "location": "Garage"})
	if !strings.Contains(message, "no authenticated caller") {
		t.Fatalf("add_item = %q, want the missing-caller message", message)
	}
}

// TestMCPInProcessDescribesEveryTool is the schema part of Step 6.8: what a
// client lists is the contract, so the tool names, the schemas and the hints a
// host reasons on are all pinned down here.
func TestMCPInProcessDescribesEveryTool(t *testing.T) {
	s := newInventorySession(t, &tools.Caller{Name: "test", CanWrite: true})

	list, err := s.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}
	want := []string{
		"add_item", "count_items", "delete_item", "get_item",
		"list_location", "move_item", "search_inventory", "update_item",
	}
	got := make([]string, 0, len(list.Tools))
	byName := make(map[string]*mcp.Tool, len(list.Tools))
	for _, tool := range list.Tools {
		got = append(got, tool.Name)
		byName[tool.Name] = tool
		input, output := schemaOf(t, tool.InputSchema), schemaOf(t, tool.OutputSchema)
		if input.Type != "object" || output.Type != "object" {
			t.Fatalf("%s schema types = %s/%s, want objects", tool.Name, input.Type, output.Type)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tools = %v, want %v", got, want)
	}

	deleteItem := byName["delete_item"]
	deleteInput := schemaOf(t, deleteItem.InputSchema)
	for _, property := range []string{"confirm", "confirmation_token", "id"} {
		if _, has := deleteInput.Properties[property]; !has {
			t.Fatalf("delete_item input is missing %q: %v", property, deleteInput.Properties)
		}
	}
	deleteOutput := schemaOf(t, deleteItem.OutputSchema)
	for _, property := range []string{"deleted", "item", "confirmation", "confirmation_token", "expires_in_seconds"} {
		if _, has := deleteOutput.Properties[property]; !has {
			t.Fatalf("delete_item output is missing %q: %v", property, deleteOutput.Properties)
		}
	}
	// The clarification is on the tools that take a place, not on delete_item,
	// which answers with how the deletion stands instead.
	moveItem := byName["move_item"]
	if _, has := schemaOf(t, moveItem.OutputSchema).Properties["clarification"]; !has {
		t.Fatal("move_item output does not advertise a clarification")
	}
	if _, has := schemaOf(t, byName["search_inventory"].InputSchema).Properties["query"]; !has {
		t.Fatal("search_inventory takes no query")
	}

	// The hints a host uses to decide whether a tool may run unattended.
	for _, tool := range list.Tools {
		if tool.Annotations == nil {
			t.Fatalf("%s came without annotations", tool.Name)
		}
		reads := slices.Contains([]string{"search_inventory", "get_item", "list_location", "count_items"}, tool.Name)
		if tool.Annotations.ReadOnlyHint != reads {
			t.Fatalf("%s readOnlyHint = %t, want %t", tool.Name, tool.Annotations.ReadOnlyHint, reads)
		}
	}
	if deleteItem.Annotations.DestructiveHint == nil || !*deleteItem.Annotations.DestructiveHint {
		t.Fatal("delete_item is not marked destructive")
	}
}

// schemaOf decodes the raw schema a tool advertises into the type the registry
// infers it with, so the test reads properties instead of guessing.
func schemaOf(t *testing.T, raw any) *jsonschema.Schema {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("encoding a tool schema: %v", err)
	}
	schema := &jsonschema.Schema{}
	if err := json.Unmarshal(encoded, schema); err != nil {
		t.Fatalf("decoding a tool schema: %v", err)
	}
	return schema
}
