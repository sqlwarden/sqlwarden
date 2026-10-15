package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/platform/clientip"
	"github.com/sqlwarden/internal/services"
	"github.com/sqlwarden/internal/settings"
)

type App = application

// Dependencies are the already-constructed resources the web application
// uses. The caller owns their lifecycle: closing the App never closes them.
// internal/architecture pins every field type, so web holds no handle that
// skips service authorization.
type Dependencies struct {
	Config config.Config
	Logger *slog.Logger
	// Services is the transport half of the service graph; every handler
	// reaches metadata, secrets, and sessions through it.
	Services *services.Services
	// SettingsAppliers put instance settings into effect below web, such as
	// the live mailer. They run before web's own appliers, in order.
	SettingsAppliers      []settings.Applier
	Edition               EditionCapabilities
	EditionHandler        http.Handler
	Authenticators        []identity.Authenticator
	DecorateAuthenticator func(identity.Authenticator) identity.Authenticator
	RequestPolicies       []identity.RequestPolicy
	PostureProviders      []identity.PostureProvider

	TrustedProxies []netip.Prefix

	Invitations orgs.InvitationPolicy
}

type application struct {
	config                config.Config
	logger                *slog.Logger
	wg                    sync.WaitGroup
	services              *services.Services
	settingsAppliers      []settings.Applier
	initialSettings       settings.Instance
	accessLogsEnabled     atomic.Bool
	invitationPolicy      orgs.InvitationPolicy
	clientIPs             clientip.Resolver
	authChain             identity.Chain
	edition               EditionCapabilities
	editionHandler        http.Handler
	authenticators        []identity.Authenticator
	decorateAuthenticator func(identity.Authenticator) identity.Authenticator
	requestPolicies       []identity.RequestPolicy
	postureProviders      []identity.PostureProvider
}

// NewApplication wires the web application from its dependencies. It applies
// the stored instance settings but starts no background work; the caller runs
// the settings watcher and job runner.
func NewApplication(deps Dependencies) (*App, error) {
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if deps.Invitations == nil {
		return nil, errors.New("web: invitation policy is required")
	}
	if deps.Services == nil {
		return nil, errors.New("web: services are required")
	}
	app := &application{
		invitationPolicy:      deps.Invitations,
		config:                deps.Config,
		logger:                logger,
		services:              deps.Services,
		settingsAppliers:      deps.SettingsAppliers,
		edition:               deps.Edition,
		editionHandler:        deps.EditionHandler,
		authenticators:        deps.Authenticators,
		decorateAuthenticator: deps.DecorateAuthenticator,
		requestPolicies:       deps.RequestPolicies,
		postureProviders:      deps.PostureProviders,
	}
	app.clientIPs = clientip.New(deps.TrustedProxies)
	app.authChain = app.newAuthChain()
	initialSettings, err := deps.Services.Settings.Instance(context.Background())
	if err != nil {
		return nil, err
	}
	if err := app.ApplyInstanceSettings(context.Background(), initialSettings); err != nil {
		return nil, err
	}
	app.initialSettings = initialSettings
	return app, nil
}

// Close waits for request-spawned background tasks. Sessions, cursors, the
// settings watcher, the job runner, and the database belong to the caller
// that created them.
func (app *application) Close() {
	startedAt := time.Now()
	app.wg.Wait()
	app.logger.Info("background workers stopped", "duration_ms", time.Since(startedAt).Milliseconds())
}

func (app *application) Handler() http.Handler {
	return app.routes()
}
