package edition

import (
	"io/fs"
	"strconv"
	"testing"

	"github.com/sqlwarden/assets"
	"github.com/sqlwarden/internal/database"
)

func TestCoreMigrationVersionMatchesEmbeddedStreams(t *testing.T) {
	for _, directory := range []string{"migrations_postgres", "migrations_sqlite"} {
		entries, err := fs.ReadDir(assets.EmbeddedFiles, directory)
		if err != nil {
			t.Fatal(err)
		}
		var highest uint64
		for _, entry := range entries {
			name := entry.Name()
			if len(name) < 6 {
				continue
			}
			version, err := strconv.ParseUint(name[:6], 10, 64)
			if err != nil {
				continue
			}
			if version > highest {
				highest = version
			}
		}
		if highest != uint64(database.CoreMigrationVersion) {
			t.Errorf("%s latest migration = %d, CoreMigrationVersion = %d", directory, highest, database.CoreMigrationVersion)
		}
	}
}
