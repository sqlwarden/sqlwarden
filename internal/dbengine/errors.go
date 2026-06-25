package dbengine

import "errors"

// ErrUnknownEngine is returned by New when no engine is registered for an id.
var ErrUnknownEngine = errors.New("dbengine: unknown engine")

// ErrUnsupported is returned when an engine or connection does not implement a
// requested optional capability. Web handlers map it to HTTP 501.
var ErrUnsupported = errors.New("dbengine: capability not supported")
