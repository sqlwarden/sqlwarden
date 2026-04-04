package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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

type ListTeamsParams struct {
	OrgID  int64
	Search string
	Sort   string
	Order  string
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

func (db *DB) ListTeams(ctx context.Context, orgID int64) ([]Team, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var teams []Team
	err := db.NewSelect().Model(&teams).Where("org_id = ?", orgID).OrderExpr("name ASC").Scan(ctx)
	return teams, err
}

func (db *DB) ListTeamsFiltered(ctx context.Context, params ListTeamsParams) ([]Team, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeTeamListParams(params)

	query := db.NewSelect().Model((*Team)(nil)).Where("org_id = ?", params.OrgID)
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(slug) LIKE ?)", search, search)
	}

	var teams []Team
	err := query.OrderExpr(fmt.Sprintf("%s %s, id %s", teamSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).Scan(ctx, &teams)
	return teams, err
}

func (db *DB) DeleteTeam(ctx context.Context, id, orgID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*Team)(nil)).Where("id = ? AND org_id = ?", id, orgID).Exec(ctx)
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

func (db *DB) ListTeamMembers(ctx context.Context, teamID int64) ([]TeamMember, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var members []TeamMember
	err := db.NewSelect().Model(&members).Where("team_id = ?", teamID).Scan(ctx)
	return members, err
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
