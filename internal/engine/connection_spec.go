package engine

import (
	"fmt"
	"net/url"
	"sort"
)

// FieldType describes how a connection field is represented by clients.
type FieldType string

const (
	FieldTypeString FieldType = "string"
	FieldTypeInt    FieldType = "int"
	FieldTypeBool   FieldType = "bool"
	FieldTypeEnum   FieldType = "enum"
)

// FieldSpec describes one parameter or secret accepted by a connection spec.
type FieldSpec struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required"`
	Default  string    `json:"default,omitempty"`
	Secret   bool      `json:"secret"`
	Options  []string  `json:"options,omitempty"`
	// NonNetwork identifies fields that do not change the server receiving
	// stored credentials.
	NonNetwork bool `json:"-"`
}

// Params contains non-secret connection parameters.
type Params map[string]string

// Secrets contains secret connection parameters.
type Secrets map[string]string

// ConnectionSpec describes an engine's connection fields and translates them
// to and from the DSN accepted by its driver. ParseDSN lifts a legacy DSN's
// native TLS mode (for example, PostgreSQL sslmode) into Params under the
// "tls_mode" key. BuildDSN always ignores that key because structured
// ConnectionConfig.TLS is the single source of truth for TLS settings.
type ConnectionSpec interface {
	Fields() []FieldSpec
	BuildDSN(params Params, secrets Secrets) (string, error)
	ParseDSN(dsn string) (Params, Secrets, error)
}

// ConnectionSpecFor returns the connection capability implemented by the
// registered engine's unconnected driver. Engine aliases are normalized by New.
func ConnectionSpecFor(driver string) (ConnectionSpec, bool) {
	d, err := New(driver)
	if err != nil {
		return nil, false
	}
	spec, ok := d.(ConnectionSpec)
	return spec, ok
}

// LegacyTLSMapper is implemented by connection specs whose ParseDSN can return
// a native TLS value in Params["tls_mode"]. It translates that value to the
// structured TLS mode used by connection TLS configuration.
type LegacyTLSMapper interface {
	LegacyTLSMode(native string) TLSMode
}

// RejectUnsupportedDSNParameters fails when a DSN query carries a parameter
// outside allowed. ParseDSN converts a DSN into structured fields and then the
// original is discarded, so a parameter that no field models would be lost
// silently. The error names the parameter and never its value.
func RejectUnsupportedDSNParameters(query url.Values, allowed ...string) error {
	var unsupported []string
	for key := range query {
		supported := false
		for _, name := range allowed {
			if key == name {
				supported = true
				break
			}
		}
		if !supported {
			unsupported = append(unsupported, key)
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return fmt.Errorf("unsupported connection string parameter %q", unsupported[0])
}
