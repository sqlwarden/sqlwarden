package metadata

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

var navigatorIconEntry = regexp.MustCompile(`(?m)^\s+([a-z_]+): \{ icon: '`)

func TestKnownIconsMatchFrontendRegistry(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "frontend", "src", "components", "ide", "navigator", "icons.ts"))
	if err != nil {
		t.Fatal(err)
	}

	var frontend []string
	for _, m := range navigatorIconEntry.FindAllStringSubmatch(string(src), -1) {
		frontend = append(frontend, m[1])
	}
	var backend []string
	for token := range KnownIcons {
		backend = append(backend, token)
	}
	slices.Sort(frontend)
	slices.Sort(backend)
	assert.Equal(t, frontend, backend)
}
