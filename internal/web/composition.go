package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/authorization"
	"github.com/sqlwarden/distribution"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
)

type enforcerAuthorizer struct{ enforcer *access.Enforcer }

func (a enforcerAuthorizer) Authorize(ctx context.Context, request authorization.Request) authorization.Decision {
	allowed := a.enforcer.Can(ctx, request.AccountID, request.OrgID, request.OwnerType, request.ResourceType, request.ResourceID, request.Permission)
	if !allowed {
		return authorization.Decision{Code: "not_permitted", Message: "You do not have permission to perform this action."}
	}
	return authorization.Decision{Allowed: true}
}

func (a enforcerAuthorizer) EffectivePermissions(ctx context.Context, request authorization.Request) ([]string, error) {
	return a.enforcer.EffectivePermissions(ctx, request.AccountID, request.OrgID, request.OwnerType, request.ResourceType, request.ResourceID)
}

type authorizerCanAdapter struct{ authorizer authorization.Authorizer }

func (a authorizerCanAdapter) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return a.authorizer.Authorize(ctx, authorization.Request{AccountID: accountID, OrgID: orgID, OwnerType: ownerType, ResourceType: resourceType, ResourceID: resourceID, Permission: permission}).Allowed
}

func (app *application) authorize(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) authorization.Decision {
	if app.authorizer == nil && app.enforcer != nil {
		app.authorizer = enforcerAuthorizer{enforcer: app.enforcer}
	}
	if app.authorizer == nil {
		return authorization.Decision{Code: "not_permitted", Message: "You do not have permission to perform this action."}
	}
	return app.authorizer.Authorize(ctx, authorization.Request{AccountID: accountID, OrgID: orgID, OwnerType: ownerType, ResourceType: resourceType, ResourceID: resourceID, Permission: permission})
}

func (app *application) hostServices(base authorization.Authorizer) distribution.HostServices {
	return distribution.HostServices{
		Logger: app.logger, DB: app.db.DB, Authorization: base,
		Accounts: distributionAccounts{app: app}, Organizations: distributionOrganizations{app: app},
		Sessions: distributionSessions{app: app}, Jobs: distributionJobs{app: app}, Request: distributionRequestContext{},
	}
}

func (app *application) installDistributionDependencies(ctx context.Context) error {
	for _, permission := range app.distribution.Permissions {
		err := app.permissions.Add(access.PermissionRegistration{
			ID: permission.ID, Label: permission.Label, Description: permission.Description, Group: permission.Group,
			Scopes: permission.Scopes, Resources: permission.Resources,
			DefaultOrgRoles: permission.DefaultOrgRoles, DefaultWorkspaceRoles: permission.DefaultWorkspaceRoles,
		})
		if err != nil {
			return fmt.Errorf("register distribution permission %q: %w", permission.ID, err)
		}
	}
	for _, migrations := range app.distribution.Migrations {
		if !app.config.DB.Automigrate {
			continue
		}
		path := migrations.PostgresPath
		if app.config.DB.Driver == "sqlite" {
			path = migrations.SQLitePath
		}
		if migrations.FS == nil || path == "" || migrations.MigrationsName == "" {
			return fmt.Errorf("distribution migration set is incomplete")
		}
		app.logger.InfoContext(ctx, "running distribution migrations", "migration.ledger", migrations.MigrationsName)
		if err := app.db.MigrateFS(migrations.FS, path, migrations.MigrationsName); err != nil {
			return fmt.Errorf("run distribution migrations %q: %w", migrations.MigrationsName, err)
		}
	}
	if err := app.enforcer.ReconcileBuiltinRolePermissions(ctx); err != nil {
		return fmt.Errorf("reconcile distribution permissions: %w", err)
	}
	app.authorizer = authorization.Constrained{Base: app.authorizer, Constraint: app.distribution.AuthorizationConstraint}
	for _, definition := range app.distribution.Jobs {
		if definition.Type == "" || definition.Handler == nil {
			return fmt.Errorf("distribution job definition is incomplete")
		}
		if _, exists := app.jobRegistry.Definition(definition.Type); exists {
			return fmt.Errorf("distribution job type %q already exists", definition.Type)
		}
		definition := definition
		app.jobRegistry.Register(jobs.Definition{
			Type: definition.Type, MaxAttempts: definition.MaxAttempts, Backoff: definition.Backoff, Timeout: definition.Timeout,
			Handler: jobs.HandlerFunc(func(ctx context.Context, runtime jobs.Runtime) (any, error) {
				result, err := definition.Handler.Handle(ctx, distributionJobRuntime{runtime: runtime})
				var jobErr distribution.JobError
				if errors.As(err, &jobErr) {
					if jobErr.Retryable {
						return nil, jobs.Retryable(jobErr.Code, jobErr.Message)
					}
					return nil, jobs.Permanent(jobErr.Code, jobErr.Message)
				}
				return result, err
			}),
		})
	}
	return nil
}

type distributionAccounts struct{ app *application }

func (s distributionAccounts) FindByID(ctx context.Context, id int64) (distribution.Account, bool, error) {
	account, found, err := s.app.db.GetAccount(ctx, id)
	return distributionAccount(account), found, err
}
func (s distributionAccounts) FindByEmail(ctx context.Context, email string) (distribution.Account, bool, error) {
	account, found, err := s.app.db.GetAccountByEmail(ctx, strings.TrimSpace(email))
	return distributionAccount(account), found, err
}
func (s distributionAccounts) Provision(ctx context.Context, email, name string) (distribution.Account, error) {
	account, err := s.app.db.InsertAccount(ctx, strings.TrimSpace(email), strings.TrimSpace(name), nil)
	return distributionAccount(account), err
}
func distributionAccount(account database.Account) distribution.Account {
	return distribution.Account{ID: account.ID, Email: account.Email, Name: account.Name, IsActive: account.IsActive}
}

