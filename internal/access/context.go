package access

import "context"

// Attributes describe the request context an authorization decision is made
// in, such as how the session was authenticated or where it came from. Core
// RBAC ignores them; an edition decorator uses them to express conditional
// access. Values must never contain secrets, since they travel with the
// decision path.
type Attributes map[string]string

// Attribute keys populated by transports that evaluate conditional access.
const (
	// AttributeAuthMethod is the credential protocol that authenticated the
	// session, matching an identity.AuthenticationMethod value.
	AttributeAuthMethod = "auth_method"
)

type attributesContextKey struct{}

// WithAttributes returns a context carrying decision attributes. It replaces
// any attributes already present.
func WithAttributes(ctx context.Context, attributes Attributes) context.Context {
	return context.WithValue(ctx, attributesContextKey{}, attributes)
}

// AttributesFrom returns the decision attributes carried by ctx, or nil when
// the transport did not set any. A conditional rule that requires an attribute
// therefore denies rather than passes when the context says nothing.
func AttributesFrom(ctx context.Context) Attributes {
	attributes, _ := ctx.Value(attributesContextKey{}).(Attributes)
	return attributes
}
