package settings

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/sqlwarden/internal/platform/invalidate"
)

// watchPollInterval is how often a Watcher re-reads the instance settings
// without a message. It bounds how long a lost invalidation stays unapplied.
const watchPollInterval = 2 * time.Second

// Applier puts instance settings into effect in this process. An error
// leaves the previously applied settings recorded, so the next wake retries.
type Applier func(ctx context.Context, in Instance) error

// Watcher applies stored instance settings changes in this process. It wakes
// on every TopicInstanceSettings message and on a poll, re-reads the settings,
// and runs its appliers in order when they differ from the last applied ones.
type Watcher struct {
	svc      *Service
	inv      invalidate.Invalidator
	logger   *slog.Logger
	appliers []Applier
	poll     time.Duration
	wake     chan struct{}

	stateMu sync.Mutex
	last    Instance
	applied bool

	lifeMu      sync.Mutex
	started     bool
	cancel      context.CancelFunc
	done        chan struct{}
	unsubscribe func()
}

// NewWatcher returns a stopped watcher. A nil inv leaves only the poll.
func NewWatcher(svc *Service, inv invalidate.Invalidator, logger *slog.Logger, appliers ...Applier) *Watcher {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Watcher{
		svc:      svc,
		inv:      inv,
		logger:   logger,
		appliers: appliers,
		poll:     watchPollInterval,
		wake:     make(chan struct{}, 1),
	}
}

// Seed records in as already applied. Callers that apply the initial
// settings synchronously seed the watcher so it does not apply them again.
func (w *Watcher) Seed(in Instance) {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	w.last = in
	w.applied = true
}

// Start subscribes to the instance settings topic and starts the watch
// goroutine. Only the first call has an effect, so a stopped watcher cannot be
// started again.
func (w *Watcher) Start(ctx context.Context) {
	w.lifeMu.Lock()
	defer w.lifeMu.Unlock()
	if w.started {
		return
	}
	w.started = true
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	if w.inv != nil {
		w.unsubscribe = w.inv.Subscribe(invalidate.TopicInstanceSettings, func(string) { w.signal() })
	}
	go w.run(ctx, w.done)
}

// Stop unsubscribes, cancels the watch goroutine, and waits for it to return.
func (w *Watcher) Stop() {
	w.lifeMu.Lock()
	cancel, done, unsubscribe := w.cancel, w.done, w.unsubscribe
	w.cancel, w.done, w.unsubscribe = nil, nil, nil
	w.lifeMu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
	if cancel != nil {
		cancel()
		<-done
	}
}

// signal coalesces wakes into the size-1 channel and never blocks, because
// it runs on the publisher's goroutine.
func (w *Watcher) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Watcher) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
		}
		w.refresh(ctx)
	}
}

// sameSettings compares the settings values only; timestamps change on every
// save, including saves that change nothing.
func sameSettings(a, b Instance) bool {
	a.CreatedAt, a.UpdatedAt = time.Time{}, time.Time{}
	b.CreatedAt, b.UpdatedAt = time.Time{}, time.Time{}
	return a == b
}

func (w *Watcher) refresh(ctx context.Context) {
	in, err := w.svc.Instance(ctx)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
			w.logger.ErrorContext(ctx, "runtime operations refresh failed", "error", err)
		}
		return
	}
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if w.applied && sameSettings(w.last, in) {
		return
	}
	for _, apply := range w.appliers {
		if err := apply(ctx, in); err != nil {
			w.logger.ErrorContext(ctx, "runtime operations update rejected", "error", err)
			return
		}
	}
	w.last = in
	w.applied = true
	w.logger.InfoContext(ctx, "runtime operations updated")
}
