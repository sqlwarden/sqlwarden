package rbac_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/rbac"
)

func TestEffectivePermissionsPerResourceKind(t *testing.T) {
	cases := map[string]struct {
		ref      rbac.ResourceRef
		wantType string
		wantID   int64
	}{
		"default org":   {rbac.ResourceRef{}, "org", testOrg},
		"explicit org":  {rbac.ResourceRef{Type: "org", ID: "1"}, "org", testOrg},
		"workspace":     {rbac.ResourceRef{Type: "workspace", ID: "9"}, "workspace", testWS},
		"environment":   {rbac.ResourceRef{Type: "environment", ID: "40"}, "environment", testEnv},
		"connection":    {rbac.ResourceRef{Type: " connection ", ID: " 50 "}, "connection", testConn},
		"sibling space": {rbac.ResourceRef{Type: "workspace", ID: "11"}, "workspace", siblingWS},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			h.policy.set(readerID)
			got, err := h.svc.EffectivePermissions(context.Background(), account(readerID), testOrg, tc.ref)
			if err != nil {
				t.Fatalf("EffectivePermissions: %v", err)
			}
			want := []string{fmt.Sprintf("%d/%d/org/%s/%d", readerID, testOrg, tc.wantType, tc.wantID)}
			if got.ResourceType != tc.wantType || got.ResourceID != tc.wantID || !slices.Equal(got.Permissions, want) {
				t.Fatalf("got %+v, want %s/%d %v", got, tc.wantType, tc.wantID, want)
			}
		})
	}
}

func TestEffectivePermissionsRejectsForeignResources(t *testing.T) {
	cases := map[string]struct {
		ref    rbac.ResourceRef
		reason string
	}{
		"other org":           {rbac.ResourceRef{Type: "org", ID: "2"}, "org_mismatch"},
		"foreign workspace":   {rbac.ResourceRef{Type: "workspace", ID: "12"}, "workspace_not_in_org"},
		"personal workspace":  {rbac.ResourceRef{Type: "workspace", ID: "13"}, "workspace_not_in_org"},
		"missing workspace":   {rbac.ResourceRef{Type: "workspace", ID: "14"}, "workspace_not_in_org"},
		"foreign environment": {rbac.ResourceRef{Type: "environment", ID: "42"}, "environment_workspace_not_in_org"},
		"missing environment": {rbac.ResourceRef{Type: "environment", ID: "999"}, "environment_not_found"},
		"foreign connection":  {rbac.ResourceRef{Type: "connection", ID: "52"}, "connection_workspace_not_in_org"},
		"missing connection":  {rbac.ResourceRef{Type: "connection", ID: "999"}, "connection_not_found"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			_, err := h.svc.EffectivePermissions(context.Background(), account(actorID), testOrg, tc.ref)
			accesstest.AssertNotFound(t, err, name)
			var rejected *rbac.ResourceRejectedError
			if !errors.As(err, &rejected) || rejected.Reason != tc.reason {
				t.Fatalf("error = %#v, want reason %s", err, tc.reason)
			}
		})
	}
}

func TestEffectivePermissionsValidation(t *testing.T) {
	cases := map[string]struct {
		ref   rbac.ResourceRef
		field string
		msg   string
	}{
		"type":         {rbac.ResourceRef{Type: "team"}, "resource_type", "Resource type must be org, workspace, environment, or connection."},
		"missing id":   {rbac.ResourceRef{Type: "workspace"}, "resource_id", "Resource is required."},
		"non-numeric":  {rbac.ResourceRef{Type: "connection", ID: "x"}, "resource_id", "Resource must be a positive integer."},
		"non-positive": {rbac.ResourceRef{Type: "org", ID: "0"}, "resource_id", "Resource must be a positive integer."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			_, err := h.svc.EffectivePermissions(context.Background(), account(actorID), testOrg, tc.ref)
			var validation *rbac.ValidationError
			if !errors.As(err, &validation) || validation.Fields[tc.field] != tc.msg {
				t.Fatalf("error = %v, want %s=%q", err, tc.field, tc.msg)
			}
		})
	}
}

func TestEffectivePermissionsRequiresMembershipBeforeResolving(t *testing.T) {
	h := newWorkspaceHarness(t)
	ctx := context.Background()
	accesstest.AssertForbidden(t, accesstest.Err(h.svc.EffectivePermissions(ctx, account(nonMemberID), testOrg, rbac.ResourceRef{Type: "team"})), "non-member")
	accesstest.AssertForbidden(t, accesstest.Err(h.svc.EffectivePermissions(ctx, account(actorID), otherOrg, rbac.ResourceRef{Type: "workspace", ID: "12"})), "foreign org")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.EffectivePermissions(ctx, account(actorID), missingOrg, rbac.ResourceRef{})), "missing org")
}
