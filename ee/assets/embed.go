//go:build enterprise

package assets

import "embed"

//go:embed migrations_postgres migrations_sqlite
var EmbeddedFiles embed.FS
