package app

import (
	"errors"

	"github.com/sqlwarden/internal/community"
	"github.com/sqlwarden/internal/edition"
)

func testEdition() edition.Edition { return community.New() }

type failingEdition struct{}

func (failingEdition) Name() string                     { return "failing" }
func (failingEdition) Licenser() edition.Licenser       { return edition.NoopLicenser{} }
func (failingEdition) Modules() []edition.Module        { return []edition.Module{failingModule{}} }
func (failingModule) Name() string                      { return "audit" }
func (failingModule) Feature() string                   { return edition.Catalog[0].Key }
func (failingModule) Register(*edition.Registrar) error { return errors.New("registration failed") }

type failingModule struct{}

type settingEdition struct{}

func (settingEdition) Name() string               { return "setting" }
func (settingEdition) Licenser() edition.Licenser { return edition.NoopLicenser{} }
func (settingEdition) Modules() []edition.Module  { return []edition.Module{settingModule{}} }

type settingModule struct{}

func (settingModule) Name() string    { return "audit" }
func (settingModule) Feature() string { return edition.Catalog[0].Key }
func (settingModule) Register(r *edition.Registrar) error {
	return r.Setting(edition.Setting{Key: "audit.retention"})
}
