// Package safety defines the SQL safety-check capability: determining
// whether a statement requires explicit user confirmation before running
// because it is likely to affect far more data than intended (e.g. an
// UPDATE/DELETE with no WHERE clause). An engine provides this by
// implementing Checker; it is stateless and never touches a live
// connection, mirroring internal/engine/classifier.
package safety
