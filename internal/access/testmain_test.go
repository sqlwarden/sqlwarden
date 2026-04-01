package access_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var pgTestDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	pgTestDSN = strings.TrimPrefix(strings.TrimPrefix(connStr, "postgres://"), "postgresql://")

	code := m.Run()

	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}
