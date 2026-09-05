package neon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
)

// driver is the Neon engine.Driver implementation: plain postgres.Driver plus
// cold-start tolerance on Connect.
type driver struct {
	postgres.Driver
}

var (
	_ engine.Driver           = (*driver)(nil)
	_ engine.TLSCapable       = (*driver)(nil)
	_ engine.SSHTunnelCapable = (*driver)(nil)
)

func (d *driver) Dialect() engine.Dialect { return engine.DialectPostgres }

// coldStartRetries/backoff bound how long Connect tolerates a suspended Neon
// compute waking up. Neon's documented cold start is typically well under a
// second and rarely exceeds a few seconds; this budgets for a slow wake
// without turning a genuinely bad DSN into a long hang.
const coldStartRetries = 3

// coldStartInitialBackoff is a var (not a const) so tests can shrink it and
// exercise the retry loop's control flow without real wall-clock delay.
var coldStartInitialBackoff = 2 * time.Second

// Connect retries postgres.Driver.Connect with linear backoff so a suspended
// Neon compute has time to wake before the connection attempt is reported as
// failed. A malformed DSN fails immediately without retrying: pgx.ParseConfig
// is a pure syntax check, so a config it accepts is worth retrying on and a
// config it rejects never will succeed no matter how many times we retry.
func (d *driver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	if _, err := pgx.ParseConfig(cfg.DSN); err != nil {
		return fmt.Errorf("neon: parse config: %w", err)
	}
	return connectWithColdStartRetry(ctx, cfg, d.Driver.Connect)
}

// connectWithColdStartRetry is Connect's retry loop, factored out so it can
// be exercised with a fake connect func in tests without a live database.
func connectWithColdStartRetry(ctx context.Context, cfg engine.ConnectionConfig, connect func(context.Context, engine.ConnectionConfig) error) error {
	var err error
	backoff := coldStartInitialBackoff
	for attempt := 0; attempt <= coldStartRetries; attempt++ {
		if err = connect(ctx, cfg); err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return fmt.Errorf("neon: connect: %w", err)
		}
		if attempt == coldStartRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff += coldStartInitialBackoff
	}
	return fmt.Errorf("neon: connect (compute may be suspended, retried %d times): %w", coldStartRetries, err)
}

func init() {
	engine.Register(engine.Registration{
		ID:          "neon",
		DisplayName: "Neon",
		Dialect:     engine.DialectPostgres,
		New:         func() engine.Driver { return &driver{} },
	})
}
