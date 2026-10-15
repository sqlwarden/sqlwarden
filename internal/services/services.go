package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exportjob"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/keyrotation"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/settings"
	"github.com/sqlwarden/internal/smtp"
)

// Deps are the stores and ports the service graph is built from. The caller
// owns their lifecycle.
type Deps struct {
	Config config.Config
	DB     *database.DB
	// Enforcer invalidates cached resource ancestry after catalog changes.
	Enforcer *access.Enforcer
	// Policy is the edition-composed policy evaluator. It defaults to
	// Enforcer.
	Policy access.PolicyEvaluator
	// Runtime is the execution runtime. Revoker closes live sessions after
	// access changes and defaults to Runtime.
	Runtime execution.Runtime
	Revoker execution.Revoker
	// Audit defaults to audit.Discard and Logger to a discarding logger.
	Audit      audit.Writer
	Logger     *slog.Logger
	FileStores files.StoreResolver
	// Credentials is the edition-composed secret provider, CredentialWriter
	// stores secret changes, and RevealPolicy is the profile's reveal
	// decision.
	Credentials      credentials.Provider
	CredentialWriter credentials.Writer
	RevealPolicy     credentials.RevealPolicy
	// TargetPolicy vets target drivers and DSNs. It defaults to the instance
	// settings policy over DB.
	TargetPolicy execution.TargetPolicy
	// SecretSealer encrypts instance secrets such as the SMTP password.
	SecretSealer settings.Sealer
	// Keyring opens and re-seals every application-encrypted value during
	// key rotation.
	Keyring KeyRing
	// Invitations is the profile's organization invitation policy. It
	// defaults to orgs.InvitationsDisabled.
	Invitations orgs.InvitationPolicy
	// SignIn is the profile's sign-in strategy. It defaults to
	// identity.SignInUnavailable.
	SignIn identity.SignInStrategy
	// Setup is the profile's first-run setup strategy. It defaults to
	// identity.FormSetup.
	Setup identity.SetupStrategy
}

// KeyRing is the application keyring surface key rotation needs.
type KeyRing interface {
	credentials.RotationKeyring
}

// Services is the built service graph.
type Services struct {
	Requirer *access.Requirer
	Settings *settings.Service
	Identity *identity.Service
	Catalog  *catalog.Service
	Orgs     *orgs.Service
	RBAC     *rbac.Service
	Query    *query.Service
	// Navigator is the schema metadata cache the query service reads through.
	Navigator *schema.Navigator
	// Files applies the default instance file revision policy. Writes that
	// must honor a workspace's effective policy use FilesForWorkspace.
	Files   *files.Service
	Exports *exportjob.Service
	Jobs    *jobs.Service
	// KeyRotation backs the rotate-keys endpoint and command.
	KeyRotation *keyrotation.Service
	JobStore    *jobs.DatabaseStore
	Policy      access.PolicyEvaluator
	Runtime     execution.Runtime
	// Mailer sends email through the mailer the current SMTP settings
	// describe. The owner of runtime settings swaps it on change.
	Mailer *settings.LiveMailer
	// AuthStore backs the request authentication chain.
	AuthStore identity.AuthStore

	deps      Deps
	fileLocks *sync.Map
}

