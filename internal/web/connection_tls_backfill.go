package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

type backfillReport struct {
	Scanned  int
	Migrated int
}

// backfillConnectionTLSConfig lifts the sslmode / SSL verification knob out of
// stored DSNs into the structured tls_config_encrypted blob, once. Rows that
// already have a TLS blob are skipped, so it is safe to run on every boot.
func (app *application) backfillConnectionTLSConfig(ctx context.Context) (backfillReport, error) {
	var rep backfillReport
	conns, err := app.db.ListAllConnections(ctx)
	if err != nil {
		return rep, fmt.Errorf("tls backfill: list connections: %w", err)
	}
	for _, conn := range conns {
		rep.Scanned++
		if conn.TLSConfigEncrypted != "" {
			continue
		}
		if _, ok := tlsSpecForDriver(conn.Driver); !ok {
			continue // SQLite / non-network engine
		}
		plainDSN, err := app.keyring.Decrypt(conn.DSNEncrypted)
		if err != nil {
			return rep, fmt.Errorf("tls backfill: decrypt connection %d: %w", conn.ID, err)
		}
		mode, newDSN, changed := extractTLSModeFromDSN(conn.Driver, plainDSN)

		doc := tlsConfigDocument{Mode: string(mode)}
		sealed, err := app.sealTLSDocument(doc)
		if err != nil {
			return rep, fmt.Errorf("tls backfill: seal connection %d: %w", conn.ID, err)
		}
		if sealed == "" {
			// mode disable + nothing else: still record an explicit blob so the
			// row is not re-scanned every boot.
			sealed, err = app.keyring.Encrypt(`{"mode":"disable"}`)
			if err != nil {
				return rep, fmt.Errorf("tls backfill: seal disable %d: %w", conn.ID, err)
			}
		}
		if err := app.db.UpdateConnectionTLSConfig(ctx, conn.ID, sealed); err != nil {
			return rep, fmt.Errorf("tls backfill: write tls %d: %w", conn.ID, err)
		}
		if changed {
			reEnc, err := app.keyring.Encrypt(newDSN)
			if err != nil {
				return rep, fmt.Errorf("tls backfill: encrypt dsn %d: %w", conn.ID, err)
			}
			if err := app.db.UpdateConnectionDSN(ctx, conn.ID, reEnc); err != nil {
				return rep, fmt.Errorf("tls backfill: write dsn %d: %w", conn.ID, err)
			}
		}
		rep.Migrated++
	}
	app.logger.InfoContext(ctx, "connection tls backfill complete",
		slog.Int("scanned", rep.Scanned), slog.Int("migrated", rep.Migrated))
	return rep, nil
}

// extractTLSModeFromDSN maps the legacy per-driver verification knob to a
// TLSMode and returns the DSN with that knob removed.
func extractTLSModeFromDSN(driver, dsn string) (engine.TLSMode, string, bool) {
	u, err := url.Parse(dsn)
	if err != nil {
		return engine.TLSModeDisable, dsn, false
	}
	q := u.Query()
	switch engine.NormalizeName(driver) {
	case "postgres":
		if _, present := q["sslmode"]; !present {
			return engine.TLSModeDisable, dsn, false
		}
		mode := postgresSSLModeToTLSMode(q.Get("sslmode"))
		q.Del("sslmode")
		q.Del("sslrootcert")
		q.Del("sslcert")
		q.Del("sslkey")
		u.RawQuery = q.Encode()
		return mode, u.String(), true
	case "oracle":
		if !strings.EqualFold(q.Get("SSL"), "true") {
			return engine.TLSModeDisable, dsn, false
		}
		mode := engine.TLSModeRequire
		if strings.Contains(strings.ToUpper(q.Get("SSL VERIFY")), "FULL") {
			mode = engine.TLSModeVerifyFull
		}
		q.Del("SSL")
		q.Del("SSL VERIFY")
		u.RawQuery = q.Encode()
		return mode, u.String(), true
	default:
		return engine.TLSModeDisable, dsn, false
	}
}

func postgresSSLModeToTLSMode(v string) engine.TLSMode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "disable", "allow":
		return engine.TLSModeDisable
	case "prefer", "require":
		return engine.TLSModeRequire
	case "verify-ca":
		return engine.TLSModeVerifyCA
	case "verify-full":
		return engine.TLSModeVerifyFull
	default:
		return engine.TLSModeRequire
	}
}
