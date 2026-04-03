package database

import (
	"context"
	"testing"
)

func TestInstanceAdminsLifecycle(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			any, err := db.HasAnyInstanceAdmin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if any {
				t.Fatal("expected no instance admins initially")
			}

			count, err := db.CountInstanceAdmins(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("expected 0 instance admins, got %d", count)
			}

			first := testUsers["alice"]
			second := testUsers["bob"]

			if err := db.InsertInstanceAdmin(ctx, first.id); err != nil {
				t.Fatal(err)
			}
			if err := db.InsertInstanceAdmin(ctx, second.id); err != nil {
				t.Fatal(err)
			}
			if err := db.InsertInstanceAdmin(ctx, first.id); err != nil {
				t.Fatal(err)
			}

			isAdmin, err := db.IsInstanceAdmin(ctx, first.id)
			if err != nil {
				t.Fatal(err)
			}
			if !isAdmin {
				t.Fatal("expected seeded account to be an instance admin")
			}

			count, err = db.CountInstanceAdmins(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("expected 2 instance admins, got %d", count)
			}

			any, err = db.HasAnyInstanceAdmin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !any {
				t.Fatal("expected instance admins to exist")
			}

			admins, err := db.ListInstanceAdmins(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(admins) != 2 {
				t.Fatalf("expected 2 admins, got %d", len(admins))
			}
			if admins[0].Account == nil || admins[1].Account == nil {
				t.Fatal("expected related accounts to be loaded")
			}

			if err := db.RemoveInstanceAdmin(ctx, first.id); err != nil {
				t.Fatal(err)
			}

			isAdmin, err = db.IsInstanceAdmin(ctx, first.id)
			if err != nil {
				t.Fatal(err)
			}
			if isAdmin {
				t.Fatal("expected account to be removed from instance admins")
			}
		})
	}
}
