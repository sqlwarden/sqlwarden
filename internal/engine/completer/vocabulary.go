package completer

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

func NewVocabulary(dialect string, suggestions []Suggestion) Vocabulary {
	seen := make(map[string]bool)
	items := make([]Suggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		key := strings.ToLower(suggestion.Kind + "\x00" + suggestion.Label)
		if suggestion.Label == "" || seen[key] {
			continue
		}
		seen[key] = true
		suggestion.ReplaceStart = 0
		suggestion.ReplaceEnd = 0
		items = append(items, suggestion)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})
	hash := sha256.New()
	_, _ = hash.Write([]byte(dialect))
	for _, item := range items {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(item.Kind))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(item.Label))
	}
	return Vocabulary{
		Dialect: dialect, Version: hex.EncodeToString(hash.Sum(nil))[:16], Suggestions: items,
	}
}
