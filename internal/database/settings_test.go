package database

import (
	"testing"

	"github.com/sqlwarden/internal/assert"
)

// seedSettingCount is the number of rows inserted by migration 000009.
const seedSettingCount = 3

func TestGetAllSettings(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Returns seeded settings after migration", func(t *testing.T) {
			db := newTestDB(t, driver)

			// Migration seeds 3 default settings.
			settings, err := db.GetAllSettings()
			assert.Nil(t, err)
			assert.Equal(t, len(settings), seedSettingCount)
			// Verify one known key from the seed.
			_, hasAuthMethod := settings["auth_method"]
			assert.True(t, hasAuthMethod)
		})

		t.Run(driver+": Returns custom settings added on top of seeds", func(t *testing.T) {
			db := newTestDB(t, driver)

			err := db.UpdateSetting("custom_key1", "val1")
			assert.Nil(t, err)
			err = db.UpdateSetting("custom_key2", "val2")
			assert.Nil(t, err)

			settings, err := db.GetAllSettings()
			assert.Nil(t, err)
			assert.Equal(t, len(settings), seedSettingCount+2)
			assert.Equal(t, settings["custom_key1"], "val1")
			assert.Equal(t, settings["custom_key2"], "val2")
		})
	}
}

func TestUpdateSetting(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Inserts a new setting", func(t *testing.T) {
			db := newTestDB(t, driver)

			err := db.UpdateSetting("smtp_host", "mail.example.com")
			assert.Nil(t, err)

			settings, err := db.GetAllSettings()
			assert.Nil(t, err)
			assert.Equal(t, settings["smtp_host"], "mail.example.com")
		})

		t.Run(driver+": Upserts an existing setting", func(t *testing.T) {
			db := newTestDB(t, driver)

			err := db.UpdateSetting("feature_flag", "off")
			assert.Nil(t, err)

			err = db.UpdateSetting("feature_flag", "on")
			assert.Nil(t, err)

			settings, err := db.GetAllSettings()
			assert.Nil(t, err)
			assert.Equal(t, settings["feature_flag"], "on")
			// seedSettingCount seeds + 1 new key = seedSettingCount+1 total
			assert.Equal(t, len(settings), seedSettingCount+1)
		})
	}
}
