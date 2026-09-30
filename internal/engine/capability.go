package engine

import (
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/parser"
	"github.com/sqlwarden/internal/engine/rewriter"
	"github.com/sqlwarden/internal/engine/safety"
	"github.com/sqlwarden/internal/engine/statement"
)

// Capability is a stable, serializable identifier for an engine feature,
// reported to the frontend so it can gate UI on what an engine supports.
type Capability string

// The capability keys an engine may report. Each corresponds to an optional
// interface the engine type implements (see CapabilitySet and capabilitiesOf).
const (
	// CapabilitySchemaDirectory provides a cheap hierarchy and object-name
	// listing through metadata.DirectoryInspector.InspectDirectory.
	CapabilitySchemaDirectory Capability = "schema.directory"
	// CapabilitySchemaObjects provides on-demand columns, keys, indexes, and
	// descriptors through metadata.DirectoryInspector.InspectObjects.
	CapabilitySchemaObjects Capability = "schema.objects"
	// CapabilitySchemaNavigator provides the lazy navigator grammar and folder
	// loaders through metadata.SchemaInspector.
	CapabilitySchemaNavigator Capability = "schema.navigator"
	// CapabilityDDL applies the bounded structured schema operations advertised
	// by ddl.Executor.DDLSpec. It does not accept arbitrary SQL.
	CapabilityDDL Capability = "schema.edit"
	// CapabilityQueryCursor streams query results in bounded forward-only pages.
	CapabilityQueryCursor Capability = "query.cursor"
	// CapabilitySQLParse strictly parses complete SQL and reports statement
	// boundaries plus an engine-private syntax tree.
	CapabilitySQLParse Capability = "sql.parse"
	// CapabilitySQLClassify assigns the conservative DQL, DML, or DDL class used
	// by runtime authorization. Unknown input receives the strictest treatment.
	CapabilitySQLClassify Capability = "sql.classify"
	// CapabilitySQLRewrite performs only a named, explicitly supported SQL
	// transformation and refuses input it cannot prove safe.
	CapabilitySQLRewrite Capability = "sql.rewrite"
	// CapabilitySQLComplete returns cursor-aware lexical and metadata-backed
	// suggestions for incomplete editor SQL.
	CapabilitySQLComplete Capability = "sql.complete"
	// CapabilitySQLGenerate produces dialect-specific statement templates from
	// inspected metadata without executing them.
	CapabilitySQLGenerate Capability = "sql.generate"
	// CapabilitySQLSafetyCheck flags UPDATE/DELETE statements with no WHERE
	// clause so the runtime can require explicit confirmation before running
	// them. Dialects without a registered checker fall back to a heuristic,
	// which is why this capability can read false while confirmation gating
	// still applies (see connectionSafetyChecker in internal/web).
	CapabilitySQLSafetyCheck Capability = "sql.safety_check"
	// CapabilitySQLExplain produces an EXPLAIN form of a single statement
	// without executing it as-is. See explain.Explainer for the analyze-mode
	// distinction reported alongside this capability.
	CapabilitySQLExplain Capability = "sql.explain"
	// CapabilityTLS accepts structured TLS material (CA bundle, client cert,
	// verification mode, server name) through engine.TLSCapable.TLSSpec.
	CapabilityTLS Capability = "connection.tls"
	// CapabilitySSHTunnel accepts a caller-supplied context dialer so the
	// connection's transport can be routed through an SSH bastion. Behavior is
	// uniform across engines, so there is no accompanying spec.
	CapabilitySSHTunnel Capability = "connection.ssh_tunnel"
)

// SSHTunnelCapable is implemented by drivers that can route their transport
// through a caller-supplied context dialer (engine.ConnectionConfig.SSHDialer).
// Resolved by type assertion on an unconnected probe, like TLSCapable. There is
// no spec: tunnelling behaves the same for every engine.
type SSHTunnelCapable interface {
	SupportsSSHTunnel() bool
}

