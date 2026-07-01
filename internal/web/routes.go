package web

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sqlwarden/assets"
	"github.com/sqlwarden/internal/access"
)

func (app *application) routes() http.Handler {
	mux := chi.NewRouter()

	mux.NotFound(app.notFound)
	mux.MethodNotAllowed(app.methodNotAllowed)

	mux.Use(app.requestLoggingContext)
	mux.Use(app.logAccess)
	mux.Use(app.recoverPanic)
	mux.Use(middleware.Compress(5))

	mux.With(app.noStoreCache).Post("/api/setup", app.setup)
	mux.With(app.noStoreCache).Get("/api/setup/status", app.setupStatus)

	mux.Route("/api/v1", func(r chi.Router) {
		// API responses must never be HTTP-cached by the browser; see noStoreCache.
		r.Use(app.noStoreCache)
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
			r.Get("/accounts/{account_id}/sessions", app.listInstanceAccountSessions)
			r.Delete("/accounts/{account_id}/sessions", app.revokeInstanceAccountSessions)
			r.Delete("/accounts/{account_id}/sessions/{session_id}", app.revokeInstanceAccountSession)
			r.Delete("/admins/{account_id}", app.removeInstanceAdmin)
			r.Post("/encryption/rotate", app.rotateEncryptionKeysHandler)
		})

		r.Group(func(r chi.Router) {
			r.Use(app.requireAccount)
			r.Get("/account", app.getAccount)
			r.Patch("/account", app.updateAccount)
			r.Patch("/account/password", app.updateAccountPassword)
			r.Get("/account/orgs", app.getAccountOrgs)
			r.Get("/account/sessions", app.listAccountSessions)
			r.Delete("/account/sessions", app.revokeAccountSessions)
			r.Delete("/account/sessions/{session_id}", app.revokeAccountSession)
			r.Get("/session", app.getSession)

			r.Get("/engines", app.listEngines)
			r.Get("/engines/{engine_id}", app.getEngine)
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
					r.Get("/sessions", app.listActiveSessions)
					r.Delete("/sessions/{session_id}", app.revokeWorkspaceDatabaseSession)

					r.Route("/files/private", func(r chi.Router) {
						r.Get("/", app.listPrivateWorkspaceFiles)
						r.Post("/", app.createPrivateWorkspaceFile)
						r.Get("/browser", app.browsePrivateWorkspaceFiles)
						r.Get("/recent", app.listRecentPrivateWorkspaceFiles)
						r.Route("/{file_id}", func(r chi.Router) {
							r.Get("/", app.getPrivateWorkspaceFile)
							r.Patch("/", app.updatePrivateWorkspaceFile)
							r.Delete("/", app.deletePrivateWorkspaceFile)
							r.Get("/content", app.getPrivateWorkspaceFileContent)
							r.Put("/content", app.updatePrivateWorkspaceFileContent)
						})
					})

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
									r.Delete("/session", app.disconnectFromDatabase)
									r.Post("/query-cursors", app.startQueryCursor)
									r.Post("/query-cursors/{query_cursor_id}/fetch", app.fetchQueryCursor)
									r.Delete("/query-cursors/{query_cursor_id}", app.closeQueryCursor)
									r.Post("/query", app.executeQuery)
									r.Get("/schema/spec", app.getConnectionSchemaSpec)
									r.Get("/schema/catalog", app.getConnectionSchemaCatalog)
									r.Post("/schema/objects", app.getConnectionSchemaObjects)
									r.Post("/schema/refresh", app.refreshConnectionSchema)
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
							r.Delete("/session", app.disconnectFromDatabase)
							r.Post("/query-cursors", app.startQueryCursor)
							r.Post("/query-cursors/{query_cursor_id}/fetch", app.fetchQueryCursor)
							r.Delete("/query-cursors/{query_cursor_id}", app.closeQueryCursor)
							r.Post("/query", app.executeQuery)
							r.Get("/schema/spec", app.getConnectionSchemaSpec)
							r.Get("/schema/catalog", app.getConnectionSchemaCatalog)
							r.Post("/schema/objects", app.getConnectionSchemaObjects)
							r.Post("/schema/refresh", app.refreshConnectionSchema)
						})
					})
				})
			})
		})

		r.Route("/orgs/{org_slug}", func(r chi.Router) {
			r.Use(app.requireAccount, app.orgCtx)

			r.Get("/", app.getOrg)
			r.With(app.requireOrgPermission("org:write")).Patch("/", app.updateOrg)
			r.With(app.requireOrgPermission("org:delete")).Delete("/", app.deleteOrg)

			r.Route("/members", func(r chi.Router) {
				r.With(app.requireOrgPermission("org:read")).Get("/", app.listOrgMembers)
				r.With(app.requireOrgPermission("org:invite")).Get("/candidates", app.listOrgMemberCandidates)
				r.With(app.requireOrgPermission("org:invite")).Post("/", app.addOrgMember)
				r.With(app.requireOrgPermission("org:read")).Get("/{account_id}", app.getOrgMember)
				r.With(app.requireOrgPermission("org:read")).Get("/{account_id}/teams", app.listOrgMemberTeams)
				r.With(app.requireOrgPermission("org:write")).Get("/{account_id}/sessions", app.listOrgMemberAccessSessions)
				r.With(app.requireOrgPermission("org:write")).Delete("/{account_id}/sessions/{session_id}", app.revokeOrgMemberAccessSession)
				r.With(app.requireOrgRole(access.BuiltinOrgOwnerRole)).Patch("/{account_id}", app.updateOrgMemberRole)
				r.With(app.requireOrgPermission("org:write")).Delete("/{account_id}", app.removeOrgMember)
			})

			r.Route("/teams", func(r chi.Router) {
				r.With(app.requireOrgPermission("org:read")).Get("/", app.listTeams)
				r.With(app.requireOrgPermission("org:write")).Post("/", app.createTeam)
				r.Route("/{team_slug}", func(r chi.Router) {
					r.With(app.requireOrgPermission("org:read")).Get("/", app.getTeam)
					r.With(app.requireOrgPermission("org:write")).Patch("/", app.updateTeam)
					r.With(app.requireOrgPermission("org:write")).Delete("/", app.deleteTeam)
					r.With(app.requireOrgPermission("org:read")).Get("/members", app.listTeamMembers)
					r.With(app.requireOrgPermission("org:write")).Post("/members", app.addTeamMember)
					r.With(app.requireOrgPermission("org:write")).Delete("/members/{account_id}", app.removeTeamMember)
				})
			})

			r.Route("/roles", func(r chi.Router) {
				r.With(app.requireOrgPermission("policy:read")).Get("/", app.listRoles)
				r.With(app.requireOrgPermission("policy:modify")).Post("/", app.createRole)
				r.With(app.requireOrgPermission("policy:read")).Get("/{role_id}", app.getRole)
				r.With(app.requireOrgPermission("policy:modify")).Delete("/{role_id}", app.deleteRole)
			})

			r.Route("/policies", func(r chi.Router) {
				r.With(app.requireOrgPermission("policy:read")).Get("/", app.listOrgPolicies)
				r.With(app.requireOrgPermission("policy:modify")).Post("/", app.grantOrgPolicy)
				r.With(app.requireOrgPermission("policy:modify")).Delete("/{binding_id}", app.revokeOrgPolicy)
			})

			r.Get("/permissions", app.listPermissions)
			r.Get("/permissions/effective", app.getEffectivePermissions)

			r.Route("/workspaces", func(r chi.Router) {
				r.Get("/", app.listWorkspaces)
				r.With(app.requireOrgPermission("ws:create")).Post("/", app.createWorkspace)
				r.Route("/{ws_id}", func(r chi.Router) {
					r.Use(app.wsCtx)
					r.Get("/", app.getWorkspace)
					r.With(app.requireWorkspacePermission("ws:write")).Patch("/", app.updateWorkspace)
					r.With(app.requireWorkspacePermission("ws:delete")).Delete("/", app.deleteWorkspace)
					r.Get("/sessions", app.listActiveSessions)
					r.Delete("/sessions/{session_id}", app.revokeWorkspaceDatabaseSession)
					r.Route("/jobs", func(r chi.Router) {
						r.Get("/", app.listWorkspaceJobs)
						r.Get("/{job_id}", app.getWorkspaceJob)
						r.Post("/{job_id}/cancel", app.cancelWorkspaceJob)
					})

					r.Get("/permissions", app.listWorkspacePermissions)

					r.Route("/files/private", func(r chi.Router) {
						r.Get("/", app.listPrivateWorkspaceFiles)
						r.Post("/", app.createPrivateWorkspaceFile)
						r.Get("/browser", app.browsePrivateWorkspaceFiles)
						r.Get("/recent", app.listRecentPrivateWorkspaceFiles)
						r.Route("/{file_id}", func(r chi.Router) {
							r.Get("/", app.getPrivateWorkspaceFile)
							r.Patch("/", app.updatePrivateWorkspaceFile)
							r.Delete("/", app.deletePrivateWorkspaceFile)
							r.Get("/content", app.getPrivateWorkspaceFileContent)
							r.Put("/content", app.updatePrivateWorkspaceFileContent)
						})
					})

					r.Route("/files/shared", func(r chi.Router) {
						r.Get("/", app.listSharedWorkspaceFiles)
						r.Post("/", app.createSharedWorkspaceFile)
						r.Get("/browser", app.browseSharedWorkspaceFiles)
						r.Get("/recent", app.listRecentSharedWorkspaceFiles)
						r.Route("/{file_id}", func(r chi.Router) {
							r.Get("/", app.getSharedWorkspaceFile)
							r.Patch("/", app.updateSharedWorkspaceFile)
							r.Delete("/", app.deleteSharedWorkspaceFile)
							r.Get("/content", app.getSharedWorkspaceFileContent)
							r.Put("/content", app.updateSharedWorkspaceFileContent)
						})
					})

					r.Route("/users", func(r chi.Router) {
						r.With(app.requireWorkspacePermission("policy:read")).Get("/", app.listWorkspaceMembers)
						r.With(app.requireWorkspacePermission("policy:read")).Get("/effective", app.listWorkspaceEffectiveMembers)
						r.With(app.requireWorkspacePermission("policy:modify")).Post("/", app.addWorkspaceMember)
						r.With(app.requireWorkspacePermission("policy:modify")).Delete("/{account_id}", app.removeWorkspaceMember)
					})

					r.Route("/teams", func(r chi.Router) {
						r.With(app.requireWorkspacePermission("policy:read")).Get("/", app.listWorkspaceTeams)
						r.With(app.requireWorkspacePermission("policy:modify")).Post("/", app.addWorkspaceTeam)
						r.With(app.requireWorkspacePermission("policy:modify")).Delete("/{team_id}", app.removeWorkspaceTeam)
					})

					r.Route("/roles", func(r chi.Router) {
						r.With(app.requireWorkspacePermission("policy:read")).Get("/", app.listWorkspaceRoles)
						r.With(app.requireWorkspacePermission("policy:modify")).Post("/", app.createWorkspaceRole)
						r.With(app.requireWorkspacePermission("policy:read")).Get("/{role_id}", app.getWorkspaceRole)
						r.With(app.requireWorkspacePermission("policy:modify")).Delete("/{role_id}", app.deleteWorkspaceRole)
					})

					r.Route("/policies", func(r chi.Router) {
						r.With(app.requireWorkspacePermission("policy:read")).Get("/", app.listWorkspacePolicies)
						r.With(app.requireWorkspacePermission("policy:modify")).Post("/", app.grantWorkspacePolicy)
						r.With(app.requireWorkspacePermission("policy:modify")).Delete("/{binding_id}", app.revokeWorkspacePolicy)
					})

					r.Route("/environments", func(r chi.Router) {
						r.Get("/", app.listEnvironments)
						r.With(app.requireWorkspacePermission("env:create")).Post("/", app.createEnvironment)
						r.Route("/{env_id}", func(r chi.Router) {
							r.Use(app.envCtx)
							r.Get("/", app.getEnvironment)
							r.With(app.requireEnvironmentPermission("env:write")).Patch("/", app.updateEnvironment)
							r.With(app.requireEnvironmentPermission("env:delete")).Delete("/", app.deleteEnvironment)

							r.Route("/connections", func(r chi.Router) {
								r.With(app.requireEnvironmentPermission("conn:create")).Post("/test", app.testConnection)
								r.Get("/", app.listConnections)
								r.With(app.requireEnvironmentPermission("conn:create")).Post("/", app.createConnection)
								r.Route("/{conn_id}", func(r chi.Router) {
									r.Use(app.connCtx)
									r.Get("/", app.getConnection)
									r.With(app.requireConnectionPermission("conn:write")).Patch("/", app.updateConnection)
									r.With(app.requireConnectionPermission("conn:delete")).Delete("/", app.deleteConnection)
									r.Post("/connect", app.connectToDatabase)
									r.Delete("/session", app.disconnectFromDatabase)
									r.Post("/query-cursors", app.startQueryCursor)
									r.Post("/query-cursors/{query_cursor_id}/fetch", app.fetchQueryCursor)
									r.Delete("/query-cursors/{query_cursor_id}", app.closeQueryCursor)
									r.Post("/query", app.executeQuery)
									r.Get("/schema/spec", app.getConnectionSchemaSpec)
									r.Get("/schema/catalog", app.getConnectionSchemaCatalog)
									r.Post("/schema/objects", app.getConnectionSchemaObjects)
									r.Post("/schema/refresh", app.refreshConnectionSchema)
								})
							})
						})
					})

					r.Route("/connections", func(r chi.Router) {
						r.With(app.requireWorkspacePermission("conn:create")).Post("/test", app.testConnection)
						r.Get("/", app.listConnections)
						r.With(app.requireWorkspacePermission("conn:create")).Post("/", app.createConnection)
						r.Route("/{conn_id}", func(r chi.Router) {
							r.Use(app.connCtx)
							r.Get("/", app.getConnection)
							r.With(app.requireConnectionPermission("conn:write")).Patch("/", app.updateConnection)
							r.With(app.requireConnectionPermission("conn:delete")).Delete("/", app.deleteConnection)
							r.Post("/connect", app.connectToDatabase)
							r.Delete("/session", app.disconnectFromDatabase)
							r.Post("/query-cursors", app.startQueryCursor)
							r.Post("/query-cursors/{query_cursor_id}/fetch", app.fetchQueryCursor)
							r.Delete("/query-cursors/{query_cursor_id}", app.closeQueryCursor)
							r.Post("/query", app.executeQuery)
							r.Get("/schema/spec", app.getConnectionSchemaSpec)
							r.Get("/schema/catalog", app.getConnectionSchemaCatalog)
							r.Post("/schema/objects", app.getConnectionSchemaObjects)
							r.Post("/schema/refresh", app.refreshConnectionSchema)
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
