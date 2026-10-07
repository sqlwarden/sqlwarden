package observability

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/lmittmann/tint"
	"github.com/sqlwarden/internal/version"
)

// NewLogger builds the process logger. Its level starts at info and is updated
// from database-backed instance settings after the application database opens.
func NewLogger(format string, out io.Writer) (*slog.Logger, error) {
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(out, opts)
	case "text":
		handler = tint.NewHandler(out, &tint.Options{Level: level})
	default:
		return nil, fmt.Errorf("unsupported log format: %s", format)
	}

	handler = &runtimeLevelHandler{Handler: handler, level: level}
	return slog.New(handler).With(
		"service", "sqlwarden",
		"version", version.Get(),
	), nil
}

type runtimeLevelHandler struct {
	slog.Handler
	level *slog.LevelVar
}

func (h *runtimeLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &runtimeLevelHandler{Handler: h.Handler.WithAttrs(attrs), level: h.level}
}

func (h *runtimeLevelHandler) WithGroup(name string) slog.Handler {
	return &runtimeLevelHandler{Handler: h.Handler.WithGroup(name), level: h.level}
}

// SetLevel changes the level of a logger built by NewLogger. It is a no-op for
// any other logger.
func SetLevel(logger *slog.Logger, level string) error {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return err
	}
	handler, ok := logger.Handler().(*runtimeLevelHandler)
	if !ok {
		return nil
	}
	handler.level.Set(parsedLevel)
	return nil
}

// ParseLevel maps a case-insensitive level name to its slog level.
func ParseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level: %s", level)
	}
}
