package database

import (
	"context"
	"errors"
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

			admins, err := db.ListInstanceAdminsPage(ctx, ListInstanceAdminsParams{
				Sort:     "created_at",
				Order:    "asc",
				Page:     1,
				PageSize: 10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if admins.Total != 2 {
				t.Fatalf("expected 2 admins total, got %d", admins.Total)
			}
			if len(admins.Items) != 2 {
				t.Fatalf("expected 2 admins on first page, got %d", len(admins.Items))
			}
			if admins.Items[0].Account == nil || admins.Items[1].Account == nil {
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

func TestRemoveInstanceAdminRejectsLastAdmin(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			admin := testUsers["alice"]
			if err := db.InsertInstanceAdmin(ctx, admin.id); err != nil {
				t.Fatal(err)
			}

			err := db.RemoveInstanceAdmin(ctx, admin.id)
			if !errors.Is(err, ErrLastInstanceAdmin) {
				t.Fatalf("expected ErrLastInstanceAdmin, got %v", err)
			}

			isAdmin, err := db.IsInstanceAdmin(ctx, admin.id)
			if err != nil {
				t.Fatal(err)
			}
			if !isAdmin {
				t.Fatal("expected last admin to remain")
			}
		})
	}
}

func TestListInstanceAdminsPage_SupportsPaginationAndSort(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			if err := db.InsertInstanceAdmin(ctx, testUsers["alice"].id); err != nil {
				t.Fatal(err)
			}
			if err := db.InsertInstanceAdmin(ctx, testUsers["bob"].id); err != nil {
				t.Fatal(err)
			}

			result, err := db.ListInstanceAdminsPage(ctx, ListInstanceAdminsParams{
				Sort:     "account_id",
				Order:    "desc",
				Page:     1,
				PageSize: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 2 {
				t.Fatalf("expected total=2, got %d", result.Total)
			}
			if len(result.Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(result.Items))
			}
			want := testUsers["alice"].id
			if testUsers["bob"].id > want {
				want = testUsers["bob"].id
			}
			if result.Items[0].AccountID != want {
				t.Fatalf("expected highest account id first, got %d", result.Items[0].AccountID)
			}
		})
	}
}
