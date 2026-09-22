package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/filestore"
)

// FileStores resolves configured workspace-file storage backends by their
// stable backend ID. Content written before a backend was retired still
// references that backend, so every referenced backend must stay configured.
type FileStores struct {
	activeBackendID string
	stores          map[string]filestore.Store
}

// NewFileStores builds every storage backend named in configuration.
func NewFileStores(cfg config.Config) (*FileStores, error) {
	activeBackendID := cfg.Files.ActiveStorageBackend
	if cfg.Files.StorageMode == config.FilesStorageModeFile || strings.TrimSpace(activeBackendID) == "" {
		activeBackendID = database.DefaultFileStorageBackendID
	}
	registry := &FileStores{
		activeBackendID: activeBackendID,
		stores:          make(map[string]filestore.Store, len(cfg.Files.StorageBackends)),
	}
	for id, backend := range cfg.Files.StorageBackends {
		switch backend.Type {
		case config.FilesStorageBackendFilesystem:
			store, err := filestore.NewFilesystem(backend.RootDir)
			if err != nil {
				return nil, fmt.Errorf("backend %q: %w", id, err)
			}
			registry.stores[id] = store
		default:
			return nil, fmt.Errorf("backend %q type %q is not implemented", id, backend.Type)
		}
	}
	return registry, nil
}

// ActiveBackendID returns the backend new content is written to.
func (r *FileStores) ActiveBackendID() string {
	return r.activeBackendID
}

// Store returns the backend for backendID, defaulting to the database default
// backend when backendID is empty.
func (r *FileStores) Store(_ context.Context, backendID string) (filestore.Store, error) {
	if backendID == "" {
		backendID = database.DefaultFileStorageBackendID
	}
	store, ok := r.stores[backendID]
	if !ok {
		return nil, files.ErrStorageBackendUnavailable
	}
	return store, nil
}

// Configured reports whether backendID is available in this process.
func (r *FileStores) Configured(backendID string) bool {
	_, ok := r.stores[backendID]
	return ok
}

// validateReferencedFileStores fails startup when saved file content references
// a backend that is not configured for this process, so reads of that content
// fail at boot rather than at request time.
func validateReferencedFileStores(ctx context.Context, db *database.DB, stores *FileStores) error {
	referenced, err := db.ListWorkspaceFileStorageBackendIDs(ctx)
	if err != nil {
		return err
	}
	for _, backendID := range referenced {
		if !stores.Configured(backendID) {
			return fmt.Errorf("file storage backend %q is referenced by saved file content but is not configured", backendID)
		}
	}
	return nil
}
