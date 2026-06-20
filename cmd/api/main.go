package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

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

	cfg, showVersion, err := web.LoadConfig(args)
	if err != nil {
		return err
	}

	if showVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}

	logger, err := web.NewLogger(cfg, os.Stdout)
	if err != nil {
		return err
	}

	app, err := web.New(cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return app.ServeHTTP(ctx)
}

// runRotateKeys re-encrypts all application-encrypted data (connection DSNs and
// any application-encrypted file content) with the configured primary
// encryption key, decrypting through any retired keys in ENCRYPTION_PREVIOUS_KEYS.
//
// It runs at infrastructure trust level: anyone who can execute the binary with
// the deployment's config and database already holds the keys, so no
// application-level authorization is applied. It is the CLI equivalent of the
// instance-admin HTTP rotate endpoint.
func runRotateKeys(args []string) error {
	cfg, _, err := web.LoadConfig(args)
	if err != nil {
		return err
	}

	logger, err := web.NewLogger(cfg, os.Stdout)
	if err != nil {
		return err
	}

	app, err := web.New(cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	report, err := app.RotateEncryptionKeys(context.Background())
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
