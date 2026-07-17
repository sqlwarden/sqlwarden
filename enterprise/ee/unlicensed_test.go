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

func TestModuleDeclaresCentralLicenseGates(t *testing.T) {
	contrib, err := start(context.Background(), extension.RuntimeDeps{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		License: placeholderLicense{},
		Events:  events.NewBus(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(contrib.Routes) != 1 || contrib.Routes[0].Feature != "stub" {
		t.Fatalf("route license declaration = %+v", contrib.Routes)
	}
	if len(contrib.Jobs) != 1 || contrib.Jobs[0].Feature != "stub" {
		t.Fatalf("job license declaration = %+v", contrib.Jobs)
	}
	if len(contrib.EventSinks) != 1 || contrib.EventSinks[0].Feature != "stub" {
		t.Fatalf("sink license declaration = %+v", contrib.EventSinks)
	}
}