// CapabilitySet is an engine's static capability report. Safe to compute and
// serialize without opening a target connection.
type CapabilitySet struct {
	Engine       EngineDescriptor    `json:"engine"`
	Capabilities map[Capability]bool `json:"capabilities"`
	// Tree accompanies schema.navigator. Serialized by the schema API, not /engines.
	Tree *metadata.Tree `json:"-"`
	// Schema accompanies schema.directory/schema.objects.
	Schema *metadata.SchemaSpec `json:"schema,omitempty"`
	// DDL accompanies schema.edit.
	DDL *ddl.Spec `json:"schema_edit,omitempty"`
	// Statements accompanies sql.generate.
	Statements *statement.Spec `json:"statements,omitempty"`
	// Explain accompanies sql.explain.
	Explain *explain.Spec `json:"explain,omitempty"`
	// TLS accompanies connection.tls.
	TLS *TLSSpec `json:"connection_tls,omitempty"`
}

// capabilitiesOf derives an engine's capabilities by type-asserting a fresh,
// unconnected probe driver against each capability interface. The booleans are
// DERIVED, never hand-declared, so a reported capability can never disagree with
// what the engine actually implements. The probe is created but never connected,
// which is why this works for the static /engines report.
func capabilitiesOf(reg Registration) CapabilitySet {
	probe := reg.New()
	set := CapabilitySet{Capabilities: map[Capability]bool{
		CapabilitySchemaDirectory: false,
		CapabilitySchemaObjects:   false,
		CapabilitySchemaNavigator: false,
		CapabilityDDL:             false,
		CapabilityQueryCursor:     false,
		CapabilitySQLGenerate:     false,
		CapabilitySQLExplain:      false,
		CapabilityTLS:             false,
		CapabilitySSHTunnel:       false,
	}}
	caps := set.Capabilities
	if si, ok := probe.(metadata.SchemaInspector); ok {
		caps[CapabilitySchemaNavigator] = true
		caps[CapabilitySchemaObjects] = true
		tree := si.Tree()
		set.Tree = &tree
	}
	if di, ok := probe.(metadata.DirectoryInspector); ok {
		caps[CapabilitySchemaDirectory] = true
		caps[CapabilitySchemaObjects] = true
		s := di.SchemaSpec()
		set.Schema = &s
	}
	if executor, ok := probe.(ddl.Executor); ok {
		caps[CapabilityDDL] = true
		s := executor.DDLSpec()
		set.DDL = &s
	}
	if generator, ok := probe.(statement.Generator); ok {
		caps[CapabilitySQLGenerate] = true
		s := generator.StatementSpec()
		set.Statements = &s
	}
	if explainer, ok := probe.(explain.Explainer); ok {
		caps[CapabilitySQLExplain] = true
		s := explainer.ExplainSpec()
		set.Explain = &s
	}
	if tc, ok := probe.(TLSCapable); ok {
		caps[CapabilityTLS] = true
		s := tc.TLSSpec()
		set.TLS = &s
	}
	if sc, ok := probe.(SSHTunnelCapable); ok {
		caps[CapabilitySSHTunnel] = sc.SupportsSSHTunnel()
	}
	_, caps[CapabilityQueryCursor] = probe.(cursor.QueryCursorDriver)
	_, caps[CapabilitySQLClassify] = probe.(classifier.Classifier)
	_, caps[CapabilitySQLSafetyCheck] = probe.(safety.Checker)
	_, caps[CapabilitySQLParse] = probe.(parser.Parser)
	_, caps[CapabilitySQLRewrite] = probe.(rewriter.Rewriter)
	_, caps[CapabilitySQLComplete] = probe.(completer.Completer)
	return set
}

// capabilityReport builds the full static capability report for an engine: its
// descriptor plus the derived capability map and schema spec.
func capabilityReport(reg Registration) CapabilitySet {
	set := capabilitiesOf(reg)
	set.Engine = EngineDescriptor{ID: reg.ID, DisplayName: reg.DisplayName, Dialect: reg.Dialect}
	return set
}
