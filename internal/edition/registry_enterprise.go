//go:build enterprise

package edition

import (
	"github.com/sqlwarden/enterprise/register"
	"github.com/sqlwarden/internal/extension"
)

func Registry() *extension.Registry { return register.Registry() }
