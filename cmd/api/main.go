package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/lmittmann/tint"
	"github.com/sqlwarden/internal/version"
	"github.com/sqlwarden/internal/web"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	err := run(logger, os.Args[1:])
	if err != nil {
		trace := string(debug.Stack())
		logger.Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, args []string) error {
	cfg, showVersion, err := web.LoadConfig(args)
	if err != nil {
		return err
	}

	if showVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
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
