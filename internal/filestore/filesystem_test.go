package filestore

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemPutGet(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	object, err := store.Put(context.Background(), "workspaces/1/private/2/query.sql", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}
	if object.SizeBytes != int64(len("select 1")) || object.ContentHash == "" {
		t.Fatalf("unexpected stored object: %+v", object)
	}

	reader, readObject, err := store.Get(context.Background(), object.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "select 1" || readObject.ContentHash != object.ContentHash {
		t.Fatalf("unexpected stored content: %q %+v", content, readObject)
	}
}

func TestFilesystemRejectsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../outside.sql", "/tmp/outside.sql", "workspaces/../../outside.sql"} {
		if _, err := store.Put(context.Background(), key, strings.NewReader("bad")); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("Put(%q) error = %v, want ErrInvalidKey", key, err)
		}
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "link/escape.sql", strings.NewReader("bad")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("symlink write error = %v, want ErrInvalidKey", err)
	}
}

func TestFilesystemDeleteMissingObjectIsIdempotent(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "objects/missing.txt"); err != nil {
		t.Fatalf("delete missing object: %v", err)
	}
}

func TestFilesystemPrunesOnlyEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "workspace/old/query.sql", strings.NewReader("select 1")); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "workspace/old/query.sql"); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneEmptyDirectories(context.Background(), "workspace/old"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspace", "old")); !os.IsNotExist(err) {
		t.Fatalf("empty directory should be pruned, stat err=%v", err)
	}

	if _, err := store.Put(context.Background(), "workspace/kept/external.txt", strings.NewReader("external")); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneEmptyDirectories(context.Background(), "workspace/kept"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspace", "kept")); err != nil {
		t.Fatalf("non-empty directory should remain: %v", err)
	}
}
