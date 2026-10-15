package catalog

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/statement"
)

// ErrUnknownEngine reports an engine or driver id with no registered engine.
var ErrUnknownEngine = errors.New("catalog: unknown engine")

// Params are a connection's driver-specific parameters.
type Params = engine.Params

// EngineView describes one registered engine and what it supports.
type EngineView struct {
	ID                    string                     `json:"id"`
	DisplayName           string                     `json:"display_name"`
	Dialect               string                     `json:"dialect"`
	Capabilities          map[engine.Capability]bool `json:"capabilities"`
	DDL                   *ddl.Spec                  `json:"schema_edit,omitempty"`
	Statements            *statement.Spec            `json:"statements,omitempty"`
	Explain               *explain.Spec              `json:"explain,omitempty"`
	TLS                   *engine.TLSSpec            `json:"connection_tls,omitempty"`
	SupportsSystemObjects bool                       `json:"supports_system_objects"`
	ShowAllDatabases      bool                       `json:"show_all_databases"`
}

// EngineCatalog lists every registered engine.
type EngineCatalog struct {
	Engines []EngineView `json:"engines"`
}

// ConnectionFields are the connection form fields of a driver.
type ConnectionFields struct {
	Fields []engine.FieldSpec `json:"fields"`
}

func engineToView(set engine.CapabilitySet) EngineView {
	v := EngineView{
		ID:           string(set.Engine.ID),
		DisplayName:  set.Engine.DisplayName,
		Dialect:      string(set.Engine.Dialect),
		Capabilities: set.Capabilities,
		DDL:          set.DDL,
		Statements:   set.Statements,
		Explain:      set.Explain,
		TLS:          set.TLS,
	}
	if set.Tree != nil {
		v.SupportsSystemObjects = set.Tree.SystemObjects
		v.ShowAllDatabases = set.Tree.DatabaseKind() != ""
	}
	return v
}

// requireAccount admits any signed-in account. The engine catalog is static
// product information, so it needs no resource permission.
func requireAccount(p access.Principal) error {
	if p.Subject.Kind != access.SubjectAccount || p.Subject.ID == 0 {
		return ErrForbidden
	}
	return nil
}

// Engines lists every registered engine.
func (s *Service) Engines(ctx context.Context, p access.Principal) (EngineCatalog, error) {
	if err := requireAccount(p); err != nil {
		return EngineCatalog{}, err
	}
	sets := engine.Engines()
	views := make([]EngineView, 0, len(sets))
	for _, set := range sets {
		views = append(views, engineToView(set))
	}
	return EngineCatalog{Engines: views}, nil
}

// Engine describes one engine.
func (s *Service) Engine(ctx context.Context, p access.Principal, engineID string) (EngineView, error) {
	if err := requireAccount(p); err != nil {
		return EngineView{}, err
	}
	set, ok := engine.Describe(engineID)
	if !ok {
		return EngineView{}, ErrUnknownEngine
	}
	return engineToView(set), nil
}

// EngineConnectionFields returns the connection form fields of a driver.
func (s *Service) EngineConnectionFields(ctx context.Context, p access.Principal, driver string) (ConnectionFields, error) {
	if err := requireAccount(p); err != nil {
		return ConnectionFields{}, err
	}
	spec, ok := s.deps.SpecLookup(driver)
	if !ok {
		return ConnectionFields{}, ErrUnknownEngine
	}
	return ConnectionFields{Fields: spec.Fields()}, nil
}
