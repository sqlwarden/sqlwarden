package database

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
)

// CountRoleBinding returns the number of accounts bound to roleID at the given resource.
func (db *DB) CountRoleBinding(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error) {
	n, err := db.NewSelect().
		TableExpr("role_bindings").
		Where("org_id = ? AND role_id = ? AND resource_type = ? AND resource_id = ? AND subject_type = 'account'", orgID, roleID, resourceType, resourceID).
		Count(ctx)
	return n, err
}

// AccountHasRoleBinding returns true if the account is directly bound to roleID at the given resource.
func (db *DB) AccountHasRoleBinding(ctx context.Context, orgID, roleID, accountID int64, resourceType string, resourceID int64) (bool, error) {
	n, err := db.NewSelect().
		TableExpr("role_bindings").
		Where("org_id = ? AND role_id = ? AND subject_type = 'account' AND subject_id = ? AND resource_type = ? AND resource_id = ?",
			orgID, roleID, accountID, resourceType, resourceID).
		Count(ctx)
	return n > 0, err
}

type RoleBinding struct {
	ID           int64      `bun:",pk,autoincrement" json:"id"`
	OrgID        int64      `bun:",notnull"          json:"org_id"`
	RoleID       int64      `bun:",notnull"          json:"role_id"`
	SubjectType  string     `bun:",notnull"          json:"subject_type"`
	SubjectID    int64      `bun:",notnull"          json:"subject_id"`
	ResourceType string     `bun:",notnull"          json:"resource_type"`
	ResourceID   int64      `bun:",notnull"          json:"resource_id"`
	ExpiresAt    *time.Time `bun:",nullzero"         json:"expires_at,omitempty"`
	CreatedBy    *int64     `bun:",nullzero"         json:"created_by,omitempty"`
	CreatedAt    time.Time  `bun:",notnull"          json:"created_at"`
}

type ListWorkspacePoliciesParams struct {
	OrgID        int64
	WorkspaceID  int64
	Search       string
	SubjectID    int64
	SubjectType  string
	Permission   string
	ResourceID   int64
	ResourceType string
	Sort         string
	Order        string
	Page         int
	PageSize     int
}

type ListOrgPoliciesParams struct {
	OrgID       int64
	Search      string
	SubjectID   int64
	SubjectType string
	Permission  string
	Sort        string
	Order       string
	Page        int
	PageSize    int
}

