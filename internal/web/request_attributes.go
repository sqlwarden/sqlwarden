package web

import (
	"net/http"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/identity"
)

func (app *application) requestAttributes(r *http.Request) access.RequestAttributes {
	return access.RequestAttributes{
		ClientIP:  app.clientIPs.Resolve(r.RemoteAddr, r.Header.Values("X-Forwarded-For")),
		UserAgent: r.Header.Get("User-Agent"),
		Time:      time.Now(),
	}
}

func (app *application) clientIP(r *http.Request) string {
	addr := app.clientIPs.Resolve(r.RemoteAddr, r.Header.Values("X-Forwarded-For"))
	if !addr.IsValid() {
		return ""
	}
	return addr.String()
}

func (app *application) newAuthChain() identity.Chain {
	core := identity.NewSessionAuthenticator(app.db, app.config.JWT.SecretKey, revocationFromContext, time.Now)
	if app.decorateAuthenticator != nil {
		core = app.decorateAuthenticator(core)
	}
	authenticators := []identity.Authenticator{core}
	authenticators = append(authenticators, app.authenticators...)
	return identity.Chain{
		Authenticators: authenticators,
		Posture:        append([]identity.PostureProvider(nil), app.postureProviders...),
		Policies:       append([]identity.RequestPolicy(nil), app.requestPolicies...),
	}
}
