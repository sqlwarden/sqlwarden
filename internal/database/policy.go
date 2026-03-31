package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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

type PermissionBinding struct {
	ID           int64      `bun:",pk,autoincrement" json:"id"`
	OrgID        int64      `bun:",notnull"          json:"org_id"`
	Permission   string     `bun:",notnull"          json:"permission"`
	SubjectType  string     `bun:",notnull"          json:"subject_type"`
	SubjectID    int64      `bun:",notnull"          json:"subject_id"`
	ResourceType string     `bun:",notnull"          json:"resource_type"`
	ResourceID   int64      `bun:",notnull"          json:"resource_id"`
	ExpiresAt    *time.Time `bun:",nullzero"         json:"expires_at,omitempty"`
	CreatedBy    *int64     `bun:",nullzero"         json:"created_by,omitempty"`
	CreatedAt    time.Time  `bun:",notnull"          json:"created_at"`
}

func (db *DB) GetRoleBinding(id, orgID int64) (RoleBinding, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
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

func (db *DB) ListRoleBindings(orgID int64, resourceType string, resourceID int64) ([]RoleBinding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var rbs []RoleBinding
	err := db.NewSelect().Model(&rbs).
		Where("org_id = ? AND resource_type = ? AND resource_id = ?", orgID, resourceType, resourceID).
		Scan(ctx)
	return rbs, err
}

func (db *DB) GetPermissionBinding(id, orgID int64) (PermissionBinding, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var pb PermissionBinding
	err := db.NewSelect().Model(&pb).Where("id = ? AND org_id = ?", id, orgID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return PermissionBinding{}, false, nil
	}
	if err != nil {
		return PermissionBinding{}, false, err
	}
	return pb, true, nil
}

func (db *DB) ListPermissionBindings(orgID int64, resourceType string, resourceID int64) ([]PermissionBinding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var pbs []PermissionBinding
	err := db.NewSelect().Model(&pbs).
		Where("org_id = ? AND resource_type = ? AND resource_id = ?", orgID, resourceType, resourceID).
		Scan(ctx)
	return pbs, err
}
