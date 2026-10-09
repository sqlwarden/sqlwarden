// Package files manages workspace-scoped file metadata and storage coordination.
//
// Every Service method takes the acting access.Principal and a Scope of
// identifiers. The service resolves the organization and workspace through the
// access.Requirer and then applies the private and shared tree rules, so callers
// never pass a pre-authorized workspace or account.
package files
