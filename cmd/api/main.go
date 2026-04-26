package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/smtp"
	"github.com/sqlwarden/internal/version"

	"github.com/lmittmann/tint"

	_ "github.com/sqlwarden/internal/driver/mysql"
	_ "github.com/sqlwarden/internal/driver/postgres"
	_ "github.com/sqlwarden/internal/driver/sqlite"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	err := run(logger)
	if err != nil {
		trace := string(debug.Stack())
		logger.Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

type config struct {
	baseURL               string
	httpPort              int
	desktopMode           bool
	personalSpacesEnabled bool
	cookie                struct {
		secretKey string
	}
	db struct {
		logQueries  bool
		driver      string
		dsn         string
		automigrate bool
	}
	encryption struct {
		key string
	}
	jwt struct {
		secretKey      string
		accessTokenTTL time.Duration
	}
	notifications struct {
		email string
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		from     string
	}
}

type application struct {
	config      config
	db          *database.DB
	logger      *slog.Logger
	mailer      *smtp.Mailer
	wg          sync.WaitGroup
	connManager *connection.Manager
	encKey      []byte
	enforcer    *access.Enforcer
}

func run(logger *slog.Logger) error {
	cfg, showVersion, err := loadConfig(os.Args[1:])
	if err != nil {
		return err
	}

	if showVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}

	db, err := database.New(cfg.db.driver, cfg.db.dsn, logger, cfg.db.logQueries)
	if err != nil {
		return err
	}
	defer db.Close()

	if cfg.db.automigrate {
		err = db.MigrateUp()
		if err != nil {
			return err
		}
	}

	mailer, err := smtp.NewMailer(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.from)
	if err != nil {
		return err
	}

	enforcer, err := access.New(db.DB)
	if err != nil {
		return fmt.Errorf("enforcer init: %w", err)
	}

	connMgr := connection.New(30 * time.Minute)
	defer connMgr.Close()

	app := &application{
		config:      cfg,
		db:          db,
		logger:      logger,
		mailer:      mailer,
		connManager: connMgr,
		encKey:      encrypt.DeriveKey(cfg.encryption.key),
		enforcer:    enforcer,
	}

	return app.serveHTTP()
}
