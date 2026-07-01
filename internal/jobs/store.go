package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

type Store struct {
	db *database.DB
}

func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Enqueue(ctx context.Context, input EnqueueInput) (Record, error) {
	if input.Type == "" {
		return Record{}, ErrUnknownType
	}
	if input.Visibility == "" {
		input.Visibility = VisibilityUser
	}
	if input.Visibility != VisibilityUser && input.Visibility != VisibilityInternal {
		return Record{}, ErrInvalidScope
	}
	if input.Visibility == VisibilityUser && (input.OrgID == nil || input.WorkspaceID == nil || input.OwnerAccountID == nil) {
		return Record{}, ErrInvalidScope
	}
	if input.RunAt.IsZero() {
		input.RunAt = time.Now()
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = 1
	}
	payload, err := marshalPayload(input.Input)
	if err != nil {
		return Record{}, err
	}
	now := time.Now()
	job := Record{
		ID:             database.NewID(),
		Type:           input.Type,
		Visibility:     input.Visibility,
		Status:         StatusQueued,
		OrgID:          input.OrgID,
		WorkspaceID:    input.WorkspaceID,
		OwnerAccountID: input.OwnerAccountID,
		RunAt:          input.RunAt,
		Priority:       input.Priority,
		MaxAttempts:    input.MaxAttempts,
		InputJSON:      payload,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = s.db.NewInsert().Model(&job).Exec(ctx)
	if err != nil {
		return Record{}, err
	}
	return job, nil
}

// HasActiveJobType reports whether a queued or running job of the same
// visibility/type already exists. Internal schedulers use this to avoid
// accumulating redundant maintenance jobs when workers are busy.
func (s *Store) HasActiveJobType(ctx context.Context, visibility, jobType string) (bool, error) {
	var exists bool
	err := s.db.NewSelect().Model((*Record)(nil)).
		ColumnExpr("COUNT(*) > 0").
		Where("visibility = ?", visibility).
		Where("type = ?", jobType).
		Where("status IN (?)", bun.In([]string{StatusQueued, StatusRunning})).
		Scan(ctx, &exists)
	return exists, err
}

func (s *Store) ListUserWorkspaceJobs(ctx context.Context, orgID, workspaceID, accountID int64, page, pageSize int) (response.Paginated[Record], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var jobs []Record
	err := s.db.NewSelect().Model(&jobs).
		Where("visibility = ?", VisibilityUser).
		Where("org_id = ? AND workspace_id = ? AND owner_account_id = ?", orgID, workspaceID, accountID).
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx)
	if err != nil {
		return response.Paginated[Record]{}, err
	}
	for i := range jobs {
		populateOutput(&jobs[i])
	}
	return response.PaginateItems(jobs, page, pageSize), nil
}

func (s *Store) GetUserWorkspaceJob(ctx context.Context, orgID, workspaceID, accountID int64, jobID string) (Record, bool, error) {
	var job Record
	err := s.db.NewSelect().Model(&job).
		Where("id = ?", jobID).
		Where("visibility = ?", VisibilityUser).
		Where("org_id = ? AND workspace_id = ? AND owner_account_id = ?", orgID, workspaceID, accountID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err == nil {
		populateOutput(&job)
	}
	return job, err == nil, err
}

func (s *Store) RequestCancelUserWorkspaceJob(ctx context.Context, orgID, workspaceID, accountID int64, jobID string) (Record, bool, error) {
	now := time.Now()
	var job Record
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		err := tx.NewSelect().Model(&job).
			Where("id = ?", jobID).
			Where("visibility = ?", VisibilityUser).
			Where("org_id = ? AND workspace_id = ? AND owner_account_id = ?", orgID, workspaceID, accountID).
			Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		switch job.Status {
		case StatusSucceeded, StatusFailed, StatusCancelled:
			return nil
		case StatusQueued:
			_, err = tx.NewUpdate().Model((*Record)(nil)).
				Set("status = ?", StatusCancelled).
				Set("cancel_requested_at = ?", now).
				Set("finished_at = ?", now).
				Set("updated_at = ?", now).
				Where("id = ?", job.ID).
				Exec(ctx)
		case StatusRunning:
			_, err = tx.NewUpdate().Model((*Record)(nil)).
				Set("cancel_requested_at = ?", now).
				Set("updated_at = ?", now).
				Where("id = ?", job.ID).
				Where("cancel_requested_at IS NULL").
				Exec(ctx)
		}
		if err != nil {
			return err
		}
		return tx.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx)
	})
	if errors.Is(err, ErrNotFound) {
		return Record{}, false, nil
	}
	if err == nil {
		populateOutput(&job)
	}
	return job, err == nil, err
}

func (s *Store) ClaimDue(ctx context.Context, workerID string, now time.Time, lease time.Duration) (Record, bool, error) {
	var claimed Record
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var candidates []Record
		if err := tx.NewSelect().Model(&candidates).
			Where("status = ?", StatusQueued).
			Where("run_at <= ?", now).
			OrderExpr("priority DESC, run_at ASC, created_at ASC, id ASC").
			Limit(5).
			Scan(ctx); err != nil {
			return err
		}
		for _, candidate := range candidates {
			startedAt := now
			claimUntil := now.Add(lease)
			res, err := tx.NewUpdate().Model((*Record)(nil)).
				Set("status = ?", StatusRunning).
				Set("attempts = attempts + 1").
				Set("claimed_by = ?", workerID).
				Set("claimed_until = ?", claimUntil).
				Set("started_at = ?", startedAt).
				Set("updated_at = ?", now).
				Where("id = ?", candidate.ID).
				Where("status = ?", StatusQueued).
				Exec(ctx)
			if err != nil {
				return err
			}
			affected, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if affected == 0 {
				continue
			}
			return tx.NewSelect().Model(&claimed).Where("id = ?", candidate.ID).Scan(ctx)
		}
		return nil
	})
	if err != nil {
		return Record{}, false, err
	}
	return claimed, claimed.ID != "", nil
}

