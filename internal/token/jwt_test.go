package token

import (
	"testing"
	"time"

	"github.com/pascaldekloe/jwt"
)

const testSecret = "test-secret-key-for-unit-tests"

func TestIssueVerifyRoundTrip(t *testing.T) {
	tokenStr, expiresAt, err := Issue("acc-123", "user@example.com", "Test User", testSecret)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("Issue returned empty token string")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("Issue returned expiry in the past")
	}
	if time.Until(expiresAt) < 23*time.Hour+59*time.Minute || time.Until(expiresAt) > 24*time.Hour+time.Minute {
		t.Fatalf("Issue expiry = %s, want about 24h", time.Until(expiresAt))
	}

	claims, err := Verify(tokenStr, testSecret)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if claims.AccountID != "acc-123" {
		t.Errorf("AccountID = %q; want %q", claims.AccountID, "acc-123")
	}
	if claims.Email != "user@example.com" {
		t.Errorf("Email = %q; want %q", claims.Email, "user@example.com")
	}
	if claims.Name != "Test User" {
		t.Errorf("Name = %q; want %q", claims.Name, "Test User")
	}
}

func TestIssueWithTTLUsesCustomLifetime(t *testing.T) {
	_, expiresAt, err := IssueWithTTL("acc-ttl", "ttl@example.com", "TTL User", testSecret, 2*time.Hour)
	if err != nil {
		t.Fatalf("IssueWithTTL returned error: %v", err)
	}

	if time.Until(expiresAt) < 119*time.Minute || time.Until(expiresAt) > 121*time.Minute {
		t.Fatalf("IssueWithTTL expiry = %s, want about 2h", time.Until(expiresAt))
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	// Construct a token with a past expiry directly via the jwt package.
	c := &jwt.Claims{
		Registered: jwt.Registered{
			Subject: "acc-expired",
			Expires: jwt.NewNumericTime(time.Now().Add(-1 * time.Minute)),
		},
		Set: map[string]any{
			"email": "old@example.com",
			"name":  "Old User",
		},
	}
	tokenBytes, err := c.HMACSign(jwt.HS256, []byte(testSecret))
	if err != nil {
		t.Fatalf("HMACSign error: %v", err)
	}

	_, err = Verify(string(tokenBytes), testSecret)
	if err == nil {
		t.Fatal("Verify should return error for expired token, got nil")
	}
}

func TestVerifyTamperedToken(t *testing.T) {
	tokenStr, _, err := Issue("acc-456", "tamper@example.com", "Tamper", testSecret)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	// Flip a byte in the middle of the token.
	b := []byte(tokenStr)
	mid := len(b) / 2
	b[mid] ^= 0xFF
	tampered := string(b)

	_, err = Verify(tampered, testSecret)
	if err == nil {
		t.Fatal("Verify should return error for tampered token, got nil")
	}
}

func TestVerifyWrongSecretKey(t *testing.T) {
	tokenStr, _, err := Issue("acc-789", "secret@example.com", "Secret User", testSecret)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	_, err = Verify(tokenStr, "wrong-secret-key")
	if err == nil {
		t.Fatal("Verify should return error for wrong secret key, got nil")
	}
}
