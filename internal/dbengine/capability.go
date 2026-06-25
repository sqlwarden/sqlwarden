package dbengine

import (
	"github.com/sqlwarden/internal/dbengine/dbsql"
	"github.com/sqlwarden/internal/dbengine/schema"
	"github.com/sqlwarden/internal/dbengine/sqlquery"
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
	p := sqlquery.ProviderFor(reg.Dialect)
	caps[CapabilitySQLParse] = p.Parser() != nil
	caps[CapabilitySQLClassify] = p.Classifier() != nil
	caps[CapabilitySQLRewrite] = p.Rewriter() != nil
	caps[CapabilitySQLComplete] = p.Completer() != nil
	return caps, spec
}
