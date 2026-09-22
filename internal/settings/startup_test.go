package settings_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/settings"
)

func TestPrepareAcceptsAValidRow(t *testing.T) {
	t.Parallel()
	store := storeWithInstance(validInstanceSettings())

	if err := settings.Prepare(context.Background(), store, "https://bootstrap.example.com"); err != nil {
		t.Fatal(err)
	}
	if store.initializeCalls != 0 {
		t.Fatalf("initialize calls = %d, want the configured base URL to be left alone", store.initializeCalls)
	}
}

func TestPrepareRejectsMissingRow(t *testing.T) {
	t.Parallel()

	err := settings.Prepare(context.Background(), &fakeStore{}, "https://bootstrap.example.com")
	if err == nil || !strings.Contains(err.Error(), "row id=1 is missing") {
		t.Fatalf("error = %v, want a missing singleton row error", err)
	}
}

func TestPrepareRejectsInvalidRow(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.QueryCursorPageSize = 0

	err := settings.Prepare(context.Background(), storeWithInstance(instance), "https://bootstrap.example.com")
	if err == nil || !strings.Contains(err.Error(), "query_cursor_page_size") {
		t.Fatalf("error = %v, want a cursor page size error", err)
	}
}

func TestPrepareSeedsTheBaseURLOnAnUnconfiguredInstance(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.BaseURL = ""
	store := storeWithInstance(instance)

	if err := settings.Prepare(context.Background(), store, "  https://bootstrap.example.com  "); err != nil {
		t.Fatal(err)
	}
	if store.initializedBaseURL != "https://bootstrap.example.com" {
		t.Fatalf("seeded base URL = %q, want the trimmed bootstrap value", store.initializedBaseURL)
	}
}

func TestPrepareFailsInsteadOfRebootstrappingAConfiguredInstance(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.BaseURL = ""
	store := storeWithInstance(instance)
	store.hasAdmin = true

	err := settings.Prepare(context.Background(), store, "https://bootstrap.example.com")
	if err == nil || !strings.Contains(err.Error(), "base_url is invalid") {
		t.Fatalf("error = %v, want a base URL validation error", err)
	}
	if store.initializeCalls != 0 {
		t.Fatalf("initialize calls = %d, want a configured instance to never be reseeded", store.initializeCalls)
	}
}
