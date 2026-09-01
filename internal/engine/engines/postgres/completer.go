package postgres

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	pgcatalog "github.com/bytebase/omni/pg/catalog"
	pgparser "github.com/bytebase/omni/pg/parser"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/completioncore"
	corepostgres "github.com/sqlwarden/internal/engine/completioncore/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
)

const preparedCompletionCatalogs = 32

var (
	_                               completer.Completer          = (*postgresDriver)(nil)
	_                               completer.CatalogInvalidator = (*postgresDriver)(nil)
	_                               completer.VocabularyProvider = (*postgresDriver)(nil)
	pgCompletionCatalogCache                                     = completer.NewPreparedCache[*pgcatalog.Catalog](preparedCompletionCatalogs)
	pgSchemaIndexCache                                           = completer.NewPreparedCache[*metadata.Index](preparedCompletionCatalogs)
	pgVocabularyOnce                sync.Once
	pgVocabulary                    completer.Vocabulary
	pgSafeType                      = regexp.MustCompile(`^[A-Za-z0-9_ ."(),\[\]]+$`)
	pgReservedCompletionIdentifiers = map[string]struct{}{
		"all": {}, "analyse": {}, "analyze": {}, "and": {}, "any": {}, "array": {}, "as": {}, "asc": {},
		"asymmetric": {}, "authorization": {}, "binary": {}, "both": {}, "case": {}, "cast": {}, "check": {},
		"collate": {}, "collation": {}, "column": {}, "concurrently": {}, "constraint": {}, "create": {},
		"cross": {}, "current_catalog": {}, "current_date": {}, "current_role": {}, "current_schema": {},
		"current_time": {}, "current_timestamp": {}, "current_user": {}, "default": {}, "deferrable": {},
		"desc": {}, "distinct": {}, "do": {}, "else": {}, "end": {}, "except": {}, "false": {}, "fetch": {},
		"for": {}, "foreign": {}, "freeze": {}, "from": {}, "full": {}, "grant": {}, "group": {}, "having": {},
		"ilike": {}, "in": {}, "initially": {}, "inner": {}, "intersect": {}, "into": {}, "is": {}, "isnull": {},
		"join": {}, "lateral": {}, "leading": {}, "left": {}, "like": {}, "limit": {}, "localtime": {},
		"localtimestamp": {}, "natural": {}, "not": {}, "notnull": {}, "null": {}, "offset": {}, "on": {},
		"only": {}, "or": {}, "order": {}, "outer": {}, "overlaps": {}, "placing": {}, "primary": {},
		"references": {}, "returning": {}, "right": {}, "select": {}, "session_user": {}, "similar": {},
		"some": {}, "symmetric": {}, "table": {}, "tablesample": {}, "then": {}, "to": {}, "trailing": {},
		"true": {}, "union": {}, "unique": {}, "user": {}, "using": {}, "variadic": {}, "verbose": {},
		"when": {}, "where": {}, "window": {}, "with": {},
	}
)

