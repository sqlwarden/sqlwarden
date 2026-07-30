// Package completion coordinates connectionless SQL completion engines.
package completion

import (
	"context"
	"errors"
	"sync"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/completer"
)

var ErrUnsupported = errors.New("SQL completion is not supported for this driver")

// Service reuses one connectionless engine per dialect. Prepared catalogs are
// owned by the engines and keyed by connection and immutable schema version.
type Service struct {
	mu      sync.Mutex
	engines map[string]completer.Completer
}

func NewService() *Service {
	return &Service{engines: make(map[string]completer.Completer)}
}

func (s *Service) Complete(ctx context.Context, driver string, req completer.Request) (completer.Result, error) {
	engine, err := s.engine(driver)
	if err != nil {
		return completer.Result{}, err
	}
	return engine.Complete(ctx, req)
}

func (s *Service) Vocabulary(driver string) (completer.Vocabulary, error) {
	engine, err := s.engine(driver)
	if err != nil {
		return completer.Vocabulary{}, err
	}
	provider, ok := engine.(completer.VocabularyProvider)
	if !ok {
		return completer.Vocabulary{}, ErrUnsupported
	}
	return provider.CompletionVocabulary(), nil
}

func (s *Service) InvalidateConnection(connectionID string) {
	s.mu.Lock()
	engines := make([]completer.Completer, 0, len(s.engines))
	for _, engine := range s.engines {
		engines = append(engines, engine)
	}
	s.mu.Unlock()
	for _, engine := range engines {
		if invalidator, ok := engine.(completer.CatalogInvalidator); ok {
			invalidator.InvalidateCompletionCatalog(connectionID)
		}
	}
}

func (s *Service) engine(driver string) (completer.Completer, error) {
	normalized := engine.NormalizeName(driver)
	s.mu.Lock()
	defer s.mu.Unlock()
	if engine, ok := s.engines[normalized]; ok {
		return engine, nil
	}
	raw, err := engine.New(normalized)
	if err != nil {
		return nil, err
	}
	engine, ok := raw.(completer.Completer)
	if !ok {
		return nil, ErrUnsupported
	}
	s.engines[normalized] = engine
	return engine, nil
}
