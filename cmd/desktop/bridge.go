package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"time"

	desktopconfig "github.com/sqlwarden/internal/desktop"
	"github.com/sqlwarden/internal/version"
	"github.com/sqlwarden/internal/web"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var openDirectory = openDirectoryNative

type DesktopBridge struct {
	mu                sync.Mutex
	ctx               context.Context
	app               *web.App
	paths             desktopconfig.Paths
	startupErr        error
	authSessionID     string
	unsavedChanges    bool
	nativeEventsReady bool
	pendingFiles      []NativeTextFile
	pendingSQLite     []string
}

type DesktopInfo struct {
	Version      string              `json:"version"`
	Paths        desktopconfig.Paths `json:"paths"`
	SecretStore  string              `json:"secret_store"`
	StartupError string              `json:"startup_error,omitempty"`
}

func newDesktopBridge(app *web.App, paths desktopconfig.Paths, startupErr error) *DesktopBridge {
	return &DesktopBridge{app: app, paths: paths, startupErr: startupErr}
}

func (b *DesktopBridge) startup(ctx context.Context) {
	b.mu.Lock()
	b.ctx = ctx
	b.mu.Unlock()
}

func (b *DesktopBridge) shutdown(ctx context.Context) {
	b.mu.Lock()
	sessionID := b.authSessionID
	b.authSessionID = ""
	b.mu.Unlock()
	if b.app != nil && sessionID != "" {
		_ = b.app.RevokeDesktopSession(ctx, sessionID)
	}
}

func (b *DesktopBridge) GetInfo() DesktopInfo {
	info := DesktopInfo{Version: version.Get(), Paths: b.paths, SecretStore: desktopconfig.SecretStore(b.paths)}
	if b.startupErr != nil {
		info.StartupError = b.startupErr.Error()
	}
	return info
}

func (b *DesktopBridge) StartSession() (web.DesktopSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startupErr != nil {
		return web.DesktopSession{}, b.startupErr
	}
	if b.app == nil {
		return web.DesktopSession{}, errors.New("desktop service is unavailable")
	}
	var (
		session web.DesktopSession
		err     error
	)
	if b.authSessionID == "" {
		session, err = b.app.NewDesktopSession(b.ctx)
	} else {
		session, err = b.app.RefreshDesktopSession(b.ctx, b.authSessionID)
	}
	if err == nil {
		b.authSessionID = session.AuthSessionID
	}
	return session, err
}

func (b *DesktopBridge) RevealDataDirectory() error {
	return revealDirectory(b.paths.DataDir)
}

func (b *DesktopBridge) RevealLogDirectory() error {
	return revealDirectory(b.paths.Logs)
}

func (b *DesktopBridge) RevealBackupDirectory() error {
	return revealDirectory(b.paths.Backups)
}

type NativeTextFile struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type NativeOpenRequests struct {
	Files       []NativeTextFile `json:"files"`
	SQLiteFiles []string         `json:"sqlite_files"`
}

// DrainOpenRequests makes OS-level open requests reliable even when they arrive
// before React has subscribed to Wails events during startup.
func (b *DesktopBridge) DrainOpenRequests() NativeOpenRequests {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nativeEventsReady = true
	requests := NativeOpenRequests{Files: b.pendingFiles, SQLiteFiles: b.pendingSQLite}
	b.pendingFiles = nil
	b.pendingSQLite = nil
	return requests
}

func (b *DesktopBridge) dispatchFileOpened(file NativeTextFile) {
	b.mu.Lock()
	ready, ctx := b.nativeEventsReady, b.ctx
	if !ready {
		b.pendingFiles = append(b.pendingFiles, file)
	}
	b.mu.Unlock()
	if ready && ctx != nil {
		wailsruntime.EventsEmit(ctx, "desktop:file-opened", file)
	}
}

func (b *DesktopBridge) dispatchSQLiteSelected(path string) {
	b.mu.Lock()
	ready, ctx := b.nativeEventsReady, b.ctx
	if !ready {
		b.pendingSQLite = append(b.pendingSQLite, path)
	}
	b.mu.Unlock()
	if ready && ctx != nil {
		wailsruntime.EventsEmit(ctx, "desktop:sqlite-selected", path)
	}
}

