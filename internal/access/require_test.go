package access

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type fakeDirectory struct {
	orgs             map[int64]Org
	orgMembers       map[[2]int64]bool
	workspaceMembers map[[3]int64]bool
	workspaces       map[int64]Workspace
	connections      map[int64]Connection
	err              error
}

func (d *fakeDirectory) GetOrg(_ context.Context, id int64) (Org, bool, error) {
	v, ok := d.orgs[id]
	return v, ok, d.err
}

func (d *fakeDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return d.orgMembers[[2]int64{orgID, accountID}], d.err
}

func (d *fakeDirectory) IsEffectiveWorkspaceMember(_ context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	return d.workspaceMembers[[3]int64{orgID, workspaceID, accountID}], d.err
}

func (d *fakeDirectory) GetWorkspace(_ context.Context, id int64) (Workspace, bool, error) {
	v, ok := d.workspaces[id]
	return v, ok, d.err
}

func (d *fakeDirectory) GetConnection(_ context.Context, id int64) (Connection, bool, error) {
	v, ok := d.connections[id]
	return v, ok, d.err
}

type fakePolicy struct {
	grants map[string]bool
}

func grantKey(accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) string {
	return fmt.Sprint(accountID, orgID, ownerType, resourceType, resourceID, permission)
}

func (p fakePolicy) Can(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return p.grants[grantKey(accountID, orgID, ownerType, resourceType, resourceID, permission)]
}

func (fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

const (
	reqAccount   int64 = 7
	reqOrg       int64 = 1
	reqWorkspace int64 = 10
	reqEnv       int64 = 20
	reqConn      int64 = 30
)

func newRequireFixture() (*fakeDirectory, fakePolicy) {
	orgID, otherOrg := reqOrg, int64(2)
	dir := &fakeDirectory{
		orgs:             map[int64]Org{reqOrg: {ID: reqOrg, Slug: "acme", Name: "Acme"}},
		orgMembers:       map[[2]int64]bool{{reqOrg, reqAccount}: true},
		workspaceMembers: map[[3]int64]bool{{reqOrg, reqWorkspace, reqAccount}: true},
		workspaces: map[int64]Workspace{
			reqWorkspace: {ID: reqWorkspace, OrgID: &orgID, OwnerType: "org", OwnerID: orgID},
			11:           {ID: 11, OrgID: &orgID, OwnerType: "org", OwnerID: orgID},
			12:           {ID: 12, OrgID: &otherOrg, OwnerType: "org", OwnerID: otherOrg},
			13:           {ID: 13, OwnerType: "account", OwnerID: reqAccount},
			14:           {ID: 14, OrgID: &orgID, OwnerType: "account", OwnerID: reqAccount},
		},
		connections: map[int64]Connection{
			reqConn: {ID: reqConn, WorkspaceID: reqWorkspace, EnvironmentID: reqEnv},
		},
	}
	policy := fakePolicy{grants: map[string]bool{
		grantKey(reqAccount, reqOrg, "org", "workspace", reqWorkspace, PermWsRead): true,
		grantKey(reqAccount, reqOrg, "org", "connection", reqConn, PermConnRead):   true,
		grantKey(reqAccount, reqOrg, "org", "connection", reqConn, PermConnUpdate): true,
	}}
	return dir, policy
}

func accountPrincipal(id int64) Principal {
	return Principal{Subject: SubjectRef{Kind: SubjectAccount, ID: id}}
}

func TestRequireOrg(t *testing.T) {
	tests := []struct {
		name      string
		principal Principal
		orgID     int64
		want      error
	}{
		{"member", accountPrincipal(reqAccount), reqOrg, nil},
		{"non-account subject", Principal{Subject: SubjectRef{Kind: SubjectServiceAccount, ID: reqAccount}}, reqOrg, ErrForbidden},
		{"team subject", Principal{Subject: SubjectRef{Kind: SubjectTeam, ID: reqAccount}}, reqOrg, ErrForbidden},
		{"non-member", accountPrincipal(99), reqOrg, ErrForbidden},
		{"missing org", accountPrincipal(reqAccount), 5, ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, policy := newRequireFixture()
			dir.orgMembers[[2]int64{5, reqAccount}] = true
			org, err := NewRequirer(dir, policy).Org(context.Background(), tt.principal, tt.orgID)
			if tt.name == "missing org" {
				tt.want = ErrNotFound
			}
			if !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Fatalf("Org error = %v, want %v", err, tt.want)
			}
			if tt.want == nil && org.ID != reqOrg {
				t.Fatalf("Org id = %d, want %d", org.ID, reqOrg)
			}
		})
	}
}

