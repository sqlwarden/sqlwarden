// Package edition selects which extension registry the binary is built
// with. The "enterprise" build tag is the only compile-time seam between
// editions: without it, no enterprise package is imported and the community
// binary contains zero enterprise code.
package edition
