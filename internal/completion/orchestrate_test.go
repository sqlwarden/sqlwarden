package completion

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/completer"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

var testDemand = metadata.Demand{Parent: metadata.ScopePath("p"), Folder: "tables"}

type fakeCompleter struct {
	calls   int
	respond func(req completer.Request, call int) (completer.Result, error)
}

func (f *fakeCompleter) Complete(_ context.Context, req completer.Request) (completer.Result, error) {
	f.calls++
	return f.respond(req, f.calls)
}

type fakeLoader struct {
	live    bool
	views   int
	ensures int
	report  LoadReport
	delay   time.Duration
}

func (l *fakeLoader) View(context.Context) (*metadata.CompletionView, error) {
	l.views++
	listings := map[metadata.ListingKey][]metadata.Child{}
	if l.ensures > 0 {
		listings[metadata.ListingKey{Parent: metadata.ScopePath("p"), Folder: "tables"}] = nil
	}
	return metadata.NewCompletionView(metadata.Tree{}, "", "", listings, nil), nil
}

func (l *fakeLoader) Live() bool { return l.live }

func (l *fakeLoader) Ensure(ctx context.Context, _ []metadata.Demand) LoadReport {
	l.ensures++
	if l.delay > 0 {
		select {
		case <-time.After(l.delay):
		case <-ctx.Done():
			return LoadReport{Started: 1}
		}
	}
	return l.report
}

func serviceWith(c completer.Completer) *Service {
	s := NewService()
	s.engines[engine.NormalizeName("fake")] = c
	return s
}

func demandsUntilLoaded(req completer.Request, _ int) (completer.Result, error) {
	if req.Metadata == nil || req.Metadata.Empty() {
		return completer.Result{Demands: []metadata.Demand{testDemand}}, nil
	}
	return completer.Result{Suggestions: []completer.Suggestion{{Label: "orders"}}}, nil
}

func TestCompleteWithMetadataFetchesAndRecompletes(t *testing.T) {
	c := &fakeCompleter{respond: demandsUntilLoaded}
	loader := &fakeLoader{live: true, report: LoadReport{Started: 1, Completed: 1}}
	result, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Suggestions) != 1 || outcome.Status != MetadataReady || !outcome.Loaded || outcome.Rounds != 1 || c.calls != 2 {
		t.Fatalf("result = %+v outcome = %+v calls %d", result, outcome, c.calls)
	}
}

func TestCompleteWithMetadataWithoutSessionIsPartialAndNeverEnsures(t *testing.T) {
	c := &fakeCompleter{respond: demandsUntilLoaded}
	loader := &fakeLoader{live: false}
	_, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != MetadataPartial || loader.ensures != 0 || c.calls != 1 {
		t.Fatalf("outcome = %+v ensures %d", outcome, loader.ensures)
	}
}

func TestCompleteWithMetadataStopsAtRoundLimit(t *testing.T) {
	c := &fakeCompleter{respond: func(_ completer.Request, call int) (completer.Result, error) {
		return completer.Result{Demands: []metadata.Demand{{Parent: metadata.ScopePath("p"), Folder: fmt.Sprint("tables", call)}}}, nil
	}}
	loader := &fakeLoader{live: true, report: LoadReport{Started: 1, Completed: 1}}
	_, outcome, _ := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if outcome.Rounds != 4 || loader.ensures != 4 || outcome.Status != MetadataPartial {
		t.Fatalf("outcome = %+v ensures %d", outcome, loader.ensures)
	}
}

func TestCompleteWithMetadataStopsOnBudget(t *testing.T) {
	c := &fakeCompleter{respond: demandsUntilLoaded}
	loader := &fakeLoader{live: true, delay: time.Second}
	s := serviceWith(c)
	s.budget = 20 * time.Millisecond
	started := time.Now()
	_, outcome, _ := s.CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if !outcome.BudgetExhausted || outcome.Status != MetadataPartial || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("outcome = %+v elapsed %v", outcome, time.Since(started))
	}
}

func TestCompleteWithMetadataKeepsFirstResultWhenLoadsFail(t *testing.T) {
	c := &fakeCompleter{respond: func(completer.Request, int) (completer.Result, error) {
		return completer.Result{Suggestions: []completer.Suggestion{{Label: "SELECT"}}, Demands: []metadata.Demand{testDemand}}, nil
	}}
	loader := &fakeLoader{live: true, report: LoadReport{Started: 1, Failed: 1}}
	result, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if err != nil || len(result.Suggestions) != 1 || outcome.Status != MetadataPartial || outcome.Loaded || loader.ensures != 1 {
		t.Fatalf("result = %+v outcome = %+v err %v ensures %d", result, outcome, err, loader.ensures)
	}
}