func TestRequireWorkspace(t *testing.T) {
	tests := []struct {
		name        string
		principal   Principal
		workspaceID int64
		permission  string
		nilPolicy   bool
		want        error
	}{
		{"permitted", accountPrincipal(reqAccount), reqWorkspace, PermWsRead, false, nil},
		{"empty permission skips policy", accountPrincipal(reqAccount), reqWorkspace, "", true, nil},
		{"non-account subject", Principal{Subject: SubjectRef{Kind: SubjectServiceAccount, ID: reqAccount}}, reqWorkspace, PermWsRead, false, ErrForbidden},
		{"non-member", accountPrincipal(99), reqWorkspace, PermWsRead, false, ErrForbidden},
		{"missing workspace", accountPrincipal(reqAccount), 404, PermWsRead, false, ErrNotFound},
		{"other org workspace", accountPrincipal(reqAccount), 12, PermWsRead, false, ErrNotFound},
		{"workspace without org id", accountPrincipal(reqAccount), 13, PermWsRead, false, ErrNotFound},
		{"non-org-owned workspace", accountPrincipal(reqAccount), 14, PermWsRead, false, ErrNotFound},
		{"permission not held", accountPrincipal(reqAccount), 11, PermWsRead, false, ErrForbidden},
		{"nil policy denies", accountPrincipal(reqAccount), reqWorkspace, PermWsRead, true, ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, policy := newRequireFixture()
			var evaluator PolicyEvaluator = policy
			if tt.nilPolicy {
				evaluator = nil
			}
			ws, err := NewRequirer(dir, evaluator).Workspace(context.Background(), tt.principal, reqOrg, tt.workspaceID, tt.permission)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Workspace error = %v", err)
				}
				if ws.ID != tt.workspaceID || ws.OwnerType != "org" {
					t.Fatalf("Workspace = %+v", ws)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Workspace error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRequireWorkspaceMember(t *testing.T) {
	tests := []struct {
		name        string
		principal   Principal
		workspaceID int64
		want        error
	}{
		{"member", accountPrincipal(reqAccount), reqWorkspace, nil},
		{"not a workspace member", accountPrincipal(reqAccount), 11, ErrForbidden},
		{"non-org-member", accountPrincipal(99), reqWorkspace, ErrForbidden},
		{"non-account subject", Principal{Subject: SubjectRef{Kind: SubjectServiceAccount, ID: reqAccount}}, reqWorkspace, ErrForbidden},
		{"missing workspace", accountPrincipal(reqAccount), 404, ErrNotFound},
		{"other org workspace", accountPrincipal(reqAccount), 12, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := newRequireFixture()
			ws, err := NewRequirer(dir, nil).WorkspaceMember(context.Background(), tt.principal, reqOrg, tt.workspaceID)
			if tt.want == nil {
				if err != nil || ws.ID != tt.workspaceID {
					t.Fatalf("WorkspaceMember = %+v, %v", ws, err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("WorkspaceMember error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRequireConnection(t *testing.T) {
	env, otherEnv := reqEnv, int64(21)
	base := ConnRef{OrgID: reqOrg, WorkspaceID: reqWorkspace, ConnectionID: reqConn}
	with := func(mutate func(*ConnRef)) ConnRef {
		ref := base
		mutate(&ref)
		return ref
	}
	tests := []struct {
		name      string
		principal Principal
		ref       ConnRef
		anyOf     []string
		nilPolicy bool
		want      error
	}{
		{"permitted", accountPrincipal(reqAccount), base, []string{PermConnRead}, false, nil},
		{"matching environment", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.EnvironmentID = &env }), []string{PermConnRead}, false, nil},
		{"multiple anyOf second held", accountPrincipal(reqAccount), base, []string{PermConnDelete, PermConnUpdate}, false, nil},
		{"multiple anyOf none held", accountPrincipal(reqAccount), base, []string{PermConnDelete, PermConnRevealSecret}, false, ErrForbidden},
		{"no permissions listed", accountPrincipal(reqAccount), base, nil, false, ErrForbidden},
		{"non-account subject", Principal{Subject: SubjectRef{Kind: SubjectServiceAccount, ID: reqAccount}}, base, []string{PermConnRead}, false, ErrForbidden},
		{"non-member", accountPrincipal(99), base, []string{PermConnRead}, false, ErrForbidden},
		{"missing workspace", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.WorkspaceID = 404 }), []string{PermConnRead}, false, ErrNotFound},
		{"other org workspace", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.WorkspaceID = 12 }), []string{PermConnRead}, false, ErrNotFound},
		{"non-org-owned workspace", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.WorkspaceID = 14 }), []string{PermConnRead}, false, ErrNotFound},
		{"missing connection", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.ConnectionID = 404 }), []string{PermConnRead}, false, ErrNotFound},
		{"connection in other workspace", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.WorkspaceID = 11 }), []string{PermConnRead}, false, ErrNotFound},
		{"connection in other environment", accountPrincipal(reqAccount), with(func(r *ConnRef) { r.EnvironmentID = &otherEnv }), []string{PermConnRead}, false, ErrNotFound},
		{"nil policy denies", accountPrincipal(reqAccount), base, []string{PermConnRead}, true, ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, policy := newRequireFixture()
			var evaluator PolicyEvaluator = policy
			if tt.nilPolicy {
				evaluator = nil
			}
			conn, ws, err := NewRequirer(dir, evaluator).Connection(context.Background(), tt.principal, tt.ref, tt.anyOf...)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Connection error = %v", err)
				}
				if conn.ID != reqConn || ws.ID != reqWorkspace {
					t.Fatalf("Connection = %+v, %+v", conn, ws)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Connection error = %v, want %v", err, tt.want)
			}
			if conn.ID != 0 || ws.ID != 0 {
				t.Fatalf("expected zero values on error, got %+v, %+v", conn, ws)
			}
		})
	}
}

