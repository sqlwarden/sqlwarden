package exportjob

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqlwarden/internal/exports"
)

// NormalizedFormat trims, lowercases, and defaults an export format.
func NormalizedFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return exports.FormatCSV
	}
	return format
}

func safeFilename(name string, now time.Time) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "query-export-" + now.UTC().Format("20060102-150405")
	}
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.', r == ' ':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, " .-_")
	if name == "" {
		name = "query-export-" + now.UTC().Format("20060102-150405")
	}
	if len(name) > 80 {
		name = strings.TrimRight(name[:80], " .-_")
	}
	return name + ".csv"
}

func uniqueName(name string, attempt int) string {
	if attempt == 0 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s-%d%s", base, attempt+1, ext)
}
