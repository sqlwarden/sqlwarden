package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	defaultIdleTimeout  = time.Minute
	defaultReadTimeout  = 5 * time.Second
	defaultWriteTimeout = 10 * time.Second
)

// TLSFiles names the certificate and key served when TLS is enabled.
type TLSFiles struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

// NewServer returns an HTTP server with the production timeouts. It does not
// listen. The caller owns ListenAndServe and Shutdown.
func NewServer(port int, handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      handler,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}
}
