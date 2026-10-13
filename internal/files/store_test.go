package files

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/database"
)

var _ Store = (*database.DB)(nil)

type recordingStore struct {
	Store
	inserted []string
}

func (s *recordingStore) InsertWorkspaceFile(ctx context.Context, file *database.WorkspaceFile) error {
	s.inserted = append(s.inserted, file.Name)
	return s.Store.InsertWorkspaceFile(ctx, file)
}

func TestServiceUsesInjectedStore(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject})
	recorder := &recordingStore{Store: f.db}
	service := NewService(Deps{
		DB:       recorder,
		Stores:   singleStoreResolver{store: f.store},
		Enforcer: f.enforcer,
		Requirer: f.requirer,
	}, Config{StorageMode: StorageModeObject}, nil)

	if _, err := service.Create(f.ctx, f.as(f.member), f.privateTree(), CreateInput{Name: "scratch.sql"}); err != nil {
		t.Fatal(err)
	}
	if len(recorder.inserted) != 1 || recorder.inserted[0] != "scratch.sql" {
		t.Fatalf("inserted = %v, want the file written through the injected store", recorder.inserted)
	}
}
