package database

import (
	"context"
	"database/sql"
	"errors"
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

func (db *DB) InsertTeam(orgID int64, slug, name string) (Team, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	team := Team{OrgID: orgID, Slug: slug, Name: name, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := db.NewInsert().Model(&team).Returning("id").Exec(ctx)
	if err != nil {
		return Team{}, err
	}
	return team, nil
}

func (db *DB) GetTeam(orgID int64, slug string) (Team, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
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

func (db *DB) GetTeamByID(id int64) (Team, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
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

func (db *DB) ListTeams(orgID int64) ([]Team, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var teams []Team
	err := db.NewSelect().Model(&teams).Where("org_id = ?", orgID).OrderExpr("name ASC").Scan(ctx)
	return teams, err
}

func (db *DB) DeleteTeam(id, orgID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*Team)(nil)).Where("id = ? AND org_id = ?", id, orgID).Exec(ctx)
	return err
}

func (db *DB) AddTeamMember(teamID, accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	member := TeamMember{TeamID: teamID, AccountID: accountID, CreatedAt: time.Now()}
	_, err := db.NewInsert().Model(&member).Exec(ctx)
	return err
}

func (db *DB) RemoveTeamMember(teamID, accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*TeamMember)(nil)).
		Where("team_id = ? AND account_id = ?", teamID, accountID).Exec(ctx)
	return err
}

func (db *DB) ListTeamMembers(teamID int64) ([]TeamMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var members []TeamMember
	err := db.NewSelect().Model(&members).Where("team_id = ?", teamID).Scan(ctx)
	return members, err
}

func (db *DB) GetAccountTeams(orgID, accountID int64) ([]Team, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var teams []Team
	err := db.NewSelect().Model(&teams).
		Join("JOIN team_members AS tm ON tm.team_id = team.id").
		Where("team.org_id = ? AND tm.account_id = ?", orgID, accountID).
		Scan(ctx)
	return teams, err
}
