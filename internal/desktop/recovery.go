package desktop

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	backupFormatVersion = 1
	maxRestoreBytes     = int64(4 << 30)
)

type backupManifest struct {
	FormatVersion int               `json:"format_version"`
	AppVersion    string            `json:"app_version"`
	CreatedAt     string            `json:"created_at"`
	Files         map[string]string `json:"files"`
}

type restoreMarker struct {
	Archive string `json:"archive"`
}

func CreateBackupArchive(paths Paths, databaseSnapshot, destination, appVersion string) error {
	entries := map[string]string{"sqlwarden.db": databaseSnapshot}
	err := filepath.WalkDir(paths.Files, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace file %q is a symbolic link", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workspace file %q is not a regular file", path)
		}
		relative, err := filepath.Rel(paths.Files, path)
		if err != nil {
			return err
		}
		entries[filepath.ToSlash(filepath.Join("files", relative))] = path
		return nil
	})
	if err != nil {
		return fmt.Errorf("enumerate backup files: %w", err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".sqlwarden-backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	archive := zip.NewWriter(temporary)
	closed := false
	defer func() {
		if !closed {
			_ = archive.Close()
			_ = temporary.Close()
		}
	}()
	manifest := backupManifest{
		FormatVersion: backupFormatVersion,
		AppVersion:    appVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Files:         make(map[string]string, len(entries)),
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hash, err := addBackupFile(archive, name, entries[name])
		if err != nil {
			_ = archive.Close()
			_ = temporary.Close()
			return err
		}
		manifest.Files[name] = hash
	}
	manifestContents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestWriter, err := archive.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err := manifestWriter.Write(append(manifestContents, '\n')); err != nil {
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, destination); err != nil {
		return err
	}
	return nil
}

func addBackupFile(archive *zip.Writer, name, source string) (string, error) {
	file, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", err
	}
	header.Name = name
	header.Method = zip.Deflate
	header.SetMode(0o600)
	w, err := archive.CreateHeader(header)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, hash), file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ValidateBackupArchive(path string) (backupManifest, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return backupManifest{}, fmt.Errorf("open backup: %w", err)
	}
	defer archive.Close()
	var manifest backupManifest
	var total int64
	entries := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		if !safeArchivePath(file.Name) || !file.Mode().IsRegular() {
			return backupManifest{}, fmt.Errorf("backup contains unsafe path %q", file.Name)
		}
		if entries[file.Name] != nil {
			return backupManifest{}, fmt.Errorf("backup contains duplicate entry %q", file.Name)
		}
		total += int64(file.UncompressedSize64)
		if total > maxRestoreBytes {
			return backupManifest{}, errors.New("backup exceeds the restore size limit")
		}
		entries[file.Name] = file
	}
	manifestFile := entries["manifest.json"]
	if manifestFile == nil {
		return backupManifest{}, errors.New("backup manifest is missing")
	}
	if err := readZipJSON(manifestFile, &manifest); err != nil {
		return backupManifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	if manifest.FormatVersion != backupFormatVersion || manifest.Files["sqlwarden.db"] == "" {
		return backupManifest{}, errors.New("backup format is unsupported or incomplete")
	}
	if len(entries) != len(manifest.Files)+1 {
		return backupManifest{}, errors.New("backup contains entries not declared by its manifest")
	}
	for name, expected := range manifest.Files {
		decoded, decodeErr := hex.DecodeString(expected)
		if decodeErr != nil || len(decoded) != sha256.Size {
			return backupManifest{}, fmt.Errorf("backup entry %q has an invalid integrity digest", name)
		}
		file := entries[name]
		if file == nil {
			return backupManifest{}, fmt.Errorf("backup entry %q is missing", name)
		}
		actual, err := zipFileHash(file)
		if err != nil {
			return backupManifest{}, err
		}
		if !strings.EqualFold(actual, expected) {
			return backupManifest{}, fmt.Errorf("backup entry %q failed integrity validation", name)
		}
	}
	return manifest, nil
}

