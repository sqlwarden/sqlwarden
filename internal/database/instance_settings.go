package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type InstanceSettings struct {
	ID                    int64     `bun:",pk" json:"-"`
	PersonalSpacesEnabled bool      `bun:",notnull" json:"personal_spaces_enabled"`
	CreatedAt             time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt             time.Time `bun:",notnull" json:"updated_at"`
}

func (db *DB) GetInstanceSettings(ctx context.Context) (InstanceSettings, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var settings InstanceSettings
	err := db.NewSelect().Model(&settings).Where("id = 1").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return InstanceSettings{}, false, nil
	}
	if err != nil {
		return InstanceSettings{}, false, err
	}
	return settings, true, nil
}

func (db *DB) UpsertInstanceSettings(ctx context.Context, personalSpacesEnabled bool) (InstanceSettings, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	settings := InstanceSettings{
		ID:                    1,
		PersonalSpacesEnabled: personalSpacesEnabled,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	_, err := db.NewInsert().
		Model(&settings).
		On("CONFLICT (id) DO UPDATE").
		Set("personal_spaces_enabled = EXCLUDED.personal_spaces_enabled").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return InstanceSettings{}, err
	}

	current, _, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return InstanceSettings{}, err
	}
	return current, nil
}

func (db *DB) ListPersonalConnectionIDs(ctx context.Context) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []int64
	err := db.NewSelect().
		TableExpr("connections c").
		ColumnExpr("c.id").
		Join("JOIN workspaces w ON w.id = c.workspace_id").
		Where("w.owner_type = 'space'").
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
}
