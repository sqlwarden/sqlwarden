// Package explain defines the optional engine capability for producing an
// EXPLAIN form of a single statement. An engine provides this by implementing
// Explainer; it is stateless and never touches a live connection or executes
// SQL — it only rewrites statement text into the Plan the caller then runs.
package explain
