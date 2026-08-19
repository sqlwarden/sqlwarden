package files

import (
	"path"
	"strings"

	"github.com/sqlwarden/internal/database"
)

// isTextLikeFile reports whether a file's declared media type, file kind, or
// name extension suggests its content is plausibly readable text. Both
// content-revisioning eligibility and content search eligibility need the
// same answer, so this is the single shared heuristic for it.
func isTextLikeFile(file database.WorkspaceFile) bool {
	if strings.HasPrefix(strings.ToLower(file.MediaType), "text/") {
		return true
	}
	switch strings.ToLower(file.FileKind) {
	case "query", "text", "text_document":
		return true
	}
	switch strings.ToLower(path.Ext(file.Name)) {
	case ".sql", ".txt", ".md", ".json", ".yaml", ".yml", ".toml":
		return true
	}
	return false
}
