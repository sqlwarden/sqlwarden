package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogMarshalsRefsWithoutColumns(t *testing.T) {
	c := Catalog{
		Dialect:  "postgres",
		Database: "app",
		Namespaces: []NamespaceCatalog{{
			Name: "public",
			Groups: []ObjectGroupCatalog{{
				Kind:    "table",
				Objects: []ObjectRef{{Namespace: "public", Kind: "table", Name: "users"}},
			}},
		}},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, `"columns"`) {
		t.Errorf("catalog must not carry columns: %s", s)
	}
	if !strings.Contains(s, `"dialect":"postgres"`) {
		t.Errorf("catalog must carry the dialect tag: %s", s)
	}
}

func TestCapabilitiesMarshal(t *testing.T) {
	caps := DriverCapabilities{
		Dialect: "postgres",
		Kinds: []KindDescriptor{{
			Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1,
			Relational: true, SupportsDiagram: true, Listing: "enumerated",
		}},
	}
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"listing":"enumerated"`) {
		t.Errorf("missing listing field: %s", data)
	}
}
