package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Team struct {
	ID        string    `bun:",pk"      json:"id"`
	TenantID  string    `bun:",notnull" json:"tenant_id"`
	Slug      string    `bun:",notnull" json:"slug"`
	Name      string    `bun:",notnull" json:"name"`
	CreatedAt time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt time.Time `bun:",notnull" json:"updated_at"`
}

type TeamMember struct {
	TeamID    string    `bun:",pk" json:"team_id"`
	AccountID int64     `bun:",pk" json:"account_id"`
	CreatedAt time.Time `bun:",notnull" json:"created_at"`
}

func (db *DB) InsertTeam(tenantID, slug, name string) (Team, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	team := Team{
		ID:        newID(),
		TenantID:  tenantID,
		Slug:      slug,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := db.NewInsert().
		Model(&team).
		Exec(ctx)
	if err != nil {
		return Team{}, err
	}

	return team, nil
}

func (db *DB) GetTeam(id string) (Team, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var team Team
	err := db.NewSelect().
		Model(&team).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, false, nil
	}
	if err != nil {
		return Team{}, false, err
	}

	return team, true, nil
}

func (db *DB) GetTeamBySlug(tenantID, slug string) (Team, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var team Team
	err := db.NewSelect().
		Model(&team).
		Where("tenant_id = ? AND slug = ?", tenantID, slug).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, false, nil
	}
	if err != nil {
		return Team{}, false, err
	}

	return team, true, nil
}

func (db *DB) GetTeamsByTenant(tenantID string) ([]Team, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var teams []Team
	err := db.NewSelect().
		Model(&teams).
		Where("tenant_id = ?", tenantID).
		Order("created_at ASC").
		Scan(ctx)

	return teams, err
}

func (db *DB) DeleteTeam(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*Team)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (db *DB) AddTeamMember(teamID string, accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	member := TeamMember{
		TeamID:    teamID,
		AccountID: accountID,
		CreatedAt: time.Now(),
	}

	_, err := db.NewInsert().
		Model(&member).
		Exec(ctx)

	return err
}

func (db *DB) RemoveTeamMember(teamID string, accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*TeamMember)(nil)).
		Where("team_id = ? AND account_id = ?", teamID, accountID).
		Exec(ctx)

	return err
}

func (db *DB) GetTeamMembers(teamID string) ([]TeamMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var members []TeamMember
	err := db.NewSelect().
		Model(&members).
		Where("team_id = ?", teamID).
		Scan(ctx)

	return members, err
}

func (db *DB) GetAccountTeams(accountID int64, tenantID string) ([]Team, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var teams []Team
	err := db.NewSelect().
		Model(&teams).
		Join("JOIN team_members AS tm ON tm.team_id = team.id").
		Where("tm.account_id = ? AND team.tenant_id = ?", accountID, tenantID).
		Scan(ctx)

	return teams, err
}
