package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
)

type WorkspaceMember struct {
	WorkspaceID int64     `bun:",pk"      json:"workspace_id"`
	AccountID   int64     `bun:",pk"      json:"account_id"`
	CreatedBy   *int64    `bun:",nullzero" json:"created_by,omitempty"`
	CreatedAt   time.Time `bun:",notnull" json:"created_at"`
}

type WorkspaceTeam struct {
	WorkspaceID int64     `bun:",pk"      json:"workspace_id"`
	TeamID      int64     `bun:",pk"      json:"team_id"`
	CreatedBy   *int64    `bun:",nullzero" json:"created_by,omitempty"`
	CreatedAt   time.Time `bun:",notnull" json:"created_at"`
}

type WorkspaceMemberListItem struct {
	WorkspaceID int64     `json:"workspace_id"`
	AccountID   int64     `json:"account_id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type WorkspaceTeamListItem struct {
	WorkspaceID int64     `json:"workspace_id"`
	TeamID      int64     `json:"team_id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type ListWorkspaceMembersParams struct {
	WorkspaceID int64
	Search      string
	Sort        string
	Order       string
	Page        int
	PageSize    int
}

type ListWorkspaceTeamsParams struct {
	WorkspaceID int64
	Search      string
	Sort        string
	Order       string
	Page        int
	PageSize    int
}

func (db *DB) AddWorkspaceMember(ctx context.Context, workspaceID, accountID int64, createdBy *int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	member := WorkspaceMember{WorkspaceID: workspaceID, AccountID: accountID, CreatedBy: createdBy, CreatedAt: time.Now()}
	_, err := db.NewInsert().Model(&member).Ignore().Exec(ctx)
	return err
}

func (db *DB) RemoveWorkspaceMember(ctx context.Context, workspaceID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*WorkspaceMember)(nil)).
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
		Exec(ctx)
	return err
}

func (db *DB) AddWorkspaceTeam(ctx context.Context, workspaceID, teamID int64, createdBy *int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	team := WorkspaceTeam{WorkspaceID: workspaceID, TeamID: teamID, CreatedBy: createdBy, CreatedAt: time.Now()}
	_, err := db.NewInsert().Model(&team).Ignore().Exec(ctx)
	return err
}

func (db *DB) RemoveWorkspaceTeam(ctx context.Context, workspaceID, teamID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*WorkspaceTeam)(nil)).
		Where("workspace_id = ? AND team_id = ?", workspaceID, teamID).
		Exec(ctx)
	return err
}

func (db *DB) ListWorkspaceMembersPage(ctx context.Context, params ListWorkspaceMembersParams) (response.Paginated[WorkspaceMemberListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeWorkspaceMemberListParams(params)

	query := `
SELECT wm.workspace_id, wm.account_id, a.email, a.name, wm.created_by, wm.created_at
FROM workspace_members AS wm
JOIN accounts AS a ON a.id = wm.account_id
WHERE wm.workspace_id = ?`
	args := []any{params.WorkspaceID}
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query += " AND (LOWER(a.name) LIKE ? OR LOWER(a.email) LIKE ?)"
		args = append(args, search, search)
	}
	query += fmt.Sprintf(" ORDER BY %s %s, wm.account_id %s", workspaceMemberSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))

	var members []WorkspaceMemberListItem
	if err := db.NewRaw(query, args...).Scan(ctx, &members); err != nil {
		return response.Paginated[WorkspaceMemberListItem]{}, err
	}
	return response.PaginateItems(members, params.Page, params.PageSize), nil
}

func (db *DB) ListWorkspaceTeamsPage(ctx context.Context, params ListWorkspaceTeamsParams) (response.Paginated[WorkspaceTeamListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeWorkspaceTeamListParams(params)

	query := `
SELECT
	wt.workspace_id,
	wt.team_id,
	t.slug,
	t.name,
	(SELECT COUNT(*) FROM team_members AS tm WHERE tm.team_id = t.id) AS member_count,
	wt.created_by,
	wt.created_at
FROM workspace_teams AS wt
JOIN teams AS t ON t.id = wt.team_id
WHERE wt.workspace_id = ?`
	args := []any{params.WorkspaceID}
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query += " AND (LOWER(t.name) LIKE ? OR LOWER(t.slug) LIKE ?)"
		args = append(args, search, search)
	}
	query += fmt.Sprintf(" ORDER BY %s %s, wt.team_id %s", workspaceTeamSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))

	var teams []WorkspaceTeamListItem
	if err := db.NewRaw(query, args...).Scan(ctx, &teams); err != nil {
		return response.Paginated[WorkspaceTeamListItem]{}, err
	}
	return response.PaginateItems(teams, params.Page, params.PageSize), nil
}

func (db *DB) ListWorkspaceTeamMemberAccountIDs(ctx context.Context, workspaceID, teamID int64) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []int64
	err := db.NewSelect().
		TableExpr("team_members").
		ColumnExpr("account_id").
		Where("team_id = ?", teamID).
		Scan(ctx, &ids)
	return ids, err
}

func (db *DB) DeleteWorkspaceMembershipsForAccount(ctx context.Context, orgID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*WorkspaceMember)(nil)).
		Where("account_id = ?", accountID).
		Where("workspace_id IN (SELECT id FROM workspaces WHERE owner_type = 'org' AND owner_id = ?)", orgID).
		Exec(ctx)
	return err
}

func normalizeWorkspaceMemberListParams(params ListWorkspaceMembersParams) ListWorkspaceMembersParams {
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "email", "created_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func normalizeWorkspaceTeamListParams(params ListWorkspaceTeamsParams) ListWorkspaceTeamsParams {
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "slug", "created_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func workspaceMemberSortColumn(sort string) string {
	switch sort {
	case "email":
		return "a.email"
	case "created_at":
		return "wm.created_at"
	default:
		return "a.name"
	}
}

func workspaceTeamSortColumn(sort string) string {
	switch sort {
	case "slug":
		return "t.slug"
	case "created_at":
		return "wt.created_at"
	default:
		return "t.name"
	}
}
