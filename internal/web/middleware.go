package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/sqlwarden/internal/observability"
	"github.com/sqlwarden/internal/response"
)

func (app *application) requestLoggingContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := observability.NormalizeRequestID(r.Header.Get(observability.RequestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}

		w.Header().Set(observability.RequestIDHeader, requestID)
		meta := &requestLogContext{RequestID: requestID}
		r = contextSetRequestLogContext(r, meta)
		r = r.WithContext(observability.WithRequestID(r.Context(), requestID))

		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			pv := recover()
			if pv == nil {
				return
			}
			if pv == http.ErrAbortHandler {
				// http.ErrAbortHandler is net/http's documented sentinel for aborting
				// an in-flight response without logging or completing it normally —
				// re-panic so the real server (not our recovery middleware) handles it.
				panic(pv)
			}
			app.serverError(w, r, fmt.Errorf("%v", pv))
		}()

		next.ServeHTTP(w, r)
	})
}

// noStoreCache prevents the browser from HTTP-caching API responses. Without it,
// heuristically-cacheable error statuses (e.g. 410 Gone from an expired session)
// get stored in the browser disk cache and replayed on later requests, so a
// transient failure keeps being served from cache even after it resolves.
func (app *application) noStoreCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (app *application) logAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.accessLogsEnabled.Load() {
			next.ServeHTTP(w, r)
			return
		}
		mw := response.NewMetricsResponseWriter(w)
		startedAt := time.Now()
		next.ServeHTTP(mw, r)

		duration := time.Since(startedAt)
		app.logger.LogAttrs(r.Context(), accessLogLevel(mw.StatusCode), "http request", accessLogAttrs(r, mw, duration)...)
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
