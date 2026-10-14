package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/schema"
)

const (
	maxCompletionSQLBytes    = 1 << 20
	maxCompletionSuggestions = 200
)

var (
	// ErrCompletionUnsupported reports a driver without a SQL completer.
	ErrCompletionUnsupported = errors.New("query: SQL completion is not supported for this driver")
	// ErrCompletionTooLarge reports SQL above the 1 MiB completion limit.
	ErrCompletionTooLarge = errors.New("query: SQL exceeds the completion limit")
	// ErrVocabularyUnsupported reports an engine without a completion
	// vocabulary.
	ErrVocabularyUnsupported = errors.New("query: engine does not provide a completion vocabulary")
	ErrUnknownEngine         = errors.New("query: unknown engine")
)

// CompletionInputError reports a malformed completion request. Its message
// is safe to show to the caller and never contains SQL text.
type CompletionInputError struct {
	message string
}

func (e *CompletionInputError) Error() string { return e.message }

func completionInputError(message string) error { return &CompletionInputError{message: message} }

// Completer runs connectionless SQL completion against loaded metadata.
type Completer interface {
	CompleteWithMetadata(ctx context.Context, driver string, req completer.Request, loader completion.Loader) (completer.Result, completion.Outcome, error)
	Vocabulary(driver string) (completer.Vocabulary, error)
}

// CompleteInput is a completion request. CursorOffset counts UTF-16 code
// units, as editors report them; suggestion ranges are returned in the same
// units.
type CompleteInput struct {
	SQL          string
	CursorOffset int
	TriggerKind  string
	TriggerChar  string
}

// Completion is the result of a completion request.
type Completion struct {
	Suggestions       []completer.Suggestion `json:"suggestions"`
	Mode              string                 `json:"mode"`
	MetadataAvailable bool                   `json:"metadata_available"`
	MetadataStatus    string                 `json:"metadata_status"`
	MetadataLoaded    bool                   `json:"metadata_loaded"`
	Context           string                 `json:"context,omitempty"`
}

type CompletionIndexObject struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Score  int    `json:"score"`
}

type CompletionIndexColumn struct {
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Nullable bool   `json:"nullable"`
}

// CompletionIndexView is the cached metadata an editor completes against
// locally.
type CompletionIndexView struct {
	Version       string `json:"version"`
	DefaultSchema string `json:"default_schema"`
	// SearchSchemas orders the schemas unqualified relation names resolve
	// in; a database-level scope contributes "".
	SearchSchemas []string `json:"search_schemas"`
	// DefaultScopeRelationsListed is true when every relation folder of every
	// search scope has a cached listing.
	DefaultScopeRelationsListed bool                    `json:"default_scope_relations_listed"`
	ColumnScore                 int                     `json:"column_score"`
	Schemas                     []string                `json:"schemas"`
	Objects                     []CompletionIndexObject `json:"objects"`
	Columns                     []CompletionIndexColumn `json:"columns"`
}

func schemaMode(persistent bool) string {
	if persistent {
		return "persistent"
	}
	return "ephemeral"
}

