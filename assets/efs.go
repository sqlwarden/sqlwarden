package assets

import (
	"embed"
)

//go:embed "emails" "migrations_postgres" "migrations_sqlite" "all:static"
var EmbeddedFiles embed.FS
