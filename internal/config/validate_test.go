package config

import (
	"strings"
	"testing"
)

func TestValidateDefaultConfig(t *testing.T) {
	if err := Validate(Default()); err != nil {
		t.Fatalf("default config must validate: %v", err)
	}
}

func TestValidateProcessKinds(t *testing.T) {
	tests := []struct {
		name    string
		kinds   []string
		driver  string
		migrate bool
		wantErr string
	}{
		{name: "all on sqlite", kinds: []string{ProcessKindAll}, driver: "sqlite", migrate: true},
		{name: "api on postgres", kinds: []string{ProcessKindAPI}, driver: "postgres"},
		{name: "jobs on postgres", kinds: []string{ProcessKindJobs}, driver: "postgres"},
		{name: "api and jobs on postgres", kinds: []string{ProcessKindAPI, ProcessKindJobs}, driver: "postgres"},
		{name: "empty", kinds: nil, driver: "sqlite", wantErr: "process_kinds must not be empty"},
		{name: "unknown", kinds: []string{"worker"}, driver: "postgres", wantErr: `process_kinds contains unknown kind "worker"`},
		{name: "connector", kinds: []string{ProcessKindConnector}, driver: "postgres", wantErr: `process kind "connector" is not implemented`},
		{name: "edge gateway", kinds: []string{ProcessKindEdgeGateway}, driver: "postgres", wantErr: `process kind "edge-gateway" is not implemented`},
		{name: "realtime", kinds: []string{ProcessKindRealtime}, driver: "postgres", wantErr: `process kind "realtime" is not implemented`},
		{name: "duplicate", kinds: []string{ProcessKindAPI, ProcessKindAPI}, driver: "postgres", wantErr: `process_kinds contains "api" more than once`},
		{name: "all combined", kinds: []string{ProcessKindAll, ProcessKindJobs}, driver: "postgres", wantErr: `process_kinds "all" cannot be combined`},
		{name: "sqlite split", kinds: []string{ProcessKindAPI}, driver: "sqlite", wantErr: `db.driver "sqlite" requires process_kinds "all"`},
		{name: "automigrate on api", kinds: []string{ProcessKindAPI}, driver: "postgres", migrate: true, wantErr: `db.automigrate must be false when process_kinds selects "api"`},
		{name: "automigrate on jobs", kinds: []string{ProcessKindJobs}, driver: "postgres", migrate: true, wantErr: `db.automigrate must be false when process_kinds selects "jobs"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.ProcessKinds = tt.kinds
			cfg.DB.Driver = tt.driver
			cfg.DB.Automigrate = tt.migrate
			err := Validate(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSelectsOnly(t *testing.T) {
	cfg := Default()
	cfg.ProcessKinds = []string{ProcessKindJobs}
	if !cfg.SelectsOnly(ProcessKindJobs) {
		t.Fatal("SelectsOnly(jobs) = false for [jobs]")
	}
	cfg.ProcessKinds = []string{ProcessKindAll}
	if cfg.SelectsOnly(ProcessKindJobs) {
		t.Fatal("SelectsOnly(jobs) = true for [all]")
	}
}

func TestHasProcessKind(t *testing.T) {
	all := Default()
	if !all.HasProcessKind(ProcessKindConnector) {
		t.Errorf("%q must cover every process kind", ProcessKindAll)
	}

	split := Default()
	split.ProcessKinds = []string{ProcessKindAPI, ProcessKindJobs}
	if !split.HasProcessKind(ProcessKindJobs) {
		t.Errorf("expected %q to be served", ProcessKindJobs)
	}
	if split.HasProcessKind(ProcessKindConnector) {
		t.Errorf("did not expect %q to be served", ProcessKindConnector)
	}
}

func TestValidateFileStorageBackends(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr bool
	}{
		{name: "default backends", mutate: func(*Config) {}},
		{
			name:    "unknown active backend",
			mutate:  func(cfg *Config) { cfg.Files.ActiveStorageBackend = "elsewhere" },
			wantErr: true,
		},
		{
			name:    "no backends",
			mutate:  func(cfg *Config) { cfg.Files.StorageBackends = nil },
			wantErr: true,
		},
		{
			name: "unimplemented backend type",
			mutate: func(cfg *Config) {
				cfg.Files.StorageBackends["local"] = FileStorageBackend{Type: FilesStorageBackendS3, RootDir: "/tmp"}
			},
			wantErr: true,
		},
		{
			name: "missing root dir",
			mutate: func(cfg *Config) {
				cfg.Files.StorageBackends["local"] = FileStorageBackend{Type: FilesStorageBackendFilesystem}
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			test.mutate(&cfg)
			if err := Validate(cfg); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestNormalizeExpandsHomePaths(t *testing.T) {
	cfg := Default()
	if err := Normalize(&cfg); err != nil {
		t.Fatal(err)
	}

	if strings.HasPrefix(cfg.DB.DSN, "~") {
		t.Errorf("db.dsn = %q, want an expanded path", cfg.DB.DSN)
	}
	if root := cfg.Files.StorageBackends["local"].RootDir; strings.HasPrefix(root, "~") {
		t.Errorf("files root dir = %q, want an expanded path", root)
	}
}

func TestValidateRejectsNonPositiveTimeouts(t *testing.T) {
	cfg := Default()
	cfg.ShutdownTimeout = 0
	if err := Validate(cfg); err == nil {
		t.Fatal("Validate() accepted a zero shutdown timeout")
	}

	cfg = Default()
	cfg.DB.MigrationTimeout = 0
	if err := Validate(cfg); err == nil {
		t.Fatal("Validate() accepted a zero migration timeout")
	}
}

func TestValidateProfile(t *testing.T) {
	cfg := Default()
	if cfg.Profile != ProfileServer {
		t.Fatalf("default profile = %q", cfg.Profile)
	}
	cfg.Profile = ProfileDesktop
	if err := Validate(cfg); err != nil {
		t.Fatalf("desktop: %v", err)
	}
	cfg.Profile = "single_user"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestValidateTrustedProxies(t *testing.T) {
	cfg := Default()
	cfg.Server.TrustedProxies = []string{"10.0.0.0/8", "192.168.1.5", "::1"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("valid proxies: %v", err)
	}
	cfg.Server.TrustedProxies = []string{"not-an-ip"}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected error for invalid proxy")
	}
}

func TestParseTrustedProxiesBareIPBecomesHostPrefix(t *testing.T) {
	got, err := ParseTrustedProxies([]string{"192.168.1.5", "fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].String() != "192.168.1.5/32" || got[1].String() != "fd00::/8" {
		t.Fatalf("got %v", got)
	}
}
