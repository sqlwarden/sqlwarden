//go:build !enterprise

package main

import (
	"errors"

	"github.com/sqlwarden/internal/community"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
)

func selectedEdition(cfg config.Config) (edition.Edition, error) {
	if cfg.License != "" {
		return nil, errors.New("the license setting requires the enterprise build")
	}
	return community.New(), nil
}