// Complete suggests completions at the cursor. Cached metadata is always
// used; a live session in the caller's scope also lets it load the metadata
// the completer demands. A missing or foreign session is not an error.
func (s *Service) Complete(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, in CompleteInput) (Completion, error) {
	started := time.Now()
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return Completion{}, err
	}
	creq, err := completerRequest(ref, in)
	if err != nil {
		return Completion{}, err
	}
	if err := s.resolveSession(ctx, req); err != nil {
		return Completion{}, err
	}
	nav, err := s.navigatorConnection(ctx, req.conn)
	if err != nil {
		return Completion{}, err
	}
	out := Completion{
		Suggestions:    []completer.Suggestion{},
		Mode:           schemaMode(nav.Persistent),
		MetadataStatus: completion.MetadataUnavailable,
	}
	driver := req.conn.Driver
	var loader completion.Loader = staticCompletionLoader{view: metadata.NewCompletionView(metadata.Tree{}, nav.DefaultScope, "", nil, nil)}
	if tree, ok := s.navigatorTree(driver); ok {
		loader = navigatorCompletionLoader{
			navigator: s.deps.Navigator, logger: s.deps.Logger, conn: nav,
			driver: driver, tree: tree, live: s.navigatorLive(req),
		}
	}
	result, outcome, err := s.deps.Completion.CompleteWithMetadata(ctx, driver, creq, loader)
	if errors.Is(err, completion.ErrUnsupported) {
		return Completion{}, ErrCompletionUnsupported
	}
	if err != nil {
		return Completion{}, err
	}
	if outcome.Status == completion.MetadataDegraded {
		s.deps.Logger.WarnContext(ctx, "schema-aware SQL completion failed; retried without metadata",
			slog.Int64("connection_id", req.conn.ID), slog.String("driver", driver))
	}
	out.MetadataStatus = outcome.Status
	out.MetadataAvailable = outcome.Available && outcome.Status != completion.MetadataDegraded
	out.MetadataLoaded = outcome.Loaded
	if len(result.Suggestions) > maxCompletionSuggestions {
		result.Suggestions = result.Suggestions[:maxCompletionSuggestions]
	}
	for i := range result.Suggestions {
		result.Suggestions[i].ReplaceStart = byteOffsetToUTF16Offset(in.SQL, result.Suggestions[i].ReplaceStart)
		result.Suggestions[i].ReplaceEnd = byteOffsetToUTF16Offset(in.SQL, result.Suggestions[i].ReplaceEnd)
	}
	out.Suggestions = result.Suggestions
	out.Context = result.Context
	if out.Context == "" {
		out.Context = "any"
	}
	s.deps.Logger.DebugContext(ctx, "SQL completion returned",
		slog.Int64("connection_id", req.conn.ID),
		slog.String("driver", driver),
		slog.String("mode", out.Mode),
		slog.String("trigger_kind", string(creq.TriggerKind)),
		slog.String("metadata_status", out.MetadataStatus),
		slog.Int("rounds", outcome.Rounds),
		slog.Int("demand_count", outcome.Demands),
		slog.Int("loads_started", outcome.Report.Started),
		slog.Int("loads_joined", outcome.Report.Joined),
		slog.Int("loads_failed", outcome.Report.Failed),
		slog.Int("loads_capped", outcome.Report.Capped),
		slog.Bool("budget_exhausted", outcome.BudgetExhausted),
		slog.Int("suggestion_count", len(out.Suggestions)),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()),
	)
	return out, nil
}

// completerRequest validates the input and converts the cursor to a byte
// offset.
func completerRequest(ref access.ConnRef, in CompleteInput) (completer.Request, error) {
	if len(in.SQL) > maxCompletionSQLBytes {
		return completer.Request{}, ErrCompletionTooLarge
	}
	byteOffset, err := utf16OffsetToByteOffset(in.SQL, in.CursorOffset)
	if err != nil {
		return completer.Request{}, err
	}
	triggerKind := completer.TriggerKind(in.TriggerKind)
	if triggerKind == "" {
		triggerKind = completer.TriggerInvoked
	}
	if triggerKind != completer.TriggerInvoked && triggerKind != completer.TriggerAutomatic {
		return completer.Request{}, completionInputError("trigger_kind must be invoked or automatic")
	}
	if utf8.RuneCountInString(in.TriggerChar) > 1 {
		return completer.Request{}, completionInputError("trigger_character must contain at most one character")
	}
	return completer.Request{
		SQL: in.SQL, CursorOffset: byteOffset, ConnectionID: strconv.FormatInt(ref.ConnectionID, 10),
		TriggerKind: triggerKind, TriggerChar: in.TriggerChar,
	}, nil
}

