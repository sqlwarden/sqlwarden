package query

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/safety"
)

// Analyzers resolves the stateless per-driver SQL capabilities the query use
// cases depend on.
type Analyzers interface {
	Classifier(driver string) classifier.Classifier
	SafetyChecker(driver string) safety.Checker
	Explainer(driver string) (explain.Explainer, bool)
}

// EngineAnalyzers resolves capabilities from the registered engines by
// type-asserting a fresh, unconnected driver instance.
type EngineAnalyzers struct{}

var _ Analyzers = EngineAnalyzers{}

// RegisteredClassifier resolves only a classifier implemented by the
// registered engine. Callers that must prove SQL properties, such as exports,
// must not fall back to a heuristic.
func RegisteredClassifier(driver string) (classifier.Classifier, bool) {
	d, err := engine.New(driver)
	if err != nil {
		return nil, false
	}
	c, ok := d.(classifier.Classifier)
	return c, ok
}

// Classifier falls back to the conservative heuristic when the driver does
// not implement classification.
func (EngineAnalyzers) Classifier(driver string) classifier.Classifier {
	if c, ok := RegisteredClassifier(driver); ok {
		return c
	}
	return classifier.NewHeuristic()
}

// SafetyChecker falls back to the conservative heuristic when the driver does
// not implement safety.Checker.
func (EngineAnalyzers) SafetyChecker(driver string) safety.Checker {
	d, err := engine.New(driver)
	if err == nil {
		if c, ok := d.(safety.Checker); ok {
			return c
		}
	}
	return safety.NewHeuristic()
}

// Explainer has no heuristic fallback: an engine either has a real EXPLAIN
// form or it does not.
func (EngineAnalyzers) Explainer(driver string) (explain.Explainer, bool) {
	d, err := engine.New(driver)
	if err != nil {
		return nil, false
	}
	e, ok := d.(explain.Explainer)
	return e, ok
}
