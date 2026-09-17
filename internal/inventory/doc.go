// Package inventory holds homey's domain logic: rooms, containers (nested),
// items and their validation rules.
//
// Every other layer — REST API (Huma), MCP server, Web UI handlers, future
// voice adapters — must go through this package. Business logic lives only
// here, and this package has no dependency on HTTP, SQL or any transport.
//
// Persistence is expressed as repository ports — see repository.go — and
// implemented by adapters such as internal/storage. imports_test.go fails
// the build if the core ever imports an adapter or a transport.
package inventory
