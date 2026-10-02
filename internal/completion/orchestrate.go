package completion

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/engine/completer"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

const (
	MetadataReady       = "ready"
	MetadataPartial     = "partial"
	MetadataDegraded    = "degraded"
	MetadataUnavailable = "unavailable"
)

// Loader is the completion metadata hook. View reads the cache only; Ensure
// may query the target and must be a no-op without a live session. Ensure
// receives deduplicated demands and reports in Capped how many it dropped
// from the tail without attempting them.
type Loader interface {
	View(ctx context.Context) (*metadata.CompletionView, error)
	Live() bool
	Ensure(ctx context.Context, demands []metadata.Demand) LoadReport
}

type LoadReport struct {
	Started   int
	Joined    int
	Cached    int
	Completed int
	Failed    int
	Capped    int
}

func (r *LoadReport) add(other LoadReport) {
	r.Started += other.Started
	r.Joined += other.Joined
	r.Cached += other.Cached
	r.Completed += other.Completed
	r.Failed += other.Failed
	r.Capped += other.Capped
}

type Outcome struct {
	Status          string
	Available       bool
	Loaded          bool
	Rounds          int
	Demands         int
	Report          LoadReport
	BudgetExhausted bool
}

// CompleteWithMetadata completes against cached metadata, then, while the
// completer reports demands and a live session exists, loads them within the
// budget and completes again, for at most maxRounds rounds. A demand Ensure
// attempted is never passed again in the same request, so a failed load is
// not retried.
func (s *Service) CompleteWithMetadata(ctx context.Context, driver string, req completer.Request, loader Loader) (completer.Result, Outcome, error) {
	outcome := Outcome{Status: MetadataUnavailable}
	view, err := loader.View(ctx)
	if err != nil {
		result, err := s.Complete(ctx, driver, req)
		return result, outcome, err
	}
	req.Metadata = view
	result, err := s.Complete(ctx, driver, req)
	if errors.Is(err, ErrUnsupported) {
		return result, outcome, err
	}
	if err != nil {
		req.Metadata = nil
		outcome.Status = MetadataDegraded
		result, err = s.Complete(ctx, driver, req)
		return result, outcome, err
	}

	budgetCtx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()
	attempted := map[metadata.Demand]bool{}
	for loader.Live() && outcome.Rounds < s.maxRounds && budgetCtx.Err() == nil {
		fresh := unattempted(result.Demands, attempted)
		if len(fresh) == 0 {
			break
		}
		outcome.Rounds++
		report := loader.Ensure(budgetCtx, fresh)
		outcome.Report.add(report)
		for _, demand := range fresh[:max(len(fresh)-report.Capped, 0)] {
			attempted[demand] = true
		}
		if report.Completed == 0 && report.Cached == 0 {
			if report.Capped > 0 {
				continue
			}
			break
		}
		outcome.Loaded = outcome.Loaded || report.Completed > 0
		next, err := loader.View(ctx)
		if err != nil {
			break
		}
		req.Metadata = next
		retry, err := s.Complete(ctx, driver, req)
		if err != nil {
			break
		}
		view, result = next, retry
	}
	outcome.BudgetExhausted = budgetCtx.Err() != nil
	outcome.Demands = len(result.Demands)
	outcome.Available = !view.Empty()
	switch {
	case outcome.Demands > 0:
		outcome.Status = MetadataPartial
	case outcome.Available:
		outcome.Status = MetadataReady
	}
	return result, outcome, nil
}

// unattempted returns demands not yet in attempted, deduplicated in order.
func unattempted(demands []metadata.Demand, attempted map[metadata.Demand]bool) []metadata.Demand {
	seen := map[metadata.Demand]bool{}
	var out []metadata.Demand
	for _, demand := range demands {
		if attempted[demand] || seen[demand] {
			continue
		}
		seen[demand] = true
		out = append(out, demand)
	}
	return out
}
