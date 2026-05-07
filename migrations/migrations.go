// Package migrations embeds the SQL migration files so they can be executed
// at runtime without requiring them to be present on the filesystem.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
