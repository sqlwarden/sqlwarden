package assets

import (
	"embed"
)

//go:embed "emails" "migrations_postgres" "migrations_sqlite"
var EmbeddedFiles embed.FS
