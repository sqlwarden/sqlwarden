package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/execution"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = filepath.Join(t.TempDir(), "sqlwarden.db")
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends = map[string]config.FileStorageBackend{
		config.DefaultFilesActiveBackend: {
			Type:    config.FilesStorageBackendFilesystem,
			RootDir: t.TempDir(),
		},
	}
	return cfg
}

// migrateTestDatabase applies migrations ahead of Build for topologies that
// reject db.automigrate on serving replicas.
func migrateTestDatabase(t *testing.T, cfg config.Config) {
	t.Helper()
	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.MigrateLocked(context.Background(), func(context.Context) error { return db.MigrateUp() }); err != nil {
		t.Fatal(err)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recordingKind is a process kind that records its lifecycle calls into a
// shared log so tests can assert start and close ordering.
type recordingKind struct {
	name     string
	events   *[]string
	startErr error
	closeErr error
	readyErr error
	done     chan error
	serving  bool
}

func newRecordingKind(name string, events *[]string) *recordingKind {
	return &recordingKind{name: name, events: events}
}

func (k *recordingKind) Name() string { return k.name }

func (k *recordingKind) Start(context.Context) error {
	*k.events = append(*k.events, "start:"+k.name)
	return k.startErr
}

func (k *recordingKind) Ready(context.Context) error { return k.readyErr }

func (k *recordingKind) Close(context.Context) error {
	*k.events = append(*k.events, "close:"+k.name)
	return k.closeErr
}

// servingKind reports a listener result, so tests can drive Run the way an HTTP
// process kind does.
type servingKind struct {
	*recordingKind
}

func newServingKind(name string, events *[]string) *servingKind {
	kind := newRecordingKind(name, events)
	kind.done = make(chan error, 1)
	kind.serving = true
	return &servingKind{recordingKind: kind}
}

func (k *servingKind) Done() <-chan error { return k.done }

func buildTestApp(t *testing.T, configured []string, kinds func(*Services) ([]ProcessKind, error)) *Application {
	t.Helper()
	cfg := testConfig(t)
	if configured != nil {
		cfg.ProcessKinds = configured
		cfg.DB.Automigrate = false
		migrateTestDatabase(t, cfg)
	}
	built, err := Build(context.Background(), Options{
		Config:       cfg,
		Logger:       testLogger(),
		ProcessKinds: kinds,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { built.Close(context.Background()) })
	return built
}

func TestBuildAcquiresResourcesInDependencyOrder(t *testing.T) {
	built := buildTestApp(t, nil, nil)

	want := []string{"application database", "database connection sessions", "query cursors"}
	got := built.ResourceOrder()
	if len(got) != len(want) {
		t.Fatalf("resource order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resource order = %v, want %v", got, want)
		}
	}
}

func TestBuildProducesCompleteServices(t *testing.T) {
	services := buildTestApp(t, nil, nil).Services

	if services.DB == nil || services.Enforcer == nil || services.PolicyEvaluator == nil || services.Keyring == nil ||
		services.ConnManager == nil || services.QueryCursors == nil || services.SchemaService == nil ||
		services.SchemaSnapshots == nil || services.CompletionService == nil ||
		services.FileStores == nil || services.JobStore == nil || services.Logger == nil || services.Edition == nil {
		t.Fatalf("Build left part of the service graph nil: %+v", services)
	}
}

func TestBuildSelectsExecutionRuntimeForProcessTopology(t *testing.T) {
	tests := []struct {
		name       string
		kinds      []string
		wantWorker bool
	}{
		{name: "all remains local", kinds: []string{config.ProcessKindAll}},
		{name: "api delegates to connector", kinds: []string{config.ProcessKindAPI}, wantWorker: true},
		{name: "co-located api and connector remains local", kinds: []string{config.ProcessKindAPI, config.ProcessKindConnector}},
		{name: "connector remains local", kinds: []string{config.ProcessKindConnector}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			services := buildTestApp(t, test.kinds, nil).Services
			_, isWorker := services.Execution.(*execution.WorkerRuntime)
			if isWorker != test.wantWorker {
				t.Fatalf("Execution is WorkerRuntime = %t, want %t", isWorker, test.wantWorker)
			}
			if services.LocalExecution == nil {
				t.Fatal("local execution runtime is missing")
			}
			wantServer := false
			for _, kind := range test.kinds {
				wantServer = wantServer || kind == config.ProcessKindConnector
			}
			if (services.ExecutionServer != nil) != wantServer {
				t.Fatalf("ExecutionServer present = %t, want %t", services.ExecutionServer != nil, wantServer)
			}
		})
	}
}

func TestBuildAllModeDoesNotRequireConnectorRPCConfiguration(t *testing.T) {
	cfg := testConfig(t)
	cfg.Connector.Address = ""
	cfg.Connector.ListenAddress = ""
	cfg.Connector.GrantSigningKey = ""
	cfg.Connector.Transport = ""
	built, err := Build(context.Background(), Options{Config: cfg, Logger: testLogger()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	if _, ok := built.Services.Execution.(*execution.LocalRuntime); !ok {
		t.Fatalf("all mode Execution = %T, want LocalRuntime", built.Services.Execution)
	}
	if built.Services.ExecutionServer != nil || built.Services.ConnectorServerCredentials != nil {
		t.Fatal("all mode unexpectedly constructed connector RPC transport")
	}
}

func TestBuildClosesAcquiredResourcesWhenProcessKindsFail(t *testing.T) {
	var opened *database.DB
	cfg := testConfig(t)

	_, err := Build(context.Background(), Options{
		Config: cfg,
		Logger: testLogger(),
		Prepare: []func(context.Context, *database.DB) error{
			func(_ context.Context, db *database.DB) error {
				opened = db
				return nil
			},
		},
		ProcessKinds: func(*Services) ([]ProcessKind, error) {
			return nil, errors.New("process kind wiring failed")
		},
	})
	if err == nil {
		t.Fatal("expected Build to fail when process kind wiring fails")
	}
	if opened == nil {
		t.Fatal("expected the prepare hook to run before process kinds are built")
	}
	if pingErr := opened.PingContext(context.Background()); pingErr == nil {
		t.Fatal("expected the application database to be closed after a failed build")
	}
}

func TestBuildClosesAcquiredResourcesWhenPrepareFails(t *testing.T) {
	var opened *database.DB
	cfg := testConfig(t)

	_, err := Build(context.Background(), Options{
		Config: cfg,
		Logger: testLogger(),
		Prepare: []func(context.Context, *database.DB) error{
			func(_ context.Context, db *database.DB) error {
				opened = db
				return errors.New("startup invariant failed")
			},
		},
	})
	if err == nil {
		t.Fatal("expected Build to fail when a prepare hook fails")
	}
	if pingErr := opened.PingContext(context.Background()); pingErr == nil {
		t.Fatal("expected the application database to be closed after a failed prepare hook")
	}
}

func TestStartAndCloseRunInReverseOrder(t *testing.T) {
	var events []string
	built := buildTestApp(t, []string{config.ProcessKindAPI, config.ProcessKindJobs}, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{
			newRecordingKind(config.ProcessKindAPI, &events),
			newRecordingKind(config.ProcessKindJobs, &events),
		}, nil
	})

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := "start:api,start:jobs,close:jobs,close:api"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("lifecycle events = %q, want %q", got, want)
	}
}

func TestStartClosesAlreadyStartedKindsWhenOneFails(t *testing.T) {
	var events []string
	failing := newRecordingKind(config.ProcessKindJobs, &events)
	failing.startErr = errors.New("listener bind failed")

	configured := []string{config.ProcessKindAPI, config.ProcessKindJobs, config.ProcessKindRealtime}
	built := buildTestApp(t, configured, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{
			newRecordingKind(config.ProcessKindAPI, &events),
			failing,
			newRecordingKind(config.ProcessKindRealtime, &events),
		}, nil
	})

	err := built.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to fail")
	}
	if !strings.Contains(err.Error(), `start process kind "jobs"`) {
		t.Fatalf("error = %v, want the failing process kind named", err)
	}

	want := "start:api,start:jobs,close:api"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("lifecycle events = %q, want %q", got, want)
	}
}

