package cursor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/pkg/result"
)

// Truncation reasons recorded on a ResultSet when a scan stops early because it
// hit a configured limit.
const (
	TruncationReasonMaxRows  = "max_result_rows"
	TruncationReasonMaxBytes = "max_result_bytes"
)

// ErrCursorClosed is returned when Fetch is called on a cursor that has been
// closed or exhausted.
var ErrCursorClosed = errors.New("query cursor is closed")

// ScanOptions bounds a single scan or cursor page. MaxRows caps the number of
// rows and MaxBytes caps the encoded size; a zero value means unbounded.
type ScanOptions struct {
	MaxRows  int
	MaxBytes int64
}

// ScanRows normalizes database rows into a ResultSet while enforcing optional
// row and byte limits. Zero limits mean unbounded and are intended for tests or
// direct driver use outside the web runtime.
func ScanRows(rows *sql.Rows, opts ScanOptions) (*result.ResultSet, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("column types: %w", err)
	}

	rs := &result.ResultSet{Columns: columnsFromTypes(colTypes)}

	for rows.Next() {
		if opts.MaxRows > 0 && len(rs.Rows) >= opts.MaxRows {
			rs.Truncated = true
			rs.TruncationReason = TruncationReasonMaxRows
			return finalizeResult(rs, rows, nil)
		}

		row, rowBytes, err := scanRow(rows, len(colTypes))
		if err != nil {
			return nil, err
		}
		if opts.MaxBytes > 0 && rs.BytesReturned+rowBytes > opts.MaxBytes {
			rs.Truncated = true
			rs.TruncationReason = TruncationReasonMaxBytes
			return finalizeResult(rs, rows, nil)
		}

		rs.Rows = append(rs.Rows, row)
		rs.BytesReturned += rowBytes
		rs.RowsReturned = len(rs.Rows)
	}

	return finalizeResult(rs, rows, rows.Err())
}

func finalizeResult(rs *result.ResultSet, rows *sql.Rows, err error) (*result.ResultSet, error) {
	rs.RowsReturned = len(rs.Rows)
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return rs, nil
}

func scanRow(rows *sql.Rows, columnCount int) (result.Row, int64, error) {
	receivers := make([]any, columnCount)
	ptrs := make([]any, columnCount)
	for i := range receivers {
		ptrs[i] = &receivers[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, 0, fmt.Errorf("scan row: %w", err)
	}

	row := make(result.Row, columnCount)
	var rowBytes int64
	for i, val := range receivers {
		cell := NormalizeValue(val)
		row[i] = cell
		rowBytes += valueSize(cell)
	}
	return row, rowBytes, nil
}

// SQLRowsCursor is the default QueryCursor implementation: a thin, concurrency-
// safe adapter over database/sql rows that yields pages via Fetch and owns
// closing the underlying rows.
type SQLRowsCursor struct {
	rows    *sql.Rows
	columns []result.Column
	mu      sync.Mutex
	closed  bool
}

// NewSQLRowsCursor adapts database/sql rows into the QueryCursor interface.
// The caller transfers ownership of rows; the cursor closes them when exhausted
// or when Close is called.
func NewSQLRowsCursor(rows *sql.Rows) (*SQLRowsCursor, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("column types: %w", err)
	}
	return &SQLRowsCursor{
		rows:    rows,
		columns: columnsFromTypes(colTypes),
	}, nil
}

func (c *SQLRowsCursor) Columns() []result.Column {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]result.Column(nil), c.columns...)
}

