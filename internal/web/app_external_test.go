package web_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/web"
)

func TestAppCanBeConstructedFromExternalPackage(t *testing.T) {
	cfg := web.DefaultConfig()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = t.TempDir() + "/sqlwarden.db"
	cfg.DB.Automigrate = true
	cfg.Files.Filesystem.RootDir = t.TempDir() + "/files"

	app, err := web.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	var _ http.Handler = app.Handler()
}
