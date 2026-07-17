// Package distribution defines compile-time inputs for a SQLWarden distribution.
// It is intentionally not a runtime plugin or compatibility framework.
package distribution

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/authorization"
	"github.com/sqlwarden/buildinfo"
	"github.com/uptrace/bun"
)

type JobError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e JobError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}
func RetryableJobError(code, message string) error {
	return JobError{Code: code, Message: message, Retryable: true}
}
func PermanentJobError(code, message string) error { return JobError{Code: code, Message: message} }

// Configure is called once after core infrastructure is ready and before
// distribution migrations, routes, jobs, and workers are started.
type Configure func(HostServices) (Dependencies, error)

// Dependencies are build-time additions supplied by a trusted distribution.
type Dependencies struct {
	AuthorizationConstraint authorization.Constraint
	Permissions             []Permission
	Migrations              []MigrationSet
	Jobs                    []Job
	InstallRoutes           RouteInstaller
	Lifecycle               Lifecycle
	Frontend                fs.FS
	Build                   buildinfo.Info
}

// HostServices are passed only to the distribution composition root. Feature
// constructors should receive the individual dependencies they actually use.
type HostServices struct {
	Logger        *slog.Logger
	DB            *bun.DB
	Authorization authorization.Authorizer
	Accounts      AccountService
	Organizations OrganizationService
	Sessions      SessionService
	Jobs          JobService
	Request       RequestContext
}

type Account struct {
	ID       int64
	Email    string
	Name     string
	IsActive bool
}

type AccountService interface {
	FindByID(context.Context, int64) (Account, bool, error)
	FindByEmail(context.Context, string) (Account, bool, error)
	Provision(context.Context, string, string) (Account, error)
}

type Organization struct {
	ID   int64
	Slug string
	Name string
}

type OrganizationService interface {
	FindByID(context.Context, int64) (Organization, bool, error)
	FindBySlug(context.Context, string) (Organization, bool, error)
	IsMember(context.Context, int64, int64) (bool, error)
	AddMember(context.Context, int64, int64) error
}

// SessionService lets a verified external identity create or revoke a normal
// SQLWarden session without exposing token and cookie internals.
type SessionService interface {
	Complete(http.ResponseWriter, *http.Request, int64) error
	Revoke(context.Context, string, *int64, string) error
}

type RequestContext interface {
	Account(*http.Request) (Account, bool)
	OrganizationID(*http.Request) (int64, bool)
	WorkspaceID(*http.Request) (int64, bool)
	EnvironmentID(*http.Request) (int64, bool)
	ConnectionID(*http.Request) (int64, bool)
}

type JobService interface {
	Enqueue(context.Context, EnqueueJob) (string, error)
}

type EnqueueJob struct {
	Type           string
	Input          any
	Priority       int
	RunAt          time.Time
	OwnerAccountID *int64
	OrgID          *int64
	WorkspaceID    *int64
	Visibility     string
	SingletonKey   string
}

type JobRuntime interface {
	ID() string
	DecodeInput(any) error
	Events() JobEventWriter
}

type JobEventWriter interface {
	Info(context.Context, string, string, any)
	Warn(context.Context, string, string, any)
	Error(context.Context, string, string, any)
}

type JobHandler interface {
	Handle(context.Context, JobRuntime) (any, error)
}

type JobHandlerFunc func(context.Context, JobRuntime) (any, error)

func (fn JobHandlerFunc) Handle(ctx context.Context, runtime JobRuntime) (any, error) {
	return fn(ctx, runtime)
}

type Job struct {
	Type        string
	Handler     JobHandler
	MaxAttempts int
	Backoff     func(int) time.Duration
	Timeout     time.Duration
}

// Permission augments the application-local catalog. Builtin role defaults are
// additive and are reconciled into existing installations.
type Permission struct {
	ID                    string
	Label                 string
	Description           string
	Group                 string
	Scopes                []string
	Resources             []string
	DefaultOrgRoles       []string
	DefaultWorkspaceRoles []string
}

// MigrationSet owns an independent migration ledger in the shared application database.
type MigrationSet struct {
	FS             fs.FS
	PostgresPath   string
	SQLitePath     string
	MigrationsName string
}

// RouteMounts contain routers with Community authentication and resource
// context middleware already installed. Paths are relative to each mount.
type RouteMounts struct {
	Public       chi.Router
	Account      chi.Router
	Instance     chi.Router
	Organization chi.Router
	Workspace    chi.Router
	Environment  chi.Router
	Connection   chi.Router
}

type RouteInstaller func(RouteMounts)

// Lifecycle owns distribution resources that need explicit startup or shutdown.
type Lifecycle interface {
	Start(context.Context) error
	Shutdown(context.Context) error
}
