package web

import (
	"context"
	"errors"
	"log/slog"

	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/jobs"
)

type credentialPorts struct {
	provider credentials.Provider
	writer   credentials.Writer
	reveal   credentials.RevealPolicy
}

// catalogService builds the connection catalog on first use, after the policy
// evaluator and runtime have been wired.
func (app *application) catalogService() *catalog.Service {
	app.catalogOnce.Do(func() {
		deps := catalog.Deps{
			Store:        app.db,
			Policy:       app.policy,
			Credentials:  app.credentialPorts.provider,
			Writer:       app.credentialPorts.writer,
			RevealPolicy: app.credentialPorts.reveal,
			Audit:        app.audit,
			Logger:       app.logger,
			TargetPolicy: app.targetPolicy,
			Prober:       app.runtime,
			Revoker:      connectionRevoker{app: app},
		}
		// A nil *Enforcer stored in the interface would not compare equal to nil.
		if app.enforcer != nil {
			deps.Ancestry = app.enforcer
		}
		app.catalog = catalog.New(deps)
	})
	return app.catalog
}

// connectionRevoker revokes live sessions for the catalog. A failed revocation
// is queued for retry instead of failing the update that already committed.
type connectionRevoker struct{ app *application }

func (r connectionRevoker) CountForConnection(ctx context.Context, connectionID string) (int, error) {
	if r.app.revoker == nil {
		return 0, errors.New("session revoker is not configured")
	}
	return r.app.revoker.CountForConnection(ctx, connectionID)
}

func (r connectionRevoker) RevokeConnection(ctx context.Context, connectionID string) (int, error) {
	in := sessionRevokeInput{Kind: sessionRevokeConnection, ConnectionID: connectionID}
	syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionRevokeSyncTimeout)
	defer cancel()
	revoked, err := r.app.applySessionRevoke(syncCtx, in)
	if err == nil {
		return revoked, nil
	}
	r.app.logger.Warn("session revocation failed; queueing retry", append(attrsToAny(in.logAttrs()), slog.String("error", err.Error()))...)
	_, _, enqueueErr := r.app.workspaceJobStore().EnqueueSingleton(context.WithoutCancel(ctx), jobs.EnqueueInput{
		Type:         jobs.TypeSessionRevoke,
		SingletonKey: in.uniqueKey(),
		Visibility:   jobs.VisibilityInternal,
		Priority:     jobs.PriorityHigh,
		MaxAttempts:  sessionRevokeMaxAttempts,
		Input:        in,
	})
	if enqueueErr != nil && !errors.Is(enqueueErr, jobs.ErrActiveExists) {
		r.app.logger.Warn("session revocation retry could not be queued", slog.String("error", enqueueErr.Error()))
	}
	return 0, nil
}
