package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/community"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/version"
	"github.com/sqlwarden/internal/web"
)

func main() {
	err := run(os.Args[1:])
	if err != nil {
		trace := string(debug.Stack())
		bootstrapLogger().Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

func bootstrapLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func run(args []string) error {
	if len(args) > 0 && args[0] == config.MigrateCommand {
		return runMigrate(args[1:])
	}
	if len(args) > 0 && args[0] == "rotate-keys" {
		return runRotateKeys(args[1:])
	}

	loaded, err := config.Load(args)
	if err != nil {
		return err
	}

	if loaded.ShowVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}

	logger, err := web.NewLogger(loaded.Config, os.Stdout)
	if err != nil {
		return err
	}
	logger.Info("effective configuration resolved", "configuration", loaded.Diagnostic)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	built, err := community.Build(ctx, app.Options{
		Config:       loaded.Config,
		Logger:       logger,
		ProcessKinds: web.ProcessKinds,
	})
	if err != nil {
		return err
	}

	return built.Run(ctx)
}

// runMigrate applies the core and edition migration streams to the application
// database and exits.
//
// It exists so topologies that run several serving replicas can migrate once,
// before those replicas start, instead of letting every replica race to migrate
// on boot. The run holds the migration lock and signals cancellation after
// db.migration_timeout, so a second concurrent run waits rather than
// interleaving. The lock remains held until the migration runner has actually
// stopped, even if a database statement does not respond to cancellation
// immediately.
//
// It opens only the application database: no listener, no background worker,
// and no service graph.
func runMigrate(args []string) error {
	loaded, err := config.Load(args)
	if err != nil {
		return err
	}
	if err := config.Normalize(&loaded.Config); err != nil {
		return err
	}

	logger, err := web.NewLogger(loaded.Config, os.Stdout)
	if err != nil {
		return err
	}

	selectedEdition := edition.NewCommunity()
	if err := edition.Validate(selectedEdition, loaded.Config); err != nil {
		return err
	}

	db, err := database.New(loaded.Config.DB.Driver, loaded.Config.DB.DSN, logger)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), loaded.Config.DB.MigrationTimeout)
	defer cancel()

	logger.Info("database migration started",
		"driver", loaded.Config.DB.Driver,
		"edition", selectedEdition.Name(),
		"timeout_ms", loaded.Config.DB.MigrationTimeout.Milliseconds(),
	)
	startedAt := time.Now()
	err = db.MigrateLocked(ctx, func(ctx context.Context) error {
		if err := db.MigrateUp(); err != nil {
			return err
		}
		return edition.Migrate(ctx, selectedEdition, db)
	})
	if err != nil {
		return err
	}
	logger.Info("database migration complete", "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}

// runRotateKeys re-encrypts all application-encrypted data (connection DSNs,
// SMTP credentials, and application-encrypted file content) with the configured primary
// encryption key, decrypting through any retired keys in ENCRYPTION_PREVIOUS_KEYS.
//
// It runs at infrastructure trust level: anyone who can execute the binary with
// the deployment's config and database already holds the keys, so no
// application-level authorization is applied. It is the CLI equivalent of the
// instance-admin HTTP rotate endpoint.
//
// It builds the service graph without process kinds, so no listener and no
// background worker runs while data is being rewritten.
func runRotateKeys(args []string) error {
	loaded, err := config.Load(args)
	if err != nil {
		return err
	}

	logger, err := web.NewLogger(loaded.Config, os.Stdout)
	if err != nil {
		return err
	}

	ctx := context.Background()
	built, err := community.Build(ctx, app.Options{
		Config: loaded.Config,
		Logger: logger,
	})
	if err != nil {
		return err
	}
	defer built.Close(ctx)

	report, err := web.NewApplication(built.Services).RotateEncryptionKeys(ctx)
	if err != nil {
		return err
	}

	logger.Info("encryption key rotation complete",
		"connections_scanned", report.ConnectionsScanned,
		"connections_rotated", report.ConnectionsRotated,
		"file_contents_scanned", report.FileContentsScanned,
		"file_contents_rotated", report.FileContentsRotated,
	)
	return nil
}
