package desktop

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/sqlwarden/internal/web"
)

func TestDesktopConfigurationBootstrapsAndReopensApplication(t *testing.T) {
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	firstConfig, paths, err := LoadWebConfig(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	firstApp, err := web.New(firstConfig, logger)
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstApp.NewDesktopSession(context.Background())
	if err != nil {
		_ = firstApp.Close()
		t.Fatal(err)
	}
	if err := firstApp.Close(); err != nil {
		t.Fatal(err)
	}

	secondConfig, secondPaths, err := LoadWebConfig(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	secondApp, err := web.New(secondConfig, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondApp.Close() })
	second, err := secondApp.NewDesktopSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if first.Identity != second.Identity {
		t.Fatalf("desktop identity changed after reopen: first=%+v second=%+v", first.Identity, second.Identity)
	}
	if paths != secondPaths || paths.Database != secondConfig.DB.DSN {
		t.Fatalf("desktop paths changed after reopen: first=%+v second=%+v", paths, secondPaths)
	}
}
