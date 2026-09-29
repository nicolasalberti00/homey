package server

// Export and import: the backup surface of Step 7.3. The document is built
// and applied by internal/backup, which owns the format and the rules; this
// layer only carries it, and lets the scope and the error mapping do their
// job — reading the whole inventory is a read, changing it is a write.

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/backup"
)

// RegisterBackup adds the export and import operations.
func RegisterBackup(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "export-inventory",
		Method:      http.MethodGet,
		Path:        "/export",
		Summary:     "Export the whole inventory",
		Description: "Every room, container and item, named rather than numbered: a room by its name, " +
			"a container by its name inside its room, an item by its name at its place. The document " +
			"carries no ids, so the same file restores a fresh instance and merges into a populated one. " +
			"Requires the read scope.",
		Tags: []string{"Backup"},
	}, func(ctx context.Context, input *struct{}) (*ExportOutput, error) {
		document, err := backup.Export(ctx, backupRepos(deps))
		if err != nil {
			return nil, mapError(deps, err)
		}
		return &ExportOutput{Body: document}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "import-inventory",
		Method:      http.MethodPost,
		Path:        "/import",
		Summary:     "Import a document",
		Description: "Applies a document produced by export-inventory. Every entry is matched by name " +
			"within its scope, so importing twice leaves the same inventory, and a document that is " +
			"missing an entity never deletes one. The whole document is validated before anything is " +
			"written, and a problem points at the entry it came from. Requires the write scope.",
		Tags: []string{"Backup"},
	}, func(ctx context.Context, input *ImportInput) (*ImportOutput, error) {
		report, err := backup.Import(ctx, backupRepos(deps), input.Body)
		if err != nil {
			return nil, mapError(deps, err)
		}
		return &ImportOutput{Body: report}, nil
	})
}

// backupRepos narrows the transport's dependencies to what a backup needs.
func backupRepos(deps Deps) backup.Repos {
	return backup.Repos{
		Rooms:      deps.Repos.Rooms,
		Containers: deps.Repos.Containers,
		Items:      deps.Repos.Items,
	}
}

// ExportOutput is the inventory as a document.
type ExportOutput struct {
	Body backup.Document `json:"body"`
}

// ImportInput is the document to apply.
type ImportInput struct {
	Body backup.Document `json:"body"`
}

// ImportOutput says what the import did.
type ImportOutput struct {
	Body backup.Report `json:"body"`
}
