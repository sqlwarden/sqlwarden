package password

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndMatches(t *testing.T) {
	restore := SetHashCostForTesting(bcrypt.MinCost)
	defer restore()

	hashed, err := Hash("testPass123!")
	if err != nil {
		t.Fatal(err)
	}
	if hashed == "" {
		t.Fatal("expected hash output")
	}

	ok, err := Matches("testPass123!", hashed)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected password to match generated hash")
	}

	ok, err = Matches("wrong-pass", hashed)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected mismatched password to return false")
	}
}

func TestSetHashCostForTestingRestoresPreviousCost(t *testing.T) {
	restoreOuter := SetHashCostForTesting(bcrypt.MinCost)
	defer restoreOuter()

	if hashCost != bcrypt.MinCost {
		t.Fatalf("expected hash cost override to be applied, got %d", hashCost)
	}

	restoreInner := SetHashCostForTesting(bcrypt.DefaultCost)
	if hashCost != bcrypt.DefaultCost {
		t.Fatalf("expected nested override to be applied, got %d", hashCost)
	}

	restoreInner()
	if hashCost != bcrypt.MinCost {
		t.Fatalf("expected nested restore to return previous test value, got %d", hashCost)
	}
}
