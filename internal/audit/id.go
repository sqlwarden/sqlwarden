package audit

import (
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Audit identities are ULIDs so an event's identity sorts in creation order.
// The chain a tamper-evidence decorator builds relies on that ordering to read
// records back in the order they were appended.
var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
)

func newEventID() string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