// Build wires the service graph from deps.
func Build(deps Deps) (*Services, error) {
	if deps.DB == nil {
		return nil, errors.New("services: database is required")
	}
	if deps.Runtime == nil {
		return nil, errors.New("services: execution runtime is required")
	}
	if deps.Credentials == nil || deps.CredentialWriter == nil || deps.RevealPolicy == nil {
		return nil, errors.New("services: credential provider, credential writer, and reveal policy are required")
	}
	if deps.SecretSealer == nil {
		return nil, errors.New("services: secret sealer is required")
	}
	if deps.Keyring == nil {
		return nil, errors.New("services: keyring is required")
	}
	if deps.Policy == nil && deps.Enforcer != nil {
		deps.Policy = deps.Enforcer
	}
	if deps.Policy == nil {
		return nil, errors.New("services: policy evaluator is required")
	}
	if deps.Revoker == nil {
		deps.Revoker = deps.Runtime
	}
	if deps.Audit == nil {
		deps.Audit = audit.Discard
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if deps.TargetPolicy == nil {
		deps.TargetPolicy = settings.NewTargetPolicy(deps.DB)
	}
	if deps.Invitations == nil {
		deps.Invitations = orgs.InvitationsDisabled
	}

	s := &Services{
		Policy:    deps.Policy,
		Runtime:   deps.Runtime,
		Mailer:    settings.NewLiveMailer(),
		AuthStore: deps.DB,
		deps:      deps,
		fileLocks: &sync.Map{},
	}
	s.Requirer = access.NewRequirer(directoryAdapter{store: deps.DB}, deps.Policy)
	s.Settings = settings.New(settings.Deps{
		Store:           deps.DB,
		Requirer:        s.Requirer,
		Sealer:          deps.SecretSealer,
		FileStorageMode: deps.Config.Files.StorageMode,
	})
	identityDeps := identity.Deps{
		Store:    deps.DB,
		SignIn:   deps.SignIn,
		Setup:    deps.Setup,
		FirstRun: firstRunStore{db: deps.DB, enforcer: deps.Enforcer},
		Requirer: s.Requirer,
		Tokens:   accessTokenPolicy{settings: s.Settings},
		Secret:   deps.Config.JWT.SecretKey,
		Audit:    deps.Audit,
		Logger:   deps.Logger,
	}
	if deps.Enforcer != nil {
		identityDeps.Cache = deps.Enforcer
	}
	s.Identity = identity.New(identityDeps)
	s.JobStore = jobs.NewStore(deps.DB)
	s.Jobs = jobs.NewService(s.JobStore, s.Requirer)

	defaults := database.DefaultInstanceSettings()
	s.Files = s.newFiles(defaults.FileRevisionsEnabled, defaults.FileRevisionsKeepLatest)

	s.Navigator = schema.NewNavigator(deps.DB, deps.Logger)
	s.Query = query.New(query.Deps{
		Store:       deps.DB,
		Settings:    queryPolicies{settings: s.Settings},
		Requirer:    s.Requirer,
		Runtime:     deps.Runtime,
		SchemaStore: deps.DB,
		Navigator:   s.Navigator,
		Audit:       deps.Audit,
		Logger:      deps.Logger,
	})

	s.Exports = exportjob.New(exportjob.Deps{
		Requirer: s.Requirer,
		Queue:    s.JobStore,
		Runtime:  deps.Runtime,
		Settings: exportPolicies{settings: s.Settings},
		Files:    exportFiles{services: s},
		Audit:    deps.Audit,
		Logger:   deps.Logger,
	})

	catalogDeps := catalog.Deps{
		Store:        deps.DB,
		Workspaces:   workspaceStore{DB: deps.DB, enforcer: deps.Enforcer},
		Policy:       deps.Policy,
		Requirer:     s.Requirer,
		Credentials:  deps.Credentials,
		Writer:       deps.CredentialWriter,
		RevealPolicy: deps.RevealPolicy,
		Audit:        deps.Audit,
		Logger:       deps.Logger,
		TargetPolicy: deps.TargetPolicy,
		Prober:       deps.Runtime,
		Revoker:      queuedRevoker{revoker: deps.Revoker, queue: s.JobStore, logger: deps.Logger},
	}
	// A nil *Enforcer stored in the interface would not compare equal to nil.
	if deps.Enforcer != nil {
		catalogDeps.Ancestry = deps.Enforcer
		catalogDeps.Cache = deps.Enforcer
	}
	s.Catalog = catalog.New(catalogDeps)

	store := orgStore{DB: deps.DB, enforcer: deps.Enforcer}
	orgDeps := orgs.Deps{
		Store:            store,
		Teams:            deps.DB,
		Requirer:         s.Requirer,
		Revoker:          queuedRevoker{revoker: deps.Revoker, queue: s.JobStore, logger: deps.Logger},
		Invitations:      store,
		InvitationPolicy: deps.Invitations,
		Mailer:           invitationMailer{live: s.Mailer},
		BaseURL:          instanceBaseURL{settings: s.Settings},
		Audit:            deps.Audit,
		Logger:           deps.Logger,
	}
	if deps.Enforcer != nil {
		orgDeps.Cache = deps.Enforcer
	}
	s.Orgs = orgs.New(orgDeps)
	s.forgetSchemaOnPurge()

	s.RBAC = rbac.New(rbac.Deps{
		Store:    rbacStore{DB: deps.DB, enforcer: deps.Enforcer},
		Requirer: s.Requirer,
		Policy:   deps.Policy,
		Audit:    deps.Audit,
		Logger:   deps.Logger,
	})
	s.KeyRotation = keyrotation.New(keyrotation.Deps{
		Connections: credentials.NewRotator(deps.DB, deps.Keyring, engine.ConnectionSpecFor),
		SMTP:        smtpRotation{settings: s.Settings, keys: deps.Keyring},
		Files:       files.NewContentRotator(deps.DB, deps.FileStores, deps.Keyring),
		Logger:      deps.Logger,
	})
	return s, nil
}

// connectionEmptyNotifier is implemented by runtimes that can report when the
// last live session of a connection closes.
type connectionEmptyNotifier interface {
	OnConnectionEmpty(func(connectionID string))
}

// forgetSchemaOnPurge drops a connection's in-memory schema cache when its
// persisted snapshots are purged or its last live session closes, so the next
// read reloads from the store or a new session.
func (s *Services) forgetSchemaOnPurge() {
	s.Orgs.OnSchemaCachePurged(s.Navigator.ForgetConnection)
	notifier, ok := s.Runtime.(connectionEmptyNotifier)
	if !ok {
		return
	}
	notifier.OnConnectionEmpty(func(connectionID string) {
		if id, err := strconv.ParseInt(connectionID, 10, 64); err == nil {
			s.Navigator.ForgetConnection(id)
		}
	})
}

// smtpRotation binds the keyring to the settings service's SMTP rotation.
type smtpRotation struct {
	settings *settings.Service
	keys     settings.KeyRotator
}

func (r smtpRotation) RotateSMTPPassword(ctx context.Context) (bool, bool, error) {
	return r.settings.RotateSMTPPassword(ctx, r.keys)
}

// FilesForWorkspace returns a file service that applies the workspace's
// effective file revision policy.
func (s *Services) FilesForWorkspace(ctx context.Context, ws access.Workspace) (*files.Service, error) {
	effective, err := s.Settings.EffectiveForWorkspace(ctx, ws)
	if err != nil {
		return nil, err
	}
	return s.newFiles(effective.FileRevisionsEnabled, effective.FileRevisionsKeepLatest), nil
}

// NewMailer builds the mailer the instance SMTP settings describe, opening
// the stored SMTP password with the keyring. The caller installs it on Mailer.
func (s *Services) NewMailer(in settings.Instance) (*smtp.Mailer, error) {
	return settings.NewMailer(in, s.deps.Keyring)
}

// SetQueryTracing switches tracing of metadata database queries, which the
// runtime settings control.
func (s *Services) SetQueryTracing(enabled bool) {
	s.deps.DB.SetQueryTracing(enabled)
}

// FilesForPrincipal returns the file service for an organization workspace
// the principal can reach. A non-member is access.ErrForbidden and a
// workspace outside the organization is access.ErrNotFound; the returned
// service still authorizes each file operation.
func (s *Services) FilesForPrincipal(ctx context.Context, p access.Principal, orgID, workspaceID int64) (*files.Service, error) {
	ws, err := s.Requirer.Workspace(ctx, p, orgID, workspaceID, "")
	if err != nil {
		return nil, err
	}
	return s.FilesForWorkspace(ctx, ws)
}

// newFiles builds a file service. Every instance shares one lock map so
// process-local serialization of a workspace tree holds across policies.
func (s *Services) newFiles(revisionsEnabled bool, keepLatest int) *files.Service {
	revisionPolicy := files.RevisionPolicyDisabled
	if revisionsEnabled {
		revisionPolicy = files.RevisionPolicyVersioned
	}
	return files.NewService(files.Deps{
		DB:       s.deps.DB,
		Stores:   s.deps.FileStores,
		Enforcer: s.deps.Policy,
		Requirer: s.Requirer,
		Audit:    s.deps.Audit,
		Logger:   s.deps.Logger,
	}, files.Config{
		StorageMode:            s.deps.Config.Files.StorageMode,
		ActiveStorageBackendID: s.deps.Config.Files.ActiveStorageBackend,
		RevisionPolicy:         revisionPolicy,
		RevisionKeepLatest:     keepLatest,
	}, s.fileLocks)
}

// queryPolicies projects each workspace's effective settings onto the query
// policy.
type queryPolicies struct{ settings *settings.Service }

func (q queryPolicies) ForWorkspace(ctx context.Context, ws access.Workspace) (query.Policy, error) {
	e, err := q.settings.EffectiveForWorkspace(ctx, ws)
	if err != nil {
		return query.Policy{}, err
	}
	return query.Policy{
		HistoryMode:      e.QueryHistoryMode,
		HistoryRetention: e.QueryHistoryRetentionCount,
		FavoritesMode:    e.QueryFavoritesMode,
		MaxRows:          e.QueryMaxResultRows,
		MaxBytes:         e.QueryMaxResultBytes,
		CursorPageSize:   e.QueryCursorPageSize,
	}, nil
}

// accessTokenPolicy reads the access token lifetime from the instance-wide
// effective settings.
type accessTokenPolicy struct{ settings *settings.Service }

func (p accessTokenPolicy) AccessTokenTTL(ctx context.Context) (time.Duration, error) {
	e, err := p.settings.EffectiveForOrg(ctx, nil)
	if err != nil {
		return 0, err
	}
	return e.JWTAccessTokenTTL, nil
}

// exportPolicies projects each workspace's effective settings onto the
// export policy.
type exportPolicies struct{ settings *settings.Service }

func (p exportPolicies) ForWorkspace(ctx context.Context, ws access.Workspace) (exportjob.Policy, error) {
	e, err := p.settings.EffectiveForWorkspace(ctx, ws)
	if err != nil {
		return exportjob.Policy{}, err
	}
	return exportjob.Policy{SyncMaxBytes: e.ExportsSyncMaxBytes, BackgroundMaxBytes: e.ExportsBackgroundMaxBytes}, nil
}

type exportFiles struct{ services *Services }

func (f exportFiles) ForWorkspace(ctx context.Context, ws access.Workspace) (exportjob.FileWriter, error) {
	return f.services.FilesForWorkspace(ctx, ws)
}

// queuedRevoker revokes live sessions for the catalog. A failed revocation
// is queued for retry instead of failing the update that already committed.
type queuedRevoker struct {
	revoker execution.Revoker
	queue   *jobs.DatabaseStore
	logger  *slog.Logger
}

func (r queuedRevoker) CountForConnection(ctx context.Context, connectionID string) (int, error) {
	return r.revoker.CountForConnection(ctx, connectionID)
}

func (r queuedRevoker) RevokeConnection(ctx context.Context, connectionID string) (int, error) {
	in := jobs.SessionRevokeInput{Kind: jobs.SessionRevokeConnection, ConnectionID: connectionID}
	return r.revoke(ctx, in, func(ctx context.Context) (int, error) {
		return r.revoker.RevokeConnection(ctx, connectionID)
	})
}

func (r queuedRevoker) RevokeWorkspaceAccount(ctx context.Context, workspaceID, accountID string) (int, error) {
	in := jobs.SessionRevokeInput{Kind: jobs.SessionRevokeWorkspaceAccount, WorkspaceID: workspaceID, AccountID: accountID}
	return r.revoke(ctx, in, func(ctx context.Context) (int, error) {
		return r.revoker.RevokeWorkspaceAccount(ctx, workspaceID, accountID)
	})
}

func (r queuedRevoker) RevokeOrgAccount(ctx context.Context, orgID, accountID string) (int, error) {
	in := jobs.SessionRevokeInput{Kind: jobs.SessionRevokeOrgAccount, OrgID: orgID, AccountID: accountID}
	return r.revoke(ctx, in, func(ctx context.Context) (int, error) {
		return r.revoker.RevokeOrgAccount(ctx, orgID, accountID)
	})
}

// revoke runs after the access change committed, so a caller's cancellation
// must neither skip the revocation nor lose the retry.
func (r queuedRevoker) revoke(ctx context.Context, in jobs.SessionRevokeInput, apply func(context.Context) (int, error)) (int, error) {
	syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobs.SessionRevokeSyncTimeout)
	defer cancel()
	revoked, err := apply(syncCtx)
	if err == nil {
		return revoked, nil
	}
	r.logger.Warn("session revocation failed; queueing retry", attrsToAny(append(in.LogAttrs(), slog.String("error", err.Error())))...)
	_, _, enqueueErr := r.queue.EnqueueSingleton(context.WithoutCancel(ctx), jobs.EnqueueInput{
		Type:         jobs.TypeSessionRevoke,
		SingletonKey: in.UniqueKey(),
		Visibility:   jobs.VisibilityInternal,
		Priority:     jobs.PriorityHigh,
		MaxAttempts:  jobs.SessionRevokeMaxAttempts,
		Input:        in,
	})
	if enqueueErr != nil && !errors.Is(enqueueErr, jobs.ErrActiveExists) {
		r.logger.Warn("session revocation retry could not be queued", slog.String("error", enqueueErr.Error()))
	}
	return 0, nil
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, len(attrs))
	for i, attr := range attrs {
		out[i] = attr
	}
	return out
}
