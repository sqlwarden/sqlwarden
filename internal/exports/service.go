package exports

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
)

const (
	FormatCSV       = "csv"
	DefaultPageSize = 1000
)

var (
	ErrUnsupportedFormat = errors.New("unsupported export format")
	ErrCursorUnsupported = errors.New("export requires cursor support")
	ErrByteLimitExceeded = errors.New("export byte limit exceeded")
)

type ProgressFunc func(rows int64, bytes int64)

type StreamOptions struct {
	Format     string
	SQL        string
	MaxBytes   int64
	PageSize   int
	OnProgress ProgressFunc
}

type StreamResult struct {
	Rows  int64 `json:"row_count"`
	Bytes int64 `json:"byte_count"`
}

// Service streams query results from a connected driver to formatted bytes.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Stream(ctx context.Context, driver engine.Driver, writer io.Writer, opts StreamOptions) (StreamResult, error) {
	if opts.Format == "" {
		opts.Format = FormatCSV
	}
	if opts.Format != FormatCSV {
		return StreamResult{}, ErrUnsupportedFormat
	}
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultPageSize
	}
	cursorDriver, ok := driver.(cursor.QueryCursorDriver)
	if !ok {
		return StreamResult{}, ErrCursorUnsupported
	}

	limited := &limitWriter{writer: writer, max: opts.MaxBytes}
	csvWriter := NewCSVWriter(limited)
	queryCursor, err := cursorDriver.StartQuery(ctx, cursor.QueryRequest{SQL: opts.SQL})
	if err != nil {
		return StreamResult{}, err
	}
	defer queryCursor.Close()

	var result StreamResult
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		page, state, err := queryCursor.Fetch(ctx, cursor.ScanOptions{MaxRows: opts.PageSize})
		if err != nil {
			return result, err
		}
		if err := csvWriter.WritePage(page); err != nil {
			return result, normalizeLimitError(err)
		}
		result.Rows += int64(state.RowsReturned)
		result.Bytes = limited.bytes
		if opts.OnProgress != nil && result.Rows > 0 {
			opts.OnProgress(result.Rows, result.Bytes)
		}
		if state.Exhausted {
			break
		}
	}
	if err := csvWriter.Flush(); err != nil {
		return result, normalizeLimitError(err)
	}
	result.Bytes = limited.bytes
	return result, nil
}

type limitWriter struct {
	writer io.Writer
	max    int64
	bytes  int64
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.max > 0 && w.bytes+int64(len(p)) > w.max {
		allowed := int(w.max - w.bytes)
		if allowed > 0 {
			n, err := w.writer.Write(p[:allowed])
			w.bytes += int64(n)
			if err != nil {
				return n, err
			}
		}
		return allowed, ErrByteLimitExceeded
	}
	n, err := w.writer.Write(p)
	w.bytes += int64(n)
	return n, err
}

func normalizeLimitError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrByteLimitExceeded) {
		return ErrByteLimitExceeded
	}
	return fmt.Errorf("write export: %w", err)
}
