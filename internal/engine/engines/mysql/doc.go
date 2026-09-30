// Package mysql implements the MySQL engine.
//
// Driver is exported so a wire/catalog-compatible engine (e.g. MariaDB,
// TiDB, Vitess, PlanetScale) can embed it by value:
//
//	type driver struct{ mysql.Driver }
//
// Every capability method (Connect, Dialect, Tree, InspectObjects,
// InspectDefinition, ...) is exported on Driver, so Go's
// method promotion satisfies engine.Driver and every optional capability
// interface through the outer type automatically. A compatible engine
// overrides only the methods it needs — for example Dialect() to report its
// own engine.Dialect — and registers itself independently with its own
// engine.Registration under its own engine.EngineID.
//
// navigator.go declares the navigator tree and one exported loader per folder
// (ListDatabases, ListTables, ListTableIndexes, ...), each a metadata.Loader. A
// compatible engine's Tree override starts from Driver.Tree() and adds or
// removes folders — e.g. TiDB drops routines, triggers, and events, which it
// does not support — rather than redeclaring the tree.
//
// catalog.go breaks object detail and DDL inspection into small, exported,
// composable functions per object kind (RelationalObjects, RoutineObjects,
// TriggerObjects, ShowCreateDefinition), each taking an explicit *sql.DB
// (reached via Driver.DB()) rather than a Driver receiver. A compatible
// engine's InspectObjects/InspectDefinition override composes only the
// functions it wants and adds its own for anything genuinely different,
// rather than re-implementing the whole method. A
// common pattern for partial divergence is to special-case one kind and
// delegate the rest to the embedded default:
//
//	func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
//		if ref.Kind == "sequence" {
//			return mysql.ShowCreateDefinition(ctx, d.DB(), ref, "SHOW CREATE SEQUENCE ", "Definition")
//		}
//		return d.Driver.InspectDefinition(ctx, ref)
//	}
package mysql
