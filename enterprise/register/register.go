// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

// Package register assembles the enterprise extension registry. The
// enterprise command composition is the only shared-code seam that imports it.
package register

import (
	"github.com/sqlwarden/enterprise/ee"
	"github.com/sqlwarden/internal/extension"
)

func Registry() *extension.Registry {
	r := extension.NewRegistry()
	r.Add(ee.NewModule())
	return r
}
