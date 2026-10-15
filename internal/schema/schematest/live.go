// Package schematest adapts a connected driver's inspector to schema.Live for
// tests that drive the navigator directly against a real engine.
package schematest

import (
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/schema"
)

// Live wraps inspector as a schema.Live. A nil inspector yields a nil Live so
// callers keep the navigator's no-session behavior.
func Live(inspector metadata.SchemaInspector) schema.Live {
	return schema.LiveFromInspector(inspector)
}
