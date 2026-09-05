// Package postgres implements the PostgreSQL engine.
//
// Driver is exported so a wire/catalog-compatible engine (e.g. Supabase,
// Neon, CockroachDB, YugabyteDB) can embed it by value:
//
//	type driver struct{ postgres.Driver }
//
// Every capability method (Connect, Dialect, SchemaSpec, InspectDirectory,
// InspectObjects, InspectDefinition, ...) is exported on Driver, so Go's
// method promotion satisfies engine.Driver and every optional capability
// interface through the outer type automatically. A compatible engine
// overrides only the methods it needs — for example Dialect() to report its
// own engine.Dialect, or TLSSpec()/SupportsSSHTunnel() if the managed
// provider restricts them — and registers itself independently with its own
// engine.Registration under its own engine.EngineID.
//
// catalog.go breaks catalog/DDL inspection into small, exported, composable
// functions per object kind (CatalogTables, CatalogMaterializedViews,
// CatalogFunctions, CatalogSequences, AttachRowCounts, RelationalObjects,
// MaterializedViewObjects, FunctionObjects, SequenceObjects, TableDDL,
// ViewDefinition, FunctionDefinition), each taking an explicit *sql.DB
// (reached via Driver.DB()) rather than a Driver receiver. A compatible
// engine's InspectDirectory/InspectObjects/InspectDefinition override
// composes only the functions it wants — e.g. CockroachDB drops
// CatalogMaterializedViews entirely — and adds its own functions for
// anything genuinely different, rather than re-implementing the whole
// method. A common pattern for partial divergence is to special-case one
// kind and delegate the rest to the embedded default:
//
//	func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
//		if ref.Kind == "some_engine_specific_kind" {
//			return someEngineSpecificDefinition(ctx, d.DB(), ref)
//		}
//		return d.Driver.InspectDefinition(ctx, ref)
//	}
package postgres
