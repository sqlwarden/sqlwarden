package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/sqlwarden/internal/execution"
)

func TestMemorySessionDirectoryContract(t *testing.T) {
	directory := execution.NewMemorySessionDirectory()
	ctx := context.Background()
	handle := execution.SessionHandle("session-1")
	now := time.Now()
	record := execution.DirectoryRecord{
		Handle:         handle,
		Scope:          execution.Scope{TenantID: "org-1", AccountID: "account-1", WorkspaceID: "workspace-1", ConnectionID: "connection-1"},
		OwnerRuntimeID: "local", RoutingAddress: "in-process", CreatedAt: now, LastSeenAt: now,
		LeaseExpiresAt: now.Add(time.Minute), ProtocolVersion: execution.ProtocolVersion, RevocationGeneration: 7,
	}
	if err := directory.Put(ctx, record); err != nil {
		t.Fatal(err)
	}
	got, found, err := directory.Get(ctx, handle)
	if err != nil || !found {
		t.Fatalf("Get() = (%+v, %v, %v), want record", got, found, err)
	}
	if got.Scope != record.Scope || got.RevocationGeneration != 7 {
		t.Fatalf("Get() = %+v, want %+v", got, record)
	}

	renewedUntil := now.Add(2 * time.Minute)
	if err := directory.Renew(ctx, handle, renewedUntil); err != nil {
		t.Fatal(err)
	}
	got, found, err = directory.Get(ctx, handle)
	if err != nil || !found || !got.LeaseExpiresAt.Equal(renewedUntil) {
		t.Fatalf("renewed Get() = (%+v, %v, %v)", got, found, err)
	}

	if err := directory.Delete(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if _, found, err := directory.Get(ctx, handle); err != nil || found {
		t.Fatalf("Get() after Delete = (found=%v, err=%v)", found, err)
	}
}

func TestMemorySessionDirectoryDoesNotReturnExpiredLease(t *testing.T) {
	directory := execution.NewMemorySessionDirectory()
	handle := execution.SessionHandle("expired")
	if err := directory.Put(context.Background(), execution.DirectoryRecord{
		Handle: handle, LeaseExpiresAt: time.Now().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := directory.Get(context.Background(), handle); err != nil || found {
		t.Fatalf("Get() = (found=%v, err=%v), want expired record absent", found, err)
	}
}
