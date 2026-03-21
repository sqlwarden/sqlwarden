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

	// API v1 routes
	mux.Route("/api/v1", func(r chi.Router) {
		r.Use(app.authenticateV1)

		r.Post("/auth/register", app.registerAccount)
		r.Post("/auth/login", app.loginAccount)
		r.Post("/auth/refresh", app.refreshToken)
		r.Post("/auth/logout", app.logoutAccount)

		r.Group(func(r chi.Router) {
			r.Use(app.requireAccount)
			r.Get("/user", app.getAccount)
			r.Get("/user/orgs", app.getAccountOrgs)
		})

		r.Route("/orgs/{org_slug}", func(r chi.Router) {
			r.Use(app.requireAccount, app.orgCtx)

			r.Get("/", app.getOrg)

			r.Route("/members", func(r chi.Router) {
				r.With(app.requirePermission("members", "read")).Get("/", app.listOrgMembers)
				r.With(app.requirePermission("members", "write")).Post("/", app.addOrgMember)
				r.With(app.requireOrgRole("owner")).Patch("/{account_id}", app.updateOrgMemberRole)
				r.With(app.requirePermission("members", "delete")).Delete("/{account_id}", app.removeOrgMember)
			})
		})
	})

	// Serve the embedded React SPA for all other routes
	staticFS, err := fs.Sub(assets.EmbeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Get("/*", app.spaHandler(staticFS))

	return mux
}

// spaHandler serves a React SPA from the given filesystem. Any request that
// does not map to an existing static file is served with index.html so that
// client-side routing works correctly.
func (app *application) spaHandler(staticFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(staticFS))

	return func(w http.ResponseWriter, r *http.Request) {
		// Strip the leading "/" so we can look up the file in the FS.
		path := r.URL.Path[1:]
		if path == "" {
			path = "index.html"
		}

		_, err := staticFS.Open(path)
		if err != nil {
			// File not found – serve index.html for client-side routing.
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
