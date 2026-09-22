package ee

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/edition/editiontest"
)

func enterpriseConfig() config.Config {
	cfg := config.Default()
	cfg.Edition.Name = config.EditionEnterprise
	cfg.Edition.LicenseFile = "/run/secrets/sqlwarden-license"
	return cfg
}

func TestEditionContract(t *testing.T) {
	cfg := enterpriseConfig()
	candidate, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	editiontest.Run(t, candidate, cfg)
}

func TestLicenseDenialIsOutermost(t *testing.T) {
	core := &allowingPolicy{}
	policy := newEnterprise(false).PolicyEvaluator(core)
	if policy.Can(context.Background(), 1, 2, "org", "workspace", 3, "workspace:read") {
		t.Fatal("unlicensed edition allowed a permission granted by the inner core policy")
	}
	if core.calls != 0 {
		t.Fatalf("inner policy calls = %d, want 0 when license denies first", core.calls)
	}
	permissions, err := policy.EffectivePermissions(context.Background(), 1, 2, "org", "workspace", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 0 {
		t.Fatalf("permissions = %v, want none", permissions)
	}
}

func TestSCIMMigrationUsesSeparateOrderedStream(t *testing.T) {
	db, err := database.New(
		"sqlite",
		filepath.Join(t.TempDir(), "enterprise.db"),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}

	candidate, err := New(enterpriseConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := edition.Migrate(context.Background(), candidate, db); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"schema_migrations", "ee_schema_migrations", "ee_scim_state"} {
		exists, err := db.NewSelect().TableExpr("sqlite_master").Where("type = 'table' AND name = ?", table).Exists(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("migration table %q does not exist", table)
		}
	}
}

func TestApplicationUsesEditionDecoratedPolicyEvaluator(t *testing.T) {
	cfg := enterpriseConfig()
	cfg.DB.DSN = filepath.Join(t.TempDir(), "application.db")
	cfg.Files.StorageBackends[config.DefaultFilesActiveBackend] = config.FileStorageBackend{
		Type: config.FilesStorageBackendFilesystem, RootDir: t.TempDir(),
	}
	built, err := coreapp.Build(context.Background(), coreapp.Options{
		Config: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Edition: newEnterprise(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer built.Close(context.Background())
	if _, ok := built.Services.PolicyEvaluator.(licensedPolicyEvaluator); !ok {
		t.Fatalf("policy evaluator type = %T, want Enterprise license decorator", built.Services.PolicyEvaluator)
	}
}

type allowingPolicy struct {
	calls int
}

func (p *allowingPolicy) Can(context.Context, int64, int64, string, string, int64, string) bool {
	p.calls++
	return true
}

func (p *allowingPolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	p.calls++
	return []string{"workspace:read"}, nil
}
