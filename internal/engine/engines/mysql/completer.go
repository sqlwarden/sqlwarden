package mysql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	mysqlcatalog "github.com/bytebase/omni/mysql/catalog"
	mysqlcompletion "github.com/bytebase/omni/mysql/completion"
	mysqlparser "github.com/bytebase/omni/mysql/parser"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/completioncore"
	coremysql "github.com/sqlwarden/internal/engine/completioncore/mysql"
)

var (
	_                   completer.Completer          = (*Driver)(nil)
	_                   completer.VocabularyProvider = (*Driver)(nil)
	mysqlVocabularyOnce sync.Once
	mysqlVocabulary     completer.Vocabulary
)

func (d *Driver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("mysql completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var resolver *completioncore.SchemaResolver
	var metadataResolver completioncore.MetadataResolver
	if req.Metadata != nil {
		resolver = completioncore.NewSchemaResolver(req.Metadata, "")
		metadataResolver = resolver
	}

	candidates, cursorContext, err := coremysql.Complete(ctx, req.SQL, req.CursorOffset, metadataResolver)
	if err != nil {
		return completer.Result{}, err
	}
	start := mysqlCompletionReplaceStart(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind := mysqlCoreCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" && kind != "type" && kind != "engine" && kind != "charset" {
			insertText = mysqlQuoteCompletionPath(candidate.Text)
			if len(candidate.Qualifier) > 0 {
				parts := make([]string, 0, len(candidate.Qualifier)+1)
				for _, part := range candidate.Qualifier {
					parts = append(parts, mysqlQuoteCompletionIdentifier(part))
				}
				insertText = strings.Join(append(parts, insertText), ".")
			}
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			DisplayLabel: candidate.DisplayText,
			Kind:         kind,
			Detail:       mysqlFirstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        completer.KindScore(kind),
		})
	}
	if req.TriggerKind == completer.TriggerAutomatic && isMySQLBareSelect(req.SQL, req.CursorOffset) {
		suggestions = mysqlCuratedSelectSuggestions(start, req.CursorOffset)
	}
	mysqlSortSuggestions(suggestions, req.SQL[start:req.CursorOffset])
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

func (d *Driver) CompletionVocabulary() completer.Vocabulary {
	mysqlVocabularyOnce.Do(func() {
		var items []completer.Suggestion
		for token := 0; token < 10000; token++ {
			if name := mysqlparser.TokenName(token); name != "" {
				items = append(items, completer.Suggestion{Label: strings.ToUpper(name), Kind: "keyword", Score: 40})
			}
		}
		for _, candidate := range mysqlcompletion.Complete("SELECT ", len("SELECT "), mysqlcatalog.New()) {
			kind := mysqlCandidateKind(candidate.Type)
			if kind == "function" || kind == "type" || kind == "charset" || kind == "engine" {
				items = append(items, completer.Suggestion{Label: candidate.Text, Kind: kind, Score: completer.KindScore(kind)})
			}
		}
		for _, name := range strings.Fields("bigint binary bit blob boolean char date datetime decimal double enum float int integer json mediumint numeric real set smallint text time timestamp tinyint varbinary varchar year") {
			items = append(items, completer.Suggestion{Label: name, Kind: "type", Score: 35})
		}
		mysqlVocabulary = completer.NewVocabulary("mysql", items)
	})
	return mysqlVocabulary
}

func isMySQLBareSelect(sqlText string, cursor int) bool {
	if cursor < 0 || cursor > len(sqlText) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(sqlText[:cursor]), "select") &&
		strings.TrimSpace(sqlText[cursor:]) == ""
}

