package web

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	pgTestDSN         string
	pgAdminDSN        string
	pgTemplateDBName  = "testdb"
	pgAdminDB         *sql.DB
	pgTestDBCounter   uint64
	pgTemplateCloneMu sync.Mutex
)

func dsnWithDatabase(connStr, dbName string) string {
	if idx := strings.LastIndex(connStr, "/"); idx != -1 {
		if q := strings.Index(connStr[idx:], "?"); q != -1 {
			return connStr[:idx+1] + dbName + connStr[idx+q:]
		}
		return connStr[:idx+1] + dbName
	}
	return connStr
}

func trimPostgresScheme(connStr string) string {
	return strings.TrimPrefix(strings.TrimPrefix(connStr, "postgres://"), "postgresql://")
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	restoreHashCost := password.SetHashCostForTesting(4)
	defer restoreHashCost()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		testcontainers.WithCmdArgs("-c", "max_connections=200"),
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

	pgAdminDSN = trimPostgresScheme(dsnWithDatabase(connStr, "postgres"))
	pgTestDSN = trimPostgresScheme(dsnWithDatabase(connStr, pgTemplateDBName))

	pgAdminDB = sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://" + pgAdminDSN)))
	if err := pgAdminDB.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect admin db: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	templateDB, err := database.New("postgres", pgTestDSN, nilLogger(), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open template db: %v\n", err)
		_ = pgAdminDB.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}
	if err := templateDB.MigrateUp(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to migrate template db: %v\n", err)
		templateDB.Close()
		_ = pgAdminDB.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}
	for _, user := range testUsers {
		acc, err := templateDB.InsertAccount(ctx, user.email, user.email, &user.hashedPassword)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to seed template account %s: %v\n", user.email, err)
			templateDB.Close()
			_ = pgAdminDB.Close()
			_ = pgContainer.Terminate(ctx)
			os.Exit(1)
		}
		user.id = acc.ID
	}
	templateDB.Close()

	if _, err := pgAdminDB.ExecContext(ctx,
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
		pgTemplateDBName,
	); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate template db connections: %v\n", err)
		_ = pgAdminDB.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	if _, err := pgAdminDB.ExecContext(ctx, fmt.Sprintf("ALTER DATABASE %s WITH ALLOW_CONNECTIONS false", pgTemplateDBName)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to lock template db connections: %v\n", err)
		_ = pgAdminDB.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	code := m.Run()

	_ = pgAdminDB.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
