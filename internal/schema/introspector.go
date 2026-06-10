package schema

import "context"

// Introspector is the capability a driver implements to report its schema.
// It is satisfied implicitly (Go structural typing); drivers need not import
// this interface, only the schema types they return. The contract trades only
// in domain types, so an implementation may gather them via SQL, HTTP, gRPC,
// or any other protocol.
type Introspector interface {
	Introspect(ctx context.Context, opts IntrospectOptions) (*Schema, error)
}

// IntrospectOptions scopes an introspection request.
type IntrospectOptions struct {
	Database string // optional; empty means the driver's current/default database
}
