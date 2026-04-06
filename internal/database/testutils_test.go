package database

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testUser struct {
	id             int64
	email          string
	password       string
	hashedPassword string
}

var testUsers = map[string]*testUser{
	"alice": {email: "fixture-alice@example.com", password: "testPass123!", hashedPassword: "$2a$04$mi5gstbTPDRpEawTIitij.rdzLFM.U8.x4U5LLzK8xVFXKXf2ng2u"},
	"bob":   {email: "fixture-bob@example.com", password: "mySecure456#", hashedPassword: "$2a$04$AG864hNeosMGVOZKBePuRejH7ElpHfFBBHTFS6/XFJS4beixwXZB."},
}

func newTestDB(t *testing.T, drivers ...string) *DB {
	t.Helper()

	driver := "postgres"

	var dsn string

	if len(drivers) > 0 {
		driver = drivers[0]
	}

	if driver == "sqlite" {
		dsn = filepath.Join(os.TempDir(), fmt.Sprintf("sqlwarden-internal-database-test-%d.db", atomic.AddUint64(&pgTestDBCounter, 1)))
	} else {
		dsn = pgTestDSN
	}

	clonedDB := false
	if driver == "postgres" && dsn == pgTestDSN {
		dbName := fmt.Sprintf("internal_database_test_%d", atomic.AddUint64(&pgTestDBCounter, 1))
		pgTemplateCloneMu.Lock()
		_, err := pgAdminDB.ExecContext(context.Background(),
			fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", dbName, pgTemplateDBName))
		pgTemplateCloneMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		dsn = trimPostgresScheme(dsnWithDatabase("postgres://"+pgAdminDSN, dbName))
		clonedDB = true
	}

	if driver == "sqlite" {
		if err := copyFile(sqliteTemplateDB, dsn); err != nil {
			t.Fatal(err)
		}
	}

	db, err := New(driver, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err != nil {
		t.Fatal(err)
	}

	if driver == "postgres" {
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
	}

	t.Cleanup(func() {
		db.Close()

		switch driver {
		case "postgres":
			if clonedDB {
				dbName := databaseNameFromDSN("postgres://" + dsn)
				pgTemplateCloneMu.Lock()
				defer pgTemplateCloneMu.Unlock()
				_, _ = pgAdminDB.ExecContext(context.Background(),
					"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
					dbName)
				_, err = pgAdminDB.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
				if err != nil {
					t.Error(err)
				}
			}
		case "sqlite":
			os.Remove(dsn)
		}
	})

	if driver == "postgres" && !clonedDB {
		err = db.MigrateUp()
		if err != nil {
			t.Fatal(err)
		}

		for _, user := range testUsers {
			account, err := db.InsertAccount(context.Background(), user.email, user.email, &user.hashedPassword)
			if err != nil {
				t.Fatal(err)
			}
			user.id = account.ID
		}
	}

	return db
}

func databaseNameFromDSN(connStr string) string {
	idx := strings.LastIndex(connStr, "/")
	if idx == -1 {
		return ""
	}
	rest := connStr[idx+1:]
	if q := strings.Index(rest, "?"); q != -1 {
		return rest[:q]
	}
	return rest
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

func testDrivers() []string {
	return []string{"postgres", "sqlite"}
}

func insertTestRole(t *testing.T, db *DB, orgID int64, workspaceID *int64, name, scopeType string, isBuiltin bool, permissions ...string) Role {
	t.Helper()

	role := Role{
		OrgID:       orgID,
		WorkspaceID: workspaceID,
		Name:        name,
		Description: name + " description",
		ScopeType:   scopeType,
		IsBuiltin:   isBuiltin,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if _, err := db.NewInsert().Model(&role).Returning("id").Exec(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, permission := range permissions {
		row := map[string]any{
			"role_id":    role.ID,
			"permission": permission,
		}
		if _, err := db.NewInsert().TableExpr("role_permissions").Model(&row).Exec(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	return role
}

func insertTestRoleBinding(t *testing.T, db *DB, orgID, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID int64) RoleBinding {
	t.Helper()

	binding := RoleBinding{
		OrgID:        orgID,
		RoleID:       roleID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		CreatedAt:    time.Now(),
	}
	if _, err := db.NewInsert().Model(&binding).Returning("id").Exec(context.Background()); err != nil {
		t.Fatal(err)
	}

	return binding
}

func insertTestScopedRoleBinding(t *testing.T, db *DB, orgID int64, workspaceID *int64, roleName, scopeType string, permissions []string, subjectType string, subjectID int64, resourceType string, resourceID int64) RoleBinding {
	t.Helper()

	role := insertTestRole(t, db, orgID, workspaceID, roleName, scopeType, false, permissions...)
	return insertTestRoleBinding(t, db, orgID, role.ID, subjectType, subjectID, resourceType, resourceID)
}
