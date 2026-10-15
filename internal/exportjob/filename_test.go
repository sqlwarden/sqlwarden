package exportjob

import (
	"testing"
	"time"
)

func TestSafeFilename(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct{ in, want string }{
		{"", "query-export-20260102-030405.csv"},
		{"report", "report.csv"},
		{"report.csv", "report.csv"},
		{"../../etc/passwd", "passwd.csv"},
		{"a/b\\c?.txt", "b-c.csv"},
		{"  ..  ", "query-export-20260102-030405.csv"},
	}
	for _, tt := range tests {
		if got := safeFilename(tt.in, now); got != tt.want {
			t.Errorf("safeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	long := safeFilename("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now)
	if len(long) != 80+len(".csv") {
		t.Errorf("long name length = %d", len(long))
	}
}

func TestUniqueName(t *testing.T) {
	t.Parallel()
	if got := uniqueName("report.csv", 0); got != "report.csv" {
		t.Errorf("attempt 0 = %q", got)
	}
	if got := uniqueName("report.csv", 2); got != "report-3.csv" {
		t.Errorf("attempt 2 = %q", got)
	}
}
