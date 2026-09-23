package helm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/config"
	"gopkg.in/yaml.v3"
)

const chartDir = "sqlwarden"

func readChartFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(chartDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func chartValues(t *testing.T) map[string]any {
	t.Helper()
	var values map[string]any
	if err := yaml.Unmarshal(readChartFile(t, "values.yaml"), &values); err != nil {
		t.Fatal(err)
	}
	return values
}

func chartSchema(t *testing.T) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(readChartFile(t, "values.schema.json"), &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func lookup(t *testing.T, root map[string]any, path ...string) any {
	t.Helper()
	var current any = root
	for _, key := range path {
		node, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is not a mapping", strings.Join(path, "."))
		}
		current, ok = node[key]
		if !ok {
			t.Fatalf("%s is missing", strings.Join(path, "."))
		}
	}
	return current
}

func templateFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := filepath.Join(chartDir, "templates")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestConnectorIsPinnedToOneReplica(t *testing.T) {
	if replicas := lookup(t, chartValues(t), "connector", "replicas"); replicas != 1 {
		t.Fatalf("connector.replicas = %v, want 1", replicas)
	}

	schema := chartSchema(t)
	constraints := lookup(t, schema, "properties", "connector", "properties", "replicas")
	node, ok := constraints.(map[string]any)
	if !ok {
		t.Fatal("connector.replicas schema is not a mapping")
	}
	if node["minimum"] != float64(1) || node["maximum"] != float64(1) {
		t.Fatalf("connector.replicas schema = %v, want minimum and maximum of 1", node)
	}
}

// The application refuses to migrate from a serving replica, so a chart that
// enabled automigrate anywhere would render a Deployment that cannot start.
func TestServingDeploymentsDisableAutomigrate(t *testing.T) {
	for _, name := range []string{"deployment-api.yaml", "deployment-connector.yaml"} {
		body := string(readChartFile(t, filepath.Join("templates", name)))
		if !strings.Contains(body, "name: DB_AUTOMIGRATE\n              value: \"false\"") {
			t.Fatalf("%s does not pin DB_AUTOMIGRATE to false", name)
		}
	}
}

func TestMigrationJobRunsAsAPreUpgradeHook(t *testing.T) {
	body := string(readChartFile(t, filepath.Join("templates", "job-migrate.yaml")))
	for _, want := range []string{
		"helm.sh/hook: pre-install,pre-upgrade",
		"- migrate",
		"name: DB_MIGRATION_TIMEOUT",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("job-migrate.yaml is missing %q", want)
		}
	}

	values := chartValues(t)
	if _, ok := lookup(t, values, "database", "migrationTimeout").(string); !ok {
		t.Fatal("database.migrationTimeout must be a duration string")
	}
	if deadline, ok := lookup(t, values, "migration", "activeDeadlineSeconds").(int); !ok || deadline <= 0 {
		t.Fatalf("migration.activeDeadlineSeconds = %v, want a positive integer", deadline)
	}
}

// Every environment variable the chart sets must name a loadable configuration
// key; a typo here is silently ignored at runtime.
func TestChartEnvironmentVariablesAreLoadableKeys(t *testing.T) {
	known := config.EnvVars()
	pattern := regexp.MustCompile(`(?m)^\s*-?\s*name: ([A-Z][A-Z0-9_]*)$`)

	for name, body := range templateFiles(t) {
		for _, match := range pattern.FindAllStringSubmatch(body, -1) {
			envVar := match[1]
			if !slices.Contains(known, envVar) {
				t.Errorf("%s sets %s, which is not a loadable configuration key", name, envVar)
			}
		}
	}
}

func TestEachProcessKindHasItsOwnServiceAccount(t *testing.T) {
	body := string(readChartFile(t, filepath.Join("templates", "serviceaccount.yaml")))
	for _, want := range []string{
		`include "sqlwarden.api.serviceAccountName"`,
		`include "sqlwarden.connector.serviceAccountName"`,
		`include "sqlwarden.migration.serviceAccountName"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("serviceaccount.yaml is missing %s", want)
		}
	}
}

func TestRBACGrantsNoKubernetesPermissions(t *testing.T) {
	body := string(readChartFile(t, filepath.Join("templates", "rbac.yaml")))
	if got := strings.Count(body, "rules: []"); got != 3 {
		t.Fatalf("rbac.yaml has %d empty role rule sets, want one for each of api, connector, and migrate", got)
	}
}

func TestSplitChartRequiresPostgres(t *testing.T) {
	values := chartValues(t)
	if driver := lookup(t, values, "database", "driver"); driver != "postgres" {
		t.Fatalf("database.driver = %v, want postgres", driver)
	}
	schema := chartSchema(t)
	driver := lookup(t, schema, "properties", "database", "properties", "driver").(map[string]any)
	allowed, ok := driver["enum"].([]any)
	if !ok || len(allowed) != 1 || allowed[0] != "postgres" {
		t.Fatalf("database.driver schema enum = %v, want [postgres]", driver["enum"])
	}
}

func TestProcessKindsHaveAllThreeProbes(t *testing.T) {
	values := chartValues(t)
	for _, kind := range []string{"api", "connector"} {
		for _, probe := range []string{"startup", "liveness", "readiness"} {
			lookup(t, values, kind, "probes", probe)
		}
		body := string(readChartFile(t, filepath.Join("templates", "deployment-"+kind+".yaml")))
		for _, want := range []string{"startupProbe:", "livenessProbe:", "readinessProbe:", "/healthz", "/readyz"} {
			if !strings.Contains(body, want) {
				t.Errorf("deployment-%s.yaml is missing %s", kind, want)
			}
		}
	}
}

// A draining replica must finish inside the pod's grace period, otherwise the
// kubelet sends SIGKILL part-way through shutdown.
func TestTerminationGracePeriodExceedsShutdownTimeout(t *testing.T) {
	values := chartValues(t)
	timeout, ok := lookup(t, values, "config", "shutdownTimeout").(string)
	if !ok {
		t.Fatal("config.shutdownTimeout must be a duration string")
	}
	grace, ok := lookup(t, values, "config", "terminationGracePeriodSeconds").(int)
	if !ok {
		t.Fatal("config.terminationGracePeriodSeconds must be an integer")
	}

	parsed := parseSeconds(t, timeout)
	if grace <= parsed {
		t.Fatalf("config.terminationGracePeriodSeconds = %d, want more than shutdownTimeout of %d seconds", grace, parsed)
	}
}

func TestResourcesAreDeclaredForEveryWorkload(t *testing.T) {
	values := chartValues(t)
	for _, kind := range []string{"api", "connector", "migration"} {
		for _, side := range []string{"requests", "limits"} {
			lookup(t, values, kind, "resources", side, "cpu")
			lookup(t, values, kind, "resources", side, "memory")
		}
	}
}

func parseSeconds(t *testing.T, value string) int {
	t.Helper()
	duration, err := time.ParseDuration(value)
	if err != nil {
		t.Fatal(err)
	}
	return int(duration.Seconds())
}
