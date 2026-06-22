package sqlquery

import (
	"context"
	"sync"

	"github.com/sqlwarden/internal/driver"
)

// Provider exposes the SQL-language capabilities available for a dialect.
// Capabilities are optional so dialects can be added incrementally.
type Provider interface {
	Classifier() Classifier
	Parser() Parser
	Completer() Completer
	Rewriter() Rewriter
}

type StaticProvider struct {
	ClassifyCapability Classifier
	ParseCapability    Parser
	CompleteCapability Completer
	RewriteCapability  Rewriter
}

func (p StaticProvider) Classifier() Classifier { return p.ClassifyCapability }
func (p StaticProvider) Parser() Parser         { return p.ParseCapability }
func (p StaticProvider) Completer() Completer   { return p.CompleteCapability }
func (p StaticProvider) Rewriter() Rewriter     { return p.RewriteCapability }

var (
	registryMu sync.RWMutex
	registry   = map[driver.Dialect]Provider{}
	fallback   = StaticProvider{ClassifyCapability: NewHeuristicClassifier()}
)

func init() {
	Register(driver.DialectPostgres, fallback)
	Register(driver.DialectMySQL, fallback)
	Register(driver.DialectSQLite, fallback)
}

// Register installs a provider for a dialect.
func Register(dialect driver.Dialect, provider Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[dialect] = provider
}

// ProviderFor returns the provider for a dialect, falling back to the heuristic
// provider so callers keep a safe baseline for unknown or future dialects.
func ProviderFor(dialect driver.Dialect) Provider {
	registryMu.RLock()
	provider, ok := registry[dialect]
	registryMu.RUnlock()
	if ok && provider != nil {
		return provider
	}
	return fallback
}

// Classify classifies SQL with the provider registered for the request dialect.
func Classify(ctx context.Context, req ClassifyRequest) (ClassifyResult, error) {
	classifier := ProviderFor(req.Dialect).Classifier()
	if classifier == nil {
		return ClassifyResult{Kind: KindUnknown}, ErrUnsupportedCapability
	}
	return classifier.Classify(ctx, req)
}

// Parse parses SQL with the provider registered for the request dialect.
func Parse(ctx context.Context, req ParseRequest) (ParseResult, error) {
	parser := ProviderFor(req.Dialect).Parser()
	if parser == nil {
		return ParseResult{}, ErrUnsupportedCapability
	}
	return parser.Parse(ctx, req)
}

// Complete returns SQL completion suggestions with the provider registered for
// the request dialect.
func Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error) {
	completer := ProviderFor(req.Dialect).Completer()
	if completer == nil {
		return CompletionResult{}, ErrUnsupportedCapability
	}
	return completer.Complete(ctx, req)
}

// Rewrite transforms SQL with the provider registered for the request dialect.
func Rewrite(ctx context.Context, req RewriteRequest) (RewriteResult, error) {
	rewriter := ProviderFor(req.Dialect).Rewriter()
	if rewriter == nil {
		return RewriteResult{SQL: req.SQL, Applied: false, Reason: "unsupported"}, ErrUnsupportedCapability
	}
	return rewriter.Rewrite(ctx, req)
}
