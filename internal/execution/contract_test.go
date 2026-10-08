package execution_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/execution/executiontest"

	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
)

// A file database per connection is required: pooled connections to
// ":memory:" each see their own empty database.
type fileProvider struct{ dir string }

func (p fileProvider) Resolve(_ context.Context, ref credentials.ConnectionRef) (credentials.Credentials, error) {
	return credentials.Credentials{Driver: "sqlite", DSN: filepath.Join(p.dir, ref.ConnectionID+".db")}, nil
}

type allowPolicy struct{}

func (allowPolicy) Check(context.Context, string, string) error { return nil }

func newLocalFactory(t testing.TB) execution.Runtime {
	provider := fileProvider{dir: t.TempDir()}
	r := execution.NewLocal(execution.LocalConfig{
		Credentials: provider,
		Policy:      allowPolicy{},
		IdleTimeout: time.Hour,
	})
	t.Cleanup(r.Shutdown)
	return r
}

func TestLocalRuntimeContract(t *testing.T) {
	executiontest.RunRuntimeContract(t, newLocalFactory, executiontest.Fixture{
		Scope: execution.Scope{OrgID: "o1", WorkspaceID: "w1", AccountID: "a1", ConnectionID: "c1"},
	})
}
