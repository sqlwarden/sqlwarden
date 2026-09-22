// Package assets embeds Enterprise Edition migration streams.
package assets

import "embed"

// EmbeddedFiles contains only Enterprise-owned migrations. Core binaries do
// not import this package or embed these files.
//
//go:embed migrations_postgres migrations_sqlite
var EmbeddedFiles embed.FS
