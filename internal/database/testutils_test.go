package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"testing"
)

type testUser struct {
	id             int
	email          string
	password       string
	hashedPassword string
}

var testUsers = map[string]*testUser{
	"alice": {email: "alice@example.com", password: "testPass123!", hashedPassword: "$2a$04$mi5gstbTPDRpEawTIitij.rdzLFM.U8.x4U5LLzK8xVFXKXf2ng2u"},
	"bob":   {email: "bob@example.com", password: "mySecure456#", hashedPassword: "$2a$04$AG864hNeosMGVOZKBePuRejH7ElpHfFBBHTFS6/XFJS4beixwXZB."},
}

func newTestDB(t *testing.T) *DB {
	t.Helper()

	dsn := os.Getenv("TEST_DB_DSN")

	if dsn == "" {
		dsn = "user:pass@localhost:5432/db?sslmode=disable"
	}

	schemaName := fmt.Sprintf("test_schema_%d", time.Now().UnixNano())
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn = fmt.Sprintf("%s%ssearch_path=%s", dsn, separator, schemaName)

	db, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer db.Close()

		_, err = db.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
		if err != nil {
			t.Error(err)
		}
	})

	_, err = db.Exec(context.Background(), fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	if err != nil {
		t.Fatal(err)
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

		user.id = id
	}

	return db
}