func (c *SQLRowsCursor) Fetch(ctx context.Context, opts ScanOptions) (*result.ResultSet, QueryCursorState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, QueryCursorState{Exhausted: true}, ErrCursorClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, QueryCursorState{Exhausted: true}, err
	}

	rs := &result.ResultSet{Columns: append([]result.Column(nil), c.columns...)}
	for opts.MaxRows <= 0 || len(rs.Rows) < opts.MaxRows {
		if !c.rows.Next() {
			if err := c.rows.Err(); err != nil {
				_ = c.closeLocked()
				return nil, QueryCursorState{Exhausted: true}, err
			}
			if err := c.closeLocked(); err != nil {
				return nil, QueryCursorState{Exhausted: true}, err
			}
			return cursorPageResult(rs, true), cursorState(rs, true), nil
		}
		row, rowBytes, err := scanRow(c.rows, len(c.columns))
		if err != nil {
			_ = c.closeLocked()
			return nil, QueryCursorState{Exhausted: true}, err
		}
		if opts.MaxBytes > 0 && rs.BytesReturned+rowBytes > opts.MaxBytes {
			rs.Truncated = true
			rs.TruncationReason = TruncationReasonMaxBytes
			return cursorPageResult(rs, false), cursorState(rs, false), nil
		}

		rs.Rows = append(rs.Rows, row)
		rs.BytesReturned += rowBytes
		rs.RowsReturned = len(rs.Rows)
	}
	return cursorPageResult(rs, false), cursorState(rs, false), nil
}

func (c *SQLRowsCursor) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *SQLRowsCursor) closeLocked() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rows.Close()
}

func cursorPageResult(rs *result.ResultSet, exhausted bool) *result.ResultSet {
	rs.RowsReturned = len(rs.Rows)
	if !exhausted && rs.TruncationReason == TruncationReasonMaxBytes {
		rs.Truncated = true
	}
	return rs
}

func cursorState(rs *result.ResultSet, exhausted bool) QueryCursorState {
	return QueryCursorState{
		Exhausted:     exhausted,
		RowsReturned:  len(rs.Rows),
		BytesReturned: rs.BytesReturned,
	}
}

func columnsFromTypes(colTypes []*sql.ColumnType) []result.Column {
	columns := make([]result.Column, 0, len(colTypes))
	for _, ct := range colTypes {
		nullable, _ := ct.Nullable()
		columns = append(columns, result.Column{
			Name:     ct.Name(),
			Type:     result.NormalizeColumnType(ct.DatabaseTypeName()),
			RawType:  ct.DatabaseTypeName(),
			Nullable: nullable,
		})
	}
	return columns
}

// NormalizeValue converts a raw database driver value into SQLWarden's
// normalized result value representation.
func NormalizeValue(v any) result.Value {
	if v == nil {
		return result.Value{Type: result.ValueTypeNull}
	}
	switch val := v.(type) {
	case int64:
		return result.Value{Type: result.ValueTypeInteger, Integer: val}
	case float64:
		return result.Value{Type: result.ValueTypeFloat, Float: val}
	case bool:
		return result.Value{Type: result.ValueTypeBool, Bool: val}
	case time.Time:
		utc := val.UTC()
		return result.Value{Type: result.ValueTypeTime, Time: &utc}
	case []byte:
		if utf8.Valid(val) {
			return result.Value{Type: result.ValueTypeText, Text: string(val)}
		}
		return result.Value{Type: result.ValueTypeBytes, Bytes: val}
	case string:
		return result.Value{Type: result.ValueTypeText, Text: val}
	default:
		return result.Value{Type: result.ValueTypeText, Text: fmt.Sprintf("%v", val)}
	}
}

func valueSize(v result.Value) int64 {
	switch v.Type {
	case result.ValueTypeNull:
		return 4
	case result.ValueTypeText:
		return int64(len(v.Text))
	case result.ValueTypeInteger:
		return 8
	case result.ValueTypeFloat:
		return 8
	case result.ValueTypeDecimal:
		return int64(len(v.Decimal))
	case result.ValueTypeBool:
		return 1
	case result.ValueTypeTime:
		if v.Time == nil {
			return 0
		}
		return int64(len(v.Time.Format(time.RFC3339Nano)))
	case result.ValueTypeBytes:
		return int64(len(v.Bytes))
	default:
		return int64(len(v.Text))
	}
}
