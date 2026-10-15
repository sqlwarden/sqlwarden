package config

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// RedactedValue replaces every sensitive value in a diagnostic. It is a fixed
// marker rather than a masked prefix so nothing about the secret leaks.
const RedactedValue = "[redacted]"

// DiagnosticEntry reports one resolved configuration key.
type DiagnosticEntry struct {
	Key      string   `json:"key"`
	Category Category `json:"category"`
	// Value is the effective value, or [RedactedValue] for sensitive keys.
	Value string `json:"value"`
	// Source names which input supplied the effective value.
	Source Source `json:"source"`
	// Sensitive reports whether the real value was withheld.
	Sensitive bool `json:"sensitive"`
}

// Diagnostic is a redacted report of effective bootstrap configuration and
// where each value came from. It is safe to log and to return to instance
// administrators: sensitive values are never included.
type Diagnostic struct {
	// Precedence lists sources from highest to lowest priority.
	Precedence []Source          `json:"precedence"`
	Entries    []DiagnosticEntry `json:"entries"`
}

func newDiagnostic(v *viper.Viper, sources map[string]Source) Diagnostic {
	entries := make([]DiagnosticEntry, 0, len(options))
	for _, opt := range options {
		entry := DiagnosticEntry{
			Key:       opt.key,
			Category:  opt.category,
			Source:    sources[opt.key],
			Sensitive: opt.sensitive,
			Value:     RedactedValue,
		}
		if !opt.sensitive {
			entry.Value = formatDiagnosticValue(v.Get(opt.key))
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })

	return Diagnostic{
		Precedence: append([]Source(nil), precedence...),
		Entries:    entries,
	}
}

func formatDiagnosticValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []string:
		return strings.Join(typed, ",")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, formatDiagnosticValue(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(typed)
	}
}

// ByCategory returns the diagnostic entries in one configuration category,
// preserving key order.
func (d Diagnostic) ByCategory(category Category) []DiagnosticEntry {
	var entries []DiagnosticEntry
	for _, entry := range d.Entries {
		if entry.Category == category {
			entries = append(entries, entry)
		}
	}
	return entries
}

// LogValue renders the diagnostic for structured logging with sensitive values
// already redacted.
func (d Diagnostic) LogValue() slog.Value {
	attrs := make([]slog.Attr, 0, len(d.Entries))
	for _, entry := range d.Entries {
		attrs = append(attrs, slog.String(entry.Key, fmt.Sprintf("%s (%s)", entry.Value, entry.Source)))
	}
	return slog.GroupValue(attrs...)
}

// String renders the diagnostic as a stable, human-readable table.
func (d Diagnostic) String() string {
	var b strings.Builder
	sourceNames := make([]string, 0, len(d.Precedence))
	for _, source := range d.Precedence {
		sourceNames = append(sourceNames, string(source))
	}
	fmt.Fprintf(&b, "precedence: %s\n", strings.Join(sourceNames, " > "))
	width := 0
	for _, entry := range d.Entries {
		if len(entry.Key) > width {
			width = len(entry.Key)
		}
	}
	for _, entry := range d.Entries {
		fmt.Fprintf(&b, "%-*s  %-9s  %-12s  %s\n", width, entry.Key, entry.Category, entry.Source, entry.Value)
	}
	return b.String()
}
