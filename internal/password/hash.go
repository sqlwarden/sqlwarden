package password

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// DefaultHashCost is the bcrypt cost used for production password hashes.
const DefaultHashCost = 12

var hashCost = DefaultHashCost

func Hash(plaintextPassword string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), hashCost)
	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}

// SetHashCostForTesting overrides the package hash cost until the returned restore
// function is called. Tests use this to reduce bcrypt work without changing
// production behavior.
func SetHashCostForTesting(cost int) func() {
	prev := hashCost
	hashCost = cost
	return func() {
		hashCost = prev
	}
}

func Matches(plaintextPassword, hashedPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plaintextPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}
