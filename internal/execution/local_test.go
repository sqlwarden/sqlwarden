package execution_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/connection"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/execution/executiontest"
)

func TestLocalRuntimeContract(t *testing.T) {
	executiontest.RunRuntimeContract(t, func(t testing.TB) (execution.SessionRuntime, execution.SessionDirectory) {
		sessions := connection.New(time.Minute)
		cursors := connection.NewQueryCursorManager(time.Minute)
		t.Cleanup(func() {
			cursors.Close()
			sessions.Close()
		})
		directory := execution.NewMemorySessionDirectory()
		return execution.NewLocalRuntime(sessions, cursors, directory, time.Minute), directory
	})
}

func TestLocalRuntimeEphemeralSessionDoesNotReplacePooledSession(t *testing.T) {
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	t.Cleanup(func() {
		cursors.Close()
		sessions.Close()
	})
	runtime := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), time.Minute)
	ctx := context.Background()
	target := execution.Target{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "runtime.db")}
	scope := execution.Scope{AccountID: "account", ConnectionID: "connection"}

	pooled, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	ephemeral, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope, Target: target, Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	if ephemeral.Handle == pooled.Handle || ephemeral.Reused {
		t.Fatalf("ephemeral Open() = %+v, pooled = %+v", ephemeral, pooled)
	}
	if err := runtime.Close(ctx, execution.CloseRequest{Handle: ephemeral.Handle}); err != nil {
		t.Fatal(err)
	}
	reused, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if reused.Handle != pooled.Handle || !reused.Reused {
		t.Fatalf("pooled session after ephemeral close = %+v, want reused %q", reused, pooled.Handle)
	}
}
