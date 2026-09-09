package ddl

import (
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
	if open < 1 || close <= open {
		return "", false
	}
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
	}
	return "", false
}
