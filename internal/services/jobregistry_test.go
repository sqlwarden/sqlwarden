package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlwarden/internal/jobs"
)

type stubReaper struct {
	batch int
	retry time.Duration
	err   error
}

func (r *stubReaper) ReapContentDeletionsOnce(_ context.Context, batchSize int, retryAfter time.Duration) (int, error) {
	r.batch, r.retry = batchSize, retryAfter
	return 2, r.err
}

type stubExports struct{ calls int }

func (e *stubExports) RunJob(context.Context, jobs.Runtime) (any, error) {
	e.calls++
	return map[string]any{"exported": true}, nil
}

type stubRevoker struct{ calls []string }

func (r *stubRevoker) RevokeConnection(_ context.Context, id string) (int, error) {
	r.calls = append(r.calls, "connection:"+id)
	return 1, nil
}

func (r *stubRevoker) RevokeWorkspaceAccount(_ context.Context, ws, acct string) (int, error) {
	r.calls = append(r.calls, "workspace_account:"+ws+":"+acct)
	return 1, nil
}

func (r *stubRevoker) RevokeOrgAccount(_ context.Context, org, acct string) (int, error) {
	r.calls = append(r.calls, "org_account:"+org+":"+acct)
	return 1, nil
}

func TestJobRegistryRegistersBuiltinTypes(t *testing.T) {
	reaper, exports, revoker := &stubReaper{}, &stubExports{}, &stubRevoker{}
	registry, err := JobRegistry(JobRegistryDeps{Files: reaper, Exports: exports, Revoker: revoker})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	noop, ok := registry.Definition("noop")
	if !ok || noop.MaxAttempts != 1 {
		t.Fatalf("noop definition = %+v, found %v", noop, ok)
	}

	reap, ok := registry.Definition(jobs.TypeFileContentReap)
	if !ok || reap.MaxAttempts != 3 || reap.Backoff(2) != 2*time.Minute {
		t.Fatalf("file reap definition = %+v, found %v", reap, ok)
	}
	if out, err := reap.Handler.Handle(ctx, jobs.Runtime{}); err != nil || out.(map[string]any)["processed"] != 2 {
		t.Fatalf("file reap output = %v, err %v", out, err)
	}
	if reaper.batch != 100 || reaper.retry != time.Minute {
		t.Fatalf("file reap called with batch %d retry %s", reaper.batch, reaper.retry)
	}
	reaper.err = errors.New("store down")
	var coded jobs.CodedError
	if _, err := reap.Handler.Handle(ctx, jobs.Runtime{}); !errors.As(err, &coded) || !coded.Retryable || coded.Code != "file_content_reap_failed" {
		t.Fatalf("file reap failure = %#v, want retryable file_content_reap_failed", err)
	}

	export, ok := registry.Definition(jobs.TypeExportQueryCSV)
	if !ok || export.MaxAttempts != 1 {
		t.Fatalf("export definition = %+v, found %v", export, ok)
	}
	if _, err := export.Handler.Handle(ctx, jobs.Runtime{}); err != nil || exports.calls != 1 {
		t.Fatalf("export handler err %v calls %d", err, exports.calls)
	}

	revoke, ok := registry.Definition(jobs.TypeSessionRevoke)
	if !ok || revoke.MaxAttempts != jobs.SessionRevokeMaxAttempts || revoke.Backoff(2) != 30*time.Second {
		t.Fatalf("session revoke definition = %+v, found %v", revoke, ok)
	}
	if _, err := revoke.Handler.Handle(ctx, jobs.Runtime{Job: jobs.Record{InputJSON: `{"kind":"connection","connection_id":"4"}`}}); err != nil {
		t.Fatal(err)
	}
	if len(revoker.calls) != 1 || revoker.calls[0] != "connection:4" {
		t.Fatalf("revoker calls = %v", revoker.calls)
	}
}

func TestJobRegistryAddsEditionJobs(t *testing.T) {
	registry, err := JobRegistry(JobRegistryDeps{
		Files: &stubReaper{}, Exports: &stubExports{}, Revoker: &stubRevoker{},
		Edition: []jobs.Definition{{Type: "ee_report", Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) { return nil, nil })}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Definition("ee_report"); !ok {
		t.Fatal("edition job is not registered")
	}
}

func TestJobRegistryRejectsDuplicateEditionJob(t *testing.T) {
	_, err := JobRegistry(JobRegistryDeps{
		Files: &stubReaper{}, Exports: &stubExports{}, Revoker: &stubRevoker{},
		Edition: []jobs.Definition{{Type: jobs.TypeSessionRevoke}},
	})
	if err == nil {
		t.Fatal("duplicate edition job type was accepted")
	}
}

func TestServicesJobRegistryUsesServiceGraph(t *testing.T) {
	_, infra, err := Build(newTestDeps(t))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := infra.JobRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, jobType := range []string{"noop", jobs.TypeFileContentReap, jobs.TypeExportQueryCSV, jobs.TypeSessionRevoke} {
		if _, ok := registry.Definition(jobType); !ok {
			t.Fatalf("%s is not registered", jobType)
		}
	}
}

func TestFileReapScheduleEnqueuesOneActiveSingleton(t *testing.T) {
	_, infra, err := Build(newTestDeps(t))
	if err != nil {
		t.Fatal(err)
	}
	schedule := infra.FileReapSchedule()
	if schedule.Every != 15*time.Minute || schedule.Name != jobs.TypeFileContentReap {
		t.Fatalf("schedule = %+v", schedule)
	}
	ctx := context.Background()
	for range 3 {
		if err := schedule.Enqueue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	job, active, err := infra.JobStore.ActiveBySingletonKey(ctx, jobs.TypeFileContentReap)
	if err != nil || !active {
		t.Fatalf("active reap job: found %v err %v", active, err)
	}
	if job.Visibility != jobs.VisibilityInternal || job.Priority != jobs.PriorityLow || job.MaxAttempts != 3 {
		t.Fatalf("reap job = %+v", job)
	}
	count, err := infra.services.deps.DB.NewSelect().Model((*jobs.Record)(nil)).Where("type = ?", jobs.TypeFileContentReap).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reap jobs = %d, want 1", count)
	}
}
