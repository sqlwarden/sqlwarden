//go:build enterprise

package main

import (
	"github.com/sqlwarden/ee"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
)

func selectedEdition(config.Config) (edition.Edition, error) { return ee.New(), nil }
