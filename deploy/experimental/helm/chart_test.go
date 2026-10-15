package helm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/config"
)

const chartDir = "sqlwarden"

func readChartFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(chartDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// chartFiles returns every file under the chart except the README, keyed by
// its path relative to the chart.
func chartFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(chartDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "README.md" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(chartDir, path)
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

func chartSchema(t *testing.T) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal([]byte(readChartFile(t, "values.schema.json")), &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestChartEnvironmentVariablesAreLoadableKeys(t *testing.T) {
	known := config.EnvVars()
	envName := regexp.MustCompile(`(?m)^\s*-?\s*name: ([A-Z][A-Z0-9_]*)$`)
	rangedKeys := regexp.MustCompile(`range \$key := list ((?:"[A-Z0-9_]+" ?)+)`)
	configMapKey := regexp.MustCompile(`(?m)^  ([A-Z][A-Z0-9_]*):`)

	for name, body := range chartFiles(t) {
		var used []string
		for _, match := range envName.FindAllStringSubmatch(body, -1) {
			used = append(used, match[1])
		}
		for _, match := range rangedKeys.FindAllStringSubmatch(body, -1) {
			for _, key := range strings.Fields(match[1]) {
				used = append(used, strings.Trim(key, `"`))
			}
		}
		if filepath.Base(name) == "configmap.yaml" {
			for _, match := range configMapKey.FindAllStringSubmatch(body, -1) {
				used = append(used, match[1])
			}
		}
		for _, envVar := range used {
			if !slices.Contains(known, envVar) {
				t.Errorf("%s sets %s, which is not a loadable configuration key", name, envVar)
			}
		}
	}
}

func TestChartHasNoConnectorWorkload(t *testing.T) {
	for name, body := range chartFiles(t) {
		if strings.Contains(strings.ToLower(body), "connector") {
			t.Errorf("%s mentions the connector process kind, which is not implemented", name)
		}
	}
	for _, kind := range []string{config.ProcessKindAll, config.ProcessKindAPI, config.ProcessKindJobs} {
		if !strings.Contains(readChartFile(t, filepath.Join("templates", "deployment-"+kind+".yaml")), `"kind" "`+kind+`"`) {
			t.Errorf("deployment-%s.yaml does not run process kind %q", kind, kind)
		}
	}
}

func TestServingDeploymentsPinAutomigrate(t *testing.T) {
	body := readChartFile(t, filepath.Join("templates", "_deployment.tpl"))
	if !strings.Contains(body, "name: DB_AUTOMIGRATE\n              value: \"false\"") {
		t.Fatal("_deployment.tpl does not pin DB_AUTOMIGRATE to false for split process kinds")
	}
	if !strings.Contains(body, `(eq $kind "all")`) {
		t.Fatal("_deployment.tpl allows automigrate outside the all process kind")
	}
}

func TestMigrationJobRunsAsAPreUpgradeHook(t *testing.T) {
	body := readChartFile(t, filepath.Join("templates", "job-migrate.yaml"))
	for _, want := range []string{
		"helm.sh/hook: pre-install,pre-upgrade",
		"- migrate",
		"name: DB_MIGRATION_TIMEOUT",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("job-migrate.yaml is missing %q", want)
		}
	}
}

func TestWorkloadsProbeLivezAndReadyz(t *testing.T) {
	body := readChartFile(t, filepath.Join("templates", "_deployment.tpl"))
	for _, want := range []string{"startupProbe:", "livenessProbe:", "readinessProbe:", "path: /livez", "path: /readyz"} {
		if !strings.Contains(body, want) {
			t.Errorf("_deployment.tpl is missing %s", want)
		}
	}
	if strings.Contains(body, "/healthz") {
		t.Error("_deployment.tpl probes /healthz, which the application does not serve")
	}
}

func TestSecretsStayOutOfTheConfigMap(t *testing.T) {
	body := readChartFile(t, filepath.Join("templates", "configmap.yaml"))
	for _, forbidden := range []string{"DB_DSN", "SECRET", "ENCRYPTION", "LICENSE", ".Values.secrets", ".Values.database.dsn"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("configmap.yaml references %s", forbidden)
		}
	}
}

func TestSchemaRejectsSQLiteOutsideSingleReplicaAllMode(t *testing.T) {
	schema := chartSchema(t)
	allOf, ok := schema["allOf"].([]any)
	if !ok {
		t.Fatal("values.schema.json has no allOf conditions")
	}
	encoded, err := json.Marshal(allOf)
	if err != nil {
		t.Fatal(err)
	}
	conditions := string(encoded)
	for _, want := range []string{
		`"mode":{"const":"split"}`,
		`"driver":{"const":"postgres"}`,
		`"driver":{"const":"sqlite"}`,
		`"mode":{"const":"all"}`,
		`"replicaCount":{"const":1}`,
	} {
		if !strings.Contains(conditions, want) {
			t.Errorf("values.schema.json conditions are missing %s", want)
		}
	}

	props, _ := schema["properties"].(map[string]any)
	mode, _ := props["mode"].(map[string]any)
	if enum, _ := mode["enum"].([]any); len(enum) != 2 {
		t.Fatalf("mode enum = %v, want [all split]", mode["enum"])
	}
}
