package web

import "net/http"

// ProductCapabilities is the frontend-facing product contract derived from
// the executable-owned runtime mode. These values describe feature
// availability; HTTP authorization remains authoritative.
type ProductCapabilities struct {
	Mode                   Mode `json:"mode"`
	NativeShell            bool `json:"native_shell"`
	MultiUser              bool `json:"multi_user"`
	OrganizationManagement bool `json:"organization_management"`
	UserManagement         bool `json:"user_management"`
	Invitations            bool `json:"invitations"`
	RBACAdministration     bool `json:"rbac_administration"`
	WorkspaceManagement    bool `json:"workspace_management"`
	NativeFileDialogs      bool `json:"native_file_dialogs"`
	LocalSQLiteFiles       bool `json:"local_sqlite_files"`
}

func (cfg Config) productCapabilities() ProductCapabilities {
	server := cfg.Mode == ModeServer
	desktop := cfg.Mode == ModeDesktop
	return ProductCapabilities{
		Mode:                   cfg.Mode,
		NativeShell:            desktop,
		MultiUser:              server,
		OrganizationManagement: server,
		UserManagement:         server,
		Invitations:            server,
		RBACAdministration:     server,
		WorkspaceManagement:    true,
		NativeFileDialogs:      desktop,
		LocalSQLiteFiles:       sqliteDriverSourceAllowed(cfg, SQLiteDriverSourceLocal),
	}
}

func (cfg Config) isServer() bool {
	return cfg.Mode == ModeServer
}

func (cfg Config) isDesktop() bool {
	return cfg.Mode == ModeDesktop
}

func (app *application) requireServerMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.config.isServer() {
			app.notFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
