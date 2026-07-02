package exports

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/pkg/result"
)

func TestCSVWriterEscapesValuesAndWritesHeader(t *testing.T) {
	ts := time.Date(2026, 7, 2, 3, 4, 5, 0, time.UTC)
	rs := &result.ResultSet{
		Columns: []result.Column{{Name: "name"}, {Name: "note"}, {Name: "count"}, {Name: "at"}, {Name: "empty"}},
		Rows: []result.Row{{
			{Type: result.ValueTypeText, Text: "Ada"},
			{Type: result.ValueTypeText, Text: "comma, quote \" newline\n"},
			{Type: result.ValueTypeInteger, Integer: 42},
			{Type: result.ValueTypeTime, Time: &ts},
			{Type: result.ValueTypeNull},
		}},
	}
	var buf bytes.Buffer
	writer := NewCSVWriter(&buf)
	if err := writer.WritePage(rs); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "name,note,count,at,empty\n") {
		t.Fatalf("csv = %q, missing header", got)
	}
	if !strings.Contains(got, "\"comma, quote \"\" newline\n\"") {
		t.Fatalf("csv = %q, missing escaped field", got)
	}
	if !strings.Contains(got, "42,2026-07-02T03:04:05Z,") {
		t.Fatalf("csv = %q, missing formatted values", got)
	}
}

func TestCSVWriterWritesHeaderForEmptyResult(t *testing.T) {
	rs := &result.ResultSet{Columns: []result.Column{{Name: "id"}, {Name: "name"}}}
	var buf bytes.Buffer
	if err := NewCSVWriter(&buf).WritePage(rs); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "id,name\n" {
		t.Fatalf("csv = %q, want header only", got)
	}
}

func TestLimitWriterEnforcesByteLimit(t *testing.T) {
	var buf bytes.Buffer
	writer := &limitWriter{writer: &buf, max: 5}
	n, err := writer.Write([]byte("abcdef"))
	if err != ErrByteLimitExceeded {
		t.Fatalf("err = %v, want ErrByteLimitExceeded", err)
	}
	if n != 5 || buf.String() != "abcde" {
		t.Fatalf("n=%d buf=%q, want 5 abcde", n, buf.String())
	}
}
