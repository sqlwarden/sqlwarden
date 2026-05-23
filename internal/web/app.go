package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/smtp"
)

type App = application

type application struct {
	config      Config
	db          *database.DB
	logger      *slog.Logger
	mailer      *smtp.Mailer
	wg          sync.WaitGroup
	connManager *connection.Manager
	encKey      []byte
	enforcer    *access.Enforcer
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, logger, cfg.DB.LogQueries)
	if err != nil {
		return nil, err
	}

	if cfg.DB.Automigrate {
		if err := db.MigrateUp(); err != nil {
			db.Close()
			return nil, err
		}
	}

	mailer, err := smtp.NewMailer(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From)
	if err != nil {
		db.Close()
		return nil, err
	}

	enforcer, err := access.New(db.DB)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("enforcer init: %w", err)
	}

	return &application{
		config:      cfg,
		db:          db,
		logger:      logger,
		mailer:      mailer,
		connManager: connection.New(30 * time.Minute),
		encKey:      encrypt.DeriveKey(cfg.Encryption.Key),
		enforcer:    enforcer,
	}, nil
}

func (app *application) Handler() http.Handler {
	return app.routes()
}

func (app *application) Close() error {
	app.wg.Wait()

	if app.connManager != nil {
		app.connManager.Close()
	}

	if app.db != nil {
		app.db.Close()
	}

	return nil
}
