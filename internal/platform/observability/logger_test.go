package observability

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoggerLevelChangesAtRuntime(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger("json", &buf)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden")
	if buf.Len() != 0 {
		t.Fatalf("debug log emitted at info level: %s", buf.String())
	}
	if err := SetLevel(logger, "debug"); err != nil {
		t.Fatal(err)
	}
	logger.Debug("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("debug log missing after live level change: %s", buf.String())
	}
	if err := SetLevel(logger, "error"); err != nil {
		t.Fatal(err)
	}
	before := buf.Len()
	logger.Info("hidden again")
	if buf.Len() != before {
		t.Fatalf("info log emitted at error level: %s", buf.String())
	}
}

func TestSetLevelChangesEnabledLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger("json", &buf)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden")
	if err := SetLevel(logger, "debug"); err != nil {
		t.Fatal(err)
	}
	logger.Debug("shown")
	if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "shown") {
		t.Fatalf("unexpected log output: %s", buf.String())
	}
}
