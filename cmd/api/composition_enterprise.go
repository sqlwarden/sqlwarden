//go:build enterprise

package main

import (
	"github.com/sqlwarden/enterprise/register"
	"github.com/sqlwarden/internal/extension"
)

func extensionRegistry() *extension.Registry {
	return register.Registry()
}
