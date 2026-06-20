package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sqlwarden/internal/observability"
	"github.com/sqlwarden/internal/response"
)

const maxRequestIDLength = 128

func (app *application) requestLoggingContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := normalizeRequestID(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}

		w.Header().Set(requestIDHeader, requestID)
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
			if pv != nil {
				app.serverError(w, r, fmt.Errorf("%v", pv))
			}
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
		mw := response.NewMetricsResponseWriter(w)
		startedAt := time.Now()
		next.ServeHTTP(mw, r)

		duration := time.Since(startedAt)
		app.logger.LogAttrs(r.Context(), accessLogLevel(mw.StatusCode), "http request", accessLogAttrs(r, mw, duration)...)
	})
}

func normalizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > maxRequestIDLength {
		return ""
	}
	for _, r := range requestID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':', r == '/', r == '=':
		default:
			return ""
		}
	}
	return requestID
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
