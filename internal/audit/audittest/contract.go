// Package audittest holds the behavioral contract every audit writer must
// satisfy, so core and edition writers are proven against the same rules.
package audittest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlwarden/internal/audit"
)

// Subject is one writer under test together with a way to read back what it
// durably recorded. Recorded must return the events that survived the write,
// in any order.
type Subject struct {
	Writer   audit.Writer
	Recorded func(ctx context.Context) ([]audit.Event, error)
}

// Run asserts the audit writer contract against the subject produced by
// newSubject. Each case gets a fresh subject so recorded state cannot leak
// between cases.
func Run(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Helper()

	t.Run("records a valid event durably", func(t *testing.T) {
		subject := newSubject(t)
		orgID := int64(7)
		accountID := int64(11)
		event := audit.Event{
			OrgID:      &orgID,
			AccountID:  &accountID,
			Action:     "contract.write",
			Resource:   "workspace",
			ResourceID: "42",
			Outcome:    audit.OutcomeSuccess,
			Metadata:   map[string]string{"scope": "contract"},
		}
		if err := subject.Writer.Write(context.Background(), event); err != nil {
			t.Fatalf("write: %v", err)
		}

		recorded := recordedEvents(t, subject)
		if len(recorded) != 1 {
			t.Fatalf("recorded %d events, want 1", len(recorded))
		}
		got := recorded[0]
		if got.ID == "" {
			t.Error("recorded event has no id")
		}
		if got.OccurredAt.IsZero() {
			t.Error("recorded event has no timestamp")
		}
		if got.Action != event.Action || got.Outcome != event.Outcome {
			t.Errorf("recorded action/outcome = %q/%q, want %q/%q", got.Action, got.Outcome, event.Action, event.Outcome)
		}
		if got.Resource != event.Resource || got.ResourceID != event.ResourceID {
			t.Errorf("recorded resource = %q/%q, want %q/%q", got.Resource, got.ResourceID, event.Resource, event.ResourceID)
		}
		if got.OrgID == nil || *got.OrgID != orgID || got.AccountID == nil || *got.AccountID != accountID {
			t.Errorf("recorded subject = %v/%v, want %d/%d", got.OrgID, got.AccountID, orgID, accountID)
		}
		if got.Metadata["scope"] != "contract" {
			t.Errorf("recorded metadata = %v, want scope=contract", got.Metadata)
		}
	})

	t.Run("preserves a caller supplied identity and time", func(t *testing.T) {
		subject := newSubject(t)
		occurred := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
		event := audit.Event{
			ID:         "contract-fixed-id",
			OccurredAt: occurred,
			Action:     "contract.fixed",
			Outcome:    audit.OutcomeDenied,
		}
		if err := subject.Writer.Write(context.Background(), event); err != nil {
			t.Fatalf("write: %v", err)
		}

		recorded := recordedEvents(t, subject)
		if len(recorded) != 1 {
			t.Fatalf("recorded %d events, want 1", len(recorded))
		}
		if recorded[0].ID != event.ID {
			t.Errorf("recorded id = %q, want %q", recorded[0].ID, event.ID)
		}
		if !recorded[0].OccurredAt.Equal(occurred) {
			t.Errorf("recorded time = %s, want %s", recorded[0].OccurredAt, occurred)
		}
	})

	t.Run("assigns a distinct identity to every generated event", func(t *testing.T) {
		subject := newSubject(t)
		for range 3 {
			err := subject.Writer.Write(context.Background(), audit.Event{
				Action:  "contract.unique",
				Outcome: audit.OutcomeSuccess,
			})
			if err != nil {
				t.Fatalf("write: %v", err)
			}
		}

		seen := map[string]bool{}
		for _, event := range recordedEvents(t, subject) {
			if seen[event.ID] {
				t.Fatalf("duplicate audit event id %q", event.ID)
			}
			seen[event.ID] = true
		}
		if len(seen) != 3 {
			t.Fatalf("recorded %d distinct events, want 3", len(seen))
		}
	})

	for _, testCase := range []struct {
		name  string
		event audit.Event
	}{
		{name: "without an action", event: audit.Event{Outcome: audit.OutcomeSuccess}},
		{name: "with an unknown outcome", event: audit.Event{Action: "contract.invalid", Outcome: "maybe"}},
	} {
		t.Run("rejects an event "+testCase.name, func(t *testing.T) {
			subject := newSubject(t)
			err := subject.Writer.Write(context.Background(), testCase.event)
			if !errors.Is(err, audit.ErrInvalidEvent) {
				t.Fatalf("write error = %v, want %v", err, audit.ErrInvalidEvent)
			}
			if recorded := recordedEvents(t, subject); len(recorded) != 0 {
				t.Fatalf("recorded %d events, want none", len(recorded))
			}
		})
	}
}

func recordedEvents(t *testing.T, subject Subject) []audit.Event {
	t.Helper()
	recorded, err := subject.Recorded(context.Background())
	if err != nil {
		t.Fatalf("read recorded events: %v", err)
	}
	return recorded
}
