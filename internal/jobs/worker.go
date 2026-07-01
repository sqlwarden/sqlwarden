package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

type WorkerConfig struct {
	WorkerID           string
	WorkerCount        int
	PollInterval       time.Duration
	ClaimLease         time.Duration
	CompletedRetention time.Duration
}

type Runner struct {
	store    *Store
	registry *Registry
	logger   *slog.Logger
	cfg      WorkerConfig
}

func NewRunner(store *Store, registry *Registry, logger *slog.Logger, cfg WorkerConfig) *Runner {
	if cfg.WorkerID == "" {
		cfg.WorkerID = "sqlwarden"
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.ClaimLease <= 0 {
		cfg.ClaimLease = 5 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{store: store, registry: registry, logger: logger, cfg: cfg}
}

// Run starts worker goroutines and blocks until ctx is cancelled and all
// in-flight jobs have stopped.
func (r *Runner) Run(ctx context.Context) {
	r.logger.InfoContext(ctx, "job runner started",
		"worker_id", r.cfg.WorkerID,
		"workers", r.cfg.WorkerCount,
		"poll_interval_ms", r.cfg.PollInterval.Milliseconds(),
		"claim_lease_ms", r.cfg.ClaimLease.Milliseconds(),
		"completed_retention_ms", r.cfg.CompletedRetention.Milliseconds(),
	)
	r.recoverExpired(ctx)

	var wg sync.WaitGroup
	for i := 0; i < r.cfg.WorkerCount; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			r.workerLoop(ctx, worker)
		}(i)
	}
	<-ctx.Done()
	wg.Wait()
	r.logger.InfoContext(context.Background(), "job runner stopped", "worker_id", r.cfg.WorkerID)
}

func (r *Runner) workerLoop(ctx context.Context, worker int) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	pruneTicker := time.NewTicker(time.Hour)
	defer pruneTicker.Stop()
	recoveryInterval := r.cfg.ClaimLease
	if recoveryInterval < time.Minute {
		recoveryInterval = time.Minute
	}
	recoveryTicker := time.NewTicker(recoveryInterval)
	defer recoveryTicker.Stop()
	for {
		if r.runOnce(ctx, worker) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-pruneTicker.C:
			if worker == 0 && r.cfg.CompletedRetention > 0 {
				r.pruneCompleted(ctx)
			}
		case <-recoveryTicker.C:
			if worker == 0 {
				r.recoverExpired(ctx)
			}
		}
	}
}

func (r *Runner) recoverExpired(ctx context.Context) {
	requeued, failed, err := r.store.RecoverExpiredRunning(ctx, time.Now())
	if err != nil {
		r.logger.ErrorContext(ctx, "job recovery failed", "error", err)
		return
	}
	if requeued > 0 || failed > 0 {
		r.logger.WarnContext(ctx, "job recovery processed stale running jobs", "requeued", requeued, "failed", failed)
	} else {
		r.logger.DebugContext(ctx, "job recovery found no stale running jobs")
	}
}

func (r *Runner) pruneCompleted(ctx context.Context) {
	processed, err := r.store.PruneCompleted(ctx, time.Now().Add(-r.cfg.CompletedRetention), 100)
	if err != nil {
		r.logger.ErrorContext(ctx, "completed job prune failed", "error", err)
		return
	}
	if processed > 0 {
		r.logger.InfoContext(ctx, "completed job prune processed batch", "processed", processed)
	}
}

func (r *Runner) runOnce(ctx context.Context, worker int) bool {
	job, found, err := r.store.ClaimDue(ctx, r.workerID(worker), time.Now(), r.cfg.ClaimLease)
	if err != nil {
		r.logger.ErrorContext(ctx, "job claim failed", "worker", worker, "error", err)
		return false
	}
	if !found {
		return false
	}
	r.logger.DebugContext(ctx, "job claimed", jobAttrs(job, worker, nil)...)
	r.execute(ctx, worker, job)
	return true
}

