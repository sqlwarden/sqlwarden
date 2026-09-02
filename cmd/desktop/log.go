//go:build !bindings

package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	desktopconfig "github.com/sqlwarden/internal/desktop"
)

const (
	desktopLogMaxBytes = 5 << 20
	desktopLogBackups  = 3
)

type rotatingLogWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	maxBytes int64
	backups  int
}

func openRotatingLog(path string, maxBytes int64, backups int) (*rotatingLogWriter, error) {
	w := &rotatingLogWriter{path: path, maxBytes: maxBytes, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size()+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	return w.file.Write(p)
}

func (w *rotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func (w *rotatingLogWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

func (w *rotatingLogWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	for index := w.backups - 1; index >= 1; index-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", w.path, index), fmt.Sprintf("%s.%d", w.path, index+1))
	}
	if w.backups > 0 {
		_ = os.Rename(w.path, w.path+".1")
	} else {
		_ = os.Remove(w.path)
	}
	return w.open()
}

func desktopLogger(paths desktopconfig.Paths) (*slog.Logger, func(), error) {
	writers := []io.Writer{os.Stderr}
	closeLog := func() {}
	if paths.Logs != "" {
		if err := os.MkdirAll(paths.Logs, 0o700); err != nil {
			return nil, closeLog, fmt.Errorf("create desktop log directory: %w", err)
		}
		writer, err := openRotatingLog(filepath.Join(paths.Logs, "sqlwarden.log"), desktopLogMaxBytes, desktopLogBackups)
		if err != nil {
			return nil, closeLog, fmt.Errorf("open desktop log: %w", err)
		}
		writers = append(writers, writer)
		closeLog = func() { _ = writer.Close() }
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo})), closeLog, nil
}
