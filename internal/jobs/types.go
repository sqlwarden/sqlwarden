package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

const (
	TypeFileContentReap = "file_content_reap"

	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	VisibilityUser     = "user"
	VisibilityInternal = "internal"

	PriorityLow      = -100
	PriorityNormal   = 0
	PriorityHigh     = 100
	PriorityCritical = 1000
)

var (
	ErrUnknownType  = errors.New("unknown job type")
	ErrNotFound     = errors.New("job not found")
	ErrNotRunnable  = errors.New("job is not runnable")
	ErrNotOwned     = errors.New("job is not owned by account")
	ErrInvalidScope = errors.New("job scope is invalid")
)

// Record mirrors a persisted job row. Payloads are stored as strings for
// SQLite/Postgres portability and decoded by type-specific handlers.
type Record struct {
	bun.BaseModel     `bun:"table:jobs"`
	ID                string     `bun:",pk" json:"id"`
	Type              string     `bun:",notnull" json:"type"`
	Visibility        string     `bun:",notnull" json:"visibility"`
	Status            string     `bun:",notnull" json:"status"`
	OrgID             *int64     `bun:",nullzero" json:"org_id,omitempty"`
	WorkspaceID       *int64     `bun:",nullzero" json:"workspace_id,omitempty"`
	OwnerAccountID    *int64     `bun:",nullzero" json:"owner_account_id,omitempty"`
	RunAt             time.Time  `bun:",notnull" json:"run_at"`
	Priority          int        `bun:",notnull" json:"priority"`
	Attempts          int        `bun:",notnull" json:"attempts"`
	MaxAttempts       int        `bun:",notnull" json:"max_attempts"`
	ClaimedBy         string     `bun:",nullzero" json:"-"`
	ClaimedUntil      *time.Time `bun:",nullzero" json:"-"`
	StartedAt         *time.Time `bun:",nullzero" json:"started_at,omitempty"`
	FinishedAt        *time.Time `bun:",nullzero" json:"finished_at,omitempty"`
	CancelRequestedAt *time.Time `bun:",nullzero" json:"cancel_requested_at,omitempty"`
	ErrorCode         string     `bun:",nullzero" json:"error_code,omitempty"`
	ErrorMessage      string     `bun:",nullzero" json:"error_message,omitempty"`
	InputJSON         string     `bun:",nullzero" json:"-"`
	OutputJSON        string     `bun:",nullzero" json:"-"`
	Output            any        `bun:"-" json:"output,omitempty"`
	CreatedAt         time.Time  `bun:",notnull" json:"created_at"`
	UpdatedAt         time.Time  `bun:",notnull" json:"updated_at"`
}

// EnqueueInput describes a new job. Callers provide type-specific input as a
// JSON-marshalable value; handlers own validation after decoding.
type EnqueueInput struct {
	Type           string
	Visibility     string
	OrgID          *int64
	WorkspaceID    *int64
	OwnerAccountID *int64
	RunAt          time.Time
	Priority       int
	MaxAttempts    int
	Input          any
}

// Handler executes one claimed job. It must honor ctx cancellation for
// cooperative shutdown and user-requested cancellation.
type Handler interface {
	Handle(ctx context.Context, job Record) (any, error)
}

type HandlerFunc func(context.Context, Record) (any, error)

func (fn HandlerFunc) Handle(ctx context.Context, job Record) (any, error) {
	return fn(ctx, job)
}

type Definition struct {
	Type        string
	Handler     Handler
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
	Timeout     time.Duration
}

type Registry struct {
	defs map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{defs: map[string]Definition{}}
}

func (r *Registry) Register(def Definition) {
	if def.MaxAttempts <= 0 {
		def.MaxAttempts = 1
	}
	if def.Backoff == nil {
		def.Backoff = func(int) time.Duration { return time.Minute }
	}
	r.defs[def.Type] = def
}

func (r *Registry) Definition(jobType string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	def, ok := r.defs[jobType]
	return def, ok
}

type CodedError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e CodedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "job failed"
}

func Retryable(code, message string) error {
	return CodedError{Code: code, Message: message, Retryable: true}
}

func Permanent(code, message string) error {
	return CodedError{Code: code, Message: message}
}

func jobError(err error) (code, message string, retryable bool) {
	if err == nil {
		return "", "", false
	}
	var coded CodedError
	if errors.As(err, &coded) {
		code = coded.Code
		message = coded.Message
		retryable = coded.Retryable
	}
	if code == "" {
		code = "job_failed"
	}
	if message == "" {
		message = err.Error()
	}
	return code, message, retryable
}

func marshalPayload(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
