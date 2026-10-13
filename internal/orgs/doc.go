// Package orgs owns organizations and their membership. Its Service
// authorizes every call against the caller's principal and reaches storage
// only through the Store port. The package also holds the storage-free rules:
// slug derivation and validation, the builtin member roles, and the invitation
// capability a profile selects. Granting a builtin role follows the rbac
// privileged-grant rule.
package orgs
