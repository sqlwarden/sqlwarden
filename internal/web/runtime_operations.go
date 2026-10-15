package web

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/platform/observability"
	"github.com/sqlwarden/internal/settings"
)

const runtimeSettingsRefreshInterval = 2 * time.Second

type runtimeOperations struct {
	LogLevel                      string
	DatabaseQueryTracingEnabled   bool
	AccessLogsEnabled             bool
	JobsWorkerCount               int
	JobsPollIntervalSeconds       int64
	JobsClaimLeaseSeconds         int64
	JobsCompletedRetentionSeconds int64
	SMTPEnabled                   bool
	SMTPHost                      string
	SMTPPort                      int
	SMTPUsername                  string
	SMTPPasswordEncrypted         string
	SMTPFrom                      string
}

func operationsFromSettings(in settings.Instance) runtimeOperations {
	return runtimeOperations{
		LogLevel: in.LogLevel, DatabaseQueryTracingEnabled: in.DatabaseQueryTracingEnabled,
		AccessLogsEnabled: in.AccessLogsEnabled,
		JobsWorkerCount:   in.JobsWorkerCount, JobsPollIntervalSeconds: in.JobsPollIntervalSeconds,
		JobsClaimLeaseSeconds: in.JobsClaimLeaseSeconds, JobsCompletedRetentionSeconds: in.JobsCompletedRetentionSeconds,
		SMTPEnabled: in.SMTPEnabled, SMTPHost: in.SMTPHost, SMTPPort: in.SMTPPort,
		SMTPUsername: in.SMTPUsername, SMTPPasswordEncrypted: in.SMTPPasswordEncrypted, SMTPFrom: in.SMTPFrom,
	}
}

func (o runtimeOperations) workerConfig() jobs.WorkerConfig {
	return jobs.WorkerConfig{
		WorkerID: "api", WorkerCount: o.JobsWorkerCount,
		PollInterval:       time.Duration(o.JobsPollIntervalSeconds) * time.Second,
		ClaimLease:         time.Duration(o.JobsClaimLeaseSeconds) * time.Second,
		CompletedRetention: time.Duration(o.JobsCompletedRetentionSeconds) * time.Second,
	}
}

func (app *application) applyRuntimeOperations(in settings.Instance) error {
	mailer, err := app.services.NewMailer(in)
	if err != nil {
		return err
	}
	if err := observability.SetLevel(app.logger, in.LogLevel); err != nil {
		return fmt.Errorf("apply runtime log level: %w", err)
	}
	app.services.SetQueryTracing(in.DatabaseQueryTracingEnabled)
	app.accessLogsEnabled.Store(in.AccessLogsEnabled)
	app.services.Mailer.Set(mailer)
	return nil
}

func (app *application) queueRuntimeOperations(in settings.Instance) {
	if app.runtimeUpdates == nil {
		return
	}
	select {
	case app.runtimeUpdates <- in:
	default:
		select {
		case <-app.runtimeUpdates:
		default:
		}
		app.runtimeUpdates <- in
	}
}

func (app *application) startRuntimeSupervisor(initial settings.Instance, runJobs bool) {
	if app.runtimeCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.runtimeCancel = cancel
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		app.runRuntimeSupervisor(ctx, initial, runJobs)
	}()
}

// runRuntimeSupervisor applies instance settings changes until ctx ends. The
// job runner is restarted on worker setting changes, and runs only when
// runJobs is set.
func (app *application) runRuntimeSupervisor(ctx context.Context, initial settings.Instance, runJobs bool) {
	current := operationsFromSettings(initial)
	var runnerStop chan struct{}
	var runnerDone chan struct{}

	startRunner := func(cfg jobs.WorkerConfig) {
		if !runJobs {
			return
		}
		runnerStop = make(chan struct{})
		runnerDone = make(chan struct{})
		runner := jobs.NewRunner(app.jobStore, app.jobRegistry, app.logger, cfg)
		go func() {
			defer close(runnerDone)
			runner.RunUntilStopped(ctx, runnerStop)
		}()
	}
	stopRunner := func() {
		if runnerStop != nil {
			close(runnerStop)
			<-runnerDone
			runnerStop = nil
		}
	}
	startRunner(current.workerConfig())
	defer stopRunner()

	apply := func(in settings.Instance) {
		next := operationsFromSettings(in)
		if next == current {
			return
		}
		if err := app.applyRuntimeOperations(in); err != nil {
			app.logger.ErrorContext(ctx, "runtime operations update rejected", "error", err)
			return
		}
		if next.workerConfig() != current.workerConfig() {
			stopRunner()
			startRunner(next.workerConfig())
		}
		current = next
		app.logger.InfoContext(ctx, "runtime operations updated")
	}

	ticker := time.NewTicker(runtimeSettingsRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-app.runtimeUpdates:
			apply(in)
		case <-ticker.C:
			in, err := app.services.Settings.Instance(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					app.logger.ErrorContext(ctx, "runtime operations refresh failed", "error", err)
				}
				continue
			}
			apply(in)
		}
	}
}