func (r *Runner) execute(parent context.Context, worker int, job Record) {
	def, ok := r.registry.Definition(job.Type)
	if !ok {
		_, err := r.store.FailOrRetry(parent, job, "unknown_job_type", "Job type is not registered.", false, 0)
		if err != nil {
			r.logger.ErrorContext(parent, "job unknown-type failure update failed", jobAttrs(job, worker, err)...)
		} else {
			r.logger.WarnContext(parent, "job failed because type is not registered", jobAttrs(job, worker, nil)...)
		}
		return
	}

	ctx, cancel := context.WithCancel(parent)
	if def.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, def.Timeout)
	}
	defer cancel()

	done := make(chan struct{})
	go r.heartbeat(ctx, job, worker, cancel, done)

	startedAt := time.Now()
	r.logger.InfoContext(ctx, "job started", jobAttrs(job, worker, nil)...)
	output, err := def.Handler.Handle(ctx, job)
	close(done)

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		exists, cancelled, hbErr := r.store.Heartbeat(context.Background(), job.ID, r.workerID(worker), r.cfg.ClaimLease)
		if hbErr != nil {
			r.logger.ErrorContext(context.Background(), "job cancellation state check failed", jobAttrs(job, worker, hbErr)...)
		}
		if exists && cancelled {
			if markErr := r.store.MarkCancelled(context.Background(), job.ID); markErr != nil {
				r.logger.ErrorContext(context.Background(), "job cancel update failed", jobAttrs(job, worker, markErr)...)
				return
			}
			r.logger.InfoContext(context.Background(), "job cancelled", append(jobAttrs(job, worker, nil), "duration_ms", time.Since(startedAt).Milliseconds())...)
			return
		}
	}

	if err != nil {
		code, message, retryable := jobError(err)
		status, updateErr := r.store.FailOrRetry(context.Background(), job, code, message, retryable, def.Backoff(job.Attempts))
		attrs := append(jobAttrs(job, worker, err), "status", status, "duration_ms", time.Since(startedAt).Milliseconds())
		if updateErr != nil {
			r.logger.ErrorContext(context.Background(), "job failure update failed", append(attrs, "update_error", updateErr)...)
			return
		}
		if status == StatusQueued {
			r.logger.WarnContext(context.Background(), "job retry scheduled", attrs...)
			return
		}
		r.logger.WarnContext(context.Background(), "job failed", attrs...)
		return
	}

	if err := r.store.Complete(context.Background(), job.ID, output); err != nil {
		r.logger.ErrorContext(context.Background(), "job completion update failed", jobAttrs(job, worker, err)...)
		return
	}
	r.logger.InfoContext(context.Background(), "job succeeded", append(jobAttrs(job, worker, nil), "duration_ms", time.Since(startedAt).Milliseconds())...)
}

func (r *Runner) heartbeat(ctx context.Context, job Record, worker int, cancel context.CancelFunc, done <-chan struct{}) {
	interval := r.cfg.ClaimLease / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			exists, cancelled, err := r.store.Heartbeat(ctx, job.ID, r.workerID(worker), r.cfg.ClaimLease)
			if err != nil {
				r.logger.ErrorContext(ctx, "job heartbeat failed", jobAttrs(job, worker, err)...)
				continue
			}
			if !exists {
				r.logger.WarnContext(ctx, "job heartbeat lost claim", jobAttrs(job, worker, nil)...)
				cancel()
				return
			}
			if cancelled {
				r.logger.InfoContext(ctx, "job cancellation requested", jobAttrs(job, worker, nil)...)
				cancel()
				return
			}
		}
	}
}

func (r *Runner) workerID(worker int) string {
	return r.cfg.WorkerID + "-" + strconv.Itoa(worker)
}

func jobAttrs(job Record, worker int, err error) []any {
	attrs := []any{
		"job.id", job.ID,
		"job.type", job.Type,
		"job.visibility", job.Visibility,
		"job.status", job.Status,
		"job.attempt", job.Attempts,
		"job.max_attempts", job.MaxAttempts,
		"job.priority", job.Priority,
		"job.run_at", job.RunAt,
		"worker", worker,
	}
	if job.OrgID != nil {
		attrs = append(attrs, "org.id", *job.OrgID)
	}
	if job.WorkspaceID != nil {
		attrs = append(attrs, "workspace.id", *job.WorkspaceID)
	}
	if job.OwnerAccountID != nil {
		attrs = append(attrs, "account.id", *job.OwnerAccountID)
	}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	return attrs
}
