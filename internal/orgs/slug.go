package orgs

import "strings"

// MaxSlugLength is the longest organization slug the API accepts.
const MaxSlugLength = 64

// Slugify converts a name to a URL-safe slug.
func Slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	s = strings.Trim(s, "-")
	if len(s) > MaxSlugLength {
		s = s[:MaxSlugLength]
	}
	return s
}

// ValidSlug returns true if s contains only lowercase letters, digits, and hyphens.
func ValidSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}
