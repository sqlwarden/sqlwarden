package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/sqlwarden/internal/settings"
)

// NewWorkerID returns an identifier for this process's job runner,
// <hostname>-<pid>-<8 hex>. Claims and heartbeats are keyed by it, so two
// replicas must never share one; the random suffix keeps containers that
// report the same hostname and pid apart.
func NewWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	var suffix [4]byte
	_, _ = rand.Read(suffix[:])
	return host + "-" + strconv.Itoa(os.Getpid()) + "-" + hex.EncodeToString(suffix[:])
}

// WorkerConfigFromSettings derives the runner configuration the instance
// settings describe.
func WorkerConfigFromSettings(in settings.Instance, workerID string) WorkerConfig {
	return WorkerConfig{
		WorkerID:           workerID,
		WorkerCount:        in.JobsWorkerCount,
		PollInterval:       time.Duration(in.JobsPollIntervalSeconds) * time.Second,
		ClaimLease:         time.Duration(in.JobsClaimLeaseSeconds) * time.Second,
		CompletedRetention: time.Duration(in.JobsCompletedRetentionSeconds) * time.Second,
	}
}

// Supervisor owns the job runner of a process and restarts it when its
// configuration changes. A restart drains in-flight jobs before the new
// runner starts. Once stopped it never starts a runner again, so a
// reconfiguration that races shutdown cannot leave one behind.
type Supervisor struct {
	store    *DatabaseStore
	registry *Registry
	logger   *slog.Logger

	// base parents every runner context. Stop cancels it before taking mu,
	// so an Apply that holds mu while draining a blocked job unblocks.
	base       context.Context
	cancelBase context.CancelFunc

	mu        sync.Mutex
	cfg       WorkerConfig
	stop      chan struct{}
	done      chan struct{}
	cancelRun context.CancelFunc
	stopped   bool
}

// NewSupervisor returns a supervisor with no runner. Apply starts one.
func NewSupervisor(store *DatabaseStore, registry *Registry, logger *slog.Logger) *Supervisor {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	base, cancel := context.WithCancel(context.Background())
	return &Supervisor{store: store, registry: registry, logger: logger, base: base, cancelBase: cancel}
}

// Apply starts the runner with cfg, or restarts it when cfg differs from the
// running configuration. It does nothing after Stop. If ctx is cancelled while
// a restart waits for in-flight jobs, those jobs are cancelled and no new
// runner starts; a later Apply starts one.
func (s *Supervisor) Apply(ctx context.Context, cfg WorkerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	if s.stop != nil {
		if cfg == s.cfg {
			return
		}
		s.logger.InfoContext(ctx, "job runner restarting for new configuration", "worker_id", cfg.WorkerID, "workers", cfg.WorkerCount)
		s.drainLocked(ctx)
	}
	if s.base.Err() != nil || ctx.Err() != nil {
		return
	}
	s.startLocked(cfg)
}

// Stop cancels in-flight jobs, stops the runner, and waits for it to return.
func (s *Supervisor) Stop() {
	s.cancelBase()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	s.drainLocked(context.Background())
}

func (s *Supervisor) startLocked(cfg WorkerConfig) {
	s.cfg = cfg
	stop, done := make(chan struct{}), make(chan struct{})
	runCtx, cancelRun := context.WithCancel(s.base)
	s.stop, s.done, s.cancelRun = stop, done, cancelRun
	runner := NewRunner(s.store, s.registry, s.logger, cfg)
	go func() {
		defer close(done)
		defer cancelRun()
		runner.RunUntilStopped(runCtx, stop)
	}()
}

// drainLocked stops the runner from claiming work and waits for it to return.
// When ctx ends first, the runner's in-flight jobs are cancelled.
func (s *Supervisor) drainLocked(ctx context.Context) {
	if s.stop == nil {
		return
	}
	close(s.stop)
	select {
	case <-s.done:
	case <-ctx.Done():
		s.cancelRun()
		<-s.done
	}
	s.cancelRun()
	s.stop, s.done, s.cancelRun = nil, nil, nil
}
