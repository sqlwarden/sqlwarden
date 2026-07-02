package exports

import (
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/sqlwarden/pkg/result"
)

// CSVWriter writes ResultSet pages as one CSV stream. The first page writes the
// header; later pages append rows only.
type CSVWriter struct {
	writer      *csv.Writer
	wroteHeader bool
}

func NewCSVWriter(w io.Writer) *CSVWriter {
	return &CSVWriter{writer: csv.NewWriter(w)}
}

func (w *CSVWriter) WritePage(page *result.ResultSet) error {
	if page == nil {
		return nil
	}
	if !w.wroteHeader {
		header := make([]string, len(page.Columns))
		for i, column := range page.Columns {
			header[i] = column.Name
		}
		if err := w.writer.Write(header); err != nil {
			return err
		}
		w.wroteHeader = true
	}
	for _, row := range page.Rows {
		record := make([]string, len(row))
		for i, value := range row {
			record[i] = valueString(value)
		}
		if err := w.writer.Write(record); err != nil {
			return err
		}
	}
	w.writer.Flush()
	return w.writer.Error()
}

func (w *CSVWriter) Flush() error {
	w.writer.Flush()
	return w.writer.Error()
}

func valueString(value result.Value) string {
	switch value.Type {
	case result.ValueTypeNull:
		return ""
	case result.ValueTypeText:
		return value.Text
	case result.ValueTypeInteger:
		return strconv.FormatInt(value.Integer, 10)
	case result.ValueTypeFloat:
		return strconv.FormatFloat(value.Float, 'f', -1, 64)
	case result.ValueTypeDecimal:
		return value.Decimal
	case result.ValueTypeBool:
		return strconv.FormatBool(value.Bool)
	case result.ValueTypeTime:
		if value.Time == nil {
			return ""
		}
		return value.Time.Format("2006-01-02T15:04:05.999999999Z07:00")
	case result.ValueTypeBytes:
		return base64.StdEncoding.EncodeToString(value.Bytes)
	default:
		return fmt.Sprint(value)
	}
}
