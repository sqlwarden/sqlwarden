package orgs

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Acme Corp", "acme-corp"},
		{"  Data_Team  ", "data-team"},
		{"Ünïcode & Co!", "ncode--co"},
		{"---", ""},
		{strings.Repeat("a", 70), strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidSlug(t *testing.T) {
	for _, s := range []string{"local", "a-1", "x"} {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false", s)
		}
	}
	for _, s := range []string{"", "Upper", "under_score", "sp ace"} {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true", s)
		}
	}
}
