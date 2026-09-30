// Package postgres implements the PostgreSQL engine.
//
// Driver is exported so a wire/catalog-compatible engine (e.g. Supabase,
// Neon, CockroachDB, YugabyteDB) can embed it by value:
//
//	type driver struct{ postgres.Driver }
//
// Every capability method (Connect, Dialect, Tree, InspectObjects,
// InspectDefinition, ...) is exported on Driver, so Go's
// method promotion satisfies engine.Driver and every optional capability
// interface through the outer type automatically. A compatible engine
// overrides only the methods it needs — for example Dialect() to report its
// own engine.Dialect, or TLSSpec()/SupportsSSHTunnel() if the managed
// provider restricts them — and registers itself independently with its own
// engine.Registration under its own engine.EngineID.
//
// navigator.go declares the navigator tree and one exported loader per folder
// (ListSchemas, ListTables, ListRelationIndexes, ...), each a metadata.Loader. A
// compatible engine's Tree override starts from Driver.Tree() and adds,
// removes, or wraps folders — e.g. CockroachDB drops the folders it does not
// support and marks its virtual schemas as system — rather than redeclaring
// the tree.
//
// catalog.go breaks object detail and DDL inspection into small, exported,
// composable functions per object kind (RelationalObjects,
// MaterializedViewObjects, FunctionObjects, SequenceObjects, TableDDL,
// ViewDefinition, FunctionDefinition), each taking an explicit *sql.DB
// (reached via Driver.DB()) rather than a Driver receiver. A compatible
// engine's InspectObjects/InspectDefinition override composes only the
// functions it wants and adds its own for anything genuinely different,
// rather than re-implementing the whole method. A common pattern for partial divergence is to special-case one
// kind and delegate the rest to the embedded default:
//
//	func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
//		if ref.Kind == "some_engine_specific_kind" {
//			return someEngineSpecificDefinition(ctx, d.DB(), ref)
//		}
//		return d.Driver.InspectDefinition(ctx, ref)
//	}
package postgres
