package dbengine

// EngineID is the canonical identifier for a database engine (e.g. "postgres").
type EngineID string

// EngineDescriptor is the static identity of an engine, safe to serialize and
// to report without opening a target connection.
type EngineDescriptor struct {
	ID          EngineID `json:"id"`
	DisplayName string   `json:"display_name"`
	Dialect     Dialect  `json:"dialect"`
}