func (b *DesktopBridge) OpenSQLFile() (NativeTextFile, error) {
	path, err := wailsruntime.OpenFileDialog(b.context(), wailsruntime.OpenDialogOptions{
		Title:   "Open SQL file",
		Filters: []wailsruntime.FileFilter{{DisplayName: "SQL files (*.sql)", Pattern: "*.sql"}},
	})
	if err != nil || path == "" {
		return NativeTextFile{}, err
	}
	contents, err := readBoundedFile(path, 20<<20)
	if err != nil {
		return NativeTextFile{}, err
	}
	return NativeTextFile{Path: path, Name: filepath.Base(path), Content: string(contents)}, nil
}

func (b *DesktopBridge) SaveSQLFile(suggestedName, content string) (string, error) {
	if len(content) > 20<<20 {
		return "", errors.New("SQL file exceeds the 20 MiB desktop limit")
	}
	if strings.TrimSpace(suggestedName) == "" {
		suggestedName = "query.sql"
	}
	path, err := wailsruntime.SaveFileDialog(b.context(), wailsruntime.SaveDialogOptions{
		Title:           "Save SQL file",
		DefaultFilename: ensureExtension(filepath.Base(suggestedName), ".sql"),
		Filters:         []wailsruntime.FileFilter{{DisplayName: "SQL files (*.sql)", Pattern: "*.sql"}},
	})
	if err != nil || path == "" {
		return path, err
	}
	return path, writeAtomic(path, []byte(content), 0o600)
}

func (b *DesktopBridge) SaveExportFile(suggestedName, content string) (string, error) {
	if len(content) > 100<<20 {
		return "", errors.New("export exceeds the 100 MiB desktop limit")
	}
	path, err := wailsruntime.SaveFileDialog(b.context(), wailsruntime.SaveDialogOptions{
		Title:           "Save export",
		DefaultFilename: filepath.Base(suggestedName),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "CSV files (*.csv)", Pattern: "*.csv"},
			{DisplayName: "JSON files (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		return path, err
	}
	return path, writeAtomic(path, []byte(content), 0o600)
}

func (b *DesktopBridge) ChooseSQLiteFile() (string, error) {
	return wailsruntime.OpenFileDialog(b.context(), wailsruntime.OpenDialogOptions{
		Title:   "Choose SQLite database",
		Filters: []wailsruntime.FileFilter{{DisplayName: "SQLite databases", Pattern: "*.db;*.sqlite;*.sqlite3"}},
	})
}

func (b *DesktopBridge) ChooseDirectory() (string, error) {
	return wailsruntime.OpenDirectoryDialog(b.context(), wailsruntime.OpenDialogOptions{Title: "Choose folder"})
}

// SetTheme keeps native window chrome aligned with the React theme. The
// preference controls whether Wails follows the OS while resolvedTheme keeps
// the native window background consistent during WebView redraws.
func (b *DesktopBridge) SetTheme(theme, resolvedTheme string) error {
	ctx := b.context()
	switch theme {
	case "system":
		wailsruntime.WindowSetSystemDefaultTheme(ctx)
	case "light":
		wailsruntime.WindowSetLightTheme(ctx)
	case "dark":
		wailsruntime.WindowSetDarkTheme(ctx)
	default:
		return errors.New("unsupported desktop theme")
	}

	switch resolvedTheme {
	case "light":
		wailsruntime.WindowSetBackgroundColour(ctx, 249, 249, 249, 255)
	case "dark":
		wailsruntime.WindowSetBackgroundColour(ctx, 22, 24, 25, 255)
	default:
		return errors.New("unsupported resolved desktop theme")
	}
	return nil
}

func (b *DesktopBridge) OpenExternalURL(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return errors.New("only http and https links can be opened externally")
	}
	wailsruntime.BrowserOpenURL(b.context(), url)
	return nil
}

func (b *DesktopBridge) OpenReleasePage() {
	wailsruntime.BrowserOpenURL(b.context(), "https://github.com/sqlwarden/sqlwarden/releases/latest")
}

