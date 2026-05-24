package database

import (
	"context"
	"testing"
)

func TestInstanceSettingsUpsert(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()

			db := newTestDB(t, driver)

			_, found, err := db.GetInstanceSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if found {
				t.Fatal("expected no instance settings row initially")
			}

			settings, err := db.UpsertInstanceSettings(context.Background(), InstanceSettings{
				InstanceName:          "Demo Instance",
				InstanceDescription:   "A shared SQLWarden instance.",
				SupportEmail:          "support@example.com",
				PublicURL:             "https://sqlwarden.example.com",
				PersonalSpacesEnabled: false,
			})
			if err != nil {
				t.Fatal(err)
			}
			if settings.PersonalSpacesEnabled {
				t.Fatal("expected personal spaces to be disabled")
			}
			if settings.InstanceName != "Demo Instance" {
				t.Fatalf("expected instance name to persist, got %q", settings.InstanceName)
			}
			if settings.InstanceDescription != "A shared SQLWarden instance." {
				t.Fatalf("expected instance description to persist, got %q", settings.InstanceDescription)
			}
			if settings.SupportEmail != "support@example.com" {
				t.Fatalf("expected support email to persist, got %q", settings.SupportEmail)
			}
			if settings.PublicURL != "https://sqlwarden.example.com" {
				t.Fatalf("expected public url to persist, got %q", settings.PublicURL)
			}

			settings, found, err = db.GetInstanceSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !found || settings.PersonalSpacesEnabled {
				t.Fatalf("expected persisted disabled settings, got %+v found=%v", settings, found)
			}

			settings, err = db.UpsertInstanceSettings(context.Background(), InstanceSettings{
				InstanceName:          "Updated Instance",
				InstanceDescription:   "",
				SupportEmail:          "",
				PublicURL:             "https://updated.example.com",
				PersonalSpacesEnabled: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !settings.PersonalSpacesEnabled {
				t.Fatal("expected personal spaces to be enabled")
			}
			if settings.InstanceName != "Updated Instance" {
				t.Fatalf("expected updated instance name, got %q", settings.InstanceName)
			}
		})
	}
}

func TestListPersonalConnectionIDs(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()

			db := newTestDB(t, driver)
			ownerID := newAccount(t, db, "owner@example.com")
			org, err := db.InsertOrg(context.Background(), "org-personal-ids", "Org")
			if err != nil {
				t.Fatal(err)
			}

			orgWS, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Org WS", "")
			if err != nil {
				t.Fatal(err)
			}
			spaceWS, err := db.InsertWorkspace(context.Background(), nil, "space", ownerID, "Personal WS", "")
			if err != nil {
				t.Fatal(err)
			}

			orgDefault, err := db.DefaultEnvironmentID(context.Background(), orgWS.ID)
			if err != nil {
				t.Fatal(err)
			}
			spaceDefault, err := db.DefaultEnvironmentID(context.Background(), spaceWS.ID)
			if err != nil {
				t.Fatal(err)
			}

			orgConn, err := db.InsertConnection(context.Background(), orgWS.ID, &orgDefault, "org-db", "sqlite", "enc", "open")
			if err != nil {
				t.Fatal(err)
			}
			spaceConn, err := db.InsertConnection(context.Background(), spaceWS.ID, &spaceDefault, "space-db", "sqlite", "enc", "open")
			if err != nil {
				t.Fatal(err)
			}

			ids, err := db.ListPersonalConnectionIDs(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if containsInt64(ids, orgConn.ID) {
				t.Fatalf("org connection %d should not be returned in personal ids", orgConn.ID)
			}
			if !containsInt64(ids, spaceConn.ID) {
				t.Fatalf("expected personal connection %d in ids %v", spaceConn.ID, ids)
			}
		})
	}
}

func containsInt64(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
