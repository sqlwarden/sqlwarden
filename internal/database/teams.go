package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

type Team struct {
	ID        int64     `bun:",pk,autoincrement" json:"id"`
	OrgID     int64     `bun:",notnull"          json:"org_id"`
	Slug      string    `bun:",notnull"          json:"slug"`
	Name      string    `bun:",notnull"          json:"name"`
	CreatedAt time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt time.Time `bun:",notnull"          json:"updated_at"`
}

type TeamMember struct {
	TeamID    int64     `bun:",pk"      json:"team_id"`
	AccountID int64     `bun:",pk"      json:"account_id"`
	CreatedAt time.Time `bun:",notnull" json:"created_at"`
}

type TeamMemberListItem struct {
	TeamID    int64     `json:"team_id"`
	AccountID int64     `json:"account_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ListTeamMembersParams struct {
	TeamID   int64
	Sort     string
	Order    string
	Page     int
	PageSize int
}

type ListTeamsParams struct {
	OrgID    int64
	Search   string
	Slug     string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

func (db *DB) InsertTeam(ctx context.Context, orgID int64, slug, name string) (Team, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	team := Team{OrgID: orgID, Slug: slug, Name: name, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := db.NewInsert().Model(&team).Returning("id").Exec(ctx)
	if err != nil {
		return Team{}, err
	}
	return team, nil
}

func (db *DB) GetTeam(ctx context.Context, orgID int64, slug string) (Team, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var team Team
	err := db.NewSelect().Model(&team).Where("org_id = ? AND slug = ?", orgID, slug).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, false, nil
	}
	if err != nil {
		return Team{}, false, err
	}
	return team, true, nil
}

func (db *DB) GetTeamByID(ctx context.Context, id int64) (Team, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var team Team
	err := db.NewSelect().Model(&team).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, false, nil
	}
	if err != nil {
		return Team{}, false, err
	}
	return team, true, nil
}

func (db *DB) ListTeamsPage(ctx context.Context, params ListTeamsParams) (response.Paginated[Team], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeTeamListParams(params)

	query := db.NewSelect().Model((*Team)(nil)).Where("org_id = ?", params.OrgID)
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(slug) LIKE ?)", search, search)
	}
	if params.Slug != "" {
		query = query.Where("slug = ?", params.Slug)
	}

	var teams []Team
	err := query.OrderExpr(fmt.Sprintf("%s %s, id %s", teamSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).Scan(ctx, &teams)
	if err != nil {
		return response.Paginated[Team]{}, err
	}
	return response.PaginateItems(teams, params.Page, params.PageSize), nil
}

func (db *DB) DeleteTeam(ctx context.Context, id, orgID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().
			Model((*RoleBinding)(nil)).
			Where("org_id = ? AND subject_type = ? AND subject_id = ?", orgID, "team", id).
			Exec(ctx); err != nil {
			return err
		}

		_, err := tx.NewDelete().Model((*Team)(nil)).Where("id = ? AND org_id = ?", id, orgID).Exec(ctx)
		return err
	})
}

func (db *DB) UpdateTeam(ctx context.Context, id, orgID int64, name string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().Model((*Team)(nil)).
		Set("name = ?", name).
		Set("updated_at = ?", time.Now()).
		Where("id = ? AND org_id = ?", id, orgID).
		Exec(ctx)
	return err
}

func (db *DB) AddTeamMember(ctx context.Context, teamID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	member := TeamMember{TeamID: teamID, AccountID: accountID, CreatedAt: time.Now()}
	_, err := db.NewInsert().Model(&member).Exec(ctx)
	return err
}

func (db *DB) RemoveTeamMember(ctx context.Context, teamID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*TeamMember)(nil)).
		Where("team_id = ? AND account_id = ?", teamID, accountID).Exec(ctx)
	return err
}

func (db *DB) ListTeamMembersPage(ctx context.Context, params ListTeamMembersParams) (response.Paginated[TeamMemberListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeTeamMemberListParams(params)

	var members []TeamMemberListItem
	query := `
SELECT tm.team_id, tm.account_id, a.email, a.name, tm.created_at
FROM team_members AS tm
JOIN accounts AS a ON a.id = tm.account_id
WHERE tm.team_id = ?
ORDER BY ` + teamMemberOrderExpr(params)
	err := db.NewRaw(query, params.TeamID).Scan(ctx, &members)
	if err != nil {
		return response.Paginated[TeamMemberListItem]{}, err
	}
	return response.PaginateItems(members, params.Page, params.PageSize), nil
}

func (db *DB) ListAccountTeamsPage(ctx context.Context, params ListTeamsParams, accountID int64) (response.Paginated[Team], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeTeamListParams(params)

	query := db.NewSelect().Model((*Team)(nil)).
		Join("JOIN team_members AS tm ON tm.team_id = team.id").
		Where("team.org_id = ? AND tm.account_id = ?", params.OrgID, accountID)
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("(LOWER(team.name) LIKE ? OR LOWER(team.slug) LIKE ?)", search, search)
	}
	if params.Slug != "" {
		query = query.Where("team.slug = ?", params.Slug)
	}

	var teams []Team
	err := query.OrderExpr(fmt.Sprintf("team.%s %s, team.id %s", teamSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).Scan(ctx, &teams)
	if err != nil {
		return response.Paginated[Team]{}, err
	}
	return response.PaginateItems(teams, params.Page, params.PageSize), nil
}

func (db *DB) GetAccountTeams(ctx context.Context, orgID, accountID int64) ([]Team, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var teams []Team
	err := db.NewSelect().Model(&teams).
		Join("JOIN team_members AS tm ON tm.team_id = team.id").
		Where("team.org_id = ? AND tm.account_id = ?", orgID, accountID).
		Scan(ctx)
	return teams, err
}

func normalizeTeamListParams(params ListTeamsParams) ListTeamsParams {
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "created_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	params.Slug = strings.TrimSpace(params.Slug)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func teamSortColumn(sort string) string {
	switch sort {
	case "created_at":
		return "created_at"
	default:
		return "name"
	}
}

func normalizeTeamMemberListParams(params ListTeamMembersParams) ListTeamMembersParams {
	switch params.Sort {
	case "account_id", "created_at":
	default:
		params.Sort = "created_at"
	}
	switch params.Order {
	case "desc":
		params.Order = "desc"
	default:
		params.Order = "asc"
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func teamMemberOrderExpr(params ListTeamMembersParams) string {
	switch params.Sort {
	case "account_id":
		return "tm.account_id " + strings.ToUpper(params.Order) + ", tm.created_at " + strings.ToUpper(params.Order)
	default:
		return "tm.created_at " + strings.ToUpper(params.Order) + ", tm.account_id " + strings.ToUpper(params.Order)
	}
}
