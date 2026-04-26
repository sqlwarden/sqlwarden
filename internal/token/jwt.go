package token

import (
	"errors"
	"time"

	"github.com/pascaldekloe/jwt"
)

const DefaultAccessTokenTTL = 24 * time.Hour

// Claims holds the parsed fields from an access token.
type Claims struct {
	AccountID string
	Email     string
	Name      string
}

// Issue signs an HS256 access token with the default lifetime.
// Returns: (tokenString, expiresAt, error)
func Issue(accountID, email, name, secretKey string) (string, time.Time, error) {
	return IssueWithTTL(accountID, email, name, secretKey, DefaultAccessTokenTTL)
}

// IssueWithTTL signs an HS256 access token with a caller-provided lifetime.
// Returns: (tokenString, expiresAt, error)
func IssueWithTTL(accountID, email, name, secretKey string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = DefaultAccessTokenTTL
	}
	expiresAt := time.Now().Add(ttl)

	c := &jwt.Claims{
		Registered: jwt.Registered{
			Subject: accountID,
			Expires: jwt.NewNumericTime(expiresAt),
		},
		Set: map[string]any{
			"email": email,
			"name":  name,
		},
	}

	tokenBytes, err := c.HMACSign(jwt.HS256, []byte(secretKey))
	if err != nil {
		return "", time.Time{}, err
	}

	return string(tokenBytes), expiresAt, nil
}

// Verify parses and validates the token signature and expiry.
// Returns Claims on success.
func Verify(tokenStr, secretKey string) (Claims, error) {
	claims, err := jwt.HMACCheck([]byte(tokenStr), []byte(secretKey))
	if err != nil {
		return Claims{}, err
	}

	if !claims.Valid(time.Now()) {
		return Claims{}, errors.New("token: expired or not yet valid")
	}

	email, _ := claims.Set["email"].(string)
	name, _ := claims.Set["name"].(string)

	return Claims{
		AccountID: claims.Subject,
		Email:     email,
		Name:      name,
	}, nil
}