func TestCloseIsIdempotentAndBlocksRestart(t *testing.T) {
	var events []string
	built := buildTestApp(t, nil, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{newRecordingKind(config.ProcessKindAll, &events)}, nil
	})

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
	if got := strings.Join(events, ","); got != "start:all,close:all" {
		t.Fatalf("lifecycle events = %q, want a single close", got)
	}
	if err := built.Start(context.Background()); err == nil {
		t.Fatal("expected Start after Close to fail")
	}
}

func TestStartCannotRunProcessKindsTwice(t *testing.T) {
	var events []string
	built := buildTestApp(t, nil, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{newRecordingKind(config.ProcessKindAll, &events)}, nil
	})

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Start(context.Background()); err == nil {
		t.Fatal("expected a second Start call to fail")
	}
	if got := strings.Join(events, ","); got != "start:all" {
		t.Fatalf("lifecycle events = %q, want one start", got)
	}
}

func TestCloseReportsProcessKindFailure(t *testing.T) {
	var events []string
	failing := newRecordingKind(config.ProcessKindAll, &events)
	failing.closeErr = errors.New("drain failed")

	built := buildTestApp(t, nil, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{failing}, nil
	})

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := built.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), `close process kind "all"`) {
		t.Fatalf("Close error = %v, want the failing process kind named", err)
	}
}