func QueueRestore(paths Paths, archivePath string) error {
	if _, err := ValidateBackupArchive(archivePath); err != nil {
		return err
	}
	pending := filepath.Join(paths.Backups, ".pending-restore.sqlwarden-backup")
	if err := copyFile(archivePath, pending, 0o600); err != nil {
		return fmt.Errorf("stage restore: %w", err)
	}
	contents, err := json.Marshal(restoreMarker{Archive: pending})
	if err != nil {
		return err
	}
	return replaceProtectedFile(filepath.Join(paths.ConfigDir, "restore.json"), append(contents, '\n'))
}

func ApplyPendingRestore(paths Paths) error {
	markerPath := filepath.Join(paths.ConfigDir, "restore.json")
	contents, err := os.ReadFile(markerPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var marker restoreMarker
	if err := json.Unmarshal(contents, &marker); err != nil {
		return fmt.Errorf("decode pending restore: %w", err)
	}
	if _, err := ValidateBackupArchive(marker.Archive); err != nil {
		return fmt.Errorf("validate pending restore: %w", err)
	}
	staging, err := os.MkdirTemp(paths.Temp, "restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractBackup(marker.Archive, staging); err != nil {
		return err
	}

	if _, err := os.Stat(paths.Database); err == nil {
		rollback := filepath.Join(paths.Backups, "pre-restore-"+time.Now().UTC().Format("20060102-150405")+".sqlwarden-backup")
		if err := CreateBackupArchive(paths, paths.Database, rollback, "rollback"); err != nil {
			return fmt.Errorf("create pre-restore rollback backup: %w", err)
		}
	}
	oldDatabase := paths.Database + ".restore-old"
	oldFiles := paths.Files + ".restore-old"
	_ = os.Remove(oldDatabase)
	_ = os.RemoveAll(oldFiles)
	if _, err := os.Stat(paths.Database); err == nil {
		if err := os.Rename(paths.Database, oldDatabase); err != nil {
			return err
		}
	}
	if err := os.Rename(filepath.Join(staging, "sqlwarden.db"), paths.Database); err != nil {
		_ = os.Rename(oldDatabase, paths.Database)
		return err
	}
	if _, err := os.Stat(paths.Files); err == nil {
		if err := os.Rename(paths.Files, oldFiles); err != nil {
			_ = os.Remove(paths.Database)
			_ = os.Rename(oldDatabase, paths.Database)
			return err
		}
	}
	restorePrevious := func() {
		_ = os.Remove(paths.Database)
		_ = os.RemoveAll(paths.Files)
		_ = os.Rename(oldDatabase, paths.Database)
		_ = os.Rename(oldFiles, paths.Files)
	}
	restoredFiles := filepath.Join(staging, "files")
	if _, err := os.Stat(restoredFiles); errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(restoredFiles, 0o700); err != nil {
			restorePrevious()
			return err
		}
	}
	if err := os.Rename(restoredFiles, paths.Files); err != nil {
		restorePrevious()
		return err
	}
	_ = os.Remove(oldDatabase)
	_ = os.RemoveAll(oldFiles)
	_ = os.Remove(markerPath)
	_ = os.Remove(marker.Archive)
	return nil
}

func extractBackup(path, destination string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name == "manifest.json" {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if !safeArchivePath(entry.Name) {
			return fmt.Errorf("unsafe backup path %q", entry.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			_ = source.Close()
			return err
		}
		_, copyErr := io.Copy(file, source)
		closeErr := file.Close()
		_ = source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func safeArchivePath(name string) bool {
	clean := filepath.ToSlash(filepath.Clean(name))
	return name != "" && clean == name && clean != "." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
}

func readZipJSON(file *zip.File, target any) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	return json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(target)
}

func zipFileHash(file *zip.File) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".sqlwarden-copy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, destination); err != nil {
		return err
	}
	return nil
}

func replaceFile(source, destination string) error {
	if runtime.GOOS == "windows" {
		_ = os.Remove(destination)
	}
	return os.Rename(source, destination)
}
