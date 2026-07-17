//go:build !enterprise

package edition

import "github.com/sqlwarden/internal/extension"

func Registry() *extension.Registry { return extension.NewRegistry() }
