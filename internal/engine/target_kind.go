package engine

// TargetKind classifies the host resource a connection string addresses.
// Shared policy code uses the capability instead of branching on engine IDs.
type TargetKind string

const (
	TargetKindLocal    TargetKind = "local"
	TargetKindInMemory TargetKind = "in_memory"
)

// TargetClassifier is implemented by engines whose targets need policy beyond
// the default behavior.
type TargetClassifier interface {
	TargetKind(dsn string) TargetKind
}
