package contracttest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDownstreamDistributionBuildsOutsideTheCommunityModule(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(current))
	temporary := t.TempDir()
	source, err := os.ReadFile(filepath.Join(root, "contracttest", "downstream", "contract_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	goMod := []byte("module example.com/sqlwarden-enterprise-contract\n\ngo 1.26.5\n\nrequire github.com/sqlwarden v0.0.0\n\nreplace github.com/sqlwarden => " + root + "\n")
	if err := os.WriteFile(filepath.Join(temporary, "go.mod"), goMod, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "contract_test.go"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "-mod=mod", "./...")
	command.Dir = temporary
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("downstream build failed: %v\n%s", err, output)
	}
}
