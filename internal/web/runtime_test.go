package web

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"

	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/jobs"
)

func TestProcessKindsBuildsSupportedTopology(t *testing.T) {
	services := &coreapp.Services{Config: config.Default()}
	services.Config.ProcessKinds = []string{config.ProcessKindAPI, config.ProcessKindConnector}

	kinds, err := ProcessKinds(services)
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 || kinds[0].Name() != config.ProcessKindAPI || kinds[1].Name() != config.ProcessKindConnector {
		t.Fatalf("ProcessKinds() = %#v", kinds)
	}
}

func TestProcessKindsRejectsUnimplementedKind(t *testing.T) {
	services := &coreapp.Services{Config: config.Default()}
	services.Config.ProcessKinds = []string{config.ProcessKindJobs}
	if _, err := ProcessKinds(services); err == nil {
		t.Fatal("expected unimplemented jobs process kind to fail")
	}
}

func TestSeparateAPIAndConnectorProcessesExecuteWithoutAffinity(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	connectorAddress := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	connector := buildRuntimeProcess(t, []string{config.ProcessKindConnector}, connectorAddress)
	if err := connector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connector.Close(context.Background()) })

	api := buildRuntimeProcess(t, []string{config.ProcessKindAPI}, connectorAddress)
	if err := api.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.Close(context.Background()) })
	if err := connector.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := api.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	opened, err := api.Services.Execution.Open(context.Background(), execution.OpenRequest{
		Scope:  execution.Scope{TenantID: "1", AccountID: "2", WorkspaceID: "3", ConnectionID: "4"},
		Target: execution.Target{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "target.db")},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := api.Services.Execution.Query(context.Background(), execution.QueryRequest{Handle: opened.Handle, SQL: "SELECT 1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || len(result.Result.Rows) != 1 {
		t.Fatalf("Query() = %+v", result)
	}
}

func buildRuntimeProcess(t *testing.T, kinds []string, connectorAddress string) *coreapp.Application {
	t.Helper()
	cfg := config.Default()
	cfg.ProcessKinds = kinds
	if len(kinds) == 1 && kinds[0] == config.ProcessKindAPI {
		cfg.HTTPPort = 0
	}
	cfg.Connector.Address = connectorAddress
	cfg.Connector.ListenAddress = connectorAddress
	cfg.DB.DSN = filepath.Join(t.TempDir(), "metadata.db")
	cfg.Files.StorageBackends[config.DefaultFilesActiveBackend] = config.FileStorageBackend{
		Type: config.FilesStorageBackendFilesystem, RootDir: t.TempDir(),
	}
	built, err := coreapp.Build(context.Background(), coreapp.Options{
		Config: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), ProcessKinds: ProcessKinds,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	return built
}

func TestAPIProcessKindRunsBackgroundWorkers(t *testing.T) {
	api := buildRuntimeProcess(t, []string{config.ProcessKindAPI}, "127.0.0.1:1")
	kind, ok := api.ProcessKinds[0].(*apiProcessKind)
	if !ok {
		t.Fatalf("ProcessKinds[0] = %T, want *apiProcessKind", api.ProcessKinds[0])
	}
	if err := kind.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kind.Close(context.Background()) })
	if kind.app.jobRegistry == nil {
		t.Fatal("API process started a job runner without a registry; queued jobs would fail as unknown_job_type")
	}
	if _, found := kind.app.jobRegistry.Definition(jobs.TypeFileContentReap); !found {
		t.Fatal("file content reap job type is not registered in an API process")
	}
	if kind.app.fileReaperCancel == nil {
		t.Fatal("API process did not start the file content deletion reaper")
	}
}
