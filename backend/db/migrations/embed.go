// Package migrations embeds the independent Voxis Source-Available schema migrations.
package migrations

import "embed"

// Files contains SQL migrations bundled into the backend binary.
//
//go:embed *.sql
var Files embed.FS
