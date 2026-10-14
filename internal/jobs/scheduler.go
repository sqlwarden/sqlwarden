package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Schedule enqueues work periodically. Enqueue must be idempotent while a
// previous run is still active, typically through EnqueueSingleton, because
// every process that runs jobs runs the same schedules.
type Schedule struct {
	Name    string
	Every   time.Duration
	Enqueue func(ctx context.Context) error
}

// Scheduler runs each Schedule on its own goroutine. Each schedule enqueues
// once when started and then once per interval.
type Scheduler struct {
	logger    *slog.Logger
	schedules []Schedule

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewScheduler returns a stopped scheduler. Schedules without an Enqueue
// function or a positive interval are ignored.
func NewScheduler(logger *slog.Logger, schedules ...Schedule) *Scheduler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	kept := make([]Schedule, 0, len(schedules))
	for _, schedule := range schedules {
		if schedule.Enqueue == nil || schedule.Every <= 0 {
			logger.Warn("job schedule ignored", "schedule", schedule.Name, "interval_ms", schedule.Every.Milliseconds())
			continue
		}
		kept = append(kept, schedule)
	}
	return &Scheduler{logger: logger, schedules: kept}
}

// Start launches the schedules. Only the first call has an effect, so a
// stopped scheduler cannot be started again.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	for _, schedule := range s.schedules {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.run(ctx, schedule)
		}()
	}
}

// Run starts the scheduler and blocks until ctx is cancelled or Stop is
// called.
func (s *Scheduler) Run(ctx context.Context) {
	s.Start(ctx)
	s.wg.Wait()
}

// Stop cancels the schedules and waits for any enqueue in progress to return.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	s.started = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

func (s *Scheduler) run(ctx context.Context, schedule Schedule) {
	s.logger.Info("job schedule started", "schedule", schedule.Name, "interval_ms", schedule.Every.Milliseconds())
	ticker := time.NewTicker(schedule.Every)
	defer ticker.Stop()
	for {
		if err := schedule.Enqueue(ctx); err != nil && ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "job schedule enqueue failed", "schedule", schedule.Name, "error", err)
		}
		select {
		case <-ctx.Done():
			s.logger.Info("job schedule stopped", "schedule", schedule.Name)
			return
		case <-ticker.C:
		}
	}
}