func TestRequireConnectionMember(t *testing.T) {
	base := ConnRef{OrgID: reqOrg, WorkspaceID: reqWorkspace, ConnectionID: reqConn}
	tests := []struct {
		name      string
		principal Principal
		ref       ConnRef
		want      error
	}{
		{"workspace member needs no connection permission", accountPrincipal(reqAccount), base, nil},
		{"org member without workspace access", accountPrincipal(8), base, ErrForbidden},
		{"non-org-member", accountPrincipal(99), base, ErrForbidden},
		{"non-account subject", Principal{Subject: SubjectRef{Kind: SubjectServiceAccount, ID: reqAccount}}, base, ErrForbidden},
		{"missing connection", accountPrincipal(reqAccount), ConnRef{OrgID: reqOrg, WorkspaceID: reqWorkspace, ConnectionID: 404}, ErrNotFound},
		{"connection in other workspace", accountPrincipal(reqAccount), ConnRef{OrgID: reqOrg, WorkspaceID: 11, ConnectionID: reqConn}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := newRequireFixture()
			dir.orgMembers[[2]int64{reqOrg, 8}] = true
			conn, ws, err := NewRequirer(dir, nil).ConnectionMember(context.Background(), tt.principal, tt.ref)
			if tt.want == nil {
				if err != nil || conn.ID != reqConn || ws.ID != reqWorkspace {
					t.Fatalf("ConnectionMember = %+v, %+v, %v", conn, ws, err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("ConnectionMember error = %v, want %v", err, tt.want)
			}
			if conn.ID != 0 || ws.ID != 0 {
				t.Fatalf("expected zero values on error, got %+v, %+v", conn, ws)
			}
		})
	}
}

func TestRequireResolveConnection(t *testing.T) {
	base := ConnRef{OrgID: reqOrg, WorkspaceID: reqWorkspace, ConnectionID: reqConn}
	tests := []struct {
		name      string
		principal Principal
		ref       ConnRef
		want      error
	}{
		{"org member needs no permission or workspace membership", accountPrincipal(8), base, nil},
		{"non-org-member", accountPrincipal(99), base, ErrForbidden},
		{"non-account subject", Principal{Subject: SubjectRef{Kind: SubjectServiceAccount, ID: reqAccount}}, base, ErrForbidden},
		{"missing connection", accountPrincipal(8), ConnRef{OrgID: reqOrg, WorkspaceID: reqWorkspace, ConnectionID: 404}, ErrNotFound},
		{"connection in other workspace", accountPrincipal(8), ConnRef{OrgID: reqOrg, WorkspaceID: 11, ConnectionID: reqConn}, ErrNotFound},
		{"other org workspace", accountPrincipal(8), ConnRef{OrgID: reqOrg, WorkspaceID: 12, ConnectionID: reqConn}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := newRequireFixture()
			dir.orgMembers[[2]int64{reqOrg, 8}] = true
			conn, ws, err := NewRequirer(dir, nil).ResolveConnection(context.Background(), tt.principal, tt.ref)
			if tt.want == nil {
				if err != nil || conn.ID != reqConn || ws.ID != reqWorkspace {
					t.Fatalf("ResolveConnection = %+v, %+v, %v", conn, ws, err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("ResolveConnection error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRequireCan(t *testing.T) {
	dir, policy := newRequireFixture()
	r := NewRequirer(dir, policy)
	p := accountPrincipal(reqAccount)
	if !r.Can(context.Background(), p, reqOrg, "org", "connection", reqConn, PermConnRead) {
		t.Fatal("Can denied a held permission")
	}
	if r.Can(context.Background(), p, reqOrg, "org", "connection", reqConn, PermConnDelete) {
		t.Fatal("Can granted a permission that is not held")
	}
	if NewRequirer(dir, nil).Can(context.Background(), p, reqOrg, "org", "connection", reqConn, PermConnRead) {
		t.Fatal("Can with nil policy granted a permission")
	}
}

func TestRequireDirectoryErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	dir, policy := newRequireFixture()
	dir.err = boom
	r := NewRequirer(dir, policy)
	if _, err := r.Workspace(context.Background(), accountPrincipal(reqAccount), reqOrg, reqWorkspace, ""); !errors.Is(err, boom) {
		t.Fatalf("Workspace error = %v, want boom", err)
	}
	if _, _, err := r.Connection(context.Background(), accountPrincipal(reqAccount), ConnRef{OrgID: reqOrg, WorkspaceID: reqWorkspace, ConnectionID: reqConn}, PermConnRead); !errors.Is(err, boom) {
		t.Fatalf("Connection error = %v, want boom", err)
	}
}