type distributionOrganizations struct{ app *application }

func (s distributionOrganizations) FindByID(ctx context.Context, id int64) (distribution.Organization, bool, error) {
	org, found, err := s.app.db.GetOrg(ctx, id)
	return distributionOrganization(org), found, err
}
func (s distributionOrganizations) FindBySlug(ctx context.Context, slug string) (distribution.Organization, bool, error) {
	org, found, err := s.app.db.GetOrgBySlug(ctx, strings.TrimSpace(slug))
	return distributionOrganization(org), found, err
}
func (s distributionOrganizations) IsMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	return s.app.db.IsOrgMember(ctx, orgID, accountID)
}
func (s distributionOrganizations) AddMember(ctx context.Context, orgID, accountID int64) error {
	if err := s.app.db.AddOrgMember(ctx, orgID, accountID); err != nil {
		return err
	}
	s.app.enforcer.InvalidatePrincipals(orgID, accountID)
	return nil
}
func distributionOrganization(org database.Organization) distribution.Organization {
	return distribution.Organization{ID: org.ID, Slug: org.Slug, Name: org.Name}
}

type distributionSessions struct{ app *application }

func (s distributionSessions) Complete(w http.ResponseWriter, r *http.Request, accountID int64) error {
	return s.app.completeDistributionSession(w, r, accountID)
}
func (s distributionSessions) Revoke(ctx context.Context, sessionID string, revokedBy *int64, reason string) error {
	return s.app.db.RevokeAuthSession(ctx, sessionID, revokedBy, reason)
}

func (app *application) completeDistributionSession(w http.ResponseWriter, r *http.Request, accountID int64) error {
	account, found, err := app.db.GetAccount(r.Context(), accountID)
	if err != nil {
		return err
	}
	if !found || !account.IsActive {
		return errors.New("account is unavailable")
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	refresh := database.NewID()
	session, _, err := app.db.CreateAuthSessionWithRefreshToken(r.Context(), account.ID, expiresAt, r.Header.Get("User-Agent"), r.RemoteAddr, token.Hash(refresh), refresh)
	if err != nil {
		return err
	}
	accessToken, _, err := token.IssueWithSessionTTL(strconv.FormatInt(account.ID, 10), session.ID, account.Email, account.Name, app.config.JWT.SecretKey, app.config.JWT.AccessTokenTTL)
	if err != nil {
		return err
	}
	http.SetCookie(w, app.refreshTokenCookie(r, refresh, 7*24*3600))
	return response.JSON(w, http.StatusOK, map[string]string{"access_token": accessToken})
}

type distributionRequestContext struct{}

func (distributionRequestContext) Account(r *http.Request) (distribution.Account, bool) {
	account := contextGetAccount(r)
	return distributionAccount(account), account.ID != 0
}
func (distributionRequestContext) OrganizationID(r *http.Request) (int64, bool) {
	id := contextGetOrg(r).ID
	return id, id != 0
}
func (distributionRequestContext) WorkspaceID(r *http.Request) (int64, bool) {
	id := contextGetWorkspace(r).ID
	return id, id != 0
}
func (distributionRequestContext) EnvironmentID(r *http.Request) (int64, bool) {
	id := contextGetEnvironment(r).ID
	return id, id != 0
}
func (distributionRequestContext) ConnectionID(r *http.Request) (int64, bool) {
	id := contextGetConnection(r).ID
	return id, id != 0
}

type distributionJobs struct{ app *application }

func (s distributionJobs) Enqueue(ctx context.Context, input distribution.EnqueueJob) (string, error) {
	definition, ok := s.app.jobRegistry.Definition(input.Type)
	if !ok {
		return "", jobs.ErrUnknownType
	}
	jobInput := jobs.EnqueueInput{Type: input.Type, SingletonKey: input.SingletonKey, Visibility: input.Visibility, OrgID: input.OrgID, WorkspaceID: input.WorkspaceID, OwnerAccountID: input.OwnerAccountID, RunAt: input.RunAt, Priority: input.Priority, MaxAttempts: definition.MaxAttempts, Input: input.Input}
	if input.SingletonKey != "" {
		record, _, err := s.app.jobStore.EnqueueSingleton(ctx, jobInput)
		return record.ID, err
	}
	record, err := s.app.jobStore.Enqueue(ctx, jobInput)
	return record.ID, err
}

type distributionJobRuntime struct{ runtime jobs.Runtime }

func (r distributionJobRuntime) ID() string { return r.runtime.Job.ID }
func (r distributionJobRuntime) DecodeInput(value any) error {
	return json.Unmarshal([]byte(r.runtime.Job.InputJSON), value)
}
func (r distributionJobRuntime) Events() distribution.JobEventWriter {
	return distributionJobEvents{writer: r.runtime.Events}
}

type distributionJobEvents struct{ writer jobs.EventWriter }

func (e distributionJobEvents) Info(ctx context.Context, code, message string, details any) {
	e.writer.Info(ctx, code, message, details)
}
func (e distributionJobEvents) Warn(ctx context.Context, code, message string, details any) {
	e.writer.Warn(ctx, code, message, details)
}
func (e distributionJobEvents) Error(ctx context.Context, code, message string, details any) {
	e.writer.Error(ctx, code, message, details)
}
