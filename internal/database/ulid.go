package database

import (
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	entropy     = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
	entropyOnce sync.Mutex
)

func newID() string {
	entropyOnce.Lock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	entropyOnce.Unlock()
	return id.String()
}
