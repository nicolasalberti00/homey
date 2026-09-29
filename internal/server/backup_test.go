package server

// Step 7.3 from the HTTP surface: a document exported by one instance is
// imported by another, and the two scopes are told apart — reading the backup
// is a read, writing it is a write.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/nicolasalberti00/homey/internal/backup"
)

// TestExportAndImportOverTheAPI walks the backup between two instances: the
// first exports, the second imports, and the second's export says what the
// first's did.
func TestExportAndImportOverTheAPI(t *testing.T) {
	source, bearer := newTestAPI(t)

	if rec := source.Post("/rooms", bearer.Write, RoomInput{Name: "Garage", Description: "dove parcheggio"}); rec.Code != http.StatusCreated {
		t.Fatalf("creating the room: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec := source.Post("/containers", bearer.Write, ContainerInput{RoomID: 1, Name: "Toolbox"}); rec.Code != http.StatusCreated {
		t.Fatalf("creating the container: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec := source.Post("/items", bearer.Write, ItemInput{
		Name:     "Trapano",
		Quantity: 1,
		Tags:     []string{"officina"},
		Location: LocationRef{Kind: "container", ID: 1},
	}); rec.Code != http.StatusCreated {
		t.Fatalf("creating the item: status = %d; body: %s", rec.Code, rec.Body.String())
	}

	rec := source.Get("/export", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	var document backup.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("decoding the export: %v", err)
	}
	if document.Format != backup.Format || len(document.Rooms) != 1 || len(document.Items) != 1 {
		t.Fatalf("export = %+v, want the room and the item", document)
	}

	target, targetBearer := newTestAPI(t)
	rec = target.Post("/import", targetBearer.Write, document)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	var report backup.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding the report: %v", err)
	}
	if report.Rooms.Created != 1 || report.Containers.Created != 1 || report.Items.Created != 1 {
		t.Fatalf("report = %+v, want everything created", report)
	}

	rec = target.Get("/export", targetBearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("export after import: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	var restored backup.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &restored); err != nil {
		t.Fatalf("decoding the export: %v", err)
	}
	document.ExportedAt, restored.ExportedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(document, restored) {
		t.Fatalf("the restored inventory differs:\nexported %+v\nrestored %+v", document, restored)
	}

	// Importing it a second time changes nothing.
	rec = target.Post("/import", targetBearer.Write, document)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding the report: %v", err)
	}
	want := backup.Report{
		Rooms:      backup.Counts{Unchanged: 1},
		Containers: backup.Counts{Unchanged: 1},
		Items:      backup.Counts{Unchanged: 1},
	}
	if !reflect.DeepEqual(report, want) {
		t.Fatalf("second report = %+v, want %+v", report, want)
	}
}

// TestImportRequiresTheWriteScope: reading the whole inventory is a read,
// changing it is a write, and the scopes say so without a special case.
func TestImportRequiresTheWriteScope(t *testing.T) {
	api, bearer := newTestAPI(t)

	rec := api.Get("/export", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("export with a read token: status = %d; body: %s", rec.Code, rec.Body.String())
	}

	rec = api.Post("/import", bearer.Read, backup.Document{Format: backup.Format, Version: backup.Version})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("import with a read token: status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

// TestImportProblemsAreProblems: a document the core refuses comes back as an
// RFC 9457 document pointing at the entry, like every other validation.
func TestImportProblemsAreProblems(t *testing.T) {
	api, bearer := newTestAPI(t)

	rec := api.Post("/import", bearer.Write, backup.Document{Format: "someone.else", Version: backup.Version})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	status, _, detail, locations := decodeProblem(t, rec)
	if status != http.StatusUnprocessableEntity || detail == "" {
		t.Fatalf("problem = %d %q, want the 422 with a detail", status, detail)
	}
	found := false
	for _, location := range locations {
		if location == "body.format" {
			found = true
		}
	}
	if !found {
		t.Fatalf("locations = %v, want body.format", locations)
	}
}

// TestExportOfAnEmptyInventory is the trivial backup: an instance with
// nothing in it still produces a document an import accepts.
func TestExportOfAnEmptyInventory(t *testing.T) {
	api, bearer := newTestAPI(t)

	rec := api.Get("/export", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	var document backup.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if document.Format != backup.Format || len(document.Rooms) != 0 {
		t.Fatalf("export = %+v, want an empty document of the right format", document)
	}
}

// TestImportAcceptsAHandWrittenDocument: a document someone typed by hand —
// no timestamp, no empty arrays — must reach the core instead of being
// turned away by the schema. The core still validates the entries themselves.
func TestImportAcceptsAHandWrittenDocument(t *testing.T) {
	api, bearer := newTestAPI(t)

	rec := rawPost(t, api, "/import", bearer.Write,
		`{"format":"homey.export","version":1,"rooms":[{"name":"Garage"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: status = %d; body: %s", rec.Code, rec.Body.String())
	}
	var report backup.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding the report: %v", err)
	}
	if report.Rooms.Created != 1 {
		t.Fatalf("report = %+v, want the room created", report)
	}

	// The same document, with an entry the core refuses.
	rec = rawPost(t, api, "/import", bearer.Write,
		`{"format":"homey.export","version":1,"rooms":[{"name":"   "}]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an invalid entry: status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	if status, _, _, locations := decodeProblem(t, rec); status != http.StatusUnprocessableEntity {
		t.Fatalf("problem status = %d; locations = %v", status, locations)
	}
}

// rawPost sends a body exactly as written, the way a client that is not
// holding the Go types would.
func rawPost(t *testing.T, api humatest.TestAPI, path, authorizationLine, body string) *httptest.ResponseRecorder {
	t.Helper()
	return api.Do(http.MethodPost, path,
		authorizationLine,
		"Content-Type: application/json",
		strings.NewReader(body))
}
