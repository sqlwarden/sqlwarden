package version

import (
	"runtime/debug"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestGet(t *testing.T) {
	t.Run("Returns version from ldflags or build info", func(t *testing.T) {
		v := Get()
		assert.True(t, v != "")

		// When built with goreleaser ldflags, version will be set
		// When built normally, it falls back to build info or "unavailable"
		if version != "" {
			assert.Equal(t, v, version)
		} else {
			expectedVersion := "unavailable"
			bi, ok := debug.ReadBuildInfo()
			if ok && bi.Main.Version != "" {
				expectedVersion = bi.Main.Version
			}
			assert.Equal(t, v, expectedVersion)
		}
	})
}

func TestGetRevision(t *testing.T) {
	t.Run("Returns revision from ldflags or VCS info", func(t *testing.T) {
		rev := GetRevision()
		assert.True(t, rev != "")

		// When built with goreleaser ldflags, commit will be set
		// When built with VCS info, will return commit hash
		// Otherwise returns "unavailable"
		if commit != "" {
			assert.Equal(t, rev, commit)
		} else {
			assert.True(t, rev == "unavailable" || len(rev) >= 7)
		}
	})
}

func TestGetBuildDate(t *testing.T) {
	t.Run("Returns build date from ldflags or unavailable", func(t *testing.T) {
		buildDate := GetBuildDate()
		assert.True(t, buildDate != "")

		// When built with goreleaser ldflags, date will be set
		// Otherwise returns "unavailable"
		if date != "" {
			assert.Equal(t, buildDate, date)
		} else {
			assert.Equal(t, buildDate, "unavailable")
		}
	})
}
