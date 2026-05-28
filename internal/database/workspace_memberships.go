package database

import (
	"context"
	"fmt"
	"sort"
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

type WorkspaceMembershipSource struct {
	Type      string     `json:"type"`
	TeamID    *int64     `json:"team_id,omitempty"`
	TeamSlug  string     `json:"team_slug,omitempty"`
	TeamName  string     `json:"team_name,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

type WorkspaceEffectiveMemberListItem struct {
	WorkspaceID    int64                       `json:"workspace_id"`
	AccountID      int64                       `json:"account_id"`
	Email          string                      `json:"email"`
	Name           string                      `json:"name"`
	IsDirectMember bool                        `json:"is_direct_member"`
	Sources        []WorkspaceMembershipSource `json:"membership_sources"`
	CreatedAt      time.Time                   `json:"created_at"`
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

// IsEffectiveWorkspaceMember reports whether an account belongs to a workspace
// either directly or through a workspace team, constrained to the same org.
func (db *DB) IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var exists bool
	err := db.NewRaw(`
SELECT EXISTS (
    SELECT 1 FROM workspace_members wm
    JOIN workspaces w ON w.id = wm.workspace_id AND w.owner_type = 'org' AND w.owner_id = ?
    JOIN org_members om ON om.org_id = ? AND om.account_id = wm.account_id
    WHERE wm.workspace_id = ? AND wm.account_id = ?
    UNION
    SELECT 1 FROM workspace_teams wt
    JOIN workspaces w ON w.id = wt.workspace_id AND w.owner_type = 'org' AND w.owner_id = ?
    JOIN teams t ON t.id = wt.team_id AND t.org_id = ?
    JOIN team_members tm ON tm.team_id = t.id AND tm.account_id = ?
    JOIN org_members om ON om.org_id = ? AND om.account_id = tm.account_id
    WHERE wt.workspace_id = ?
)`, orgID, orgID, workspaceID, accountID, orgID, orgID, accountID, orgID, workspaceID).Scan(ctx, &exists)
	return exists, err
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

func (db *DB) ListWorkspaceEffectiveMembersPage(ctx context.Context, orgID int64, params ListWorkspaceMembersParams) (response.Paginated[WorkspaceEffectiveMemberListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeWorkspaceMemberListParams(params)

	// Query 1: direct workspace members — all columns non-nullable, no scan issues.
	directQuery := `
SELECT a.id AS account_id, a.email, a.name, wm.created_at
FROM workspace_members AS wm
JOIN accounts AS a ON a.id = wm.account_id
JOIN org_members AS om ON om.account_id = a.id AND om.org_id = ?
WHERE wm.workspace_id = ?`
	directArgs := []any{orgID, params.WorkspaceID}

	// Query 2: team-based members — all columns non-nullable.
	teamQuery := `
SELECT a.id AS account_id, a.email, a.name, t.id AS team_id, t.slug AS team_slug, t.name AS team_name, wt.created_at
FROM workspace_teams AS wt
JOIN teams AS t ON t.id = wt.team_id AND t.org_id = ?
JOIN team_members AS tm ON tm.team_id = t.id
JOIN accounts AS a ON a.id = tm.account_id
JOIN org_members AS om ON om.account_id = a.id AND om.org_id = ?
WHERE wt.workspace_id = ?`
	teamArgs := []any{orgID, orgID, params.WorkspaceID}

	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		directQuery += " AND (LOWER(a.name) LIKE ? OR LOWER(a.email) LIKE ?)"
		directArgs = append(directArgs, search, search)
		teamQuery += " AND (LOWER(a.name) LIKE ? OR LOWER(a.email) LIKE ?)"
		teamArgs = append(teamArgs, search, search)
	}

	var directRows []workspaceDirectMemberRow
	if err := db.NewRaw(directQuery, directArgs...).Scan(ctx, &directRows); err != nil {
		return response.Paginated[WorkspaceEffectiveMemberListItem]{}, err
	}

	var teamRows []workspaceTeamMemberRow
	if err := db.NewRaw(teamQuery, teamArgs...).Scan(ctx, &teamRows); err != nil {
		return response.Paginated[WorkspaceEffectiveMemberListItem]{}, err
	}

	members := aggregateWorkspaceEffectiveMembers(params.WorkspaceID, directRows, teamRows)
	sort.Slice(members, func(i, j int) bool {
		cmp := compareWorkspaceEffectiveMembers(members[i], members[j], params.Sort)
		if params.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

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

type workspaceDirectMemberRow struct {
	AccountID int64     `bun:"account_id"`
	Email     string    `bun:"email"`
	Name      string    `bun:"name"`
	CreatedAt time.Time `bun:"created_at"`
}

type workspaceTeamMemberRow struct {
	AccountID int64     `bun:"account_id"`
	Email     string    `bun:"email"`
	Name      string    `bun:"name"`
	TeamID    int64     `bun:"team_id"`
	TeamSlug  string    `bun:"team_slug"`
	TeamName  string    `bun:"team_name"`
	CreatedAt time.Time `bun:"created_at"`
}

func aggregateWorkspaceEffectiveMembers(workspaceID int64, directRows []workspaceDirectMemberRow, teamRows []workspaceTeamMemberRow) []WorkspaceEffectiveMemberListItem {
	byAccountID := make(map[int64]*WorkspaceEffectiveMemberListItem)

	getOrCreate := func(accountID int64, email, name string) *WorkspaceEffectiveMemberListItem {
		if item, ok := byAccountID[accountID]; ok {
			return item
		}
		item := &WorkspaceEffectiveMemberListItem{
			WorkspaceID: workspaceID,
			AccountID:   accountID,
			Email:       email,
			Name:        name,
		}
		byAccountID[accountID] = item
		return item
	}

	for _, row := range directRows {
		item := getOrCreate(row.AccountID, row.Email, row.Name)
		item.IsDirectMember = true
		item.Sources = append(item.Sources, WorkspaceMembershipSource{
			Type:      "direct",
			CreatedAt: &row.CreatedAt,
		})
		setEffectiveMemberCreatedAt(item, row.CreatedAt)
	}

	for _, row := range teamRows {
		item := getOrCreate(row.AccountID, row.Email, row.Name)
		teamID := row.TeamID
		source := WorkspaceMembershipSource{
			Type:      "team",
			TeamID:    &teamID,
			TeamSlug:  row.TeamSlug,
			TeamName:  row.TeamName,
			CreatedAt: &row.CreatedAt,
		}
		setEffectiveMemberCreatedAt(item, row.CreatedAt)
		item.Sources = append(item.Sources, source)
	}

	members := make([]WorkspaceEffectiveMemberListItem, 0, len(byAccountID))
	for _, item := range byAccountID {
		sort.Slice(item.Sources, func(i, j int) bool {
			if item.Sources[i].Type != item.Sources[j].Type {
				return item.Sources[i].Type == "direct"
			}
			return item.Sources[i].TeamName < item.Sources[j].TeamName
		})
		members = append(members, *item)
	}
	return members
}

func setEffectiveMemberCreatedAt(item *WorkspaceEffectiveMemberListItem, createdAt time.Time) {
	if item.CreatedAt.IsZero() || createdAt.Before(item.CreatedAt) {
		item.CreatedAt = createdAt
	}
}

func compareWorkspaceEffectiveMembers(a, b WorkspaceEffectiveMemberListItem, sortBy string) int {
	var cmp int
	switch sortBy {
	case "email":
		cmp = strings.Compare(strings.ToLower(a.Email), strings.ToLower(b.Email))
	case "created_at":
		if a.CreatedAt.Equal(b.CreatedAt) {
			return compareInt64(a.AccountID, b.AccountID)
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	default:
		cmp = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	if cmp != 0 {
		return cmp
	}
	return compareInt64(a.AccountID, b.AccountID)
}

func compareInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
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
