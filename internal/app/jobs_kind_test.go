package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/platform/observability"
	"github.com/sqlwarden/internal/settings"
)

var instanceAdmin = access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: 1}, InstanceAdmin: true}

func startBuilt(t *testing.T, opts Options) *Application {
	t.Helper()
	built, err := Build(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	return built
}

func TestAllProcessRunsQueuedJobs(t *testing.T) {
	built := startBuilt(t, Options{Config: testConfig(t), Logger: discardLogger(), Edition: testEdition()})
	ctx := context.Background()
	const key = "process-kind-jobs"
	if _, _, err := built.infra.JobStore.EnqueueSingleton(ctx, jobs.EnqueueInput{
		Type:         "noop",
		SingletonKey: key,
		Visibility:   jobs.VisibilityInternal,
		MaxAttempts:  1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := built.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, active, err := built.infra.JobStore.ActiveBySingletonKey(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !active {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("queued job still active; the jobs kind did not run it")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSettingsKindAppliesPublishedChange(t *testing.T) {
	logger, err := observability.NewLogger("text", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	built := startBuilt(t, Options{Config: testConfig(t), Logger: logger, Edition: testEdition()})
	ctx := context.Background()
	if err := built.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !logger.Enabled(ctx, slog.LevelInfo) {
		t.Fatal("info logging disabled before the change")
	}
	level := "error"
	if _, err := built.services.Settings.UpdateInstance(ctx, instanceAdmin, settings.UpdateInstanceInput{LogLevel: &level}); err != nil {
		t.Fatal(err)
	}
	// The deadline is below the watcher's poll interval, so only the
	// published change can apply the new level in time.
	deadline := time.Now().Add(time.Second)
	for logger.Enabled(ctx, slog.LevelInfo) {
		if time.Now().After(deadline) {
			t.Fatal("instance settings change was not applied")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCloseCancelsInFlightJobDuringRunnerRestart(t *testing.T) {
	started := make(chan struct{})
	built := startBuilt(t, Options{Config: testConfig(t), Logger: discardLogger(), Edition: jobEdition{def: jobs.Definition{
		Type:        "blocking",
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(ctx context.Context, _ jobs.Runtime) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	}}})
	ctx := context.Background()
	if _, _, err := built.infra.JobStore.EnqueueSingleton(ctx, jobs.EnqueueInput{
		Type:         "blocking",
		SingletonKey: "close-blocking",
		Visibility:   jobs.VisibilityInternal,
		MaxAttempts:  1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := built.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("blocking job never started")
	}

	current, err := built.services.Settings.Instance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workers := current.JobsWorkerCount + 1
	if _, err := built.services.Settings.UpdateInstance(ctx, instanceAdmin, settings.UpdateInstanceInput{JobsWorkerCount: &workers}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	closed := make(chan error, 1)
	go func() { closed <- built.Close(ctx) }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked on an in-flight job during a runner restart")
	}
}

func TestBuildRejectsDuplicateEditionJob(t *testing.T) {
	_, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger(), Edition: jobEdition{def: jobs.Definition{
		Type:    "noop",
		Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) { return nil, nil }),
	}}})
	if err == nil {
		t.Fatal("Build accepted an edition job that duplicates a core job")
	}
	if openDatabases.Load() != 0 {
		t.Fatalf("%d databases left open after the failed build", openDatabases.Load())
	}
}

type jobEdition struct{ def jobs.Definition }

func (jobEdition) Name() string               { return "job" }
func (jobEdition) Licenser() edition.Licenser { return edition.NoopLicenser{} }
func (e jobEdition) Modules() []edition.Module {
	return []edition.Module{jobModule(e)}
}

type jobModule struct{ def jobs.Definition }

func (jobModule) Name() string                          { return "jobs" }
func (jobModule) Feature() string                       { return edition.Catalog[0].Key }
func (m jobModule) Register(r *edition.Registrar) error { return r.Job(m.def) }