func (b *DesktopBridge) SaveDiagnostics() (string, error) {
	path, err := wailsruntime.SaveFileDialog(b.context(), wailsruntime.SaveDialogOptions{
		Title:           "Save diagnostics",
		DefaultFilename: "sqlwarden-diagnostics.json",
		Filters:         []wailsruntime.FileFilter{{DisplayName: "JSON files (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return path, err
	}
	diagnostic := map[string]any{
		"generated_at":     time.Now().UTC().Format(time.RFC3339),
		"version":          version.Get(),
		"operating_system": stdruntime.GOOS,
		"architecture":     stdruntime.GOARCH,
		"startup_error":    b.GetInfo().StartupError,
		"secret_store":     b.GetInfo().SecretStore,
		"paths": map[string]string{
			"config": redactHome(b.paths.ConfigDir),
			"data":   redactHome(b.paths.DataDir),
			"cache":  redactHome(b.paths.Cache),
			"logs":   redactHome(b.paths.Logs),
		},
	}
	contents, err := json.MarshalIndent(diagnostic, "", "  ")
	if err != nil {
		return "", err
	}
	return path, writeAtomic(path, append(contents, '\n'), 0o600)
}

func (b *DesktopBridge) CreateBackup() (string, error) {
	if b.app == nil {
		return "", errors.New("desktop service is unavailable")
	}
	destination, err := wailsruntime.SaveFileDialog(b.context(), wailsruntime.SaveDialogOptions{
		Title:            "Create SQLWarden backup",
		DefaultDirectory: b.paths.Backups,
		DefaultFilename:  "sqlwarden-" + time.Now().UTC().Format("20060102-150405") + ".sqlwarden-backup",
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "SQLWarden backups (*.sqlwarden-backup)", Pattern: "*.sqlwarden-backup",
		}},
	})
	if err != nil || destination == "" {
		return destination, err
	}
	snapshot, err := os.CreateTemp(b.paths.Temp, "database-snapshot-*.db")
	if err != nil {
		return "", err
	}
	snapshotPath := snapshot.Name()
	_ = snapshot.Close()
	_ = os.Remove(snapshotPath)
	defer os.Remove(snapshotPath)
	if err := b.app.BackupDesktopDatabase(b.context(), snapshotPath); err != nil {
		return "", err
	}
	if err := desktopconfig.CreateBackupArchive(b.paths, snapshotPath, destination, version.Get()); err != nil {
		return "", err
	}
	return destination, nil
}

func (b *DesktopBridge) RestoreBackup() (string, error) {
	archive, err := wailsruntime.OpenFileDialog(b.context(), wailsruntime.OpenDialogOptions{
		Title:            "Restore SQLWarden backup",
		DefaultDirectory: b.paths.Backups,
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "SQLWarden backups (*.sqlwarden-backup)", Pattern: "*.sqlwarden-backup",
		}},
	})
	if err != nil || archive == "" {
		return archive, err
	}
	manifest, err := desktopconfig.ValidateBackupArchive(archive)
	if err != nil {
		return "", err
	}
	choice, err := wailsruntime.MessageDialog(b.context(), wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.WarningDialog,
		Title:         "Restore backup?",
		Message:       fmt.Sprintf("Restore the backup created %s? SQLWarden will restart, and a rollback backup will be created first.", manifest.CreatedAt),
		Buttons:       []string{"Cancel", "Restore and restart"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil || choice != "Restore and restart" {
		return "", err
	}
	if err := desktopconfig.QueueRestore(b.paths, archive); err != nil {
		return "", err
	}
	ctx := b.context()
	time.AfterFunc(500*time.Millisecond, func() { wailsruntime.Quit(ctx) })
	return archive, nil
}

func (b *DesktopBridge) SetUnsavedChanges(unsaved bool) {
	b.mu.Lock()
	b.unsavedChanges = unsaved
	b.mu.Unlock()
}

func (b *DesktopBridge) hasUnsavedChanges() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unsavedChanges
}

func (b *DesktopBridge) context() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ctx
}

func (b *DesktopBridge) focusWindow() {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx != nil {
		wailsruntime.WindowUnminimise(ctx)
		wailsruntime.WindowShow(ctx)
	}
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds the %d MiB desktop limit", limit>>20)
	}
	return os.ReadFile(path)
}

func writeAtomic(path string, contents []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".sqlwarden-save-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if stdruntime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(temporaryPath, path)
}

func ensureExtension(name, extension string) string {
	if strings.EqualFold(filepath.Ext(name), extension) {
		return name
	}
	return name + extension
}

func redactHome(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if relative, relErr := filepath.Rel(home, path); relErr == nil && relative != "." && !strings.HasPrefix(relative, "..") {
			return filepath.Join("~", relative)
		}
	}
	return path
}

func revealDirectory(path string) error {
	if path == "" {
		return errors.New("desktop directory is unavailable")
	}
	return openDirectory(path)
}
