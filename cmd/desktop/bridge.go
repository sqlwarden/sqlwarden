package main

import (
	"context"
	"errors"
	"sync"

	desktopconfig "github.com/sqlwarden/internal/desktop"
	"github.com/sqlwarden/internal/version"
	"github.com/sqlwarden/internal/web"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var openDirectory = openDirectoryNative

type DesktopBridge struct {
	mu            sync.Mutex
	ctx           context.Context
	app           *web.App
	paths         desktopconfig.Paths
	startupErr    error
	authSessionID string
}

type DesktopInfo struct {
	Version      string              `json:"version"`
	Paths        desktopconfig.Paths `json:"paths"`
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
	info := DesktopInfo{Version: version.Get(), Paths: b.paths}
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

func (b *DesktopBridge) focusWindow() {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx != nil {
		runtime.WindowUnminimise(ctx)
		runtime.WindowShow(ctx)
	}
}

func revealDirectory(path string) error {
	if path == "" {
		return errors.New("desktop directory is unavailable")
	}
	return openDirectory(path)
}
