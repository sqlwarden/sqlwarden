// Package community supplies the default AGPL edition.
package community

import "github.com/sqlwarden/internal/edition"

type Community struct{}

func New() Community                         { return Community{} }
func (Community) Name() string               { return edition.CommunityName }
func (Community) Licenser() edition.Licenser { return edition.NoopLicenser{} }
func (Community) Modules() []edition.Module  { return nil }

var _ edition.Edition = Community{}
