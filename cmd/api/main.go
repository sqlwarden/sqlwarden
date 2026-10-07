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
	"github.com/sqlwarden/internal/platform/observability"
	"github.com/sqlwarden/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		bootstrapLogger().Error(err.Error(), "trace", string(debug.Stack()))
		os.Exit(1)
	}
}

func bootstrapLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func parseCommand(args []string) (app.Command, []string) {
	if len(args) == 0 {
		return app.CommandServe, args
	}
	switch args[0] {
	case "serve":
		return app.CommandServe, args[1:]
	case config.MigrateCommand:
		return app.CommandMigrate, args[1:]
	case "rotate-keys":
		return app.CommandRotateKeys, args[1:]
	default:
		return app.CommandServe, args
	}
}

func run(args []string) error {
	command, rest := parseCommand(args)
	loaded, err := config.Load(rest)
	if err != nil {
		return err
	}
	if loaded.ShowVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}
	logger, err := observability.NewLogger(loaded.Config.Log.Format, os.Stdout)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	built, err := app.Build(ctx, app.Options{Config: loaded.Config, Logger: logger, Command: command})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := built.Close(context.WithoutCancel(ctx)); closeErr != nil {
			logger.Error("application close failed", "error", closeErr)
		}
	}()

	switch command {
	case app.CommandMigrate:
		return nil
	case app.CommandRotateKeys:
		return rotateKeys(ctx, built, logger)
	default:
		return built.Run(ctx)
	}
}

// rotateKeys re-encrypts all application-encrypted data with the primary key,
// decrypting through any retired keys in encryption.previous_keys.
//
// It runs at infrastructure trust level: anyone who can run the binary with
// the deployment's config and database already holds the keys, so no
// application authorization applies. It is the CLI equivalent of the
// instance-admin rotate endpoint.
func rotateKeys(ctx context.Context, built *app.Application, logger *slog.Logger) error {
	report, err := built.RotateEncryptionKeys(ctx)
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
