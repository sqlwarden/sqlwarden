package main

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/assets"
	"github.com/sqlwarden/internal/access"
)

func (app *application) routes() http.Handler {
	mux := chi.NewRouter()

	mux.NotFound(app.notFound)
	mux.MethodNotAllowed(app.methodNotAllowed)

	mux.Use(app.logAccess)
	mux.Use(app.recoverPanic)

	mux.Post("/api/setup", app.setup)
	mux.Get("/api/setup/status", app.setupStatus)

	mux.Route("/api/v1", func(r chi.Router) {
		r.Use(app.authenticateV1)

		r.Post("/auth/register", app.registerAccount)
		r.Post("/auth/login", app.loginAccount)
		r.Post("/auth/refresh", app.refreshToken)
		r.Post("/auth/logout", app.logoutAccount)

		r.With(app.requireAccount, app.requireInstanceAdmin).Post("/orgs", app.createOrg)

		r.Route("/instance", func(r chi.Router) {
			r.Use(app.requireAccount, app.requireInstanceAdmin)
			r.Get("/admins", app.listInstanceAdmins)
			r.Get("/accounts", app.listInstanceAccounts)
			r.Get("/orgs", app.listOrganizations)
			r.Get("/settings", app.getInstanceSettings)
			r.Patch("/settings", app.updateInstanceSettings)
			r.Post("/admins", app.addInstanceAdmin)
			r.Post("/accounts", app.createInstanceAccount)
			r.Delete("/admins/{account_id}", app.removeInstanceAdmin)
		})

		r.Group(func(r chi.Router) {
			r.Use(app.requireAccount)
			r.Get("/account", app.getAccount)
			r.Get("/account/orgs", app.getAccountOrgs)
			r.Get("/session", app.getSession)
		})

		r.Route("/me", func(r chi.Router) {
			r.Use(app.requireAccount)

			r.Get("/", app.getAccount)

			r.Route("/workspaces", func(r chi.Router) {
				r.Use(app.requirePersonalSpacesEnabled)
				r.Get("/", app.listMyWorkspaces)
				r.Post("/", app.createMyWorkspace)
				r.Route("/{ws_id}", func(r chi.Router) {
					r.Use(app.spaceWsCtx)
					r.Get("/", app.getWorkspace)
					r.Patch("/", app.updateWorkspace)
					r.Delete("/", app.deleteWorkspace)

					r.Route("/environments", func(r chi.Router) {
						r.Get("/", app.listMyEnvironments)
						r.Post("/", app.createMyEnvironment)
						r.Route("/{env_id}", func(r chi.Router) {
							r.Use(app.spaceEnvCtx)
							r.Get("/", app.getEnvironment)
							r.Patch("/", app.updateEnvironment)
							r.Delete("/", app.deleteEnvironment)

							r.Route("/connections", func(r chi.Router) {
								r.Post("/test", app.testConnection)
								r.Get("/", app.listMyConnections)
								r.Post("/", app.createMyConnection)
								r.Route("/{conn_id}", func(r chi.Router) {
									r.Use(app.spaceConnCtx)
									r.Get("/", app.getConnection)
									r.Patch("/", app.updateConnection)
									r.Delete("/", app.deleteConnection)
									r.Post("/connect", app.connectToDatabase)
									r.Post("/query", app.executeQuery)
								})
							})
						})
					})

					r.Route("/connections", func(r chi.Router) {
						r.Post("/test", app.testConnection)
						r.Get("/", app.listMyConnections)
						r.Post("/", app.createMyConnection)
						r.Route("/{conn_id}", func(r chi.Router) {
							r.Use(app.spaceConnCtx)
							r.Get("/", app.getConnection)
							r.Patch("/", app.updateConnection)
							r.Delete("/", app.deleteConnection)
							r.Post("/connect", app.connectToDatabase)
							r.Post("/query", app.executeQuery)
						})
					})
				})
			})
		})

		r.Route("/orgs/{org_slug}", func(r chi.Router) {
			r.Use(app.requireAccount, app.orgCtx)

			r.Get("/", app.getOrg)
			r.Patch("/", app.updateOrg)
			r.Delete("/", app.deleteOrg)

			r.Route("/members", func(r chi.Router) {
				r.With(app.requirePermission("org:read")).Get("/", app.listOrgMembers)
				r.With(app.requirePermission("org:invite")).Post("/", app.addOrgMember)
				r.With(app.requirePermission("org:read")).Get("/{account_id}", app.getOrgMember)
				r.With(app.requirePermission("org:read")).Get("/{account_id}/teams", app.listOrgMemberTeams)
				r.With(app.requireOrgRole(access.BuiltinOrgOwnerRole)).Patch("/{account_id}", app.updateOrgMemberRole)
				r.With(app.requirePermission("org:write")).Delete("/{account_id}", app.removeOrgMember)
			})

			r.Route("/teams", func(r chi.Router) {
				r.With(app.requirePermission("org:read")).Get("/", app.listTeams)
				r.With(app.requirePermission("org:write")).Post("/", app.createTeam)
				r.Route("/{team_slug}", func(r chi.Router) {
					r.With(app.requirePermission("org:read")).Get("/", app.getTeam)
					r.With(app.requirePermission("org:write")).Patch("/", app.updateTeam)
					r.With(app.requirePermission("org:write")).Delete("/", app.deleteTeam)
					r.With(app.requirePermission("org:read")).Get("/members", app.listTeamMembers)
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

			r.Route("/policies", func(r chi.Router) {
				r.With(app.requirePermission("policy:read")).Get("/", app.listOrgPolicies)
				r.With(app.requirePermission("policy:modify")).Post("/", app.grantOrgPolicy)
				r.With(app.requirePermission("policy:modify")).Delete("/{binding_id}", app.revokeOrgPolicy)
			})

			r.Get("/permissions", app.listPermissions)
			r.Get("/permissions/effective", app.getEffectivePermissions)

			r.Route("/workspaces", func(r chi.Router) {
				r.Get("/", app.listWorkspaces)
				r.With(app.requirePermission("ws:create")).Post("/", app.createWorkspace)
				r.Route("/{ws_id}", func(r chi.Router) {
					r.Use(app.wsCtx)
					r.Get("/", app.getWorkspace)
					r.With(app.requirePermission("ws:write")).Patch("/", app.updateWorkspace)
					r.With(app.requirePermission("ws:delete")).Delete("/", app.deleteWorkspace)

					r.Get("/permissions", app.listWorkspacePermissions)

					r.Route("/users", func(r chi.Router) {
						r.With(app.requirePermission("policy:read")).Get("/", app.listWorkspaceMembers)
						r.With(app.requirePermission("policy:read")).Get("/effective", app.listWorkspaceEffectiveMembers)
						r.With(app.requirePermission("policy:modify")).Post("/", app.addWorkspaceMember)
						r.With(app.requirePermission("policy:modify")).Delete("/{account_id}", app.removeWorkspaceMember)
					})

					r.Route("/teams", func(r chi.Router) {
						r.With(app.requirePermission("policy:read")).Get("/", app.listWorkspaceTeams)
						r.With(app.requirePermission("policy:modify")).Post("/", app.addWorkspaceTeam)
						r.With(app.requirePermission("policy:modify")).Delete("/{team_id}", app.removeWorkspaceTeam)
					})

					r.Route("/roles", func(r chi.Router) {
						r.With(app.requirePermission("policy:read")).Get("/", app.listWorkspaceRoles)
						r.With(app.requirePermission("policy:modify")).Post("/", app.createWorkspaceRole)
						r.With(app.requirePermission("policy:read")).Get("/{role_id}", app.getWorkspaceRole)
						r.With(app.requirePermission("policy:modify")).Delete("/{role_id}", app.deleteWorkspaceRole)
					})

					r.Route("/policies", func(r chi.Router) {
						r.With(app.requirePermission("policy:read")).Get("/", app.listWorkspacePolicies)
						r.With(app.requirePermission("policy:modify")).Post("/", app.grantWorkspacePolicy)
						r.With(app.requirePermission("policy:modify")).Delete("/{binding_id}", app.revokeWorkspacePolicy)
					})

					r.Route("/environments", func(r chi.Router) {
						r.Get("/", app.listEnvironments)
						r.With(app.requirePermission("env:create")).Post("/", app.createEnvironment)
						r.Route("/{env_id}", func(r chi.Router) {
							r.Use(app.envCtx)
							r.Get("/", app.getEnvironment)
							r.With(app.requirePermission("env:write")).Patch("/", app.updateEnvironment)
							r.With(app.requirePermission("env:delete")).Delete("/", app.deleteEnvironment)

							r.Route("/connections", func(r chi.Router) {
								r.With(app.requirePermission("conn:create")).Post("/test", app.testConnection)
								r.Get("/", app.listConnections)
								r.With(app.requirePermission("conn:create")).Post("/", app.createConnection)
								r.Route("/{conn_id}", func(r chi.Router) {
									r.Use(app.connCtx)
									r.Get("/", app.getConnection)
									r.With(app.requirePermission("conn:write")).Patch("/", app.updateConnection)
									r.With(app.requirePermission("conn:delete")).Delete("/", app.deleteConnection)
									r.Post("/connect", app.connectToDatabase)
									r.Post("/query", app.executeQuery)
								})
							})
						})
					})

					r.Route("/connections", func(r chi.Router) {
						r.With(app.requirePermission("conn:create")).Post("/test", app.testConnection)
						r.Get("/", app.listConnections)
						r.With(app.requirePermission("conn:create")).Post("/", app.createConnection)
						r.Route("/{conn_id}", func(r chi.Router) {
							r.Use(app.connCtx)
							r.Get("/", app.getConnection)
							r.With(app.requirePermission("conn:write")).Patch("/", app.updateConnection)
							r.With(app.requirePermission("conn:delete")).Delete("/", app.deleteConnection)
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
