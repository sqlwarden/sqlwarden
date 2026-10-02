package completer

import (
	"strings"
	"unicode"
)

// MatchTier classifies how closely a completion label matches a typed prefix.
// Higher tiers rank first; engine-provided context scores break ties.
func MatchTier(label, prefix string) int {
	prefix = strings.ToLower(strings.Trim(strings.TrimSpace(prefix), "`\""))
	if prefix == "" {
		return 0
	}
	label = strings.ToLower(label)
	if label == prefix {
		return 5
	}
	if strings.HasPrefix(label, prefix) {
		return 4
	}
	if identifierSegmentHasPrefix(label, prefix) {
		return 3
	}
	if strings.Contains(label, prefix) {
		return 2
	}
	if len([]rune(prefix)) >= 3 && isSubsequence(label, prefix) {
		return 1
	}
	return 0
}

func identifierSegmentHasPrefix(label, prefix string) bool {
	segmentStart := true
	for i, r := range label {
		if segmentStart && strings.HasPrefix(label[i:], prefix) {
			return true
		}
		segmentStart = !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	return false
}

func isSubsequence(label, prefix string) bool {
	remaining := []rune(prefix)
	for _, r := range label {
		if len(remaining) > 0 && r == remaining[0] {
			remaining = remaining[1:]
		}
	}
	return len(remaining) == 0
}

// kindScores ranks suggestion kinds so that, at equal match quality, columns
// come first, then relations (tables before views), then containers, then the
// remaining catalog objects. Every engine shares it so the backend and the
// client-side index order candidates the same way.
var kindScores = map[string]int{
	"column":            100,
	"table":             90,
	"view":              85,
	"materialized_view": 84,
	"foreign_table":     83,
	"external_table":    83,
	"synonym":           82,
	"database":          70,
	"schema":            70,
	"sequence":          65,
	"function":          60,
	"procedure":         58,
	"index":             55,
	"trigger":           54,
	"event":             53,
	"engine":            45,
	"charset":           45,
	"keyword":           40,
	"type":              35,
}

// KindScore is the context score for a suggestion kind; unknown kinds rank
// below keywords.
func KindScore(kind string) int {
	if score, ok := kindScores[kind]; ok {
		return score
	}
	return 20
}
