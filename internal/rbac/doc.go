// Package rbac administers role-based access control: custom roles, policy
// bindings, and the permission catalog. The access package stays the
// enforcer; this package writes the rows it evaluates. Its Service authorizes
// every call against the caller's principal and keeps the RBAC invariants:
// builtin roles are immutable, a role's scope is validated before it is
// created, bindings are idempotent, roles still bound cannot be deleted,
// owner-level permissions can be granted or revoked only by their holders, and
// an organization's last owner binding cannot be revoked.
package rbac