func (d *postgresDriver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("postgres completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var catalog *pgcatalog.Catalog
	var resolver completioncore.MetadataResolver
	if req.Schema != nil && req.Schema.Directory != nil {
		key := completionCatalogKey(req.ConnectionID, req.Schema.Version)
		var err error
		defaultSchema := postgresCompletionDefaultSchema(req.Schema.Directory)
		var index *metadata.Index
		if key == "" {
			catalog, err = buildPostgresCompletionCatalog(req.Schema.Directory, req.Schema.Objects)
			index = metadata.NewIndex(*req.Schema)
		} else {
			catalog, err = pgCompletionCatalogCache.GetOrBuild(ctx, key, func() (*pgcatalog.Catalog, error) {
				return buildPostgresCompletionCatalog(req.Schema.Directory, req.Schema.Objects)
			})
			if err == nil {
				index, err = pgSchemaIndexCache.GetOrBuild(ctx, key, func() (*metadata.Index, error) {
					return metadata.NewIndex(*req.Schema), nil
				})
			}
		}
		if err != nil {
			return completer.Result{}, err
		}
		resolver = completioncore.NewSchemaResolver(index, defaultSchema)
	}

	completionSQL, completionCursor := postgresCompletionStatement(req.SQL, req.CursorOffset)
	candidates, cursorContext, err := corepostgres.Complete(
		ctx,
		completionSQL,
		completionCursor,
		catalog,
		resolver,
	)
	if err != nil {
		return completer.Result{}, err
	}
	if len(candidates) == 0 {
		if recoverySQL, recoveryCursor, ok := postgresCompletionRecoveryStatement(
			completionSQL,
			completionCursor,
		); ok {
			recoveryCandidates, recoveryContext, err := corepostgres.Complete(
				ctx,
				recoverySQL,
				recoveryCursor,
				catalog,
				resolver,
			)
			if err != nil {
				return completer.Result{}, err
			}
			if len(recoveryCandidates) > 0 {
				candidates, cursorContext = recoveryCandidates, recoveryContext
				completionSQL, completionCursor = recoverySQL, recoveryCursor
			}
		}
	}
	if req.Schema != nil && req.Schema.Directory != nil {
		candidates = filterPostgresRelations(
			candidates,
			req.Schema.Directory,
			completionSQL,
			completionCursor,
		)
	}
	start := completionReplaceStart(req.SQL, req.CursorOffset, '"')
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind, score := postgresCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" && kind != "type" {
			insertText = postgresQuoteCompletionPath(candidate.Text)
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			DisplayLabel: candidate.DisplayText,
			Kind:         kind,
			Detail:       firstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        score,
		})
	}
	if req.TriggerKind == completer.TriggerAutomatic && isBareSelect(req.SQL, req.CursorOffset) {
		suggestions = curatedSelectSuggestions(start, req.CursorOffset)
	}
	sortSuggestions(suggestions, req.SQL[start:req.CursorOffset])
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}
	position := cursorContext.Position
	if position == "" {
		position = completioncore.PositionAny
	}
	return completer.Result{Suggestions: suggestions, Context: position}, nil
}

func postgresCompletionStatement(sql string, cursor int) (string, int) {
	if cursor < 0 || cursor > len(sql) {
		return sql, cursor
	}
	start, end := 0, len(sql)
	lexer := pgparser.NewLexer(sql)
	for {
		token := lexer.NextToken()
		if token.Type == 0 {
			break
		}
		if token.Type != ';' {
			continue
		}
		if token.End <= cursor {
			start = token.End
			continue
		}
		if token.Loc >= cursor {
			end = token.End
			break
		}
	}
	return sql[start:end], cursor - start
}

func postgresCompletionRecoveryStatement(sql string, cursor int) (string, int, bool) {
	if cursor < 0 || cursor > len(sql) {
		return "", 0, false
	}
	lexer := pgparser.NewLexer(sql)
	depth := 0
	firstTopLevelSelect := -1
	latestTopLevelSelect := -1
	for {
		token := lexer.NextToken()
		if token.Type == 0 || token.Loc >= cursor {
			break
		}
		switch token.Type {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case pgparser.SELECT:
			if depth != 0 || !postgresTokenStartsLine(sql, token.Loc) {
				continue
			}
			if firstTopLevelSelect == -1 {
				firstTopLevelSelect = token.Loc
			} else {
				latestTopLevelSelect = token.Loc
			}
		}
	}
	if firstTopLevelSelect == -1 || latestTopLevelSelect == -1 {
		return "", 0, false
	}
	return sql[latestTopLevelSelect:], cursor - latestTopLevelSelect, true
}

func postgresTokenStartsLine(sql string, offset int) bool {
	if offset < 0 || offset > len(sql) {
		return false
	}
	lineStart := strings.LastIndexByte(sql[:offset], '\n') + 1
	return strings.TrimSpace(sql[lineStart:offset]) == ""
}

func postgresCompletionDefaultSchema(directory *metadata.Directory) string {
	if directory == nil {
		return "public"
	}
	if selected := directory.DefaultScope.Name("schema"); selected != "" {
		return selected
	}
	var schemas []string
	for _, node := range directory.ScopeNodes() {
		name := node.Path.Name("schema")
		if name == "" {
			continue
		}
		schemas = append(schemas, name)
		if name == "public" {
			return "public"
		}
	}
	if len(schemas) == 1 {
		return schemas[0]
	}
	return "public"
}

