package completioncore

// Position names the grammar role the cursor sits in, so clients can rank
// schema objects above keywords when only an identifier is valid.
const (
	PositionColumn   = "column"
	PositionRelation = "relation"
	PositionValue    = "value"
	PositionKeyword  = "keyword"
	PositionAny      = "any"
)

// Context is the completion-position classification for one request.
type Context struct {
	Position string
}
