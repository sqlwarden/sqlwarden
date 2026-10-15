package web

import (
	"log/slog"
	"net/http"

	"github.com/sqlwarden/internal/audit"
)

// emitAudit records event and never fails the request: a write failure is
// logged and the request proceeds.
func (app *application) emitAudit(r *http.Request, event audit.Event) {
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}
	event.Metadata["client_ip"] = app.clientIP(r)
	if err := audit.Emit(r.Context(), app.audit, event); err != nil {
		app.logWarn(r, "audit write failed", slog.String("action", event.Action), slog.Any("error", err))
	}
}