func filterPostgresRelations(
	candidates []completioncore.Candidate,
	directory *metadata.Directory,
	sql string,
	cursor int,
) []completioncore.Candidate {
	if directory == nil {
		return candidates
	}
	qualified := completionHasQualifier(sql, cursor)
	defaultSchema := postgresCompletionDefaultSchema(directory)
	if defaultSchema == "" {
		return candidates
	}
	allRelations := make(map[string]struct{})
	defaultRelations := make(map[string]struct{})
	foundDefaultSchema := false
	for _, node := range directory.ScopeNodes() {
		namespace := node.Path.Name("schema")
		if namespace == "" {
			continue
		}
		isDefault := strings.EqualFold(namespace, defaultSchema)
		foundDefaultSchema = foundDefaultSchema || isDefault
		for _, group := range node.Groups {
			if !postgresRelationKind(group.Kind) {
				continue
			}
			for _, ref := range group.Objects {
				name := strings.ToLower(ref.Name)
				allRelations[name] = struct{}{}
				if isDefault {
					defaultRelations[name] = struct{}{}
				}
			}
		}
	}
	if !foundDefaultSchema {
		return candidates
	}
	localRelations := postgresCTENames(sql)
	result := make([]completioncore.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !postgresRelationCandidate(candidate.Type) {
			result = append(result, candidate)
			continue
		}
		name := strings.ToLower(candidate.Text)
		if _, catalogRelation := allRelations[name]; !catalogRelation {
			// Omni may echo the unfinished identifier as a table candidate even
			// when it is absent from the catalog. Preserve only query-local CTEs.
			if _, localRelation := localRelations[name]; localRelation {
				result = append(result, candidate)
			}
			continue
		}
		if _, visible := defaultRelations[name]; qualified || visible {
			result = append(result, candidate)
		}
	}
	return result
}

func postgresCTENames(sql string) map[string]struct{} {
	result := make(map[string]struct{})
	tokens := pgparser.Tokenize(sql)
	if len(tokens) == 0 || tokens[0].Type != pgparser.WITH {
		return result
	}
	i := 1
	if i < len(tokens) && tokens[i].Type == pgparser.RECURSIVE {
		i++
	}
	for i < len(tokens) && pgparser.IsIdentifierTokenType(tokens[i].Type) {
		name := strings.ToLower(postgresCompletionUnquoteIdentifier(tokens[i].Str))
		result[name] = struct{}{}
		i++
		if i < len(tokens) && tokens[i].Type == '(' {
			close := postgresMatchingParen(tokens, i)
			if close < 0 {
				break
			}
			i = close + 1
		}
		if i < len(tokens) && tokens[i].Type == pgparser.AS {
			i++
		}
		if i >= len(tokens) || tokens[i].Type != '(' {
			break
		}
		close := postgresMatchingParen(tokens, i)
		if close < 0 {
			break
		}
		i = close + 1
		if i >= len(tokens) || tokens[i].Type != ',' {
			break
		}
		i++
	}
	return result
}

func postgresMatchingParen(tokens []pgparser.Token, open int) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Type {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func postgresCompletionUnquoteIdentifier(identifier string) string {
	if len(identifier) >= 2 && identifier[0] == '"' && identifier[len(identifier)-1] == '"' {
		return strings.ReplaceAll(identifier[1:len(identifier)-1], `""`, `"`)
	}
	return identifier
}

func postgresRelationKind(kind string) bool {
	switch kind {
	case "table", "foreign_table", "view", "materialized_view", "sequence":
		return true
	default:
		return false
	}
}

func postgresRelationCandidate(kind completioncore.CandidateType) bool {
	switch kind {
	case completioncore.CandidateTable,
		completioncore.CandidateForeignTable,
		completioncore.CandidateView,
		completioncore.CandidateMaterializedView,
		completioncore.CandidateSequence:
		return true
	default:
		return false
	}
}

func completionHasQualifier(sql string, cursor int) bool {
	if cursor < 0 || cursor > len(sql) {
		return false
	}
	index := cursor
	for index > 0 {
		character := sql[index-1]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '$' {
			index--
			continue
		}
		break
	}
	return index > 0 && sql[index-1] == '.'
}

func (d *postgresDriver) CompletionVocabulary() completer.Vocabulary {
	pgVocabularyOnce.Do(func() {
		items := make([]completer.Suggestion, 0, len(pgparser.Keywords)+256)
		for _, keyword := range pgparser.Keywords {
			items = append(items, completer.Suggestion{Label: strings.ToUpper(keyword.Name), Kind: "keyword", Score: 40})
		}
		for _, name := range pgcatalog.New().AllProcNames() {
			items = append(items, completer.Suggestion{Label: name, Kind: "function", Score: 60})
		}
		for _, name := range strings.Fields("bigint bigserial bit boolean bytea char date decimal double integer interval json jsonb numeric real serial smallint text time timestamp uuid varchar xml") {
			items = append(items, completer.Suggestion{Label: name, Kind: "type", Score: 35})
		}
		pgVocabulary = completer.NewVocabulary("postgres", items)
	})
	return pgVocabulary
}

func isBareSelect(sqlText string, cursor int) bool {
	if cursor < 0 || cursor > len(sqlText) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(sqlText[:cursor]), "select") &&
		strings.TrimSpace(sqlText[cursor:]) == ""
}

