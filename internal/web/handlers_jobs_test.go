package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/jobs"
	"github.com/stretchr/testify/assert"
)

func TestWorkspaceJobsListShowsOnlyCurrentUsersVisibleJobs(t *testing.T) {
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "jobs-owner"), "Jobs Owner", "Jobs Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Jobs Workspace", "")
	if err := app.db.AddWorkspaceMember(context.Background(), ws.ID, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	other := seedAccount(t, app, uniqueEmail(t, "jobs-other"), "Jobs Other")
	store := jobs.NewStore(app.db)
	ctx := context.Background()
	userJob, err := store.Enqueue(ctx, jobs.EnqueueInput{
		Type:           "noop",
		Visibility:     jobs.VisibilityUser,
		OrgID:          &org.ID,
		WorkspaceID:    &ws.ID,
		OwnerAccountID: &owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	internalJob, err := store.Enqueue(ctx, jobs.EnqueueInput{Type: "noop", Visibility: jobs.VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(ctx, jobs.EnqueueInput{
		Type:           "noop",
		Visibility:     jobs.VisibilityUser,
		OrgID:          &org.ID,
		WorkspaceID:    &ws.ID,
		OwnerAccountID: &other.ID,
	}); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/jobs", nil, token), app.routes())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var body struct {
		Items []jobs.Record `json:"items"`
		Total int           `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &body)
	assert.Equal(t, 1, body.Total)
	if assert.Len(t, body.Items, 1) {
		assert.Equal(t, userJob.ID, body.Items[0].ID)
	}

	internalURL := "/api/v1/orgs/" + org.Slug + "/workspaces/" + strconv.FormatInt(ws.ID, 10) + "/jobs/" + internalJob.ID
	res = send(t, newAuthRequest(t, http.MethodGet, internalURL, nil, token), app.routes())
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestWorkspaceJobGetAndCancelAreScopedToOwnerAndWorkspace(t *testing.T) {
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "jobs-scope-owner"), "Jobs Scope Owner", "Jobs Scope Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Jobs Scope Workspace", "")
	otherWS := seedWorkspaceForAccount(t, app, org, owner, "Other Jobs Scope Workspace", "")
	if err := app.db.AddWorkspaceMember(context.Background(), ws.ID, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddWorkspaceMember(context.Background(), otherWS.ID, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	store := jobs.NewStore(app.db)
	ctx := context.Background()
	job, err := store.Enqueue(ctx, jobs.EnqueueInput{
		Type:           "noop",
		Visibility:     jobs.VisibilityUser,
		OrgID:          &org.ID,
		WorkspaceID:    &ws.ID,
		OwnerAccountID: &owner.ID,
		RunAt:          time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	getURL := "/api/v1/orgs/" + org.Slug + "/workspaces/" + strconv.FormatInt(ws.ID, 10) + "/jobs/" + job.ID
	res := send(t, newAuthRequest(t, http.MethodGet, getURL, nil, token), app.routes())
	assert.Equal(t, http.StatusOK, res.StatusCode)

	wrongWorkspaceURL := "/api/v1/orgs/" + org.Slug + "/workspaces/" + strconv.FormatInt(otherWS.ID, 10) + "/jobs/" + job.ID
	res = send(t, newAuthRequest(t, http.MethodGet, wrongWorkspaceURL, nil, token), app.routes())
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	cancelURL := getURL + "/cancel"
	res = send(t, newAuthRequest(t, http.MethodPost, cancelURL, nil, token), app.routes())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var cancelled jobs.Record
	decodeJSONResponse(t, res.BodyBytes, &cancelled)
	assert.Equal(t, jobs.StatusCancelled, cancelled.Status)
	assert.NotNil(t, cancelled.CancelRequestedAt)

	res = send(t, newAuthRequest(t, http.MethodPost, cancelURL, nil, token), app.routes())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	decodeJSONResponse(t, res.BodyBytes, &cancelled)
	assert.Equal(t, jobs.StatusCancelled, cancelled.Status)
}

func TestWorkspaceJobEventsAreScopedAndIncremental(t *testing.T) {
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "jobs-events-owner"), "Jobs Events Owner", "Jobs Events Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Jobs Events Workspace", "")
	otherWS := seedWorkspaceForAccount(t, app, org, owner, "Other Jobs Events Workspace", "")
	if err := app.db.AddWorkspaceMember(context.Background(), ws.ID, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddWorkspaceMember(context.Background(), otherWS.ID, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	store := jobs.NewStore(app.db)
	ctx := context.Background()
	job, err := store.Enqueue(ctx, jobs.EnqueueInput{
		Type:           "export",
		Visibility:     jobs.VisibilityUser,
		OrgID:          &org.ID,
		WorkspaceID:    &ws.ID,
		OwnerAccountID: &owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendEvent(ctx, jobs.EventInput{JobID: job.ID, Level: jobs.EventLevelInfo, Code: "query_started", Message: "Query started."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendEvent(ctx, jobs.EventInput{JobID: job.ID, Level: jobs.EventLevelInfo, Code: "file_saved", Message: "File saved."})
	if err != nil {
		t.Fatal(err)
	}

	eventsURL := "/api/v1/orgs/" + org.Slug + "/workspaces/" + strconv.FormatInt(ws.ID, 10) + "/jobs/" + job.ID + "/events?page_size=1"
	res := send(t, newAuthRequest(t, http.MethodGet, eventsURL, nil, token), app.routes())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var page jobs.EventPage
	decodeJSONResponse(t, res.BodyBytes, &page)
	if assert.Len(t, page.Items, 1) {
		assert.Equal(t, first.ID, page.Items[0].ID)
	}
	assert.Equal(t, first.ID, page.NextAfterID)

	res = send(t, newAuthRequest(t, http.MethodGet, eventsURL+"&after_id="+page.NextAfterID, nil, token), app.routes())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	decodeJSONResponse(t, res.BodyBytes, &page)
	if assert.Len(t, page.Items, 1) {
		assert.Equal(t, second.ID, page.Items[0].ID)
	}

	wrongWorkspaceURL := "/api/v1/orgs/" + org.Slug + "/workspaces/" + strconv.FormatInt(otherWS.ID, 10) + "/jobs/" + job.ID + "/events"
	res = send(t, newAuthRequest(t, http.MethodGet, wrongWorkspaceURL, nil, token), app.routes())
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
