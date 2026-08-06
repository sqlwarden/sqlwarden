package web

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/smtp"
)

const runtimeSettingsRefreshInterval = 2 * time.Second

type runtimeOperations struct {
	LogLevel                      string
	DatabaseQueryTracingEnabled   bool
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

func operationsFromSettings(settings database.InstanceSettings) runtimeOperations {
	return runtimeOperations{
		LogLevel: settings.LogLevel, DatabaseQueryTracingEnabled: settings.DatabaseQueryTracingEnabled,
		JobsWorkerCount: settings.JobsWorkerCount, JobsPollIntervalSeconds: settings.JobsPollIntervalSeconds,
		JobsClaimLeaseSeconds: settings.JobsClaimLeaseSeconds, JobsCompletedRetentionSeconds: settings.JobsCompletedRetentionSeconds,
		SMTPEnabled: settings.SMTPEnabled, SMTPHost: settings.SMTPHost, SMTPPort: settings.SMTPPort,
		SMTPUsername: settings.SMTPUsername, SMTPPasswordEncrypted: settings.SMTPPasswordEncrypted, SMTPFrom: settings.SMTPFrom,
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

func (app *application) applyRuntimeOperations(settings database.InstanceSettings) error {
	mailer := smtp.NewDisabledMailer(settings.SMTPFrom)
	if settings.SMTPEnabled {
		password := ""
		if settings.SMTPPasswordEncrypted != "" {
			var err error
			password, err = app.keyring.Decrypt(settings.SMTPPasswordEncrypted)
			if err != nil {
				return fmt.Errorf("decrypt SMTP password: %w", err)
			}
		}
		var err error
		mailer, err = smtp.NewMailer(settings.SMTPHost, settings.SMTPPort, settings.SMTPUsername, password, settings.SMTPFrom)
		if err != nil {
			return fmt.Errorf("configure SMTP: %w", err)
		}
	}
	if err := setLoggerLevel(app.logger, settings.LogLevel); err != nil {
		return fmt.Errorf("apply runtime log level: %w", err)
	}
	app.db.SetQueryTracing(settings.DatabaseQueryTracingEnabled)
	app.mailerMu.Lock()
	app.mailer = mailer
	app.mailerMu.Unlock()
	return nil
}

func (app *application) sendEmail(once bool, recipient string, data any, patterns ...string) error {
	app.mailerMu.RLock()
	defer app.mailerMu.RUnlock()
	if app.mailer == nil {
		return smtp.ErrDisabled
	}
	if once {
		return app.mailer.SendOnce(recipient, data, patterns...)
	}
	return app.mailer.Send(recipient, data, patterns...)
}

func (app *application) queueRuntimeOperations(settings database.InstanceSettings) {
	if app.runtimeUpdates == nil {
		return
	}
	select {
	case app.runtimeUpdates <- settings:
	default:
		select {
		case <-app.runtimeUpdates:
		default:
		}
		app.runtimeUpdates <- settings
	}
}

func (app *application) startRuntimeSupervisor(initial database.InstanceSettings) {
	if app.runtimeCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.runtimeCancel = cancel
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		app.runRuntimeSupervisor(ctx, initial)
	}()
}

func (app *application) runRuntimeSupervisor(ctx context.Context, initial database.InstanceSettings) {
	current := operationsFromSettings(initial)
	var runnerStop chan struct{}
	var runnerDone chan struct{}

	startRunner := func(cfg jobs.WorkerConfig) {
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

	apply := func(settings database.InstanceSettings) {
		next := operationsFromSettings(settings)
		if next == current {
			return
		}
		if err := app.applyRuntimeOperations(settings); err != nil {
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
		case settings := <-app.runtimeUpdates:
			apply(settings)
		case <-ticker.C:
			settings, err := app.instanceSettings(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					app.logger.ErrorContext(ctx, "runtime operations refresh failed", "error", err)
				}
				continue
			}
			apply(settings)
		}
	}
}