func curatedSelectSuggestions(start, end int) []completer.Suggestion {
	kinds := map[string]string{
		"*": "keyword", "DISTINCT": "keyword", "CASE": "keyword", "NULL": "keyword",
		"COUNT": "function", "SUM": "function", "AVG": "function", "MIN": "function",
		"MAX": "function", "COALESCE": "function",
	}
	order := []string{"*", "DISTINCT", "CASE", "NULL", "COUNT", "SUM", "AVG", "MIN", "MAX", "COALESCE"}
	result := make([]completer.Suggestion, 0, len(order))
	for _, label := range order {
		result = append(result, completer.Suggestion{
			Label: label, Kind: kinds[label], InsertText: label, ReplaceStart: start, ReplaceEnd: end, Score: 60,
		})
	}
	return result
}

func (d *postgresDriver) InvalidateCompletionCatalog(connectionID string) {
	pgCompletionCatalogCache.InvalidatePrefix(connectionID + ":")
	pgSchemaIndexCache.InvalidatePrefix(connectionID + ":")
}

func buildPostgresCompletionCatalog(directory *metadata.Directory, objects []metadata.Object) (*pgcatalog.Catalog, error) {
	native := pgcatalog.New()
	created := map[string]bool{"public": true, "pg_catalog": true}
	for _, node := range directory.ScopeNodes() {
		namespace := node.Path.Name("schema")
		if namespace == "" || created[namespace] {
			continue
		}
		if err := execPostgresCatalog(native, "CREATE SCHEMA "+pgQuoteIdent(namespace)); err != nil {
			return nil, fmt.Errorf("prepare postgres schema %q: %w", namespace, err)
		}
		created[namespace] = true
	}
	for _, object := range objects {
		statement := postgresCompletionDDL(object, false)
		if statement == "" {
			continue
		}
		if err := execPostgresCatalog(native, statement); err != nil && object.Relational != nil {
			if fallbackErr := execPostgresCatalog(native, postgresCompletionDDL(object, true)); fallbackErr != nil {
				return nil, fmt.Errorf("prepare postgres completion object %s.%s: %w", object.Ref.Scope.Name("schema"), object.Ref.Name, fallbackErr)
			}
		}
	}
	native.SetSearchPath([]string{postgresCompletionDefaultSchema(directory)})
	return native, nil
}

