// Package connectionspectest defines the shared engine.ConnectionSpec contract.
package connectionspectest

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
)

// OnlyRequested reports whether the test binary was invoked with exactly the
// ConnectionSpec filter. Engine TestMain functions use it to avoid starting
// integration containers for this connectionless contract suite.
func OnlyRequested() bool {
	for index, argument := range os.Args {
		switch {
		case argument == "-test.run" && index+1 < len(os.Args):
			return os.Args[index+1] == "ConnectionSpec"
		case strings.HasPrefix(argument, "-test.run="):
			return strings.TrimPrefix(argument, "-test.run=") == "ConnectionSpec"
		}
	}
	return false
}

// Case supplies one engine's representative connection values.
type Case struct {
	Driver          string
	Params          engine.Params
	Secrets         engine.Secrets
	LegacyTLSDSN    string
	ExpectedTLSMode string
	MissingPortDSN  string
	DefaultPort     string
}

// Run verifies the shared ConnectionSpec behavior for an engine registration.
func Run(t *testing.T, tc Case) {
	t.Helper()
	spec, ok := engine.ConnectionSpecFor(tc.Driver)
	if !ok {
		t.Fatalf("ConnectionSpecFor(%q) not found", tc.Driver)
	}

	t.Run("fields", func(t *testing.T) {
		secretFields := 0
		for _, field := range spec.Fields() {
			if !field.Secret {
				continue
			}
			secretFields++
			if field.Key != "password" {
				t.Errorf("secret field = %q, want password", field.Key)
			}
		}
		wantSecretFields := 0
		if _, hasPassword := tc.Secrets["password"]; hasPassword {
			wantSecretFields = 1
		}
		if secretFields != wantSecretFields {
			t.Errorf("secret field count = %d, want %d", secretFields, wantSecretFields)
		}
	})

	if _, hasHost := tc.Params["host"]; hasHost {
		t.Run("ipv6_round_trip", func(t *testing.T) {
			wantParams := cloneParams(tc.Params)
			wantParams["host"] = "2001:db8::1"
			dsn, err := spec.BuildDSN(wantParams, tc.Secrets)
			if err != nil {
				t.Fatalf("BuildDSN: %v", err)
			}
			params, secrets, err := spec.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("ParseDSN: %v", err)
			}
			if !reflect.DeepEqual(params, wantParams) {
				t.Errorf("params = %#v, want %#v", params, wantParams)
			}
			if !reflect.DeepEqual(secrets, tc.Secrets) {
				t.Errorf("secrets = %#v, want %#v", secrets, tc.Secrets)
			}
		})
	}

	if tc.MissingPortDSN != "" {
		t.Run("missing_port_uses_engine_default", func(t *testing.T) {
			wantParams := cloneParams(tc.Params)
			wantParams["port"] = tc.DefaultPort
			params, secrets, err := spec.ParseDSN(tc.MissingPortDSN)
			if err != nil {
				t.Fatalf("ParseDSN: %v", err)
			}
			if !reflect.DeepEqual(params, wantParams) {
				t.Errorf("params = %#v, want %#v", params, wantParams)
			}
			if !reflect.DeepEqual(secrets, tc.Secrets) {
				t.Errorf("secrets = %#v, want %#v", secrets, tc.Secrets)
			}
		})
	}

	t.Run("round_trip", func(t *testing.T) {
		dsn, err := spec.BuildDSN(tc.Params, tc.Secrets)
		if err != nil {
			t.Fatalf("BuildDSN: %v", err)
		}
		params, secrets, err := spec.ParseDSN(dsn)
		if err != nil {
			t.Fatalf("ParseDSN: %v", err)
		}
		if !reflect.DeepEqual(params, tc.Params) {
			t.Errorf("params = %#v, want %#v", params, tc.Params)
		}
		if !reflect.DeepEqual(secrets, tc.Secrets) {
			t.Errorf("secrets = %#v, want %#v", secrets, tc.Secrets)
		}
		if _, exists := params["password"]; exists {
			t.Error("password was returned in Params")
		}
	})

	if _, hasPassword := tc.Secrets["password"]; hasPassword {
		t.Run("round_trip_without_password", func(t *testing.T) {
			dsn, err := spec.BuildDSN(tc.Params, engine.Secrets{})
			if err != nil {
				t.Fatalf("BuildDSN: %v", err)
			}
			params, secrets, err := spec.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("ParseDSN: %v", err)
			}
			if !reflect.DeepEqual(params, tc.Params) {
				t.Errorf("params = %#v, want %#v", params, tc.Params)
			}
			if len(secrets) != 0 {
				t.Error("an absent password was returned as a secret")
			}
		})
	}

	t.Run("required_fields", func(t *testing.T) {
		for _, field := range spec.Fields() {
			if !field.Required || field.Secret {
				continue
			}
			t.Run(field.Key, func(t *testing.T) {
				params := cloneParams(tc.Params)
				delete(params, field.Key)
				_, err := spec.BuildDSN(params, tc.Secrets)
				if err == nil {
					t.Fatal("BuildDSN succeeded with a missing required field")
				}
				assertNoValues(t, err.Error(), tc.Params, tc.Secrets)
			})
		}
	})

	t.Run("tls_mode_not_built", func(t *testing.T) {
		params := cloneParams(tc.Params)
		params["tls_mode"] = "contract-tls-sentinel"
		dsn, err := spec.BuildDSN(params, tc.Secrets)
		if err != nil {
			t.Fatalf("BuildDSN: %v", err)
		}
		if strings.Contains(dsn, "contract-tls-sentinel") {
			t.Error("BuildDSN emitted tls_mode")
		}
	})

	if tc.LegacyTLSDSN != "" {
		t.Run("legacy_tls_mode_is_lifted", func(t *testing.T) {
			params, _, err := spec.ParseDSN(tc.LegacyTLSDSN)
			if err != nil {
				t.Fatalf("ParseDSN: %v", err)
			}
			if got := params["tls_mode"]; got != tc.ExpectedTLSMode {
				t.Errorf("tls_mode = %q, want %q", got, tc.ExpectedTLSMode)
			}
		})
	}

	t.Run("unsupported_parameter_is_rejected", func(t *testing.T) {
		const (
			key   = "contract_unknown_param"
			value = "contract-unknown-value"
		)
		dsn, err := spec.BuildDSN(tc.Params, tc.Secrets)
		if err != nil {
			t.Fatalf("BuildDSN: %v", err)
		}
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		_, _, err = spec.ParseDSN(dsn + separator + key + "=" + value)
		if err == nil {
			t.Fatal("ParseDSN accepted an unsupported parameter")
		}
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not name the parameter: %v", err)
		}
		if strings.Contains(err.Error(), value) {
			t.Error("error echoed the parameter value")
		}
	})

	t.Run("garbage_is_redacted", func(t *testing.T) {
		const garbage = "not-a-dsn-contract-secret"
		_, _, err := spec.ParseDSN(garbage)
		if err == nil {
			t.Fatal("ParseDSN accepted garbage")
		}
		if strings.Contains(err.Error(), garbage) {
			t.Error("ParseDSN error echoed the DSN")
		}
	})
}

func cloneParams(in engine.Params) engine.Params {
	out := make(engine.Params, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func assertNoValues(t *testing.T, message string, params engine.Params, secrets engine.Secrets) {
	t.Helper()
	for _, values := range []map[string]string{params, secrets} {
		for _, value := range values {
			if value != "" && strings.Contains(message, value) {
				t.Errorf("error contains a connection value")
			}
		}
	}
}
