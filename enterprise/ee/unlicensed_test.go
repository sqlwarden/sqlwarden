// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

package ee

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
)

func TestModuleDeclaresCentralCapabilityGates(t *testing.T) {
	contrib, err := start(context.Background(), extension.RuntimeDeps{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Capabilities: placeholderLicense{},
		Events:       events.NewBus(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(contrib.Routes) != 1 || contrib.Routes[0].Capability != "stub" {
		t.Fatalf("route capability declaration = %+v", contrib.Routes)
	}
	if len(contrib.Jobs) != 1 || contrib.Jobs[0].Capability != "stub" {
		t.Fatalf("job capability declaration = %+v", contrib.Jobs)
	}
	if len(contrib.EventSinks) != 1 || contrib.EventSinks[0].Capability != "stub" {
		t.Fatalf("sink capability declaration = %+v", contrib.EventSinks)
	}
}
