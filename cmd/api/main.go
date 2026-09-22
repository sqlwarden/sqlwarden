package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
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

	built, err := app.Build(ctx, app.Options{
		Config:       loaded.Config,
		Logger:       logger,
		Prepare:      []func(context.Context, *database.DB) error{web.PrepareInstanceSettings(loaded.Config)},
		ProcessKinds: web.ProcessKinds,
	})
	if err != nil {
		return err
	}

	return built.Run(ctx)
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
	built, err := app.Build(ctx, app.Options{
		Config:  loaded.Config,
		Logger:  logger,
		Prepare: []func(context.Context, *database.DB) error{web.PrepareInstanceSettings(loaded.Config)},
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