type mixedLoader struct {
	fail   metadata.Demand
	cap    int
	listed map[metadata.Demand]bool
	passed map[metadata.Demand]int
}

func (l *mixedLoader) View(context.Context) (*metadata.CompletionView, error) {
	listings := map[metadata.ListingKey][]metadata.Child{}
	for demand := range l.listed {
		listings[metadata.ListingKey{Parent: demand.Parent, Folder: demand.Folder}] = nil
	}
	return metadata.NewCompletionView(metadata.Tree{}, "", "", listings, nil), nil
}

func (l *mixedLoader) Live() bool { return true }

func (l *mixedLoader) Ensure(_ context.Context, demands []metadata.Demand) LoadReport {
	var report LoadReport
	if l.cap > 0 && len(demands) > l.cap {
		report.Capped = len(demands) - l.cap
		demands = demands[:l.cap]
	}
	for _, demand := range demands {
		l.passed[demand]++
		report.Started++
		if demand == l.fail {
			report.Failed++
			continue
		}
		l.listed[demand] = true
		report.Completed++
	}
	return report
}

func TestCompleteWithMetadataDoesNotRetryFailedLoads(t *testing.T) {
	failing := metadata.Demand{Parent: metadata.ScopePath("p"), Folder: "views"}
	loader := &mixedLoader{fail: failing, listed: map[metadata.Demand]bool{}, passed: map[metadata.Demand]int{}}
	c := &fakeCompleter{respond: func(req completer.Request, _ int) (completer.Result, error) {
		var demands []metadata.Demand
		for _, demand := range []metadata.Demand{failing, testDemand} {
			if !loader.listed[demand] {
				demands = append(demands, demand)
			}
		}
		return completer.Result{Demands: demands}, nil
	}}
	_, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if loader.passed[failing] != 1 || loader.passed[testDemand] != 1 {
		t.Fatalf("passed = %v outcome = %+v", loader.passed, outcome)
	}
	if outcome.Rounds != 1 || outcome.Status != MetadataPartial || !outcome.Loaded {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestCompleteWithMetadataRetriesWithoutMetadataOnError(t *testing.T) {
	c := &fakeCompleter{respond: func(req completer.Request, _ int) (completer.Result, error) {
		if req.Metadata != nil {
			return completer.Result{}, errors.New("boom")
		}
		return completer.Result{Suggestions: []completer.Suggestion{{Label: "SELECT"}}}, nil
	}}
	_, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, &fakeLoader{live: true})
	if err != nil || outcome.Status != MetadataDegraded {
		t.Fatalf("outcome = %+v err %v", outcome, err)
	}
}

func TestCompleteWithMetadataUnsupportedDriver(t *testing.T) {
	_, _, err := NewService().CompleteWithMetadata(context.Background(), "definitely-not-a-driver", completer.Request{}, &fakeLoader{})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestCompleteWithMetadataPassesCappedDemandsInLaterRounds(t *testing.T) {
	second := metadata.Demand{Parent: metadata.ScopePath("p"), Folder: "views"}
	loader := &mixedLoader{cap: 1, listed: map[metadata.Demand]bool{}, passed: map[metadata.Demand]int{}}
	c := &fakeCompleter{respond: func(req completer.Request, _ int) (completer.Result, error) {
		var demands []metadata.Demand
		for _, demand := range []metadata.Demand{testDemand, second} {
			if !loader.listed[demand] {
				demands = append(demands, demand)
			}
		}
		return completer.Result{Demands: demands}, nil
	}}
	_, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if loader.passed[testDemand] != 1 || loader.passed[second] != 1 || outcome.Rounds != 2 || outcome.Status != MetadataReady {
		t.Fatalf("passed = %v outcome = %+v", loader.passed, outcome)
	}
}

func TestCompleteWithMetadataTriesCappedDemandsAfterAFailedRound(t *testing.T) {
	failing := metadata.Demand{Parent: metadata.ScopePath("p"), Folder: "views"}
	loader := &mixedLoader{fail: failing, cap: 1, listed: map[metadata.Demand]bool{}, passed: map[metadata.Demand]int{}}
	c := &fakeCompleter{respond: func(req completer.Request, _ int) (completer.Result, error) {
		var demands []metadata.Demand
		for _, demand := range []metadata.Demand{failing, testDemand} {
			if !loader.listed[demand] {
				demands = append(demands, demand)
			}
		}
		return completer.Result{Demands: demands}, nil
	}}
	_, outcome, err := serviceWith(c).CompleteWithMetadata(context.Background(), "fake", completer.Request{}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if loader.passed[failing] != 1 || loader.passed[testDemand] != 1 || outcome.Rounds != 2 || !outcome.Loaded {
		t.Fatalf("passed = %v outcome = %+v", loader.passed, outcome)
	}
}
