// Package services builds the application service graph from stores and
// ports. [Build] is the only place that wires the access requirer, the
// connection catalog, query, files, exports, and jobs together; the
// composition root and web tests call it and hand the resulting [Services]
// bundle to the transport layer.
//
// The package must not import internal/web, internal/app, or
// internal/profile: adapters and the composition root depend on it, never the
// other way around.
package services
