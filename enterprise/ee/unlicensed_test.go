// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise License. See enterprise/LICENSE.

package ee

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/license"
)

func unlicensedDeps() *extension.Deps {
	return &extension.Deps{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		License: Extension{}.LicenseService(),
		Events:  events.NewBus(),
	}
}

func TestStubJobRefusesWithoutLicense(t *testing.T) {
	deps := unlicensedDeps()
	defs := Extension{}.Jobs(deps)
	if len(defs) != 1 {
		t.Fatalf("expected 1 job definition, got %d", len(defs))
	}

	_, err := defs[0].Handler.Handle(context.Background(), jobs.Runtime{})
	if !errors.Is(err, license.ErrNotLicensed) {
		t.Fatalf("unlicensed job error = %v, want ErrNotLicensed", err)
	}
}

func TestStubEventSinkIsInertWithoutLicense(t *testing.T) {
	deps := unlicensedDeps()
	sink := Extension{}.EventSink(deps)

	// Must be a no-op, never a panic or side effect, when unlicensed.
	sink.HandleEvent(context.Background(), events.Event{Action: "auth.login", Outcome: "success"})
}
