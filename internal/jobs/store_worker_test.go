package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
)

func newTestStore(t *testing.T) (*Store, *database.DB) {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "jobs.db"), slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	return NewStore(db), db
}

func newJobScope(t *testing.T, db *database.DB) (accountID, orgID, workspaceID int64) {
	t.Helper()
	ctx := context.Background()
	account, err := db.InsertAccount(ctx, "jobs-"+database.NewID()+"@example.com", "Jobs User", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := db.InsertOrg(ctx, "jobs-"+database.NewID(), "Jobs Org")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddOrgMember(ctx, org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Jobs Workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddWorkspaceMember(ctx, ws.ID, account.ID, nil); err != nil {
		t.Fatal(err)
	}
	return account.ID, org.ID, ws.ID
}

func TestStoreClaimDueJobsOnlyOnce(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	future, err := store.Enqueue(ctx, EnqueueInput{
		Type:       "noop",
		Visibility: VisibilityInternal,
		RunAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	due, err := store.Enqueue(ctx, EnqueueInput{
		Type:       "noop",
		Visibility: VisibilityInternal,
		RunAt:      time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, found, err := store.ClaimDue(ctx, "worker-a", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !found || claimed.ID != due.ID {
		t.Fatalf("claimed %q, found=%v, want due job %q", claimed.ID, found, due.ID)
	}
	claimed, found, err = store.ClaimDue(ctx, "worker-b", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("claimed second job %+v; future job %q should not be due", claimed, future.ID)
	}
}

func TestStoreClaimDuePrefersHigherPriority(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	low, err := store.Enqueue(ctx, EnqueueInput{
		Type:       "noop",
		Visibility: VisibilityInternal,
		RunAt:      time.Now().Add(-time.Minute),
		Priority:   PriorityLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	high, err := store.Enqueue(ctx, EnqueueInput{
		Type:       "noop",
		Visibility: VisibilityInternal,
		RunAt:      time.Now().Add(-time.Minute),
		Priority:   PriorityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, found, err := store.ClaimDue(ctx, "worker-a", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !found || claimed.ID != high.ID {
		t.Fatalf("claimed %q, found=%v, want high-priority job %q", claimed.ID, found, high.ID)
	}
	claimed, found, err = store.ClaimDue(ctx, "worker-b", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !found || claimed.ID != low.ID {
		t.Fatalf("claimed %q, found=%v, want low-priority job %q", claimed.ID, found, low.ID)
	}
}

func TestHasActiveJobTypeOnlyCountsQueuedAndRunning(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	active, err := store.HasActiveJobType(ctx, VisibilityInternal, TypeFileContentReap)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("expected no active jobs")
	}

	job, err := store.Enqueue(ctx, EnqueueInput{Type: TypeFileContentReap, Visibility: VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	active, err = store.HasActiveJobType(ctx, VisibilityInternal, TypeFileContentReap)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected queued job to count as active")
	}
	if err := store.Complete(ctx, job.ID, nil); err != nil {
		t.Fatal(err)
	}
	active, err = store.HasActiveJobType(ctx, VisibilityInternal, TypeFileContentReap)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("expected completed job not to count as active")
	}
}

func TestStoreEnqueueSingletonAllowsOnlyOneActiveJob(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	first, created, err := store.EnqueueSingleton(ctx, EnqueueInput{
		Type:         TypeFileContentReap,
		SingletonKey: TypeFileContentReap,
		Visibility:   VisibilityInternal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.SingletonKey != TypeFileContentReap {
		t.Fatalf("created=%v singleton_key=%q, want created singleton", created, first.SingletonKey)
	}
	_, created, err = store.EnqueueSingleton(ctx, EnqueueInput{
		Type:         TypeFileContentReap,
		SingletonKey: TypeFileContentReap,
		Visibility:   VisibilityInternal,
	})
	if !errors.Is(err, ErrActiveExists) {
		t.Fatalf("error = %v, want ErrActiveExists", err)
	}
	if created {
		t.Fatal("duplicate singleton job reported created")
	}

	if err := store.Complete(ctx, first.ID, nil); err != nil {
		t.Fatal(err)
	}
	second, created, err := store.EnqueueSingleton(ctx, EnqueueInput{
		Type:         TypeFileContentReap,
		SingletonKey: TypeFileContentReap,
		Visibility:   VisibilityInternal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created || second.ID == first.ID {
		t.Fatalf("second=%+v created=%v, want new singleton after completion", second, created)
	}
}

func TestStoreEnqueueRejectsSingletonKey(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.Enqueue(context.Background(), EnqueueInput{
		Type:         "noop",
		SingletonKey: "noop",
		Visibility:   VisibilityInternal,
	})
	if !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("error = %v, want ErrInvalidScope", err)
	}
}

func TestStoreAppendsAndListsUserWorkspaceJobEvents(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	accountID, orgID, workspaceID := newJobScope(t, db)
	job, err := store.Enqueue(ctx, EnqueueInput{
		Type:           "export",
		Visibility:     VisibilityUser,
		OrgID:          &orgID,
		WorkspaceID:    &workspaceID,
		OwnerAccountID: &accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendEvent(ctx, EventInput{JobID: job.ID, Level: EventLevelInfo, Code: "query_started", Message: "Query started."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendEvent(ctx, EventInput{
		JobID:   job.ID,
		Level:   EventLevelWarn,
		Code:    "rows_truncated",
		Message: "Rows were truncated.",
		Details: map[string]any{"rows": 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.AppendEvent(ctx, EventInput{JobID: job.ID, Level: EventLevelInfo, Code: "file_saved", Message: "File saved."})
	if err != nil {
		t.Fatal(err)
	}

	page, err := store.ListUserWorkspaceJobEvents(ctx, orgID, workspaceID, accountID, job.ID, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != first.ID || page.Items[1].ID != second.ID {
		t.Fatalf("page items = %+v, want first two events", page.Items)
	}
	if page.NextAfterID != second.ID {
		t.Fatalf("next_after_id = %q, want %q", page.NextAfterID, second.ID)
	}
	if details, ok := page.Items[1].Details.(map[string]any); !ok || details["rows"].(float64) != 100 {
		t.Fatalf("details = %#v, want rows detail", page.Items[1].Details)
	}

	page, err = store.ListUserWorkspaceJobEvents(ctx, orgID, workspaceID, accountID, job.ID, page.NextAfterID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != third.ID {
		t.Fatalf("page after marker = %+v, want third event", page.Items)
	}
}

func TestJobEventsCascadeWhenJobIsPruned(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	accountID, orgID, workspaceID := newJobScope(t, db)
	job, err := store.Enqueue(ctx, EnqueueInput{
		Type:           "noop",
		Visibility:     VisibilityUser,
		OrgID:          &orgID,
		WorkspaceID:    &workspaceID,
		OwnerAccountID: &accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(ctx, EventInput{JobID: job.ID, Level: EventLevelInfo, Code: "test_event", Message: "Test event."}); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, job.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewUpdate().Model((*Record)(nil)).
		Set("finished_at = ?", time.Now().Add(-time.Hour)).
		Where("id = ?", job.ID).
		Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if processed, err := store.PruneCompleted(ctx, time.Now(), 100); err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v, want one pruned job", processed, err)
	}
	var count int
	if err := db.NewSelect().Model((*Event)(nil)).ColumnExpr("COUNT(*)").Where("job_id = ?", job.ID).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("job event count = %d, want cascade delete", count)
	}
}

func TestStoreRejectsEventsForInternalJobs(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	job, err := store.Enqueue(ctx, EnqueueInput{Type: "noop", Visibility: VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendEvent(ctx, EventInput{JobID: job.ID, Level: EventLevelInfo, Code: "internal_event", Message: "Internal event."})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error = %v, want ErrInvalidEvent", err)
	}
}

func TestRecoverExpiredRunningJobs(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	job, err := store.Enqueue(ctx, EnqueueInput{Type: "noop", Visibility: VisibilityInternal, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	claimed, found, err := store.ClaimDue(ctx, "worker", time.Now(), -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !found || claimed.ID != job.ID {
		t.Fatalf("claimed %+v, found=%v, want %s", claimed, found, job.ID)
	}

	requeued, failed, err := store.RecoverExpiredRunning(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 1 || failed != 0 {
		t.Fatalf("requeued=%d failed=%d, want 1/0", requeued, failed)
	}
}

func TestRecoverExpiredRunningJobFailsWhenAttemptsExhausted(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	job, err := store.Enqueue(ctx, EnqueueInput{Type: "noop", Visibility: VisibilityInternal, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimDue(ctx, "worker", time.Now(), -time.Minute); err != nil || !found {
		t.Fatalf("claim found=%v err=%v", found, err)
	}

	requeued, failed, err := store.RecoverExpiredRunning(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 0 || failed != 1 {
		t.Fatalf("requeued=%d failed=%d, want 0/1", requeued, failed)
	}
	var stored Record
	if err := db.NewSelect().Model(&stored).Where("id = ?", job.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusFailed || stored.ErrorCode != "worker_lost" {
		t.Fatalf("status=%s error=%s, want failed/worker_lost", stored.Status, stored.ErrorCode)
	}
}

func TestRunnerRecordsUserJobLifecycleAndHandlerEvents(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	accountID, orgID, workspaceID := newJobScope(t, db)
	registry := NewRegistry()
	registry.Register(Definition{
		Type: "export",
		Handler: HandlerFunc(func(ctx context.Context, runtime Runtime) (any, error) {
			runtime.Events.Info(ctx, "export_started", "Export started.", map[string]any{"format": "csv"})
			return map[string]any{"file_id": "file-1"}, nil
		}),
	})
	job, err := store.Enqueue(ctx, EnqueueInput{
		Type:           "export",
		Visibility:     VisibilityUser,
		OrgID:          &orgID,
		WorkspaceID:    &workspaceID,
		OwnerAccountID: &accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(store, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), WorkerConfig{ClaimLease: time.Minute})
	if !runner.runOnce(ctx, 0) {
		t.Fatal("runner did not claim job")
	}

	var stored Record
	if err := db.NewSelect().Model(&stored).Where("id = ?", job.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusSucceeded {
		t.Fatalf("status = %s, want succeeded", stored.Status)
	}
	page, err := store.ListUserWorkspaceJobEvents(ctx, orgID, workspaceID, accountID, job.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]string, 0, len(page.Items))
	for _, event := range page.Items {
		codes = append(codes, event.Code)
	}
	want := []string{"job_started", "export_started", "job_succeeded"}
	if len(codes) != len(want) {
		t.Fatalf("event codes = %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("event codes = %v, want %v", codes, want)
		}
	}
}

func TestRunnerCompletesJobAndStoresOutput(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	registry := NewRegistry()
	registry.Register(Definition{
		Type: "answer",
		Handler: HandlerFunc(func(context.Context, Runtime) (any, error) {
			return map[string]int{"answer": 42}, nil
		}),
	})
	job, err := store.Enqueue(ctx, EnqueueInput{Type: "answer", Visibility: VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(store, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), WorkerConfig{ClaimLease: time.Minute})
	if !runner.runOnce(ctx, 0) {
		t.Fatal("runner did not claim job")
	}

	var stored Record
	if err := db.NewSelect().Model(&stored).Where("id = ?", job.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusSucceeded {
		t.Fatalf("status = %s, want %s", stored.Status, StatusSucceeded)
	}
	var output map[string]int
	if err := json.Unmarshal([]byte(stored.OutputJSON), &output); err != nil {
		t.Fatal(err)
	}
	if output["answer"] != 42 {
		t.Fatalf("output = %#v, want answer 42", output)
	}
}

func TestRunnerRetriesRetryableFailure(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	registry := NewRegistry()
	registry.Register(Definition{
		Type:        "retry",
		MaxAttempts: 2,
		Backoff:     func(int) time.Duration { return time.Millisecond },
		Handler: HandlerFunc(func(context.Context, Runtime) (any, error) {
			return nil, Retryable("temporary", "Temporary failure.")
		}),
	})
	job, err := store.Enqueue(ctx, EnqueueInput{Type: "retry", Visibility: VisibilityInternal, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(store, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), WorkerConfig{ClaimLease: time.Minute})
	if !runner.runOnce(ctx, 0) {
		t.Fatal("runner did not claim job")
	}

	var stored Record
	if err := db.NewSelect().Model(&stored).Where("id = ?", job.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusQueued || stored.Attempts != 1 {
		t.Fatalf("status=%s attempts=%d, want queued/1", stored.Status, stored.Attempts)
	}
}

func TestRunningJobCancellationCancelsHandler(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	accountID, orgID, workspaceID := newJobScope(t, db)
	started := make(chan struct{})
	registry := NewRegistry()
	registry.Register(Definition{
		Type: "blocking",
		Handler: HandlerFunc(func(ctx context.Context, _ Runtime) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	})
	job, err := store.Enqueue(ctx, EnqueueInput{
		Type:           "blocking",
		Visibility:     VisibilityUser,
		OrgID:          &orgID,
		WorkspaceID:    &workspaceID,
		OwnerAccountID: &accountID,
	})
	if err != nil {
		t.Fatal(err)
	}

	runnerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runner := NewRunner(store, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), WorkerConfig{
		WorkerCount:  1,
		PollInterval: 10 * time.Millisecond,
		ClaimLease:   30 * time.Millisecond,
	})
	done := make(chan struct{})
	go func() {
		runner.Run(runnerCtx)
		close(done)
	}()
	<-started
	if _, found, err := store.RequestCancelUserWorkspaceJob(ctx, orgID, workspaceID, accountID, job.ID); err != nil || !found {
		t.Fatalf("cancel found=%v err=%v", found, err)
	}

	deadline := time.After(2 * time.Second)
	for {
		var stored Record
		if err := db.NewSelect().Model(&stored).Where("id = ?", job.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if stored.Status == StatusCancelled {
			cancel()
			<-done
			return
		}
		select {
		case <-deadline:
			t.Fatalf("job status = %s, want cancelled", stored.Status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
