package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	path, err := s.pathForWrite(key)
	if err != nil {
		return StoredObject{}, err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".sqlwarden-write-*")
	if err != nil {
		return StoredObject{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temp, hash), content)
	closeErr := temp.Close()
	if copyErr != nil {
		return StoredObject{}, copyErr
	}
	if closeErr != nil {
		return StoredObject{}, closeErr
	}
	if err = os.Chmod(tempPath, 0o640); err != nil {
		return StoredObject{}, err
	}
	if err = os.Rename(tempPath, path); err != nil {
		return StoredObject{}, err
	}

	info, err := os.Stat(path)
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
	path, err := s.pathForRead(key)
	if err != nil {
		return nil, StoredObject{}, err
	}
	file, err := os.Open(path)
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
	path, err := s.pathForRead(key)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *Filesystem) pathForRead(key string) (string, error) {
	path, parts, err := s.validPath(key)
	if err != nil {
		return "", err
	}
	current := s.root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrInvalidKey
		}
	}
	return path, nil
}

func (s *Filesystem) pathForWrite(key string) (string, error) {
	path, parts, err := s.validPath(key)
	if err != nil {
		return "", err
	}
	current := s.root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(current, 0o750); err != nil {
				return "", err
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", ErrInvalidKey
		}
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalidKey
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func (s *Filesystem) validPath(key string) (string, []string, error) {
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
	path := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, ErrInvalidKey
	}
	return path, parts, nil
}
