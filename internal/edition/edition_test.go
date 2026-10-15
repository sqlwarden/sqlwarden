package edition

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/jobs"
)

type testEdition struct {
	name     string
	licenser Licenser
	modules  []Module
}

func (e testEdition) Name() string       { return e.name }
func (e testEdition) Licenser() Licenser { return e.licenser }
func (e testEdition) Modules() []Module  { return e.modules }

type testModule struct {
	name, feature string
	register      func(*Registrar) error
}

func (m testModule) Name() string    { return m.name }
func (m testModule) Feature() string { return m.feature }
func (m testModule) Register(r *Registrar) error {
	if m.register == nil {
		return nil
	}
	return m.register(r)
}

func licensed(features ...string) Licenser {
	return LicenserFunc(func(context.Context) (License, error) {
		result := make(map[string]bool, len(features))
		for _, feature := range features {
			result[feature] = true
		}
		return License{Features: result}, nil
	})
}

func TestComposeFiltersUnlicensedModulesAndReportsState(t *testing.T) {
	called := false
	composition, err := Compose(context.Background(), testEdition{
		name:     editionNameForTest,
		licenser: licensed(),
		modules: []Module{testModule{name: "audit", feature: Catalog[0].Key, register: func(*Registrar) error {
			called = true
			return nil
		}}},
	}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("unlicensed module was registered")
	}
	if got := composition.Capabilities().Features[0].State; got != StateUnlicensed {
		t.Fatalf("state = %q, want %q", got, StateUnlicensed)
	}
}

func TestComposeTreatsExpiredLicenseAsUnlicensed(t *testing.T) {
	expired := time.Now().Add(-time.Second)
	composition, err := Compose(context.Background(), testEdition{
		name: editionNameForTest,
		licenser: LicenserFunc(func(context.Context) (License, error) {
			return License{Features: map[string]bool{Catalog[0].Key: true}, ExpiresAt: &expired}, nil
		}),
		modules: []Module{testModule{name: "audit", feature: Catalog[0].Key}},
	}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if got := composition.Capabilities().Features[0].State; got != StateUnlicensed {
		t.Fatalf("state = %q, want %q", got, StateUnlicensed)
	}
}

func TestComposeRejectsInvalidModulesAndDuplicateContributions(t *testing.T) {
	tests := map[string][]Module{
		"nil module": {nil},
		"duplicate module": {
			testModule{name: "audit", feature: Catalog[0].Key},
			testModule{name: "audit", feature: Catalog[0].Key},
		},
		"duplicate route": {testModule{name: "audit", feature: Catalog[0].Key, register: func(r *Registrar) error {
			if err := r.Route(httpHandler{}); err != nil {
				return err
			}
			return r.Route(httpHandler{})
		}}},
		"duplicate migration": {testModule{name: "audit", feature: Catalog[0].Key, register: func(r *Registrar) error {
			migration := testMigration{name: "enterprise"}
			if err := r.Migration(migration); err != nil {
				return err
			}
			return r.Migration(migration)
		}}},
		"duplicate job": {testModule{name: "audit", feature: Catalog[0].Key, register: func(r *Registrar) error {
			definition := jobs.Definition{Type: "audit.reconcile"}
			if err := r.Job(definition); err != nil {
				return err
			}
			return r.Job(definition)
		}}},
	}
	for name, modules := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Compose(context.Background(), testEdition{name: editionNameForTest, licenser: licensed(Catalog[0].Key), modules: modules}, Dependencies{})
			if err == nil {
				t.Fatal("Compose succeeded")
			}
		})
	}
}

