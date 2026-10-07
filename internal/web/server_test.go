package web

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/config"
)

func TestServerConfiguration(t *testing.T) {
	t.Run("Default timeouts are reasonable", func(t *testing.T) {
		assert.True(t, defaultIdleTimeout > 0)
		assert.True(t, defaultReadTimeout > 0)
		assert.True(t, defaultWriteTimeout > defaultReadTimeout)

		if shutdown := config.Default().ShutdownTimeout; shutdown <= defaultWriteTimeout {
			t.Errorf("default shutdown timeout %s must be greater than default write timeout %s", shutdown, defaultWriteTimeout)
		}
	})
}

func TestNewServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("Applies the production timeouts", func(t *testing.T) {
		srv := NewServer(7000, http.NotFoundHandler(), logger)
		assert.Equal(t, srv.Addr, ":7000")
		assert.Equal(t, srv.IdleTimeout, defaultIdleTimeout)
		assert.Equal(t, srv.ReadTimeout, defaultReadTimeout)
		assert.Equal(t, srv.WriteTimeout, defaultWriteTimeout)
	})

	t.Run("Invalid port configuration causes an error", func(t *testing.T) {
		err := NewServer(-1, http.NotFoundHandler(), logger).ListenAndServe()
		assert.NotNil(t, err)
	})
}

func GetFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port
}
