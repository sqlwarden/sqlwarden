package database

import "testing"

func TestNewID(t *testing.T) {
	first := NewID()
	second := NewID()

	if first == "" || second == "" {
		t.Fatal("expected generated IDs to be non-empty")
	}
	if first == second {
		t.Fatal("expected generated IDs to be unique")
	}
}
