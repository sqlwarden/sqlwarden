package dbengine

import (
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/completer"
	"github.com/sqlwarden/internal/dbengine/dbsql"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/dbengine/schema"
)

// Capability is a stable, serializable identifier for an engine feature.
type Capability string

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

// capabilitiesOf derives capabilities from the interfaces the driver implements
// and the SQL provider registered for the dialect. Booleans are DERIVED, never
// hand-declared, so a reported capability can never disagree with the
// implementation. The probe driver is created but never connected.
func capabilitiesOf(reg Registration) (map[Capability]bool, *schema.SchemaSpec) {
	probe := reg.NewDriver()
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
	if _, ok := probe.(dbsql.QueryCursorDriver); ok {
		caps[CapabilityQueryCursor] = true
	}
	caps[CapabilitySQLClassify] = classifier.For(reg.Dialect) != nil
	caps[CapabilitySQLParse] = parser.For(reg.Dialect) != nil
	caps[CapabilitySQLRewrite] = rewriter.For(reg.Dialect) != nil
	caps[CapabilitySQLComplete] = completer.For(reg.Dialect) != nil
	return caps, spec
}
