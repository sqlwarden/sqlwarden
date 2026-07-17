//go:build !enterprise

package main

import "github.com/sqlwarden/internal/extension"

func extensionRegistry() *extension.Registry {
	return extension.NewRegistry()
}
