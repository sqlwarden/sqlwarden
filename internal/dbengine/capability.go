package dbengine

import (
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/completer"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/dbengine/schema"
)

// Capability is a stable, serializable identifier for an engine feature,
// reported to the frontend so it can gate UI on what an engine supports.
type Capability string

// The capability keys an engine may report. Each corresponds to an optional
// interface the engine type implements (see CapabilitySet and capabilitiesOf).
const (
	CapabilitySchemaCatalog Capability = "schema.catalog"
	CapabilitySchemaObjects Capability = "schema.objects"
	CapabilityQueryCursor   Capability = "query.cursor"
	CapabilitySQLParse      Capability = "sql.parse"
	CapabilitySQLClassify   Capability = "sql.classify"
	CapabilitySQLRewrite    Capability = "sql.rewrite"
	CapabilitySQLComplete   Capability = "sql.complete"
)

// CapabilitySet is an engine's static capability report. Safe to compute and
// serialize without opening a target connection.
type CapabilitySet struct {
	Engine       EngineDescriptor    `json:"engine"`
	Capabilities map[Capability]bool `json:"capabilities"`
	Schema       *schema.SchemaSpec  `json:"schema,omitempty"`
}

// capabilitiesOf derives an engine's capabilities by type-asserting a fresh,
// unconnected probe driver against each capability interface. The booleans are
// DERIVED, never hand-declared, so a reported capability can never disagree with
// what the engine actually implements. The probe is created but never connected,
// which is why this works for the static /engines report.
func capabilitiesOf(reg Registration) (map[Capability]bool, *schema.SchemaSpec) {
	probe := reg.New()
	caps := map[Capability]bool{
		CapabilitySchemaCatalog: false,
		CapabilitySchemaObjects: false,
		CapabilityQueryCursor:   false,
	}
	var spec *schema.SchemaSpec
	if si, ok := probe.(schema.SchemaInspector); ok {
		caps[CapabilitySchemaCatalog] = true
		caps[CapabilitySchemaObjects] = true
		s := si.SchemaSpec()
		spec = &s
	}
	_, caps[CapabilityQueryCursor] = probe.(cursor.QueryCursorDriver)
	_, caps[CapabilitySQLClassify] = probe.(classifier.Classifier)
	_, caps[CapabilitySQLParse] = probe.(parser.Parser)
	_, caps[CapabilitySQLRewrite] = probe.(rewriter.Rewriter)
	_, caps[CapabilitySQLComplete] = probe.(completer.Completer)
	return caps, spec
}

// capabilityReport builds the full static capability report for an engine: its
// descriptor plus the derived capability map and schema spec.
func capabilityReport(reg Registration) CapabilitySet {
	caps, spec := capabilitiesOf(reg)
	return CapabilitySet{
		Engine:       EngineDescriptor{ID: reg.ID, DisplayName: reg.DisplayName, Dialect: reg.Dialect},
		Capabilities: caps,
		Schema:       spec,
	}
}
