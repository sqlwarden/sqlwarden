package token

import (
	"testing"
)

func TestGenerateUnique(t *testing.T) {
	plain1, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	plain2, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if plain1 == plain2 {
		t.Error("Generate produced duplicate plaintexts on successive calls")
	}
}

func TestHashDeterministic(t *testing.T) {
	input := "some-plaintext-token-value"
	h1 := Hash(input)
	h2 := Hash(input)
	if h1 != h2 {
		t.Errorf("Hash is not deterministic: %q != %q", h1, h2)
	}
}

func TestHashDifferentInputs(t *testing.T) {
	h1 := Hash("token-one")
	h2 := Hash("token-two")
	if h1 == h2 {
		t.Error("Hash produced same output for different inputs")
	}
}

func TestGenerateLengths(t *testing.T) {
	plain, hash, err := Generate()
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	// 32 bytes = 64 hex chars
	if len(plain) != 64 {
		t.Errorf("plaintext length = %d; want 64", len(plain))
	}
	// SHA-256 = 32 bytes = 64 hex chars
	if len(hash) != 64 {
		t.Errorf("hash length = %d; want 64", len(hash))
	}
}
