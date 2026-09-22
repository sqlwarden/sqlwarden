package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticRedactsSensitiveValues(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "super-secret-jwt")
	t.Setenv("ENCRYPTION_PREVIOUS_KEYS", "retired-secret-key")

	loaded, err := Load([]string{"--db-dsn", "postgres://user:hunter2@db.example.com/sqlwarden", "--db-driver", "postgres"})
	if err != nil {
		t.Fatal(err)
	}

	secrets := []string{"super-secret-jwt", "retired-secret-key", "hunter2", defaultCookieSecretKey, defaultEncryptionKey}
	rendered := loaded.Diagnostic.String()
	encoded, err := json.Marshal(loaded.Diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if strings.Contains(rendered, secret) {
			t.Errorf("rendered diagnostic leaked %q", secret)
		}
		if strings.Contains(string(encoded), secret) {
			t.Errorf("encoded diagnostic leaked %q", secret)
		}
	}

	for _, entry := range loaded.Diagnostic.Entries {
		if entry.Sensitive && entry.Value != RedactedValue {
			t.Errorf("sensitive entry %q has value %q, want %q", entry.Key, entry.Value, RedactedValue)
		}
		if entry.Source == "" {
			t.Errorf("entry %q has no source", entry.Key)
		}
	}
}

func TestDiagnosticReportsEffectiveValuesAndSources(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")

	loaded, err := Load([]string{"--http-port", "9300"})
	if err != nil {
		t.Fatal(err)
	}

	values := map[string]DiagnosticEntry{}
	for _, entry := range loaded.Diagnostic.Entries {
		values[entry.Key] = entry
	}

	if entry := values["http_port"]; entry.Value != "9300" || entry.Source != SourceFlag {
		t.Errorf("http_port entry = %+v, want value 9300 from %q", entry, SourceFlag)
	}
	if entry := values["log.format"]; entry.Value != "text" || entry.Source != SourceEnv {
		t.Errorf("log.format entry = %+v, want value text from %q", entry, SourceEnv)
	}
	if entry := values["session_directory"]; entry.Value != SessionDirectoryStatic || entry.Source != SourceDefault {
		t.Errorf("session_directory entry = %+v, want value %q from %q", entry, SessionDirectoryStatic, SourceDefault)
	}
	if entry := values["process_kinds"]; entry.Value != ProcessKindAll {
		t.Errorf("process_kinds entry = %+v, want value %q", entry, ProcessKindAll)
	}
}

func TestDiagnosticCoversEveryLoadableKey(t *testing.T) {
	loaded, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Diagnostic.Entries) != len(options) {
		t.Fatalf("diagnostic has %d entries, want %d", len(loaded.Diagnostic.Entries), len(options))
	}
	for i := 1; i < len(loaded.Diagnostic.Entries); i++ {
		if loaded.Diagnostic.Entries[i-1].Key >= loaded.Diagnostic.Entries[i].Key {
			t.Fatalf("diagnostic entries are not sorted by key: %q then %q",
				loaded.Diagnostic.Entries[i-1].Key, loaded.Diagnostic.Entries[i].Key)
		}
	}

	for _, category := range []Category{CategoryBootstrap, CategorySecrets, CategoryEdition} {
		if len(loaded.Diagnostic.ByCategory(category)) == 0 {
			t.Errorf("no diagnostic entries in category %q", category)
		}
	}
	if got := loaded.Diagnostic.ByCategory(CategoryRuntime); len(got) != 0 {
		t.Errorf("runtime settings are database-owned, but the diagnostic reported %d entries", len(got))
	}

	if want := len(precedence); len(loaded.Diagnostic.Precedence) != want {
		t.Fatalf("diagnostic precedence has %d sources, want %d", len(loaded.Diagnostic.Precedence), want)
	}
	if loaded.Diagnostic.Precedence[0] != SourceFlag || loaded.Diagnostic.Precedence[len(precedence)-1] != SourceDefault {
		t.Fatalf("unexpected precedence order: %v", loaded.Diagnostic.Precedence)
	}
}
