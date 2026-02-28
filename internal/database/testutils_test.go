package database

import (
	"context"
	"fmt"
	"os"
	"strings"
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
	"alice": {email: "alice@example.com", password: "testPass123!", hashedPassword: "$2a$04$mi5gstbTPDRpEawTIitij.rdzLFM.U8.x4U5LLzK8xVFXKXf2ng2u"},
	"bob":   {email: "bob@example.com", password: "mySecure456#", hashedPassword: "$2a$04$AG864hNeosMGVOZKBePuRejH7ElpHfFBBHTFS6/XFJS4beixwXZB."},
}

func newTestDB(t *testing.T, drivers ...string) *DB {
	t.Helper()

	driver := "postgres"

	dsn := "user:pass@localhost:5432/db?sslmode=disable"

	if len(drivers) > 0 {
		driver = drivers[0]
	}

	if driver == "sqlite" {
		dsn = fmt.Sprintf("test_%d.db", time.Now().UnixNano())
	}

	var schemaName string

	if driver == "postgres" {
		schemaName = fmt.Sprintf("test_schema_%d", time.Now().UnixNano())
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn = fmt.Sprintf("%s%ssearch_path=%s", dsn, separator, schemaName)
	}

	db, err := New(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer db.Close()

		switch driver {
		case "postgres":
			_, err = db.ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
			if err != nil {
				t.Error(err)
			}
		case "sqlite":
			os.Remove(dsn)
		}
	})

	if driver == "postgres" {
		_, err = db.ExecContext(context.Background(), fmt.Sprintf("CREATE SCHEMA %s", schemaName))
		if err != nil {
			t.Fatal(err)
		}
	}

	err = db.MigrateUp()
	if err != nil {
		t.Fatal(err)
	}

	for _, user := range testUsers {
		id, err := db.InsertUser(user.email, user.hashedPassword)
		if err != nil {
			t.Fatal(err)
		}

		user.id = int64(id)
	}

	return db
}
