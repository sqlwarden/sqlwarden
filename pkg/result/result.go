package result

import (
	"strings"
	"time"
)

// ResultSet holds a normalized query result with typed columns and rows.
type ResultSet struct {
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
}

// Column describes a single column in a ResultSet.
type Column struct {
	Name     string     `json:"name"`
	Type     ColumnType `json:"type"`
	RawType  string     `json:"raw_type"`
	Nullable bool       `json:"nullable"`
}

// ColumnType is a normalized, database-agnostic column type.
type ColumnType string

const (
	ColumnTypeText     ColumnType = "text"
	ColumnTypeInteger  ColumnType = "integer"
	ColumnTypeDecimal  ColumnType = "decimal"
	ColumnTypeBoolean  ColumnType = "boolean"
	ColumnTypeDateTime ColumnType = "datetime"
	ColumnTypeJSON     ColumnType = "json"
	ColumnTypeUUID     ColumnType = "uuid"
	ColumnTypeBytes    ColumnType = "bytes"
)

// Row is an ordered slice of Values corresponding to the ResultSet's Columns.
type Row []Value

// Value holds a single cell value with its normalized type.
type Value struct {
	Type    ValueType  `json:"type"`
	Text    string     `json:"text,omitempty"`
	Integer int64      `json:"integer,omitempty"`
	Float   float64    `json:"float,omitempty"`
	Decimal string     `json:"decimal,omitempty"`
	Bool    bool       `json:"bool,omitempty"`
	Time    *time.Time `json:"time,omitempty"`
	Bytes   []byte     `json:"bytes,omitempty"`
}

// ValueType identifies the kind of value stored in a Value.
type ValueType string

const (
	ValueTypeNull    ValueType = "null"
	ValueTypeText    ValueType = "text"
	ValueTypeInteger ValueType = "integer"
	ValueTypeFloat   ValueType = "float"
	ValueTypeDecimal ValueType = "decimal"
	ValueTypeBool    ValueType = "bool"
	ValueTypeTime    ValueType = "time"
	ValueTypeBytes   ValueType = "bytes"
)

// NormalizeColumnType maps a raw database type string to a canonical ColumnType.
// Matching is case-insensitive and trims surrounding whitespace.
func NormalizeColumnType(dbType string) ColumnType {
	t := strings.ToLower(strings.TrimSpace(dbType))

	// Special case: tinyint(1) is boolean.
	if t == "tinyint(1)" {
		return ColumnTypeBoolean
	}

	switch t {
	// Integer types.
	case "int", "int2", "int4", "int8", "bigint", "smallint", "tinyint",
		"serial", "bigserial", "smallserial", "integer":
		return ColumnTypeInteger

	// Decimal / floating-point types.
	case "numeric", "decimal", "money", "float", "float4", "float8",
		"double", "double precision", "real":
		return ColumnTypeDecimal

	// Boolean types.
	case "bool", "boolean":
		return ColumnTypeBoolean

	// Date / time types.
	case "date", "time", "timetz", "timestamp", "timestamptz", "datetime",
		"timestamp with time zone", "timestamp without time zone":
		return ColumnTypeDateTime

	// JSON types.
	case "json", "jsonb":
		return ColumnTypeJSON

	// UUID types.
	case "uuid", "uniqueidentifier":
		return ColumnTypeUUID

	// Binary types.
	case "blob", "bytea", "binary", "varbinary":
		return ColumnTypeBytes
	}

	// Everything else is text.
	return ColumnTypeText
}