func TestDecoratorOrderAndCapabilityCopies(t *testing.T) {
	var order []string
	module := testModule{name: "audit", feature: Catalog[0].Key, register: func(r *Registrar) error {
		for _, name := range []string{"first", "second"} {
			name := name
			if err := r.DecorateAudit(func(next audit.Writer) audit.Writer {
				return audit.WriterFunc(func(ctx context.Context, event audit.Event) error {
					order = append(order, name)
					return next.Write(ctx, event)
				})
			}); err != nil {
				return err
			}
		}
		return nil
	}}
	composition, err := Compose(context.Background(), testEdition{name: editionNameForTest, licenser: licensed(Catalog[0].Key), modules: []Module{module}}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	writer := composition.Audit(audit.WriterFunc(func(context.Context, audit.Event) error {
		order = append(order, "core")
		return nil
	}))
	if err := writer.Write(context.Background(), audit.Event{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"second", "first", "core"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	copy := composition.Capabilities()
	copy.Features[0].State = StateUpgrade
	if !composition.Capabilities().Available(Catalog[0].Key) {
		t.Fatal("mutating capability copy changed composition")
	}
}

func TestPolicyDecoratorCannotBypassCoreAndMayRestrict(t *testing.T) {
	module := testModule{name: "audit", feature: Catalog[0].Key, register: func(r *Registrar) error {
		return r.DecoratePolicy(func(access.PolicyEvaluator) access.PolicyEvaluator {
			return fixedPolicy{allow: true, permissions: []string{"read", "write"}}
		})
	}}
	composition, err := Compose(context.Background(), testEdition{name: editionNameForTest, licenser: licensed(Catalog[0].Key), modules: []Module{module}}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	policy := composition.Policy(fixedPolicy{permissions: []string{"read"}})
	if policy.Can(context.Background(), 0, 0, "", "", 0, "read") {
		t.Fatal("decorator bypassed core denial")
	}
	permissions, err := policy.EffectivePermissions(context.Background(), 0, 0, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(permissions, []string{"read"}) {
		t.Fatalf("permissions = %v", permissions)
	}
}

func TestComposeDelegatesLicenserError(t *testing.T) {
	want := errors.New("license unavailable")
	_, err := Compose(context.Background(), testEdition{name: editionNameForTest, licenser: LicenserFunc(func(context.Context) (License, error) {
		return License{}, want
	})}, Dependencies{})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func TestMigrateValidatesEveryStreamBeforeApplyingAnyAndPreservesOrder(t *testing.T) {
	var order []string
	compatible := CoreCompatibility{Minimum: database.CoreMigrationVersion, Maximum: database.CoreMigrationVersion}
	streams := []MigrationStream{
		testMigration{name: "first", compatibility: compatible, migrate: func() { order = append(order, "first") }},
		testMigration{name: "second", compatibility: compatible, migrate: func() { order = append(order, "second") }},
	}
	if err := Migrate(context.Background(), nil, streams); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Fatalf("migration order = %v", order)
	}

	order = nil
	streams = append(streams, testMigration{name: "incompatible", compatibility: CoreCompatibility{Minimum: database.CoreMigrationVersion + 1}, migrate: func() { order = append(order, "incompatible") }})
	if err := Migrate(context.Background(), nil, streams); err == nil {
		t.Fatal("incompatible stream was accepted")
	}
	if len(order) != 0 {
		t.Fatalf("migration ran before compatibility validation completed: %v", order)
	}
}

const editionNameForTest = "test"

type httpHandler struct{}

func (httpHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

type testMigration struct {
	name          string
	compatibility CoreCompatibility
	migrate       func()
}

func (m testMigration) Name() string { return m.name }
func (m testMigration) CoreCompatibility() CoreCompatibility {
	return m.compatibility
}
func (m testMigration) Migrate(context.Context, *database.DB) error {
	if m.migrate != nil {
		m.migrate()
	}
	return nil
}

type fixedPolicy struct {
	allow       bool
	permissions []string
}

func (p fixedPolicy) Can(context.Context, int64, int64, string, string, int64, string) bool {
	return p.allow
}
func (p fixedPolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return append([]string(nil), p.permissions...), nil
}
