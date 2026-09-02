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
	// listing through metadata.SchemaInspector.InspectDirectory.
	CapabilitySchemaDirectory Capability = "schema.directory"
	// CapabilitySchemaObjects provides on-demand columns, keys, indexes, and
	// descriptors through metadata.SchemaInspector.InspectObjects.
	CapabilitySchemaObjects Capability = "schema.objects"
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
)

// CapabilitySet is an engine's static capability report. Safe to compute and
// serialize without opening a target connection.
type CapabilitySet struct {
	Engine       EngineDescriptor    `json:"engine"`
	Capabilities map[Capability]bool `json:"capabilities"`
	// Schema accompanies schema.directory/schema.objects.
	Schema *metadata.SchemaSpec `json:"schema,omitempty"`
	// DDL accompanies schema.edit.
	DDL *ddl.Spec `json:"schema_edit,omitempty"`
	// Statements accompanies sql.generate.
	Statements *statement.Spec `json:"statements,omitempty"`
	// Explain accompanies sql.explain.
	Explain *explain.Spec `json:"explain,omitempty"`
}

// capabilitiesOf derives an engine's capabilities by type-asserting a fresh,
// unconnected probe driver against each capability interface. The booleans are
// DERIVED, never hand-declared, so a reported capability can never disagree with
// what the engine actually implements. The probe is created but never connected,
// which is why this works for the static /engines report.
func capabilitiesOf(reg Registration) (map[Capability]bool, *metadata.SchemaSpec, *ddl.Spec, *statement.Spec, *explain.Spec) {
	probe := reg.New()
	caps := map[Capability]bool{
		CapabilitySchemaDirectory: false,
		CapabilitySchemaObjects:   false,
		CapabilityDDL:             false,
		CapabilityQueryCursor:     false,
		CapabilitySQLGenerate:     false,
		CapabilitySQLExplain:      false,
	}
	var spec *metadata.SchemaSpec
	var ddlSpec *ddl.Spec
	var statementSpec *statement.Spec
	var explainSpec *explain.Spec
	if si, ok := probe.(metadata.SchemaInspector); ok {
		caps[CapabilitySchemaDirectory] = true
		caps[CapabilitySchemaObjects] = true
		s := si.SchemaSpec()
		spec = &s
	}
	if executor, ok := probe.(ddl.Executor); ok {
		caps[CapabilityDDL] = true
		s := executor.DDLSpec()
		ddlSpec = &s
	}
	if generator, ok := probe.(statement.Generator); ok {
		caps[CapabilitySQLGenerate] = true
		s := generator.StatementSpec()
		statementSpec = &s
	}
	if explainer, ok := probe.(explain.Explainer); ok {
		caps[CapabilitySQLExplain] = true
		s := explainer.ExplainSpec()
		explainSpec = &s
	}
	_, caps[CapabilityQueryCursor] = probe.(cursor.QueryCursorDriver)
	_, caps[CapabilitySQLClassify] = probe.(classifier.Classifier)
	_, caps[CapabilitySQLSafetyCheck] = probe.(safety.Checker)
	_, caps[CapabilitySQLParse] = probe.(parser.Parser)
	_, caps[CapabilitySQLRewrite] = probe.(rewriter.Rewriter)
	_, caps[CapabilitySQLComplete] = probe.(completer.Completer)
	return caps, spec, ddlSpec, statementSpec, explainSpec
}

// capabilityReport builds the full static capability report for an engine: its
// descriptor plus the derived capability map and schema spec.
func capabilityReport(reg Registration) CapabilitySet {
	caps, spec, ddlSpec, statementSpec, explainSpec := capabilitiesOf(reg)
	return CapabilitySet{
		Engine:       EngineDescriptor{ID: reg.ID, DisplayName: reg.DisplayName, Dialect: reg.Dialect},
		Capabilities: caps,
		Schema:       spec,
		DDL:          ddlSpec,
		Statements:   statementSpec,
		Explain:      explainSpec,
	}
}