func TestReadyReportsLifecycleState(t *testing.T) {
	var events []string
	kind := newRecordingKind(config.ProcessKindAll, &events)
	built := buildTestApp(t, nil, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{kind}, nil
	})

	if err := built.Ready(context.Background()); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Ready before Start = %v, want %v", err, ErrNotStarted)
	}
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Ready(context.Background()); err != nil {
		t.Fatalf("Ready after Start = %v, want nil", err)
	}

	kind.readyErr = errors.New("database unreachable")
	err := built.Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), `process kind "all" is not ready`) {
		t.Fatalf("Ready = %v, want the unready process kind named", err)
	}

	kind.readyErr = nil
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Ready(context.Background()); err == nil {
		t.Fatal("expected Ready to fail after Close")
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	var events []string
	built := buildTestApp(t, nil, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{newServingKind(config.ProcessKindAll, &events)}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := built.Run(ctx); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if got := strings.Join(events, ","); got != "start:all,close:all" {
		t.Fatalf("lifecycle events = %q", got)
	}
}

func TestRunStopsWhenServingProcessKindFails(t *testing.T) {
	var events []string
	kind := newServingKind(config.ProcessKindAll, &events)
	built := buildTestApp(t, nil, func(*Services) ([]ProcessKind, error) {
		return []ProcessKind{kind}, nil
	})

	kind.done <- errors.New("listener failed")

	err := built.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), `process kind "all" stopped`) {
		t.Fatalf("Run = %v, want the failed listener reported", err)
	}
	if got := strings.Join(events, ","); got != "start:all,close:all" {
		t.Fatalf("lifecycle events = %q", got)
	}
}

func TestBuildStartCloseDoesNotLeakGoroutines(t *testing.T) {
	cycle := func() {
		var events []string
		built, err := Build(context.Background(), Options{
			Config: testConfig(t),
			Logger: testLogger(),
			ProcessKinds: func(*Services) ([]ProcessKind, error) {
				return []ProcessKind{newRecordingKind(config.ProcessKindAll, &events)}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := built.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := built.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	cycle()
	baseline := settledGoroutines()
	for i := 0; i < 3; i++ {
		cycle()
	}

	if got := settledGoroutines(); got > baseline {
		t.Fatalf("goroutines after repeated build/start/close = %d, want at most %d", got, baseline)
	}
}

// settledGoroutines waits for goroutine teardown to finish before counting, so
// a reaper that has been told to stop is not mistaken for a leak.
func settledGoroutines() int {
	count := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		next := runtime.NumGoroutine()
		if next >= count {
			return count
		}
		count = next
	}
	return count
}

func TestBuildStartsNoReaperBeforeStart(t *testing.T) {
	built := buildTestApp(t, nil, nil)

	before := settledGoroutines()
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := runtime.NumGoroutine()

	if after <= before {
		t.Fatalf("goroutines before start = %d, after start = %d; expected Start to launch the idle reapers", before, after)
	}
}

func TestValidateProcessKindSelection(t *testing.T) {
	var events []string
	tests := []struct {
		name      string
		configure []string
		kinds     []ProcessKind
		wantErr   string
	}{
		{
			name:      "matching selection",
			configure: []string{config.ProcessKindAll},
			kinds:     []ProcessKind{newRecordingKind(config.ProcessKindAll, &events)},
		},
		{
			name:      "no kinds built",
			configure: []string{config.ProcessKindAll},
		},
		{
			name:      "configured but not built",
			configure: []string{config.ProcessKindAPI, config.ProcessKindJobs},
			kinds:     []ProcessKind{newRecordingKind(config.ProcessKindAPI, &events)},
			wantErr:   `process kind "jobs" is configured but was not built`,
		},
		{
			name:      "built but not configured",
			configure: []string{config.ProcessKindAPI},
			kinds: []ProcessKind{
				newRecordingKind(config.ProcessKindAPI, &events),
				newRecordingKind(config.ProcessKindJobs, &events),
			},
			wantErr: `process kind "jobs" was built but is not configured`,
		},
		{
			name:      "built twice",
			configure: []string{config.ProcessKindAll},
			kinds: []ProcessKind{
				newRecordingKind(config.ProcessKindAll, &events),
				newRecordingKind(config.ProcessKindAll, &events),
			},
			wantErr: `process kind "all" was built more than once`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.ProcessKinds = test.configure
			err := validateProcessKindSelection(cfg, test.kinds)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateProcessKindSelection() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("validateProcessKindSelection() = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestBuildRejectsInvalidConfiguration(t *testing.T) {
	cfg := testConfig(t)
	cfg.Connector.Replicas = 2

	_, err := Build(context.Background(), Options{Config: cfg, Logger: testLogger()})
	if err == nil {
		t.Fatal("expected Build to re-validate configuration")
	}
	if !strings.Contains(err.Error(), "connector.replicas") {
		t.Fatalf("error = %v, want the connector replica rule", err)
	}
}

func TestBuildDefaultsShutdownDeadline(t *testing.T) {
	built := buildTestApp(t, nil, nil)
	if built.shutdownDeadline != defaultShutdownDeadline {
		t.Fatalf("shutdown deadline = %s, want %s", built.shutdownDeadline, defaultShutdownDeadline)
	}
}

func TestEnsureSQLiteParentDir(t *testing.T) {
	cfg := config.Default()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = filepath.Join(t.TempDir(), "nested", "sqlwarden.db")
	if err := ensureSQLiteParentDir(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), Options{Config: cfg, Logger: testLogger()}); err != nil {
		t.Fatal(fmt.Errorf("build against a nested sqlite path: %w", err))
	}
}