func populateOutput(job *Record) {
	if job == nil || job.OutputJSON == "" {
		return
	}
	var output any
	if err := json.Unmarshal([]byte(job.OutputJSON), &output); err == nil {
		job.Output = output
	}
}

func (s *Store) Complete(ctx context.Context, jobID string, output any) error {
	payload, err := marshalPayload(output)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = s.db.NewUpdate().Model((*Record)(nil)).
		Set("status = ?", StatusSucceeded).
		Set("output_json = ?", payload).
		Set("claimed_by = NULL").
		Set("claimed_until = NULL").
		Set("finished_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", jobID).
		Exec(ctx)
	return err
}

func (s *Store) FailOrRetry(ctx context.Context, job Record, code, message string, retryable bool, delay time.Duration) (string, error) {
	now := time.Now()
	nextStatus := StatusFailed
	runAt := job.RunAt
	finishedAt := &now
	if retryable && job.Attempts < job.MaxAttempts {
		nextStatus = StatusQueued
		runAt = now.Add(delay)
		finishedAt = nil
	}
	q := s.db.NewUpdate().Model((*Record)(nil)).
		Set("status = ?", nextStatus).
		Set("run_at = ?", runAt).
		Set("claimed_by = NULL").
		Set("claimed_until = NULL").
		Set("error_code = ?", code).
		Set("error_message = ?", message).
		Set("updated_at = ?", now).
		Where("id = ?", job.ID)
	if finishedAt == nil {
		q = q.Set("finished_at = NULL")
	} else {
		q = q.Set("finished_at = ?", *finishedAt)
	}
	_, err := q.Exec(ctx)
	return nextStatus, err
}

func (s *Store) MarkCancelled(ctx context.Context, jobID string) error {
	now := time.Now()
	_, err := s.db.NewUpdate().Model((*Record)(nil)).
		Set("status = ?", StatusCancelled).
		Set("claimed_by = NULL").
		Set("claimed_until = NULL").
		Set("finished_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", jobID).
		Exec(ctx)
	return err
}

func (s *Store) Heartbeat(ctx context.Context, jobID, workerID string, lease time.Duration) (bool, bool, error) {
	var job Record
	err := s.db.NewSelect().Model(&job).Where("id = ? AND claimed_by = ?", jobID, workerID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if job.CancelRequestedAt != nil {
		return true, true, nil
	}
	now := time.Now()
	_, err = s.db.NewUpdate().Model((*Record)(nil)).
		Set("claimed_until = ?", now.Add(lease)).
		Set("updated_at = ?", now).
		Where("id = ? AND claimed_by = ? AND status = ?", jobID, workerID, StatusRunning).
		Exec(ctx)
	return true, false, err
}

func (s *Store) RecoverExpiredRunning(ctx context.Context, now time.Time) (int, int, error) {
	var expired []Record
	if err := s.db.NewSelect().Model(&expired).
		Where("status = ?", StatusRunning).
		Where("claimed_until IS NOT NULL AND claimed_until < ?", now).
		Scan(ctx); err != nil {
		return 0, 0, err
	}
	requeued := 0
	failed := 0
	for _, job := range expired {
		status := StatusFailed
		runAt := job.RunAt
		finishedAt := &now
		if job.Attempts < job.MaxAttempts {
			status = StatusQueued
			runAt = now
			finishedAt = nil
		}
		q := s.db.NewUpdate().Model((*Record)(nil)).
			Set("status = ?", status).
			Set("run_at = ?", runAt).
			Set("claimed_by = NULL").
			Set("claimed_until = NULL").
			Set("error_code = ?", "worker_lost").
			Set("error_message = ?", "Job worker claim expired.").
			Set("updated_at = ?", now).
			Where("id = ?", job.ID).
			Where("status = ?", StatusRunning)
		if finishedAt == nil {
			q = q.Set("finished_at = NULL")
		} else {
			q = q.Set("finished_at = ?", *finishedAt)
		}
		res, err := q.Exec(ctx)
		if err != nil {
			return requeued, failed, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return requeued, failed, err
		}
		if affected == 0 {
			continue
		}
		if status == StatusQueued {
			requeued++
		} else {
			failed++
		}
	}
	return requeued, failed, nil
}

func (s *Store) PruneCompleted(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	var jobs []Record
	if err := s.db.NewSelect().Model(&jobs).
		Where("status IN (?)", bun.In([]string{StatusSucceeded, StatusFailed, StatusCancelled})).
		Where("finished_at IS NOT NULL AND finished_at < ?", olderThan).
		OrderExpr("finished_at ASC").
		Limit(limit).
		Scan(ctx); err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	res, err := s.db.NewDelete().Model((*Record)(nil)).Where("id IN (?)", bun.In(ids)).Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
