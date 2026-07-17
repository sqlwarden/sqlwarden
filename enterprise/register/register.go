// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

// Package register assembles the enterprise extension registry. It is the
// only package internal/edition's enterprise seam imports.
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
