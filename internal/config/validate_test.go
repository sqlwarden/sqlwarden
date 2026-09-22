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

func TestLoadRejectsConnectorReplicasWithStaticSessionDirectory(t *testing.T) {
	_, err := Load([]string{"--connector-replicas", "2"})
	if err == nil {
		t.Fatal("expected connector.replicas>1 with the static session directory to fail startup")
	}
	for _, want := range []string{"connector.replicas=2", SessionDirectoryRedis, SessionDirectoryStatic} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestValidateSessionDirectory(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		replicas  int
		wantErr   bool
	}{
		{name: "static with one replica", directory: SessionDirectoryStatic, replicas: 1},
		{name: "static with two replicas", directory: SessionDirectoryStatic, replicas: 2, wantErr: true},
		{name: "zero replicas", directory: SessionDirectoryStatic, replicas: 0, wantErr: true},
		{name: "redis is unimplemented", directory: SessionDirectoryRedis, replicas: 2, wantErr: true},
		{name: "unknown directory", directory: "etcd", replicas: 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.SessionDirectory = test.directory
			cfg.Connector.Replicas = test.replicas
			if err := Validate(cfg); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidateConnectorTransport(t *testing.T) {
	cfg := Default()
	cfg.ProcessKinds = []string{ProcessKindAPI}
	cfg.Connector.Transport = ConnectorTransportTLS
	if err := Validate(cfg); err != nil {
		t.Fatalf("API-only TLS config should not require a server certificate: %v", err)
	}

	cfg.ProcessKinds = []string{ProcessKindConnector}
	if err := Validate(cfg); err == nil {
		t.Fatal("connector TLS listener must require its certificate and key")
	}

	cfg = Default()
	cfg.ProcessKinds = []string{ProcessKindAPI}
	cfg.Connector.GrantSigningKey = "too-short"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected a short connector grant signing key to fail")
	}

	cfg = Default()
	cfg.Connector.Address = ""
	cfg.Connector.ListenAddress = ""
	cfg.Connector.GrantSigningKey = ""
	if err := Validate(cfg); err != nil {
		t.Fatalf("all mode must not require connector RPC configuration: %v", err)
	}
}

func TestValidateConnectorAddressRequiresPlainHostPort(t *testing.T) {
	for _, address := range []string{"https://connector.internal:6021", "connector.internal", ":6021", "connector.internal:0", "connector.internal:70000"} {
		t.Run(address, func(t *testing.T) {
			cfg := Default()
			cfg.ProcessKinds = []string{ProcessKindAPI}
			cfg.Connector.Address = address
			if err := Validate(cfg); err == nil {
				t.Fatalf("Validate() accepted connector.address %q", address)
			}
		})
	}

	cfg := Default()
	cfg.ProcessKinds = []string{ProcessKindConnector}
	cfg.Connector.ListenAddress = ":0"
	if err := Validate(cfg); err == nil {
		t.Fatal("Validate() accepted connector.listen_address with port zero")
	}
}

func TestValidateProcessKinds(t *testing.T) {
	tests := []struct {
		name    string
		kinds   []string
		wantErr bool
	}{
		{name: "all", kinds: []string{ProcessKindAll}},
		{name: "split kinds", kinds: []string{ProcessKindAPI, ProcessKindJobs}},
		{name: "empty", kinds: nil, wantErr: true},
		{name: "unknown kind", kinds: []string{"worker"}, wantErr: true},
		{name: "duplicate kind", kinds: []string{ProcessKindAPI, ProcessKindAPI}, wantErr: true},
		{name: "all combined with another kind", kinds: []string{ProcessKindAll, ProcessKindAPI}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.ProcessKinds = test.kinds
			if err := Validate(cfg); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
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

func TestValidateEdition(t *testing.T) {
	tests := []struct {
		name        string
		edition     string
		licenseFile string
		wantErr     bool
	}{
		{name: "community", edition: EditionCommunity},
		{name: "community with license", edition: EditionCommunity, licenseFile: "/etc/sqlwarden/license.jwt", wantErr: true},
		{name: "enterprise with license", edition: EditionEnterprise, licenseFile: "/etc/sqlwarden/license.jwt"},
		{name: "enterprise without license", edition: EditionEnterprise, wantErr: true},
		{name: "unknown edition", edition: "pro", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.Edition.Name = test.edition
			cfg.Edition.LicenseFile = test.licenseFile
			if err := Validate(cfg); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
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
