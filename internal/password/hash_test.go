package password

import (
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestHash(t *testing.T) {
	t.Run("Returns valid bcrypt hash with cost of 12", func(t *testing.T) {
		hashedPassword, err := Hash("superS3cret")
		assert.Nil(t, err)
		assert.MatchesRegexp(t, hashedPassword, `^\$2a\$12\$[./0-9A-Za-z]{53}$`)
	})
}

func TestMatches(t *testing.T) {
	t.Run("Returns true when password matches hash", func(t *testing.T) {

		hash := "$2a$12$2.z81tzl7RCi6QrX3thr.uYG68lLAB4dBoRqqqVDEvIdfopMuAMyu"

		match, err := Matches("s3cretP455word", hash)
		assert.Nil(t, err)
		assert.Equal(t, match, true)
	})

	t.Run("Returns false when password does not match hash", func(t *testing.T) {

		hash := "$2a$12$2.z81tzl7RCi6QrX3thr.uYG68lLAB4dBoRqqqVDEvIdfopMuAMyu"

		match, err := Matches("wrongS3cretP455word", hash)
		assert.Nil(t, err)
		assert.Equal(t, match, false)
	})

	t.Run("Returns error when hash format is invalid", func(t *testing.T) {
		matches, err := Matches("s3cretP455word", "not-a-bcrypt-hash")
		assert.NotNil(t, err)
		assert.Equal(t, matches, false)
	})
}