func mysqlCuratedSelectSuggestions(start, end int) []completer.Suggestion {
	order := []string{"*", "DISTINCT", "CASE", "NULL", "COUNT", "SUM", "AVG", "MIN", "MAX", "COALESCE"}
	result := make([]completer.Suggestion, 0, len(order))
	for _, label := range order {
		kind := "keyword"
		if label == "COUNT" || label == "SUM" || label == "AVG" || label == "MIN" || label == "MAX" || label == "COALESCE" {
			kind = "function"
		}
		result = append(result, completer.Suggestion{
			Label: label, Kind: kind, InsertText: label, ReplaceStart: start, ReplaceEnd: end, Score: 60,
		})
	}
	return result
}

func mysqlCandidateKind(candidateType mysqlcompletion.CandidateType) string {
	switch candidateType {
	case mysqlcompletion.CandidateColumn:
		return "column"
	case mysqlcompletion.CandidateDatabase:
		return "database"
	case mysqlcompletion.CandidateTable:
		return "table"
	case mysqlcompletion.CandidateView:
		return "view"
	case mysqlcompletion.CandidateFunction:
		return "function"
	case mysqlcompletion.CandidateProcedure:
		return "procedure"
	case mysqlcompletion.CandidateIndex:
		return "index"
	case mysqlcompletion.CandidateTrigger:
		return "trigger"
	case mysqlcompletion.CandidateEvent:
		return "event"
	case mysqlcompletion.CandidateEngine:
		return "engine"
	case mysqlcompletion.CandidateCharset:
		return "charset"
	case mysqlcompletion.CandidateType_:
		return "type"
	case mysqlcompletion.CandidateKeyword:
		return "keyword"
	default:
		return "text"
	}
}

func mysqlCoreCandidateKind(candidateType completioncore.CandidateType) string {
	switch candidateType {
	case completioncore.CandidateColumn:
		return "column"
	case completioncore.CandidateTable:
		return "table"
	case completioncore.CandidateView:
		return "view"
	case completioncore.CandidateDatabase:
		// Prefer relations in ordinary unqualified slots. Database names remain
		// available for explicit qualification and prefix matching.
		return "database"
	case completioncore.CandidateFunction:
		return "function"
	case completioncore.CandidateProcedure:
		return "procedure"
	case completioncore.CandidateIndex:
		return "index"
	case completioncore.CandidateTrigger:
		return "trigger"
	case completioncore.CandidateEvent:
		return "event"
	case completioncore.CandidateEngine:
		return "engine"
	case completioncore.CandidateCharset:
		return "charset"
	case completioncore.CandidateTypeName:
		return "type"
	case completioncore.CandidateKeyword:
		return "keyword"
	default:
		return "text"
	}
}

func mysqlCompletionQuoteIdent(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func mysqlQuoteCompletionIdentifier(identifier string) string {
	if isSafeMySQLIdentifier(identifier) {
		return identifier
	}
	return mysqlCompletionQuoteIdent(identifier)
}

func mysqlQuoteCompletionPath(identifier string) string {
	parts := strings.Split(identifier, ".")
	for i, part := range parts {
		parts[i] = mysqlQuoteCompletionIdentifier(part)
	}
	return strings.Join(parts, ".")
}

func isSafeMySQLIdentifier(identifier string) bool {
	if identifier == "" || mysqlparser.IsKeyword(identifier) || !isMySQLIdentifierStart(identifier[0]) {
		return false
	}
	for i := 1; i < len(identifier); i++ {
		c := identifier[i]
		if !isMySQLIdentifierStart(c) && (c < '0' || c > '9') && c != '$' {
			return false
		}
	}
	return true
}

func isMySQLIdentifierStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func mysqlCompletionReplaceStart(sql string, cursor int) int {
	start := cursor
	for start > 0 {
		c := sql[start-1]
		if isMySQLIdentifierStart(c) || (c >= '0' && c <= '9') || c == '$' {
			start--
			continue
		}
		break
	}
	if start > 0 && sql[start-1] == '`' {
		start--
	}
	return start
}

func mysqlFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func mysqlSortSuggestions(suggestions []completer.Suggestion, prefix string) {
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
