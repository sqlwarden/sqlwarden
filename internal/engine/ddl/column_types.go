package ddl

import (
	"regexp"
	"strconv"
	"strings"
)

// ParameterizedColumnType describes a closed numeric type grammar. Names and
// suffixes come from the driver; only range-checked integers come from input.
type ParameterizedColumnType struct {
	Name       string                `json:"name"`
	Suffix     string                `json:"suffix,omitempty"`
	Parameters []ColumnTypeParameter `json:"parameters"`
}

type ColumnTypeParameter struct {
	Name     string `json:"name"`
	Min      int    `json:"min"`
	Max      int    `json:"max"`
	Optional bool   `json:"optional,omitempty"`
}

func (s Spec) CanonicalColumnType(value string) (string, bool) {
	if canonical, ok := CanonicalColumnType(value, s.ColumnTypes); ok {
		return canonical, true
	}
	value = strings.TrimSpace(value)
	open := strings.IndexByte(value, '(')
	close := strings.IndexByte(value, ')')
	if open >= 1 && close > open {
		name := strings.TrimSpace(value[:open])
		suffix := strings.Join(strings.Fields(value[close+1:]), " ")
		parts := strings.Split(value[open+1:close], ",")
		for _, rule := range s.ParameterizedColumnTypes {
			if !strings.EqualFold(name, rule.Name) || !strings.EqualFold(suffix, rule.Suffix) || len(parts) > len(rule.Parameters) {
				continue
			}
			canonical := make([]string, len(parts))
			valid := true
			for i, parameter := range rule.Parameters {
				if i >= len(parts) {
					valid = valid && parameter.Optional
					continue
				}
				part := strings.TrimSpace(parts[i])
				number, err := strconv.Atoi(part)
				if err != nil || number < parameter.Min || number > parameter.Max {
					valid = false
					break
				}
				canonical[i] = strconv.Itoa(number)
			}
			if valid {
				result := rule.Name + "(" + strings.Join(canonical, ",") + ")"
				if rule.Suffix != "" {
					result += " " + rule.Suffix
				}
				return result, true
			}
			// The name (and suffix) matched a known rule but the arguments
			// didn't range-check: this is a malformed instance of a known
			// type, not an unknown extension type, so it must not fall
			// through to the custom-type escape hatch below.
			return "", false
		}
	}
	if s.AllowCustomColumnTypes && !s.namesKnownColumnType(value) && ValidCustomColumnTypeSyntax(value) {
		return value, true
	}
	return "", false
}

// namesKnownColumnType reports whether value's leading type name already
// belongs to this Spec's closed vocabulary, so CanonicalColumnType can tell
// "malformed known type" (reject) apart from "unrecognized extension type"
// (defer to the database when AllowCustomColumnTypes is set).
func (s Spec) namesKnownColumnType(value string) bool {
	name := value
	if open := strings.IndexByte(value, '('); open >= 1 {
		name = value[:open]
	} else if space := strings.IndexByte(value, ' '); space >= 1 {
		name = value[:space]
	}
	name = strings.TrimSpace(name)
	for _, rule := range s.ParameterizedColumnTypes {
		if strings.EqualFold(name, rule.Name) {
			return true
		}
	}
	for _, candidate := range s.ColumnTypes {
		if fields := strings.Fields(candidate); len(fields) > 0 && strings.EqualFold(name, fields[0]) {
			return true
		}
	}
	return false
}

// customColumnTypePattern is deliberately conservative: it accepts the shapes
// extension-added types actually take (bare or schema-qualified names,
// multi-word suffixes, a parenthesized argument list, trailing array
// brackets) and nothing else, so a value that matches is always safe to
// interpolate directly into generated DDL without further quoting.
var customColumnTypePattern = regexp.MustCompile(
	`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?(?: [A-Za-z_][A-Za-z0-9_]*)*(?:\([A-Za-z0-9_, ]+\))?(?:\[\])*$`,
)

const maxCustomColumnTypeLength = 128

// ValidCustomColumnTypeSyntax reports whether value is safe to pass through
// to the database as a column type when a driver's Spec.AllowCustomColumnTypes
// is set. It checks only syntax; the database decides whether the type exists.
func ValidCustomColumnTypeSyntax(value string) bool {
	return value != "" && len(value) <= maxCustomColumnTypeLength && customColumnTypePattern.MatchString(value)
}
