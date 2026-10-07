package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
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

var subcommands = []string{"serve", config.MigrateCommand, "rotate-keys"}

func commandByName(name string) (app.Command, bool) {
	switch name {
	case "serve":
		return app.CommandServe, true
	case config.MigrateCommand:
		return app.CommandMigrate, true
	case "rotate-keys":
		return app.CommandRotateKeys, true
	}
	return app.CommandServe, false
}

func unknownSubcommandError(name string) error {
	return fmt.Errorf("unknown subcommand %q; valid subcommands are %s", name, strings.Join(subcommands, ", "))
}

// parseCommand splits a leading subcommand from the flags. Arguments that
// start with "-" belong to the default serve command, and so does an empty
// argument list. Any other first argument must be a known subcommand.
func parseCommand(args []string) (command app.Command, rest []string, explicit bool, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return app.CommandServe, args, false, nil
	}
	command, ok := commandByName(args[0])
	if !ok {
		return app.CommandServe, nil, false, unknownSubcommandError(args[0])
	}
	return command, args[1:], true, nil
}

// resolveCommand accepts one subcommand placed after the flags, such as
// "--config x.yaml migrate", when no subcommand led the argument list.
func resolveCommand(command app.Command, explicit bool, positional []string) (app.Command, error) {
	if len(positional) == 0 {
		return command, nil
	}
	if explicit {
		return command, fmt.Errorf("unexpected argument %q after the subcommand", positional[0])
	}
	trailing, ok := commandByName(positional[0])
	if !ok {
		return command, unknownSubcommandError(positional[0])
	}
	if len(positional) > 1 {
		return command, fmt.Errorf("unexpected argument %q after subcommand %q", positional[1], positional[0])
	}
	return trailing, nil
}

func run(args []string) error {
	command, rest, explicit, err := parseCommand(args)
	if err != nil {
		return err
	}
	loaded, err := config.Load(rest)
	if err != nil {
		return err
	}
	command, err = resolveCommand(command, explicit, loaded.Args)
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

	selected, err := selectedEdition(loaded.Config)
	if err != nil {
		return err
	}
	built, err := app.Build(ctx, app.Options{Config: loaded.Config, Logger: logger, Command: command, Edition: selected})
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
