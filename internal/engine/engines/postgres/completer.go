package postgres

import (
	"context"
	"fmt"
	"slices"
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

var (
	_                               completer.Completer          = (*Driver)(nil)
	_                               completer.VocabularyProvider = (*Driver)(nil)
	pgVocabularyOnce                sync.Once
	pgVocabulary                    completer.Vocabulary
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

func (d *Driver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("postgres completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var resolver *completioncore.SchemaResolver
	var metadataResolver completioncore.MetadataResolver
	if req.Metadata != nil {
		resolver = completioncore.NewSchemaResolver(req.Metadata, postgresCompletionDefaultSchema(req.Metadata))
		metadataResolver = resolver
	}

	completionSQL, completionCursor := postgresCompletionStatement(req.SQL, req.CursorOffset)
	candidates, cursorContext, err := corepostgres.Complete(
		ctx,
		completionSQL,
		completionCursor,
		metadataResolver,
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
				metadataResolver,
			)
			if err != nil {
				return completer.Result{}, err
			}
			if len(recoveryCandidates) > 0 {
				candidates, cursorContext = recoveryCandidates, recoveryContext
			}
		}
	}
	start := completionReplaceStart(req.SQL, req.CursorOffset, '"')
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind := postgresCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" && kind != "type" {
			insertText = postgresQuoteCompletionPath(candidate.Text)
			if len(candidate.Qualifier) > 0 {
				parts := make([]string, 0, len(candidate.Qualifier)+1)
				for _, part := range candidate.Qualifier {
					parts = append(parts, postgresQuoteCompletionIdentifier(part))
				}
				insertText = strings.Join(append(parts, insertText), ".")
			}
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			DisplayLabel: candidate.DisplayText,
			Kind:         kind,
			Detail:       firstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        completer.KindScore(kind),
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
	result := completer.Result{Suggestions: suggestions, Context: position}
	if resolver != nil {
		result.Demands = resolver.Demands()
	}
	return result, nil
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

func postgresCompletionDefaultSchema(view *metadata.CompletionView) string {
	if schema := view.DefaultScope().Name("schema"); schema != "" {
		return schema
	}
	var schemas []string
	for _, path := range view.ScopePaths() {
		if last, _ := path.Last(); last.Kind == "schema" {
			schemas = append(schemas, last.Name)
		}
	}
	if slices.Contains(schemas, "public") {
		return "public"
	}
	if len(schemas) == 1 {
		return schemas[0]
	}
	return "public"
}

func (d *Driver) CompletionVocabulary() completer.Vocabulary {
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

func postgresCandidateKind(candidateType completioncore.CandidateType) string {
	switch candidateType {
	case completioncore.CandidateColumn:
		return "column"
	case completioncore.CandidateTable:
		return "table"
	case completioncore.CandidateView:
		return "view"
	case completioncore.CandidateMaterializedView:
		return "materialized_view"
	case completioncore.CandidateSchema:
		// In unqualified relation slots, the current schema is useful but less
		// likely than one of its tables. A typed "metadata." prefix still wins
		// through CodeMirror's prefix matching.
		return "schema"
	case completioncore.CandidateSequence:
		return "sequence"
	case completioncore.CandidateFunction:
		return "function"
	case completioncore.CandidateTypeName:
		return "type"
	case completioncore.CandidateKeyword:
		return "keyword"
	default:
		return "text"
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
