package architecture

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunityCommandDependencyGraphExcludesEnterprise(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cmd/sqlwarden")
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == "github.com/sqlwarden/ee" || strings.HasPrefix(dependency, "github.com/sqlwarden/ee/") {
			t.Fatalf("Community command depends on Enterprise package %q", dependency)
		}
	}
}

func TestEveryEnterpriseGoFileHasBuildConstraint(t *testing.T) {
	root := filepath.Join(repoRoot(t), "ee")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		found := false
		for line := 0; line < 5 && scanner.Scan(); line++ {
			if strings.TrimSpace(scanner.Text()) == "//go:build enterprise" {
				found = true
				break
			}
		}
		if !found {
			rel, _ := filepath.Rel(repoRoot(t), path)
			t.Errorf("%s has no enterprise build constraint", filepath.ToSlash(rel))
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseMigrationsOnlyCreatePrefixedTables(t *testing.T) {
	root := filepath.Join(repoRoot(t), "ee", "assets")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		words := strings.Fields(string(contents))
		for index, word := range words {
			if strings.EqualFold(word, "TABLE") && index+1 < len(words) {
				name := strings.Trim(words[index+1], "`\"")
				if strings.EqualFold(name, "IF") {
					continue
				}
				if !strings.HasPrefix(name, "ee_") {
					t.Errorf("%s creates non-Enterprise table %q", path, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	enterpriseSource, err := os.ReadFile(filepath.Join(repoRoot(t), "ee", "enterprise.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enterpriseSource), `"schema_migrations_ee"`) {
		t.Fatal("Enterprise migration stream does not use schema_migrations_ee")
	}
}
