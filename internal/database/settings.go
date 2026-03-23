package database

import (
	"context"
	"time"
)

type InstanceSetting struct {
	Key       string    `bun:"key,pk"        json:"key"`
	Value     string    `bun:"value,notnull" json:"value"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

func (db *DB) GetAllSettings() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var settings []InstanceSetting
	if err := db.NewSelect().Model(&settings).Scan(ctx); err != nil {
		return nil, err
	}
	m := make(map[string]string, len(settings))
	for _, s := range settings {
		m[s.Key] = s.Value
	}
	return m, nil
}

func (db *DB) UpdateSetting(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	setting := &InstanceSetting{Key: key, Value: value, UpdatedAt: time.Now()}
	_, err := db.NewInsert().
		Model(setting).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
