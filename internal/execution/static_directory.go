package execution

import (
	"context"
	"strings"
	"time"
)

// StaticSessionDirectory routes every handle to one configured connector. Its
// mutations are deliberately no-ops because a single destination requires no
// shared ownership state. Configuration validation must prohibit its use with
// more than one connector replica.
type StaticSessionDirectory struct {
	address string
}

// NewStaticSessionDirectory returns a directory pinned to connectorAddress.
func NewStaticSessionDirectory(connectorAddress string) *StaticSessionDirectory {
	return &StaticSessionDirectory{address: strings.TrimSpace(connectorAddress)}
}

// Put is a no-op for a directory with one possible owner.
func (d *StaticSessionDirectory) Put(context.Context, DirectoryRecord) error { return nil }

// Get resolves every handle to the configured connector address.
func (d *StaticSessionDirectory) Get(_ context.Context, handle SessionHandle) (DirectoryRecord, bool, error) {
	if d.address == "" {
		return DirectoryRecord{}, false, nil
	}
	return DirectoryRecord{
		Handle: handle, OwnerRuntimeID: "connector", RoutingAddress: d.address,
		ProtocolVersion: ProtocolVersion,
	}, true, nil
}

// Renew is a no-op for a static route.
func (d *StaticSessionDirectory) Renew(context.Context, SessionHandle, time.Time) error { return nil }

// Delete is a no-op for a static route.
func (d *StaticSessionDirectory) Delete(context.Context, SessionHandle) error { return nil }

var _ SessionDirectory = (*StaticSessionDirectory)(nil)
