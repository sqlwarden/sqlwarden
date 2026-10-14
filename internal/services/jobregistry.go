package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/jobs"
)

const (
	fileReapInterval  = 15 * time.Minute
	fileReapRetry     = time.Minute
	fileReapBatchSize = 100
	fileReapAttempts  = 3
)

// FileContentReaper deletes stored content of deleted workspace files in
// batches.
type FileContentReaper interface {
	ReapContentDeletionsOnce(ctx context.Context, batchSize int, retryAfter time.Duration) (int, error)
}

// ExportJobRunner runs a background export job.
type ExportJobRunner interface {
	RunJob(ctx context.Context, runtime jobs.Runtime) (any, error)
}

// JobRegistryDeps are the handlers' collaborators. Edition definitions are
// added after the built-in ones and may not reuse a built-in type.
type JobRegistryDeps struct {
	Files   FileContentReaper
	Exports ExportJobRunner
	Revoker jobs.SessionRevoker
	Logger  *slog.Logger
	Edition []jobs.Definition
}

// JobRegistry builds the registry of every job type this build runs.
func JobRegistry(deps JobRegistryDeps) (*jobs.Registry, error) {
	if deps.Files == nil || deps.Exports == nil {
		return nil, errors.New("services: job registry requires the file reaper and export runner")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	registry := jobs.NewRegistry()
	registry.Register(jobs.Definition{
		Type:        "noop",
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
			return map[string]any{"ok": true}, nil
		}),
	})
	registry.Register(jobs.Definition{
		Type:        jobs.TypeFileContentReap,
		MaxAttempts: fileReapAttempts,
		Backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Minute
		},
		Handler: jobs.HandlerFunc(func(ctx context.Context, _ jobs.Runtime) (any, error) {
			processed, err := deps.Files.ReapContentDeletionsOnce(ctx, fileReapBatchSize, fileReapRetry)
			if err != nil {
				return nil, jobs.Retryable("file_content_reap_failed", err.Error())
			}
			if processed > 0 {
				logger.InfoContext(ctx, "file content deletion job processed batch", "processed", processed)
			} else {
				logger.DebugContext(ctx, "file content deletion job found no work")
			}
			return map[string]any{"processed": processed}, nil
		}),
	})
	registry.Register(jobs.Definition{
		Type:        jobs.TypeExportQueryCSV,
		MaxAttempts: 1,
		Handler:     jobs.HandlerFunc(deps.Exports.RunJob),
	})
	registry.Register(jobs.Definition{
		Type:        jobs.TypeSessionRevoke,
		MaxAttempts: jobs.SessionRevokeMaxAttempts,
		Backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * 15 * time.Second
		},
		Handler: jobs.SessionRevokeHandler(deps.Revoker),
	})
	for _, definition := range deps.Edition {
		if _, exists := registry.Definition(definition.Type); exists {
			return nil, fmt.Errorf("services: duplicate job definition %q", definition.Type)
		}
		registry.Register(definition)
	}
	return registry, nil
}

// JobRegistry builds the job registry over this service graph, with the
// edition's definitions added.
func (i *Infra) JobRegistry(edition ...jobs.Definition) (*jobs.Registry, error) {
	return JobRegistry(JobRegistryDeps{
		Files:   i.services.Files,
		Exports: i.services.Exports,
		Revoker: i.services.deps.Revoker,
		Logger:  i.services.deps.Logger,
		Edition: edition,
	})
}

// FileReapSchedule enqueues the file content deletion job every 15 minutes
// unless one is already queued or running.
func (i *Infra) FileReapSchedule() jobs.Schedule {
	return jobs.Schedule{
		Name:    jobs.TypeFileContentReap,
		Every:   fileReapInterval,
		Enqueue: i.enqueueFileContentReap,
	}
}

func (i *Infra) enqueueFileContentReap(ctx context.Context) error {
	logger := i.services.deps.Logger
	active, err := i.JobStore.HasActiveJobType(ctx, jobs.VisibilityInternal, jobs.TypeFileContentReap)
	if err != nil {
		return err
	}
	if active {
		logger.DebugContext(ctx, "file content deletion reaper job already active", "job.type", jobs.TypeFileContentReap)
		return nil
	}
	job, created, err := i.JobStore.EnqueueSingleton(ctx, jobs.EnqueueInput{
		Type:         jobs.TypeFileContentReap,
		SingletonKey: jobs.TypeFileContentReap,
		Visibility:   jobs.VisibilityInternal,
		Priority:     jobs.PriorityLow,
		MaxAttempts:  fileReapAttempts,
	})
	if errors.Is(err, jobs.ErrActiveExists) {
		logger.DebugContext(ctx, "file content deletion reaper job already active", "job.type", jobs.TypeFileContentReap)
		return nil
	}
	if err != nil {
		return err
	}
	if created {
		logger.InfoContext(ctx, "file content deletion reaper job queued", "job.id", job.ID, "job.type", job.Type, "job.priority", job.Priority)
	}
	return nil
}