// CompletionIndex returns the cached completion metadata of the connection.
// A live session in the caller's scope lets the navigator fill the default
// scope first.
func (s *Service) CompletionIndex(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) (CompletionIndexView, error) {
	started := time.Now()
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return CompletionIndexView{}, err
	}
	if err := s.resolveSession(ctx, req); err != nil {
		return CompletionIndexView{}, err
	}
	nav, err := s.navigatorConnection(ctx, req.conn)
	if err != nil {
		return CompletionIndexView{}, err
	}
	var view *metadata.CompletionView
	if tree, ok := s.navigatorTree(req.conn.Driver); ok {
		view, err = s.deps.Navigator.CompletionView(ctx, nav, tree, s.navigatorLive(req))
		if err != nil {
			return CompletionIndexView{}, err
		}
	} else {
		view = metadata.NewCompletionView(metadata.Tree{}, nav.DefaultScope, "", nil, nil)
	}
	out, err := projectCompletionIndex(view)
	if err != nil {
		return CompletionIndexView{}, err
	}
	s.deps.Logger.DebugContext(ctx, "completion index returned",
		slog.Int64("connection_id", req.conn.ID),
		slog.String("mode", schemaMode(nav.Persistent)),
		slog.Int("schema_count", len(out.Schemas)),
		slog.Int("object_count", len(out.Objects)),
		slog.Int("column_count", len(out.Columns)),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()),
	)
	return out, nil
}

// CompletionVocabulary returns the static keyword and function vocabulary of
// an engine. Any signed-in account may read it.
func (s *Service) CompletionVocabulary(ctx context.Context, p access.Principal, engineID string) (completer.Vocabulary, error) {
	if p.Subject.Kind != access.SubjectAccount || p.Subject.ID == 0 {
		return completer.Vocabulary{}, ErrForbidden
	}
	if _, ok := s.deps.Engines.Describe(engineID); !ok {
		return completer.Vocabulary{}, ErrUnknownEngine
	}
	vocabulary, err := s.deps.Completion.Vocabulary(engineID)
	if errors.Is(err, completion.ErrUnsupported) {
		return completer.Vocabulary{}, ErrVocabularyUnsupported
	}
	if err != nil {
		return completer.Vocabulary{}, err
	}
	return vocabulary, nil
}

// navigatorCompletionLoader adapts the schema navigator to the completion
// loader hook. A nil live inspector means there is no usable session, so
// Ensure never queries the target.
type navigatorCompletionLoader struct {
	navigator *schema.Navigator
	logger    *slog.Logger
	conn      schema.Connection
	driver    string
	tree      metadata.Tree
	live      schema.Live
}

func (l navigatorCompletionLoader) View(ctx context.Context) (*metadata.CompletionView, error) {
	view, err := l.navigator.CompletionView(ctx, l.conn, l.tree, l.live)
	if err != nil {
		l.logger.WarnContext(ctx, "completion metadata unavailable",
			slog.Int64("connection_id", l.conn.ID),
			slog.String("driver", l.driver),
			slog.Any("error", err),
		)
	}
	return view, err
}

func (l navigatorCompletionLoader) Live() bool { return l.live != nil }

func (l navigatorCompletionLoader) Ensure(ctx context.Context, demands []metadata.Demand) completion.LoadReport {
	report := l.navigator.EnsureForCompletion(ctx, l.conn, l.tree, l.live, demands)
	if report.Failed > 0 {
		l.logger.WarnContext(ctx, "completion metadata loads failed",
			slog.Int64("connection_id", l.conn.ID),
			slog.String("driver", l.driver),
			slog.Int("loads_failed", report.Failed),
		)
	}
	return completion.LoadReport(report)
}

type staticCompletionLoader struct{ view *metadata.CompletionView }

func (l staticCompletionLoader) View(context.Context) (*metadata.CompletionView, error) {
	return l.view, nil
}
func (staticCompletionLoader) Live() bool { return false }
func (staticCompletionLoader) Ensure(context.Context, []metadata.Demand) completion.LoadReport {
	return completion.LoadReport{}
}

