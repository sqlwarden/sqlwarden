// Package settings is the application service for database-backed runtime
// settings: the instance row, per-organization overrides, and the validated
// effective values every other layer reads.
//
// It owns three responsibilities that used to live in the HTTP transport:
// startup seeding and validation of the instance row ([Prepare]), validation of
// any candidate instance row ([Validate]), and resolution of effective settings
// for an organization or workspace ([Service]).
//
// Effective settings are always resolved from a valid instance row. Overrides
// may only narrow an instance limit, never loosen it, so an organization cannot
// grant itself more than the instance allows.
package settings