type WorkspacePolicyListItem struct {
	BindingKind  string    `json:"binding_kind"`
	BindingID    int64     `json:"binding_id"`
	SubjectID    int64     `json:"subject_id"`
	SubjectType  string    `json:"subject_type"`
	SubjectName  string    `json:"subject_name"`
	ResourceID   int64     `json:"resource_id"`
	ResourceType string    `json:"resource_type"`
	ResourceName string    `json:"resource_name"`
	RoleID       int64     `json:"role_id,omitempty"`
	RoleName     string    `json:"role_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type workspacePolicyListRow struct {
	item            WorkspacePolicyListItem
	rolePermissions []string
}

func (db *DB) GetRoleBinding(ctx context.Context, id, orgID int64) (RoleBinding, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var rb RoleBinding
	err := db.NewSelect().Model(&rb).Where("id = ? AND org_id = ?", id, orgID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return RoleBinding{}, false, nil
	}
	if err != nil {
		return RoleBinding{}, false, err
	}
	return rb, true, nil
}

func (db *DB) listOrgPolicies(ctx context.Context, orgID int64) ([]RoleBinding, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var rbs []RoleBinding
	if err := db.NewSelect().Model(&rbs).
		Where("org_id = ? AND resource_type = 'org' AND resource_id = ?", orgID, orgID).
		Scan(ctx); err != nil {
		return nil, err
	}
	return rbs, nil
}

// ListWorkspacePolicies returns all role bindings for resources owned
// by the workspace: the workspace itself, its environments, and its connections.
func (db *DB) listWorkspacePolicies(ctx context.Context, orgID, wsID int64) ([]RoleBinding, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	const where = `org_id = ? AND (
		(resource_type = 'workspace'   AND resource_id = ?) OR
		(resource_type = 'environment' AND resource_id IN (SELECT id FROM environments WHERE workspace_id = ?)) OR
		(resource_type = 'connection'  AND resource_id IN (SELECT id FROM connections  WHERE workspace_id = ?))
	)`

	var rbs []RoleBinding
	if err := db.NewSelect().Model(&rbs).Where(where, orgID, wsID, wsID, wsID).Scan(ctx); err != nil {
		return nil, err
	}
	return rbs, nil
}

func (db *DB) ListWorkspacePoliciesPage(ctx context.Context, params ListWorkspacePoliciesParams) (response.Paginated[WorkspacePolicyListItem], error) {
	params = normalizeWorkspacePolicyParams(params)

	roleBindings, err := db.listWorkspacePolicies(ctx, params.OrgID, params.WorkspaceID)
	if err != nil {
		return response.Paginated[WorkspacePolicyListItem]{}, err
	}

	rows, err := db.policyListItems(ctx, params.OrgID, roleBindings)
	if err != nil {
		return response.Paginated[WorkspacePolicyListItem]{}, err
	}

	filtered := make([]workspacePolicyListRow, 0, len(rows))
	search := strings.ToLower(strings.TrimSpace(params.Search))
	for _, row := range rows {
		item := row.item
		if params.SubjectID > 0 && item.SubjectID != params.SubjectID {
			continue
		}
		if params.SubjectType != "" && item.SubjectType != params.SubjectType {
			continue
		}
		if params.Permission != "" {
			matched := false
			for _, permission := range row.rolePermissions {
				if permission == params.Permission {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if params.ResourceID > 0 && item.ResourceID != params.ResourceID {
			continue
		}
		if params.ResourceType != "" && item.ResourceType != params.ResourceType {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.SubjectName,
				item.ResourceName,
				item.RoleName,
				strings.Join(row.rolePermissions, " "),
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		filtered = append(filtered, row)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareWorkspacePolicyItem(filtered[i].item, filtered[j].item, params.Sort)
		if params.Order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})

	total := len(filtered)
	start := (params.Page - 1) * params.PageSize
	if start > total {
		start = total
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}

	items := make([]WorkspacePolicyListItem, 0, end-start)
	for _, row := range filtered[start:end] {
		items = append(items, row.item)
	}

	return response.Paginated[WorkspacePolicyListItem]{
		Items:    items,
		Page:     params.Page,
		PageSize: params.PageSize,
		Total:    total,
	}, nil
}

func (db *DB) ListOrgPoliciesPage(ctx context.Context, params ListOrgPoliciesParams) (response.Paginated[WorkspacePolicyListItem], error) {
	params = normalizeOrgPolicyParams(params)

	roleBindings, err := db.listOrgPolicies(ctx, params.OrgID)
	if err != nil {
		return response.Paginated[WorkspacePolicyListItem]{}, err
	}

	rows, err := db.policyListItems(ctx, params.OrgID, roleBindings)
	if err != nil {
		return response.Paginated[WorkspacePolicyListItem]{}, err
	}

	filtered := make([]workspacePolicyListRow, 0, len(rows))
	search := strings.ToLower(strings.TrimSpace(params.Search))
	for _, row := range rows {
		item := row.item
		if params.SubjectID > 0 && item.SubjectID != params.SubjectID {
			continue
		}
		if params.SubjectType != "" && item.SubjectType != params.SubjectType {
			continue
		}
		if params.Permission != "" {
			matched := false
			for _, permission := range row.rolePermissions {
				if permission == params.Permission {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.SubjectName,
				item.ResourceName,
				item.RoleName,
				strings.Join(row.rolePermissions, " "),
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		filtered = append(filtered, row)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareWorkspacePolicyItem(filtered[i].item, filtered[j].item, params.Sort)
		if params.Order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})

	total := len(filtered)
	start := (params.Page - 1) * params.PageSize
	if start > total {
		start = total
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}

	items := make([]WorkspacePolicyListItem, 0, end-start)
	for _, row := range filtered[start:end] {
		items = append(items, row.item)
	}

	return response.Paginated[WorkspacePolicyListItem]{
		Items:    items,
		Page:     params.Page,
		PageSize: params.PageSize,
		Total:    total,
	}, nil
}

func (db *DB) policyListItems(ctx context.Context, orgID int64, roleBindings []RoleBinding) ([]workspacePolicyListRow, error) {
	items := make([]workspacePolicyListRow, 0, len(roleBindings))
	for _, rb := range roleBindings {
		item, rolePermissions, err := db.roleBindingListItem(ctx, orgID, rb)
		if err != nil {
			return nil, err
		}
		items = append(items, workspacePolicyListRow{item: item, rolePermissions: rolePermissions})
	}
	return items, nil
}

func (db *DB) roleBindingListItem(ctx context.Context, orgID int64, binding RoleBinding) (WorkspacePolicyListItem, []string, error) {
	item := WorkspacePolicyListItem{
		BindingKind:  "role",
		BindingID:    binding.ID,
		SubjectID:    binding.SubjectID,
		SubjectType:  binding.SubjectType,
		ResourceID:   binding.ResourceID,
		ResourceType: binding.ResourceType,
		RoleID:       binding.RoleID,
		CreatedAt:    binding.CreatedAt,
	}

	role, found, err := db.GetRole(ctx, binding.RoleID, orgID)
	if err != nil {
		return WorkspacePolicyListItem{}, nil, err
	}
	var rolePermissions []string
	if found {
		item.RoleName = role.Name
		rolePermissions = append(rolePermissions, role.Permissions...)
	}

	if err := db.populatePolicyNames(ctx, &item); err != nil {
		return WorkspacePolicyListItem{}, nil, err
	}
	return item, rolePermissions, nil
}

func (db *DB) populatePolicyNames(ctx context.Context, item *WorkspacePolicyListItem) error {
	switch item.SubjectType {
	case "account":
		account, found, err := db.GetAccount(ctx, item.SubjectID)
		if err != nil {
			return err
		}
		if found {
			item.SubjectName = account.Name
		}
	case "team":
		team, found, err := db.GetTeamByID(ctx, item.SubjectID)
		if err != nil {
			return err
		}
		if found {
			item.SubjectName = team.Name
		}
	}

	switch item.ResourceType {
	case "org":
		org, found, err := db.GetOrg(ctx, item.ResourceID)
		if err != nil {
			return err
		}
		if found {
			item.ResourceName = org.Name
		}
	case "workspace":
		ws, found, err := db.GetWorkspace(ctx, item.ResourceID)
		if err != nil {
			return err
		}
		if found {
			item.ResourceName = ws.Name
		}
	case "environment":
		env, found, err := db.GetEnvironment(ctx, item.ResourceID)
		if err != nil {
			return err
		}
		if found {
			item.ResourceName = env.Name
		}
	case "connection":
		conn, found, err := db.GetConnection(ctx, item.ResourceID)
		if err != nil {
			return err
		}
		if found {
			item.ResourceName = conn.Name
		}
	}

	return nil
}

func normalizeOrgPolicyParams(params ListOrgPoliciesParams) ListOrgPoliciesParams {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	if params.Sort == "" {
		params.Sort = "created_at"
	}
	switch params.Sort {
	case "subject_name", "resource_name", "created_at":
	default:
		params.Sort = "created_at"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "desc"
	}
	params.Search = strings.TrimSpace(params.Search)
	if params.SubjectID < 0 {
		params.SubjectID = 0
	}
	params.SubjectType = strings.TrimSpace(params.SubjectType)
	params.Permission = strings.TrimSpace(params.Permission)
	return params
}

func normalizeWorkspacePolicyParams(params ListWorkspacePoliciesParams) ListWorkspacePoliciesParams {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	if params.Sort == "" {
		params.Sort = "created_at"
	}
	switch params.Sort {
	case "subject_name", "resource_name", "created_at":
	default:
		params.Sort = "created_at"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "desc"
	}
	params.Search = strings.TrimSpace(params.Search)
	if params.SubjectID < 0 {
		params.SubjectID = 0
	}
	params.SubjectType = strings.TrimSpace(params.SubjectType)
	params.Permission = strings.TrimSpace(params.Permission)
	if params.ResourceID < 0 {
		params.ResourceID = 0
	}
	params.ResourceType = strings.TrimSpace(params.ResourceType)
	return params
}

func compareWorkspacePolicyItem(left, right WorkspacePolicyListItem, sortBy string) int {
	switch sortBy {
	case "subject_name":
		if left.SubjectName != right.SubjectName {
			return strings.Compare(left.SubjectName, right.SubjectName)
		}
	case "resource_name":
		if left.ResourceName != right.ResourceName {
			return strings.Compare(left.ResourceName, right.ResourceName)
		}
	default:
		if !left.CreatedAt.Equal(right.CreatedAt) {
			if left.CreatedAt.Before(right.CreatedAt) {
				return -1
			}
			return 1
		}
	}
	if left.BindingID < right.BindingID {
		return -1
	}
	if left.BindingID > right.BindingID {
		return 1
	}
	return 0
}
