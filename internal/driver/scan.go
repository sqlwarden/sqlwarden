package driver

import (
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/pkg/result"
)

const (
	TruncationReasonMaxRows  = "max_result_rows"
	TruncationReasonMaxBytes = "max_result_bytes"
)

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

	rs := &result.ResultSet{}
	for _, ct := range colTypes {
		nullable, _ := ct.Nullable()
		rs.Columns = append(rs.Columns, result.Column{
			Name:     ct.Name(),
			Type:     result.NormalizeColumnType(ct.DatabaseTypeName()),
			RawType:  ct.DatabaseTypeName(),
			Nullable: nullable,
		})
	}

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
