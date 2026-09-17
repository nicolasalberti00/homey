// Package migrations embeds the SQL schema migrations so they ship inside the
// homey binary. They are applied by internal/storage with golang-migrate.
package migrations

import "embed"

// FS holds every *.sql migration. File names follow the golang-migrate
// convention: <version>_<name>.up.sql / <version>_<name>.down.sql.
//
//go:embed *.sql
var FS embed.FS
