package engine

import "errors"

// ErrUnknownEngine is returned by New when no engine is registered for an id.
var ErrUnknownEngine = errors.New("engine: unknown engine")

// ErrUnsupported is returned when an engine or connection does not implement a
// requested optional capability. Web handlers map it to HTTP 501.
var ErrUnsupported = errors.New("engine: capability not supported")
