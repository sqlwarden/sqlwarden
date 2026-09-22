// Package catalog is the application service for the resource catalog:
// organizations, workspaces, environments, and connections.
//
// The catalog owns the rules that govern those resources — input validation,
// tenant and workspace ownership checks, hierarchy seeding, authorization
// cache invalidation, credential sealing, and connection lifecycle semantics.
// Transports map requests and responses only: they decode input, call a use
// case, and translate [Errors] into their own protocol.
//
// Boundaries:
//
//   - The catalog never imports a transport package. It returns domain errors
//     and value results; it does not know about HTTP status codes.
//   - Persistence goes through [Store]. Transaction composition — inserting a
//     resource, its resource_hierarchy row, and its seeded role bindings as one
//     unit — is a [Store] responsibility, so no use case can create a resource
//     without its hierarchy and policy rows.
//   - Authorization cache invalidation goes through [Grants], live target
//     sessions through [Sessions], and DSN encryption through [Sealer].
//   - TLS and SSH configuration documents stay with the transport that defines
//     their wire schema and driver-aware validation. The catalog accepts them
//     as already-sealed opaque ciphertext and stores them unchanged.
package catalog
