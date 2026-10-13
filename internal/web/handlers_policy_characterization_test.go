package web

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/access"
)

type policyCharacterizationIDs struct {
	customRole       int64
	inUseRole        int64
	foreignRole      int64
	ownerRole        int64
	privilegedRole   int64
	wsCustomRole     int64
	wsInUseRole      int64
	wsBuiltinRole    int64
	foreignWSRole    int64
	envRole          int64
	ownerBinding     int64
	targetBinding    int64
	wsTargetBinding  int64
	wsSubjectBinding int64
}

var policyCharacterizationRegistry sync.Map

func policyIDs(f *accessCharacterizationFixture) policyCharacterizationIDs {
	ids, _ := policyCharacterizationRegistry.Load(f)
	return ids.(policyCharacterizationIDs)
}

func queryInt64(t *testing.T, f *accessCharacterizationFixture, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := testDB(f.app).NewRaw(query, args...).Scan(context.Background(), &id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newPolicyCharacterizationFixture(t *testing.T) *accessCharacterizationFixture {
	t.Helper()
	f := newAccessCharacterizationFixture(t)
	app := f.app
	wsID := f.workspace.ID
	wsIDStr := strconv.FormatInt(wsID, 10)
	var ids policyCharacterizationIDs

	ids.customRole = createRoleForTest(t, app, f.org.ID, nil, "org", access.PermOrgRead)
	ids.inUseRole = createRoleForTest(t, app, f.org.ID, nil, "org", access.PermOrgRead)
	ids.privilegedRole = createRoleForTest(t, app, f.org.ID, nil, "org", access.PermOrgDelete)
	ids.foreignRole = createRoleForTest(t, app, f.otherOrg.ID, nil, "org", access.PermOrgRead)
	if res := grantOrgPolicyRole(t, app, f.tokens["owner"], f.org.Slug, ids.inUseRole, access.SubjectTypeAccount, f.target.ID); res.StatusCode != http.StatusNoContent {
		t.Fatalf("grant org policy status = %d", res.StatusCode)
	}
	ids.targetBinding = queryInt64(t, f, "SELECT id FROM role_bindings WHERE org_id = ? AND role_id = ? AND subject_id = ?", f.org.ID, ids.inUseRole, f.target.ID)
	ids.ownerRole = queryInt64(t, f, "SELECT id FROM roles WHERE org_id = ? AND name = ? AND workspace_id IS NULL", f.org.ID, access.BuiltinOrgOwnerRole)
	ids.ownerBinding = queryInt64(t, f, "SELECT id FROM role_bindings WHERE org_id = ? AND role_id = ? AND resource_type = 'org'", f.org.ID, ids.ownerRole)

	policyAdminRole := createRoleForTest(t, app, f.org.ID, nil, "org", access.PermPolicyRead, access.PermPolicyModify)
	policyAdmin, policyAdminToken := seedAccountWithToken(t, app, uniqueEmail(t, "policy-char-admin"), "Policy Characterization Admin")
	if err := testDB(app).AddOrgMember(context.Background(), f.org.ID, policyAdmin.ID); err != nil {
		t.Fatal(err)
	}
	f.tokens["policyAdmin"] = policyAdminToken
	if res := grantOrgPolicyRole(t, app, f.tokens["owner"], f.org.Slug, policyAdminRole, access.SubjectTypeAccount, policyAdmin.ID); res.StatusCode != http.StatusNoContent {
		t.Fatalf("grant policy admin status = %d", res.StatusCode)
	}

	ids.wsCustomRole = createRoleForTest(t, app, f.org.ID, &wsID, "workspace", access.PermWsRead)
	ids.wsInUseRole = createRoleForTest(t, app, f.org.ID, &wsID, "workspace", access.PermWsRead)
	ids.envRole = createRoleForTest(t, app, f.org.ID, &wsID, "environment", access.PermEnvRead)
	foreignWSID := f.foreignWS.ID
	ids.foreignWSRole = createRoleForTest(t, app, f.otherOrg.ID, &foreignWSID, "workspace", access.PermWsRead)
	ids.wsBuiltinRole = queryInt64(t, f, "SELECT id FROM roles WHERE org_id = ? AND name = ? AND workspace_id = ?", f.org.ID, access.BuiltinWorkspaceAdminRole, wsID)
	if res := grantWorkspacePolicyRole(t, app, f.tokens["owner"], f.org.Slug, wsIDStr, ids.wsInUseRole, access.SubjectTypeAccount, f.target.ID, "workspace", wsID); res.StatusCode != http.StatusNoContent {
		t.Fatalf("grant workspace policy status = %d", res.StatusCode)
	}
	ids.wsTargetBinding = queryInt64(t, f, "SELECT id FROM role_bindings WHERE org_id = ? AND role_id = ? AND subject_id = ?", f.org.ID, ids.wsInUseRole, f.target.ID)

	policyCharacterizationRegistry.Store(f, ids)
	return f
}

func orgPolicyPath(suffix string, id func(policyCharacterizationIDs) int64) func(*accessCharacterizationFixture) string {
	return func(f *accessCharacterizationFixture) string {
		p := f.orgPath() + suffix
		if id != nil {
			p += "/" + strconv.FormatInt(id(policyIDs(f)), 10)
		}
		return p
	}
}

func wsPolicyPath(suffix string, id func(policyCharacterizationIDs) int64) func(*accessCharacterizationFixture) string {
	return func(f *accessCharacterizationFixture) string {
		p := f.wsPath() + suffix
		if id != nil {
			p += "/" + strconv.FormatInt(id(policyIDs(f)), 10)
		}
		return p
	}
}

func staticBody(body map[string]any) func(*accessCharacterizationFixture) map[string]any {
	return func(*accessCharacterizationFixture) map[string]any { return body }
}

func TestOrgRoleRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	custom := func(ids policyCharacterizationIDs) int64 { return ids.customRole }
	inUse := func(ids policyCharacterizationIDs) int64 { return ids.inUseRole }
	foreign := func(ids policyCharacterizationIDs) int64 { return ids.foreignRole }
	owner := func(ids policyCharacterizationIDs) int64 { return ids.ownerRole }
	roles := func(f *accessCharacterizationFixture) string { return f.orgPath() + "/roles" }
	validRole := map[string]any{"name": "Characterized Role", "scope_type": "org", "permissions": []string{access.PermOrgRead}}
	withBody := func(overrides map[string]any) func(*accessCharacterizationFixture) map[string]any {
		return func(*accessCharacterizationFixture) map[string]any {
			body := map[string]any{}
			for k, v := range validRole {
				body[k] = v
			}
			for k, v := range overrides {
				body[k] = v
			}
			return body
		}
	}
	renameBody := staticBody(map[string]any{"name": "Renamed Role", "permissions": []string{access.PermOrgRead}})

	routes := []accessRoute{
		{
			name: "list roles", method: http.MethodGet, path: roles,
			member: &notPermitted,
			extra: []accessCase{
				{name: "invalid scope", who: "owner", path: func(f *accessCharacterizationFixture) string { return roles(f) + "?scope=bogus" }, want: expectValidation},
				{name: "invalid builtin flag", who: "owner", path: func(f *accessCharacterizationFixture) string { return roles(f) + "?builtin=maybe" }, want: expectValidation},
				{name: "policy reader", who: "policyAdmin", want: expectStatus(http.StatusOK)},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "get role", method: http.MethodGet, path: orgPolicyPath("/roles", custom),
			member: &notPermitted,
			extra: []accessCase{
				{name: "unknown role", who: "owner", path: func(f *accessCharacterizationFixture) string { return roles(f) + "/999999" }, want: expectNotFound},
				{name: "malformed role id", who: "owner", path: func(f *accessCharacterizationFixture) string { return roles(f) + "/abc" }, want: expectNotFound},
				{name: "role of another org", who: "owner", path: orgPolicyPath("/roles", foreign), want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "create role", method: http.MethodPost, path: roles, body: withBody(nil),
			member: &notPermitted,
			extra: []accessCase{
				{name: "policy admin", who: "policyAdmin", want: expectStatus(http.StatusCreated)},
				{name: "missing name", who: "owner", body: withBody(map[string]any{"name": ""}), want: expectValidation},
				{name: "workspace scope", who: "owner", body: withBody(map[string]any{"scope_type": "workspace"}), want: expectValidation},
				{name: "unknown scope", who: "owner", body: withBody(map[string]any{"scope_type": "bogus"}), want: expectValidation},
				{name: "workspace id on org role", who: "owner", body: withBody(map[string]any{"workspace_id": 1}), want: expectValidation},
				{name: "connection permission on org role", who: "owner", body: withBody(map[string]any{"permissions": []string{access.PermConnExecute}}), want: expectStatus(http.StatusCreated)},
				{name: "unknown permission", who: "owner", body: withBody(map[string]any{"permissions": []string{"bogus:perm"}}), want: expectValidation},
			},
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "update role", method: http.MethodPatch, path: orgPolicyPath("/roles", custom), body: renameBody,
			member: &notPermitted,
			extra: []accessCase{
				{name: "builtin role", who: "owner", path: orgPolicyPath("/roles", owner), want: expectNotPermitted},
				{name: "unknown role", who: "owner", path: func(f *accessCharacterizationFixture) string { return roles(f) + "/999999" }, want: expectNotFound},
				{name: "role of another org", who: "owner", path: orgPolicyPath("/roles", foreign), want: expectNotFound},
				{name: "missing name", who: "owner", body: staticBody(map[string]any{"name": ""}), want: expectValidation},
				{name: "role in use", who: "owner", path: orgPolicyPath("/roles", inUse), want: expectStatus(http.StatusOK)},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "delete role", method: http.MethodDelete, path: orgPolicyPath("/roles", custom),
			member: &notPermitted,
			extra: []accessCase{
				{name: "builtin role", who: "owner", path: orgPolicyPath("/roles", owner), want: expectNotPermitted},
				{name: "unknown role", who: "owner", path: func(f *accessCharacterizationFixture) string { return roles(f) + "/999999" }, want: expectNotFound},
				{name: "role of another org", who: "owner", path: orgPolicyPath("/roles", foreign), want: expectNotFound},
				{name: "role in use", who: "owner", path: orgPolicyPath("/roles", inUse), want: accessExpect{http.StatusConflict, apiErrorResourceInUse}},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newPolicyCharacterizationFixture, routes)
}

func TestOrgPolicyRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	policies := func(f *accessCharacterizationFixture) string { return f.orgPath() + "/policies" }
	targetBinding := func(ids policyCharacterizationIDs) int64 { return ids.targetBinding }
	ownerBinding := func(ids policyCharacterizationIDs) int64 { return ids.ownerBinding }
	wsBinding := func(ids policyCharacterizationIDs) int64 { return ids.wsTargetBinding }
	grant := func(role func(policyCharacterizationIDs) int64, subjectType string, subject func(*accessCharacterizationFixture) int64) func(*accessCharacterizationFixture) map[string]any {
		return func(f *accessCharacterizationFixture) map[string]any {
			return map[string]any{"role_id": role(policyIDs(f)), "subject_type": subjectType, "subject_id": subject(f)}
		}
	}
	targetID := func(f *accessCharacterizationFixture) int64 { return f.target.ID }
	custom := func(ids policyCharacterizationIDs) int64 { return ids.customRole }
	privileged := func(ids policyCharacterizationIDs) int64 { return ids.privilegedRole }
	ownerRole := func(ids policyCharacterizationIDs) int64 { return ids.ownerRole }
	foreignRole := func(ids policyCharacterizationIDs) int64 { return ids.foreignRole }
	wsRole := func(ids policyCharacterizationIDs) int64 { return ids.wsCustomRole }

	routes := []accessRoute{
		{
			name: "list policies", method: http.MethodGet, path: policies,
			member: &notPermitted,
			extra:  []accessCase{{name: "policy reader", who: "policyAdmin", want: expectStatus(http.StatusOK)}},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "get policy", method: http.MethodGet, path: orgPolicyPath("/policies", targetBinding),
			member: &notPermitted,
			extra: []accessCase{
				{name: "unknown binding", who: "owner", path: func(f *accessCharacterizationFixture) string { return policies(f) + "/999999" }, want: expectNotFound},
				{name: "malformed binding id", who: "owner", path: func(f *accessCharacterizationFixture) string { return policies(f) + "/abc" }, want: expectNotFound},
				{name: "workspace binding", who: "owner", path: orgPolicyPath("/policies", wsBinding), want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "grant policy", method: http.MethodPost, path: policies, body: grant(custom, access.SubjectTypeAccount, targetID),
			member: &notPermitted,
			extra: []accessCase{
				{name: "policy admin granting protected role", who: "policyAdmin", body: grant(privileged, access.SubjectTypeAccount, targetID), want: expectNotPermitted},
				{name: "policy admin granting owner role", who: "policyAdmin", body: grant(ownerRole, access.SubjectTypeAccount, targetID), want: expectNotPermitted},
				{name: "policy admin granting plain role", who: "policyAdmin", want: expectStatus(http.StatusNoContent)},
				{name: "owner granting protected role", who: "owner", body: grant(privileged, access.SubjectTypeAccount, targetID), want: expectStatus(http.StatusNoContent)},
				{name: "missing role", who: "owner", body: staticBody(map[string]any{"subject_type": "account", "subject_id": 1}), want: expectValidation},
				{name: "invalid subject type", who: "owner", body: grant(custom, "bogus", targetID), want: expectValidation},
				{name: "missing subject", who: "owner", body: grant(custom, access.SubjectTypeAccount, func(*accessCharacterizationFixture) int64 { return 0 }), want: expectValidation},
				{name: "subject outside org", who: "owner", body: grant(custom, access.SubjectTypeAccount, func(f *accessCharacterizationFixture) int64 { return f.nonMember.ID }), want: expectNotFound},
				{name: "unknown subject", who: "owner", body: grant(custom, access.SubjectTypeAccount, func(*accessCharacterizationFixture) int64 { return 999999 }), want: expectNotFound},
				{name: "role of another org", who: "owner", body: grant(foreignRole, access.SubjectTypeAccount, targetID), want: expectNotFound},
				{name: "workspace scoped role", who: "owner", body: grant(wsRole, access.SubjectTypeAccount, targetID), want: expectValidation},
				{name: "malformed body", who: "owner", body: staticBody(map[string]any{"role_id": "x"}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "revoke policy", method: http.MethodDelete, path: orgPolicyPath("/policies", targetBinding),
			member: &notPermitted,
			extra: []accessCase{
				{name: "last owner binding", who: "owner", path: orgPolicyPath("/policies", ownerBinding), want: expectValidation},
				{name: "unknown binding", who: "owner", path: func(f *accessCharacterizationFixture) string { return policies(f) + "/999999" }, want: expectNotFound},
				{name: "malformed binding id", who: "owner", path: func(f *accessCharacterizationFixture) string { return policies(f) + "/abc" }, want: expectNotFound},
				{name: "workspace binding", who: "owner", path: orgPolicyPath("/policies", wsBinding), want: expectNotFound},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newPolicyCharacterizationFixture, routes)
}

func TestOrgProtectedPolicyRevokeCharacterization(t *testing.T) {
	f := newPolicyCharacterizationFixture(t)
	ids := policyIDs(f)
	if res := grantOrgPolicyRole(t, f.app, f.tokens["owner"], f.org.Slug, ids.ownerRole, access.SubjectTypeAccount, f.target.ID); res.StatusCode != http.StatusNoContent {
		t.Fatalf("grant owner role status = %d", res.StatusCode)
	}
	bindingID := queryInt64(t, f, "SELECT id FROM role_bindings WHERE org_id = ? AND role_id = ? AND subject_id = ?", f.org.ID, ids.ownerRole, f.target.ID)
	path := f.orgPath() + "/policies/" + strconv.FormatInt(bindingID, 10)

	policyAdmin := createRoleForTest(t, f.app, f.org.ID, nil, "org", access.PermPolicyRead, access.PermPolicyModify)
	if res := grantOrgPolicyRole(t, f.app, f.tokens["owner"], f.org.Slug, policyAdmin, access.SubjectTypeAccount, f.member.ID); res.StatusCode != http.StatusNoContent {
		t.Fatalf("grant policy admin status = %d", res.StatusCode)
	}
	res := send(t, newAuthRequest(t, http.MethodDelete, path, nil, f.tokens["member"]), f.app.routes())
	if res.StatusCode != http.StatusForbidden || res.ErrorCode() != apiErrorNotPermitted {
		t.Fatalf("policy admin revoking owner binding = %d %q, want 403 %q", res.StatusCode, res.ErrorCode(), apiErrorNotPermitted)
	}
	res = send(t, newAuthRequest(t, http.MethodDelete, path, nil, f.tokens["owner"]), f.app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("owner revoking a non-last owner binding = %d, want 204", res.StatusCode)
	}
}

func TestPermissionCatalogRoutesCharacterization(t *testing.T) {
	effective := func(query string) func(*accessCharacterizationFixture) string {
		return func(f *accessCharacterizationFixture) string { return f.orgPath() + "/permissions/effective" + query }
	}
	wsEffective := func(f *accessCharacterizationFixture) string {
		return effective("?resource_type=workspace&resource_id=" + strconv.FormatInt(f.workspace.ID, 10))(f)
	}
	routes := []accessRoute{
		{
			name: "list org permissions", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/permissions" },
			member: &accessExpect{status: http.StatusOK},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "list workspace permissions", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.wsPath() + "/permissions" },
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectStatus(http.StatusOK)},
				{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string { return f.foreignWSPath() + "/permissions" }, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "org effective permissions", method: http.MethodGet, path: effective(""),
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				{name: "invalid resource type", who: "owner", path: effective("?resource_type=bogus"), want: expectValidation},
				{name: "org resource id mismatch", who: "owner", path: effective("?resource_type=org&resource_id=999999"), want: expectNotFound},
				{name: "workspace without resource id", who: "owner", path: effective("?resource_type=workspace"), want: expectValidation},
				{name: "non-numeric resource id", who: "owner", path: effective("?resource_type=workspace&resource_id=abc"), want: expectValidation},
				{name: "non-positive resource id", who: "owner", path: effective("?resource_type=workspace&resource_id=0"), want: expectValidation},
				{name: "unknown workspace", who: "owner", path: effective("?resource_type=workspace&resource_id=999999"), want: expectNotFound},
				{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return effective("?resource_type=workspace&resource_id=" + strconv.FormatInt(f.foreignWS.ID, 10))(f)
				}, want: expectNotFound},
				{name: "environment of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return effective("?resource_type=environment&resource_id=" + strconv.FormatInt(f.foreignEnvID, 10))(f)
				}, want: expectNotFound},
				{name: "connection without resource id", who: "owner", path: effective("?resource_type=connection"), want: expectValidation},
				{name: "workspace for org member", who: "member", path: wsEffective, want: expectStatus(http.StatusOK)},
			},
			allow: expectStatus(http.StatusOK),
		},
	}
	runAccessCharacterization(t, newPolicyCharacterizationFixture, routes)
}

func TestWorkspaceRoleRoutesCharacterization(t *testing.T) {
	roles := func(f *accessCharacterizationFixture) string { return f.wsPath() + "/roles" }
	custom := func(ids policyCharacterizationIDs) int64 { return ids.wsCustomRole }
	inUse := func(ids policyCharacterizationIDs) int64 { return ids.wsInUseRole }
	builtin := func(ids policyCharacterizationIDs) int64 { return ids.wsBuiltinRole }
	foreignWS := func(ids policyCharacterizationIDs) int64 { return ids.foreignWSRole }
	orgRole := func(ids policyCharacterizationIDs) int64 { return ids.customRole }
	validRole := map[string]any{"name": "Characterized Workspace Role", "scope_type": "workspace", "permissions": []string{access.PermWsRead}}
	withBody := func(overrides map[string]any) func(*accessCharacterizationFixture) map[string]any {
		return func(*accessCharacterizationFixture) map[string]any {
			body := map[string]any{}
			for k, v := range validRole {
				body[k] = v
			}
			for k, v := range overrides {
				body[k] = v
			}
			return body
		}
	}
	missing := func(f *accessCharacterizationFixture) string { return roles(f) + "/999999" }
	foreignWorkspace := func(suffix string) accessCase {
		return accessCase{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string { return f.foreignWSPath() + suffix }, want: expectNotFound}
	}

	routes := []accessRoute{
		{
			name: "list workspace roles", method: http.MethodGet, path: roles,
			member: &expectNotPermitted,
			extra:  []accessCase{{name: "workspace member", who: "wsMember", want: expectNotPermitted}, foreignWorkspace("/roles")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "get workspace role", method: http.MethodGet, path: wsPolicyPath("/roles", custom),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				{name: "unknown role", who: "owner", path: missing, want: expectNotFound},
				{name: "org role", who: "owner", path: wsPolicyPath("/roles", orgRole), want: expectNotFound},
				{name: "role of another workspace", who: "owner", path: wsPolicyPath("/roles", foreignWS), want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "create workspace role", method: http.MethodPost, path: roles, body: withBody(nil),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				foreignWorkspace("/roles"),
				{name: "missing name", who: "owner", body: withBody(map[string]any{"name": ""}), want: expectValidation},
				{name: "org scope", who: "owner", body: withBody(map[string]any{"scope_type": "org"}), want: expectValidation},
				{name: "connection permission on workspace role", who: "owner", body: withBody(map[string]any{"permissions": []string{access.PermConnExecute}}), want: expectStatus(http.StatusCreated)},
				{name: "duplicate name", who: "owner", body: withBody(map[string]any{"name": "Workspace Admin"}), want: expectValidation},
			},
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "update workspace role", method: http.MethodPatch, path: wsPolicyPath("/roles", custom),
			body:   staticBody(map[string]any{"name": "Renamed Workspace Role", "permissions": []string{access.PermWsRead}}),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				{name: "builtin role", who: "owner", path: wsPolicyPath("/roles", builtin), want: expectNotPermitted},
				{name: "unknown role", who: "owner", path: missing, want: expectNotFound},
				{name: "role of another workspace", who: "owner", path: wsPolicyPath("/roles", foreignWS), want: expectNotFound},
				{name: "missing name", who: "owner", body: staticBody(map[string]any{"name": ""}), want: expectValidation},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "delete workspace role", method: http.MethodDelete, path: wsPolicyPath("/roles", custom),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				{name: "builtin role", who: "owner", path: wsPolicyPath("/roles", builtin), want: expectNotPermitted},
				{name: "unknown role", who: "owner", path: missing, want: expectNotFound},
				{name: "role of another workspace", who: "owner", path: wsPolicyPath("/roles", foreignWS), want: expectNotFound},
				{name: "role in use", who: "owner", path: wsPolicyPath("/roles", inUse), want: accessExpect{http.StatusConflict, apiErrorResourceInUse}},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newPolicyCharacterizationFixture, routes)
}

func TestWorkspacePolicyRoutesCharacterization(t *testing.T) {
	policies := func(f *accessCharacterizationFixture) string { return f.wsPath() + "/policies" }
	binding := func(ids policyCharacterizationIDs) int64 { return ids.wsTargetBinding }
	orgBinding := func(ids policyCharacterizationIDs) int64 { return ids.targetBinding }
	custom := func(ids policyCharacterizationIDs) int64 { return ids.wsCustomRole }
	envRole := func(ids policyCharacterizationIDs) int64 { return ids.envRole }
	orgRole := func(ids policyCharacterizationIDs) int64 { return ids.customRole }
	foreignWS := func(ids policyCharacterizationIDs) int64 { return ids.foreignWSRole }
	grant := func(role func(policyCharacterizationIDs) int64, extra map[string]any) func(*accessCharacterizationFixture) map[string]any {
		return func(f *accessCharacterizationFixture) map[string]any {
			body := map[string]any{"role_id": role(policyIDs(f)), "subject_type": "account", "subject_id": f.target.ID}
			for k, v := range extra {
				body[k] = v
			}
			return body
		}
	}
	foreignWorkspace := func(suffix string) accessCase {
		return accessCase{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string { return f.foreignWSPath() + suffix }, want: expectNotFound}
	}

	routes := []accessRoute{
		{
			name: "list workspace policies", method: http.MethodGet, path: policies,
			member: &expectNotPermitted,
			extra:  []accessCase{{name: "workspace member", who: "wsMember", want: expectNotPermitted}, foreignWorkspace("/policies")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "get workspace policy", method: http.MethodGet, path: wsPolicyPath("/policies", binding),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				{name: "unknown binding", who: "owner", path: func(f *accessCharacterizationFixture) string { return policies(f) + "/999999" }, want: expectNotFound},
				{name: "org binding", who: "owner", path: wsPolicyPath("/policies", orgBinding), want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "grant workspace policy", method: http.MethodPost, path: policies, body: grant(custom, nil),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				foreignWorkspace("/policies"),
				{name: "missing role", who: "owner", body: staticBody(map[string]any{"subject_type": "account", "subject_id": 1}), want: expectValidation},
				{name: "unknown subject", who: "owner", body: grant(custom, map[string]any{"subject_id": 999999}), want: expectNotFound},
				{name: "subject outside org", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return grant(custom, map[string]any{"subject_id": f.nonMember.ID})(f)
				}, want: expectNotFound},
				{name: "org scoped role", who: "owner", body: grant(orgRole, nil), want: expectValidation},
				{name: "role of another workspace", who: "owner", body: grant(foreignWS, nil), want: expectNotFound},
				{name: "environment role on workspace", who: "owner", body: grant(envRole, nil), want: expectValidation},
				{name: "environment of another workspace", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return grant(envRole, map[string]any{"resource_type": "environment", "resource_id": f.foreignEnvID})(f)
				}, want: expectNotFound},
				{name: "environment in workspace", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return grant(envRole, map[string]any{"resource_type": "environment", "resource_id": f.spareEnvID})(f)
				}, want: expectStatus(http.StatusNoContent)},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "revoke workspace policy", method: http.MethodDelete, path: wsPolicyPath("/policies", binding),
			member: &expectNotPermitted,
			extra: []accessCase{
				{name: "workspace member", who: "wsMember", want: expectNotPermitted},
				{name: "unknown binding", who: "owner", path: func(f *accessCharacterizationFixture) string { return policies(f) + "/999999" }, want: expectNotFound},
				{name: "org binding", who: "owner", path: wsPolicyPath("/policies", orgBinding), want: expectNotFound},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newPolicyCharacterizationFixture, routes)
}
