package result

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizeColumnType(t *testing.T) {
	tests := []struct {
		raw  string
		want ColumnType
	}{
		// Integer family.
		{"int", ColumnTypeInteger},
		{"int4", ColumnTypeInteger},
		{"bigint", ColumnTypeInteger},
		{"smallint", ColumnTypeInteger},
		{"integer", ColumnTypeInteger},
		{"INT", ColumnTypeInteger},        // case-insensitive
		{"  bigint  ", ColumnTypeInteger}, // surrounding whitespace

		// tinyint(1) is boolean, plain tinyint is integer.
		{"tinyint(1)", ColumnTypeBoolean},
		{"tinyint", ColumnTypeInteger},

		// Decimal family.
		{"numeric", ColumnTypeDecimal},
		{"decimal", ColumnTypeDecimal},
		{"float8", ColumnTypeDecimal},
		{"double precision", ColumnTypeDecimal},
		{"real", ColumnTypeDecimal},

		// Boolean family.
		{"bool", ColumnTypeBoolean},
		{"boolean", ColumnTypeBoolean},

		// DateTime family.
		{"timestamp", ColumnTypeDateTime},
		{"timestamptz", ColumnTypeDateTime},
		{"datetime", ColumnTypeDateTime},
		{"timestamp with time zone", ColumnTypeDateTime},
		{"timestamp without time zone", ColumnTypeDateTime},
		{"date", ColumnTypeDateTime},
		{"time", ColumnTypeDateTime},

		// JSON family.
		{"json", ColumnTypeJSON},
		{"jsonb", ColumnTypeJSON},

		// UUID family.
		{"uuid", ColumnTypeUUID},
		{"uniqueidentifier", ColumnTypeUUID},

		// Bytes family.
		{"bytea", ColumnTypeBytes},
		{"blob", ColumnTypeBytes},
		{"binary", ColumnTypeBytes},
		{"varbinary", ColumnTypeBytes},

		// Text / fallback.
		{"text", ColumnTypeText},
		{"varchar", ColumnTypeText},
		{"unknown_type", ColumnTypeText},
		{"", ColumnTypeText},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got := NormalizeColumnType(tc.raw)
			if got != tc.want {
				t.Errorf("NormalizeColumnType(%q) = %q; want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestResultSetJSONRoundTrip(t *testing.T) {
	nowVal := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	now := &nowVal

	original := ResultSet{
		Columns: []Column{
			{Name: "id", Type: ColumnTypeInteger, RawType: "int4", Nullable: false},
			{Name: "name", Type: ColumnTypeText, RawType: "text", Nullable: true},
			{Name: "active", Type: ColumnTypeBoolean, RawType: "bool", Nullable: false},
			{Name: "score", Type: ColumnTypeDecimal, RawType: "numeric", Nullable: true},
			{Name: "created_at", Type: ColumnTypeDateTime, RawType: "timestamp", Nullable: false},
		},
		Rows: []Row{
			{
				{Type: ValueTypeInteger, Integer: 42},
				{Type: ValueTypeText, Text: "Alice"},
				{Type: ValueTypeBool, Bool: true},
				{Type: ValueTypeDecimal, Decimal: "3.14"},
				{Type: ValueTypeTime, Time: now},
			},
			{
				{Type: ValueTypeInteger, Integer: 99},
				{Type: ValueTypeNull},
				{Type: ValueTypeBool, Bool: false},
				{Type: ValueTypeNull},
				{Type: ValueTypeTime, Time: now},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ResultSet
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(decoded.Columns) != len(original.Columns) {
		t.Fatalf("columns count: got %d, want %d", len(decoded.Columns), len(original.Columns))
	}
	for i, col := range original.Columns {
		dc := decoded.Columns[i]
		if dc.Name != col.Name || dc.Type != col.Type || dc.RawType != col.RawType || dc.Nullable != col.Nullable {
			t.Errorf("column[%d] mismatch: got %+v, want %+v", i, dc, col)
		}
	}

	if len(decoded.Rows) != len(original.Rows) {
		t.Fatalf("rows count: got %d, want %d", len(decoded.Rows), len(original.Rows))
	}
	for ri, row := range original.Rows {
		for ci, val := range row {
			dv := decoded.Rows[ri][ci]
			if dv.Type != val.Type {
				t.Errorf("row[%d][%d] type: got %q, want %q", ri, ci, dv.Type, val.Type)
			}
		}
	}
}

func TestValueNullOmitempty(t *testing.T) {
	v := Value{Type: ValueTypeNull}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(m) != 1 {
		t.Errorf("null Value serialized %d fields, want 1 (only 'type'); got: %v", len(m), m)
	}
	if m["type"] != string(ValueTypeNull) {
		t.Errorf("type field: got %q, want %q", m["type"], ValueTypeNull)
	}
}
