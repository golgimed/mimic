// Package migrations embeds the SQL migration files into the compiled binary
// so no MIGRATIONS_DIR/on-disk lookup is needed at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
