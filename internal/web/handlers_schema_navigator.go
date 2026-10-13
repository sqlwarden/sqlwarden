package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type navigatorItemView struct {
	Kind       string             `json:"kind"`
	Name       string             `json:"name"`
	Path       queryapp.ScopePath `json:"path"`
	Attributes map[string]any     `json:"attributes,omitempty"`
	System     bool               `json:"system"`
	Current    bool               `json:"current"`
}

type navigatorListingView struct {
	Path      queryapp.ScopePath  `json:"path"`
	Folder    string              `json:"folder"`
	Items     []navigatorItemView `json:"items"`
	FetchedAt time.Time           `json:"fetched_at"`
	Source    string              `json:"source"`
}

type schemaTreeResponse struct {
	queryapp.Tree
	Editor     *queryapp.DDLSpec       `json:"editor,omitempty"`
	Statements *queryapp.StatementSpec `json:"statements,omitempty"`
}

type navigatorRefreshRequest struct {
	Path queryapp.ScopePath `json:"path"`
}

func listingView(listing queryapp.Listing) navigatorListingView {
	items := make([]navigatorItemView, 0, len(listing.Items))
	for _, item := range listing.Items {
		items = append(items, navigatorItemView{
			Kind: item.Kind, Name: item.Name, Attributes: item.Attributes, System: item.System, Current: item.Current,
			Path: listing.Parent.Child(queryapp.ScopeSegment{Kind: item.Kind, Name: item.Name}),
		})
	}
	return navigatorListingView{Path: listing.Parent, Folder: listing.Folder, Items: items, FetchedAt: listing.FetchedAt, Source: listing.Source}
}

func (app *application) getConnectionSchemaTree(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	spec, err := app.queryService().SchemaTree(r.Context(), p, connectionRef(r))
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	out := schemaTreeResponse{Tree: spec.Tree, Editor: spec.Editor, Statements: spec.Statements}
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaNodes(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	folder := r.URL.Query().Get("folder")
	var parent queryapp.ScopePath
	if raw := r.URL.Query().Get("path"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &parent); err != nil {
			app.errorMessage(w, r, http.StatusBadRequest, "path query parameter must be a JSON array of segments.", nil)
			return
		}
	}
	start := time.Now()
	listing, err := app.queryService().SchemaNode(r.Context(), p, connectionRef(r), requestSessionID(r), parent, folder)
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	app.logDebug(r, "schema listing served",
		slog.Int64("connection_id", contextGetPathIDs(r).ConnectionID),
		slog.String("folder", folder),
		slog.Int("depth", parent.Depth()),
		slog.String("source", listing.Source),
		slog.Int("items", len(listing.Items)),
		slog.Duration("duration", time.Since(start)),
	)
	if err := response.JSON(w, http.StatusOK, listingView(listing)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshConnectionSchemaNodes(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input navigatorRefreshRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	start := time.Now()
	listings, err := app.queryService().RefreshSchema(r.Context(), p, connectionRef(r), requestSessionID(r), input.Path)
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	app.logInfo(r, "schema subtree refreshed",
		slog.Int64("connection_id", contextGetPathIDs(r).ConnectionID),
		slog.Int("depth", input.Path.Depth()),
		slog.Int("listings", len(listings)),
		slog.Duration("duration", time.Since(start)),
	)
	views := make([]navigatorListingView, 0, len(listings))
	for _, listing := range listings {
		views = append(views, listingView(listing))
	}
	if err := response.JSON(w, http.StatusOK, map[string]any{"items": views}); err != nil {
		app.serverError(w, r, err)
	}
}
