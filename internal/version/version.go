package version

import (
	"fmt"
	"runtime/debug"
)

// These variables are set via ldflags during build
var (
	version = ""
	commit  = ""
	date    = ""
)

func Get() string {
	// If version was set via ldflags (by goreleaser), use it
	if version != "" {
		return version
	}

	// Otherwise, try to get it from build info
	bi, ok := debug.ReadBuildInfo()
	if ok {
		return bi.Main.Version
	}

	return "unavailable"
}

func GetRevision() string {
	// If commit was set via ldflags (by goreleaser), use it
	if commit != "" {
		return commit
	}

	var revision string
	var modified bool

	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					modified = true
				}
			}
		}
	}

	if revision == "" {
		return "unavailable"
	}

	if modified {
		return fmt.Sprintf("%s+dirty", revision)
	}

	return revision
}

// GetBuildDate returns the build date
func GetBuildDate() string {
	if date != "" {
		return date
	}
	return "unavailable"
}
