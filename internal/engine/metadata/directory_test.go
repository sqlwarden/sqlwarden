package metadata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectoryMarshalsRefsWithoutColumns(t *testing.T) {
	scope := NewScopePath(ScopeSegment{Kind: "database", Name: "app"}, ScopeSegment{Kind: "schema", Name: "public"})
	directory := Directory{
		Engine: "postgres",
		Roots: []ScopeNode{{
			Path: scope,
			Groups: []ObjectGroup{{
				Kind:    "table",
				Objects: []ObjectRef{{Scope: scope, Kind: "table", Name: "users"}},
			}},
		}},
	}
	data, err := json.Marshal(directory)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, `"columns"`) {
		t.Errorf("directory must not carry columns: %s", s)
	}
	if !strings.Contains(s, `"engine":"postgres"`) {
		t.Errorf("directory must carry the engine tag: %s", s)
	}
}
