package web

import (
	"net/http"
	"strconv"
	"testing"
)

func TestWorkspaceRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	foreignWorkspace := accessCase{name: "workspace of another org", who: "owner",
		path: func(f *accessCharacterizationFixture) string { return f.foreignWSPath() }, want: expectNotFound}
	unknownWorkspace := accessCase{name: "unknown workspace", who: "owner",
		path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/workspaces/999999" }, want: expectNotFound}
	malformedWorkspace := accessCase{name: "malformed workspace id", who: "owner",
		path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/workspaces/abc" }, want: expectNotFound}
	wsPath := func(f *accessCharacterizationFixture) string { return f.wsPath() }
	renameBody := func(*accessCharacterizationFixture) map[string]any {
		return map[string]any{"name": "Renamed Workspace"}
	}

	routes := []accessRoute{
		{
			name: "list workspaces", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/workspaces" },
			member: &accessExpect{status: http.StatusOK},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "create workspace", method: http.MethodPost,
			path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/workspaces" },
			body: func(*accessCharacterizationFixture) map[string]any {
				return map[string]any{"name": "Characterized New Workspace"}
			},
			member: &notPermitted,
			extra: []accessCase{
				{name: "duplicate name", who: "owner", body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"name": "Access Characterization Workspace"}
				}, want: expectStatus(http.StatusCreated)},
				{name: "missing name", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
			},
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "get workspace", method: http.MethodGet,
			path:   wsPath,
			member: &expectNotFound,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectStatus(http.StatusOK)},
				foreignWorkspace, unknownWorkspace, malformedWorkspace,
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "update workspace", method: http.MethodPatch,
			path:   wsPath,
			body:   renameBody,
			member: &notPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				foreignWorkspace, unknownWorkspace,
				{name: "immutable org_id", who: "owner", body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"name": "Renamed", "org_id": 1}
				}, want: expectValidation},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "delete workspace", method: http.MethodDelete,
			path:   wsPath,
			member: &notPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				foreignWorkspace, unknownWorkspace,
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}

func TestEnvironmentRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	envBase := func(f *accessCharacterizationFixture) string { return f.wsPath() + "/environments" }
	spareEnv := func(f *accessCharacterizationFixture) string {
		return envBase(f) + "/" + strconv.FormatInt(f.spareEnvID, 10)
	}
	foreignWorkspace := accessCase{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
		return f.foreignWSPath() + "/environments/" + strconv.FormatInt(f.foreignEnvID, 10)
	}, want: expectNotFound}
	foreignEnvironment := accessCase{name: "environment of another workspace", who: "owner", path: func(f *accessCharacterizationFixture) string {
		return envBase(f) + "/" + strconv.FormatInt(f.foreignEnvID, 10)
	}, want: expectNotFound}
	unknownEnvironment := accessCase{name: "unknown environment", who: "owner", path: func(f *accessCharacterizationFixture) string {
		return envBase(f) + "/999999"
	}, want: expectNotFound}
	renameBody := func(*accessCharacterizationFixture) map[string]any {
		return map[string]any{"name": "Renamed Environment"}
	}

	routes := []accessRoute{
		{
			name: "list environments", method: http.MethodGet,
			path:   envBase,
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.foreignWSPath() + "/environments"
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "create environment", method: http.MethodPost,
			path: envBase,
			body: func(*accessCharacterizationFixture) map[string]any {
				return map[string]any{"name": "Characterized Environment"}
			},
			member: &notPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				{name: "duplicate name", who: "owner", body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"name": "Spare"}
				}, want: expectValidation},
				{name: "missing name", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
				{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.foreignWSPath() + "/environments"
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "get environment", method: http.MethodGet,
			path:   spareEnv,
			member: &expectNotFound,
			extra:  []accessCase{foreignWorkspace, foreignEnvironment, unknownEnvironment},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "update environment", method: http.MethodPatch,
			path:   spareEnv,
			body:   renameBody,
			member: &notPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				foreignWorkspace, foreignEnvironment, unknownEnvironment,
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "delete environment", method: http.MethodDelete,
			path:   spareEnv,
			member: &notPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				foreignWorkspace, foreignEnvironment, unknownEnvironment,
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}
