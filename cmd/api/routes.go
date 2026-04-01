package main

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/assets"
)

func (app *application) routes() http.Handler {
	mux := chi.NewRouter()

	mux.NotFound(app.notFound)
	mux.MethodNotAllowed(app.methodNotAllowed)

	mux.Use(app.logAccess)
	mux.Use(app.recoverPanic)

	mux.Route("/api/v1", func(r chi.Router) {
		r.Use(app.authenticateV1)

		r.Post("/auth/register", app.registerAccount)
		r.Post("/auth/login", app.loginAccount)
		r.Post("/auth/refresh", app.refreshToken)
		r.Post("/auth/logout", app.logoutAccount)

		r.With(app.requireAccount).Post("/orgs", app.createOrg)

		r.Group(func(r chi.Router) {
			r.Use(app.requireAccount)
			r.Get("/user", app.getAccount)
			r.Get("/user/orgs", app.getAccountOrgs)
		})

		r.Route("/orgs/{org_slug}", func(r chi.Router) {
			r.Use(app.requireAccount, app.orgCtx)

			r.Get("/", app.getOrg)

			r.Route("/members", func(r chi.Router) {
				r.With(app.requirePermission("org:read")).Get("/", app.listOrgMembers)
				r.With(app.requirePermission("org:invite")).Post("/", app.addOrgMember)
				r.With(app.requireOrgRole("owner")).Patch("/{account_id}", app.updateOrgMemberRole)
				r.With(app.requirePermission("org:write")).Delete("/{account_id}", app.removeOrgMember)
			})

			r.Route("/teams", func(r chi.Router) {
				r.Get("/", app.listTeams)
				r.With(app.requirePermission("org:write")).Post("/", app.createTeam)
				r.Route("/{team_slug}", func(r chi.Router) {
					r.Get("/", app.getTeam)
					r.With(app.requirePermission("org:write")).Delete("/", app.deleteTeam)
					r.Get("/members", app.listTeamMembers)
					r.With(app.requirePermission("org:write")).Post("/members", app.addTeamMember)
					r.With(app.requirePermission("org:write")).Delete("/members/{account_id}", app.removeTeamMember)
				})
			})

			r.Route("/roles", func(r chi.Router) {
				r.With(app.requirePermission("policy:read")).Get("/", app.listRoles)
				r.With(app.requirePermission("policy:modify")).Post("/", app.createRole)
				r.With(app.requirePermission("policy:read")).Get("/{role_id}", app.getRole)
				r.With(app.requirePermission("policy:modify")).Delete("/{role_id}", app.deleteRole)
			})

			r.Get("/permissions", app.listPermissions)

			r.Route("/workspaces", func(r chi.Router) {
				r.Get("/", app.listWorkspaces)
				r.With(app.requireOrgRole("admin")).Post("/", app.createWorkspace)
				r.Route("/{ws_id}", func(r chi.Router) {
					r.Use(app.wsCtx)
					r.Get("/", app.getWorkspace)
					r.With(app.requirePermission("ws:write")).Patch("/", app.updateWorkspace)
					r.With(app.requireOrgRole("admin")).Delete("/", app.deleteWorkspace)

					r.Get("/access", app.listWorkspaceBindings)
					r.With(app.requirePermission("policy:modify")).Post("/access", app.grantWorkspaceRoleBinding)
					r.With(app.requirePermission("policy:modify")).Delete("/access/{binding_id}", app.revokeWorkspaceBinding)

					r.Route("/environments", func(r chi.Router) {
						r.Get("/", app.listEnvironments)
						r.With(app.requirePermission("env:create")).Post("/", app.createEnvironment)
						r.Route("/{env_id}", func(r chi.Router) {
							r.Use(app.envCtx)
							r.Get("/", app.getEnvironment)
							r.With(app.requirePermission("env:write")).Patch("/", app.updateEnvironment)
							r.With(app.requirePermission("env:delete")).Delete("/", app.deleteEnvironment)

							r.Get("/access", app.listEnvironmentBindings)
							r.With(app.requirePermission("policy:modify")).Post("/access", app.grantEnvironmentRoleBinding)
							r.With(app.requirePermission("policy:modify")).Delete("/access/{binding_id}", app.revokeEnvironmentBinding)
						})
					})

					r.Route("/connections", func(r chi.Router) {
						r.Post("/test", app.testConnection)
						r.Get("/", app.listConnections)
						r.With(app.requirePermission("conn:create")).Post("/", app.createConnection)
						r.Route("/{conn_id}", func(r chi.Router) {
							r.Use(app.connCtx)
							r.Get("/", app.getConnection)
							r.With(app.requirePermission("conn:delete")).Delete("/", app.deleteConnection)

							r.Get("/access", app.listConnectionBindings)
							r.With(app.requirePermission("policy:modify")).Post("/access", app.grantConnectionRoleBinding)
							r.With(app.requirePermission("policy:modify")).Delete("/access/{binding_id}", app.revokeConnectionBinding)

							r.Post("/connect", app.connectToDatabase)
							r.Post("/query", app.executeQuery)
						})
					})
				})
			})
		})
	})

	staticFS, err := fs.Sub(assets.EmbeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Get("/*", app.spaHandler(staticFS))

	return mux
}

func (app *application) spaHandler(staticFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(staticFS))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[1:]
		if path == "" {
			path = "index.html"
		}

		_, err := staticFS.Open(path)
		if err != nil {
			indexFile, err := staticFS.Open("index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			defer indexFile.Close()

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.Copy(w, indexFile)
			return
		}

		fileServer.ServeHTTP(w, r)
	}
}
