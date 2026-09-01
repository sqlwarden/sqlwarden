package web

import "testing"

func TestProductCapabilitiesAreDerivedFromMode(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want ProductCapabilities
	}{
		{
			name: "server",
			mode: ModeServer,
			want: ProductCapabilities{
				Mode:                   ModeServer,
				MultiUser:              true,
				OrganizationManagement: true,
				UserManagement:         true,
				Invitations:            true,
				RBACAdministration:     true,
				WorkspaceManagement:    true,
			},
		},
		{
			name: "desktop",
			mode: ModeDesktop,
			want: ProductCapabilities{
				Mode:                ModeDesktop,
				NativeShell:         true,
				WorkspaceManagement: true,
				NativeFileDialogs:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Mode = tt.mode
			got := cfg.productCapabilities()
			// Local SQLite access follows the existing driver-source policy; it is
			// not an independent product-mode axis.
			tt.want.LocalSQLiteFiles = sqliteDriverSourceAllowed(cfg, SQLiteDriverSourceLocal)
			if got != tt.want {
				t.Fatalf("capabilities = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestProductCapabilitiesReportConfiguredSQLitePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeDesktop
	cfg.Drivers.SQLite.AllowedSources = []string{"remote"}

	if cfg.productCapabilities().LocalSQLiteFiles {
		t.Fatal("local SQLite capability should reflect the configured driver-source policy")
	}
}
