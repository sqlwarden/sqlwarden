package files

import (
	"testing"

	"github.com/sqlwarden/internal/database"
)

func TestIsTextLikeFile(t *testing.T) {
	tests := []struct {
		name string
		file database.WorkspaceFile
		want bool
	}{
		{"text media type", database.WorkspaceFile{MediaType: "text/plain", Name: "notes"}, true},
		{"query file kind", database.WorkspaceFile{FileKind: "query", Name: "untitled"}, true},
		{"text_document file kind", database.WorkspaceFile{FileKind: "text_document", Name: "untitled"}, true},
		{"sql extension", database.WorkspaceFile{Name: "report.sql"}, true},
		{"yaml extension", database.WorkspaceFile{Name: "config.yaml"}, true},
		{"toml extension", database.WorkspaceFile{Name: "settings.toml"}, true},
		{"binary with no hints", database.WorkspaceFile{Name: "orders.bin"}, false},
		{"image media type", database.WorkspaceFile{MediaType: "image/png", Name: "logo.png"}, false},
		{"unrecognized extension", database.WorkspaceFile{Name: "archive.zip"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTextLikeFile(tt.file); got != tt.want {
				t.Fatalf("isTextLikeFile(%+v) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