func utf16OffsetToByteOffset(text string, offset int) (int, error) {
	if offset < 0 {
		return 0, completionInputError("cursor_offset must not be negative")
	}
	units := 0
	for byteOffset, r := range text {
		if units == offset {
			return byteOffset, nil
		}
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > offset {
			return 0, completionInputError("cursor_offset splits a UTF-16 surrogate pair")
		}
		units += width
	}
	if units == offset {
		return len(text), nil
	}
	return 0, completionInputError("cursor_offset is outside SQL text")
}

func byteOffsetToUTF16Offset(text string, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(text) {
		byteOffset = len(text)
	}
	units := 0
	for i, r := range text {
		if i >= byteOffset {
			break
		}
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	return units
}

func relationKindsOf(tree metadata.Tree, scope metadata.ScopePath) []string {
	node, ok := tree.Node(tree.NodeKindOf(scope))
	if !ok || !node.Scope {
		return nil
	}
	var kinds []string
	for _, folder := range node.Folders {
		for _, kind := range append([]string{folder.Child}, folder.MixedKinds...) {
			if child, ok := tree.Node(kind); ok && child.Relational && !slices.Contains(kinds, kind) {
				kinds = append(kinds, kind)
			}
		}
	}
	return kinds
}

// searchRelationScopes keeps the search scopes that hold relations. A
// fallback scope is dropped once its parent's listing is loaded without it.
func searchRelationScopes(view *metadata.CompletionView) []metadata.ScopePath {
	var out []metadata.ScopePath
	for i, scope := range view.SearchScopes() {
		if i > 0 {
			if siblings, loaded := view.Scopes(scope.Parent()); loaded && !slices.Contains(siblings, scope) {
				continue
			}
		}
		if len(relationKindsOf(view.Tree(), scope)) > 0 {
			out = append(out, scope)
		}
	}
	return out
}

func searchRelationsListed(view *metadata.CompletionView, scopes []metadata.ScopePath) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if len(view.ObjectDemands(scope, relationKindsOf(view.Tree(), scope)...)) > 0 {
			return false
		}
	}
	return true
}

func projectCompletionIndex(view *metadata.CompletionView) (CompletionIndexView, error) {
	scopes := searchRelationScopes(view)
	searchSchemas := []string{}
	for _, scope := range scopes {
		if name := scope.Name("schema"); !slices.Contains(searchSchemas, name) {
			searchSchemas = append(searchSchemas, name)
		}
	}
	out := CompletionIndexView{
		DefaultSchema:               view.DefaultScope().Name("schema"),
		SearchSchemas:               searchSchemas,
		DefaultScopeRelationsListed: searchRelationsListed(view, scopes),
		ColumnScore:                 completer.KindScore("column"),
		Schemas:                     []string{},
		Objects:                     []CompletionIndexObject{},
		Columns:                     []CompletionIndexColumn{},
	}
	schemas := map[string]struct{}{}
	for _, path := range view.ScopePaths() {
		if schema := path.Name("schema"); schema != "" {
			schemas[schema] = struct{}{}
		}
	}
	for _, ref := range view.Refs() {
		schema := ref.Scope.Name("schema")
		out.Objects = append(out.Objects, CompletionIndexObject{Schema: schema, Name: ref.Name, Kind: ref.Kind, Score: completer.KindScore(ref.Kind)})
		columns, _ := view.Columns(ref)
		for _, column := range columns {
			out.Columns = append(out.Columns, CompletionIndexColumn{
				Schema: schema, Table: ref.Name, Name: column.Name, Type: column.DataType, Nullable: column.Nullable,
			})
		}
	}
	for schema := range schemas {
		out.Schemas = append(out.Schemas, schema)
	}
	sort.Strings(out.Schemas)
	raw, err := json.Marshal(out)
	if err != nil {
		return CompletionIndexView{}, fmt.Errorf("hash completion index: %w", err)
	}
	sum := sha256.Sum256(raw)
	out.Version = hex.EncodeToString(sum[:])
	return out, nil
}
