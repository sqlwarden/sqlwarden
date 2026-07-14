package filestore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var ErrInvalidKey = errors.New("invalid storage key")

type StoredObject struct {
	Key          string
	SizeBytes    int64
	ContentHash  string
	ModifiedTime time.Time
}

type Store interface {
	Put(context.Context, string, io.Reader) (StoredObject, error)
	Get(context.Context, string) (io.ReadCloser, StoredObject, error)
	Delete(context.Context, string) error
}

// EmptyDirectoryPruner is implemented by directory-backed stores whose visible
// layout should not retain empty paths after tracked objects move or are removed.
type EmptyDirectoryPruner interface {
	PruneEmptyDirectories(context.Context, string) error
}

type Filesystem struct {
	root string
}

func NewFilesystem(root string) (*Filesystem, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("filesystem root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve filesystem root: %w", err)
	}
	if err = os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create filesystem root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("evaluate filesystem root: %w", err)
	}
	return &Filesystem{root: root}, nil
}

func (s *Filesystem) Root() string {
	return s.root
}

func (s *Filesystem) Put(_ context.Context, key string, content io.Reader) (StoredObject, error) {
	clean, parts, err := validKey(key)
	if err != nil {
		return StoredObject{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return StoredObject{}, err
	}
	defer root.Close()
	if err = ensureWritePath(root, clean, parts); err != nil {
		return StoredObject{}, err
	}

	temp, tempName, err := createRootTemp(root, filepath.Dir(clean))
	if err != nil {
		return StoredObject{}, err
	}
	defer root.Remove(tempName)

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temp, hash), content)
	if copyErr != nil {
		temp.Close()
		return StoredObject{}, copyErr
	}
	if err = temp.Chmod(0o640); err != nil {
		temp.Close()
		return StoredObject{}, err
	}
	closeErr := temp.Close()
	if closeErr != nil {
		return StoredObject{}, closeErr
	}
	if err = root.Rename(tempName, clean); err != nil {
		return StoredObject{}, err
	}

	info, err := root.Stat(clean)
	if err != nil {
		return StoredObject{}, err
	}
	return StoredObject{
		Key:          key,
		SizeBytes:    size,
		ContentHash:  hex.EncodeToString(hash.Sum(nil)),
		ModifiedTime: info.ModTime(),
	}, nil
}

func (s *Filesystem) Get(_ context.Context, key string) (io.ReadCloser, StoredObject, error) {
	clean, parts, err := validKey(key)
	if err != nil {
		return nil, StoredObject{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, StoredObject{}, err
	}
	defer root.Close()
	if err = rejectSymlinkComponents(root, parts); err != nil {
		return nil, StoredObject{}, err
	}
	file, err := root.Open(clean)
	if err != nil {
		return nil, StoredObject{}, err
	}

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		file.Close()
		return nil, StoredObject{}, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, StoredObject{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, StoredObject{}, err
	}
	return file, StoredObject{
		Key:          key,
		SizeBytes:    size,
		ContentHash:  hex.EncodeToString(hash.Sum(nil)),
		ModifiedTime: info.ModTime(),
	}, nil
}

func (s *Filesystem) Delete(_ context.Context, key string) error {
	clean, parts, err := validKey(key)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = rejectSymlinkComponents(root, parts); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	err = root.Remove(clean)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Filesystem) PruneEmptyDirectories(_ context.Context, key string) error {
	dir, _, err := validKey(key)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	for dir != "." {
		err = root.Remove(dir)
		switch {
		case err == nil, errors.Is(err, os.ErrNotExist):
			dir = filepath.Dir(dir)
		case errors.Is(err, syscall.ENOTEMPTY), errors.Is(err, syscall.EEXIST):
			return nil
		default:
			return err
		}
	}
	return nil
}

func rejectSymlinkComponents(root *os.Root, parts []string) error {
	current := ""
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidKey
		}
	}
	return nil
}

func ensureWritePath(root *os.Root, clean string, parts []string) error {
	current := ""
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = root.Mkdir(current, 0o750); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrInvalidKey
		}
	}
	if info, err := root.Lstat(clean); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalidKey
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func validKey(key string) (string, []string, error) {
	if key == "" || filepath.IsAbs(key) {
		return "", nil, ErrInvalidKey
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", nil, ErrInvalidKey
	}
	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", nil, ErrInvalidKey
		}
	}
	return clean, parts, nil
}

func createRootTemp(root *os.Root, dir string) (*os.File, string, error) {
	for range 100 {
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			return nil, "", err
		}
		name := filepath.Join(dir, ".sqlwarden-write-"+hex.EncodeToString(random))
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, name, err
	}
	return nil, "", errors.New("create temporary file: exhausted attempts")
}