func execPostgresCatalog(catalog *pgcatalog.Catalog, sql string) error {
	results, err := catalog.Exec(sql, nil)
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

func postgresCompletionDDL(object metadata.Object, fallbackTypes bool) string {
	namespace := object.Ref.Scope.Name("schema")
	if namespace == "" {
		namespace = "public"
	}
	qualified := pgQuoteIdent(namespace) + "." + pgQuoteIdent(object.Ref.Name)
	switch object.Ref.Kind {
	case "table", "foreign_table":
		return "CREATE TABLE " + qualified + " (" + postgresCompletionColumns(object, fallbackTypes) + ")"
	case "view":
		return "CREATE VIEW " + qualified + " AS SELECT " + postgresCompletionSelectColumns(object)
	case "materialized_view":
		return "CREATE MATERIALIZED VIEW " + qualified + " AS SELECT " + postgresCompletionSelectColumns(object)
	case "sequence":
		return "CREATE SEQUENCE " + qualified
	}
	return ""
}

func postgresCompletionColumns(object metadata.Object, fallbackTypes bool) string {
	if object.Relational == nil || len(object.Relational.Columns) == 0 {
		return pgQuoteIdent("__sqlwarden_placeholder") + " text"
	}
	columns := make([]string, 0, len(object.Relational.Columns))
	for _, column := range object.Relational.Columns {
		dataType := strings.TrimSpace(column.DataType)
		if fallbackTypes || dataType == "" || !pgSafeType.MatchString(dataType) {
			dataType = "text"
		}
		columns = append(columns, pgQuoteIdent(column.Name)+" "+dataType)
	}
	return strings.Join(columns, ", ")
}

func postgresCompletionSelectColumns(object metadata.Object) string {
	if object.Relational == nil || len(object.Relational.Columns) == 0 {
		return "NULL::text AS " + pgQuoteIdent("__sqlwarden_placeholder")
	}
	columns := make([]string, 0, len(object.Relational.Columns))
	for _, column := range object.Relational.Columns {
		columns = append(columns, "NULL::text AS "+pgQuoteIdent(column.Name))
	}
	return strings.Join(columns, ", ")
}

func postgresCandidateKind(candidateType completioncore.CandidateType) (string, int) {
	switch candidateType {
	case completioncore.CandidateColumn:
		return "column", 100
	case completioncore.CandidateTable:
		return "table", 90
	case completioncore.CandidateView:
		return "view", 85
	case completioncore.CandidateMaterializedView:
		return "materialized_view", 84
	case completioncore.CandidateSchema:
		// In unqualified relation slots, the current schema is useful but less
		// likely than one of its tables. A typed "metadata." prefix still wins
		// through CodeMirror's prefix matching.
		return "schema", 70
	case completioncore.CandidateSequence:
		return "sequence", 70
	case completioncore.CandidateFunction:
		return "function", 60
	case completioncore.CandidateTypeName:
		return "type", 35
	case completioncore.CandidateKeyword:
		return "keyword", 40
	default:
		return "text", 20
	}
}

func pgQuoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func postgresQuoteCompletionIdentifier(identifier string) string {
	if isSafePostgresIdentifier(identifier) {
		return identifier
	}
	return pgQuoteIdent(identifier)
}

func postgresQuoteCompletionPath(identifier string) string {
	parts := strings.Split(identifier, ".")
	for i, part := range parts {
		parts[i] = postgresQuoteCompletionIdentifier(part)
	}
	return strings.Join(parts, ".")
}

func isSafePostgresIdentifier(identifier string) bool {
	if identifier == "" || identifier[0] < 'a' || identifier[0] > 'z' {
		return false
	}
	if _, reserved := pgReservedCompletionIdentifiers[identifier]; reserved {
		return false
	}
	for i := 1; i < len(identifier); i++ {
		c := identifier[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '$' {
			return false
		}
	}
	return true
}

func completionCatalogKey(connectionID, version string) string {
	if connectionID == "" || version == "" {
		return ""
	}
	return connectionID + ":" + version
}

func completionReplaceStart(sql string, cursor int, quote byte) int {
	start := cursor
	for start > 0 {
		c := sql[start-1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '$' {
			start--
			continue
		}
		break
	}
	if start > 0 && sql[start-1] == quote {
		start--
	}
	return start
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sortSuggestions(suggestions []completer.Suggestion, prefix string) {
	sort.SliceStable(suggestions, func(i, j int) bool {
		leftTier := completer.MatchTier(suggestions[i].Label, prefix)
		rightTier := completer.MatchTier(suggestions[j].Label, prefix)
		if leftTier != rightTier {
			return leftTier > rightTier
		}
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		if suggestions[i].Kind != suggestions[j].Kind {
			return suggestions[i].Kind < suggestions[j].Kind
		}
		return strings.ToLower(suggestions[i].Label) < strings.ToLower(suggestions[j].Label)
	})
}
