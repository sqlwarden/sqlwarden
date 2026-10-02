package sqlite

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/completioncore"
	coresqlite "github.com/sqlwarden/internal/engine/completioncore/sqlite"
)

var (
	_                    completer.Completer          = (*sqliteDriver)(nil)
	_                    completer.VocabularyProvider = (*sqliteDriver)(nil)
	sqliteVocabularyOnce sync.Once
	sqliteVocabulary     completer.Vocabulary
)

func (d *sqliteDriver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("sqlite completion cursor offset %d is out of range", req.CursorOffset)
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

	candidates, cursorContext, err := coresqlite.Complete(ctx, req.SQL, req.CursorOffset, metadataResolver)
	if err != nil {
		return completer.Result{}, err
	}

	start := sqliteCompletionReplaceStart(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind := sqliteCoreCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" && kind != "type" && !sqliteIsBareIdent(candidate.Text) {
			insertText = sqliteQuoteIdent(candidate.Text)
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			Kind:         kind,
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        completer.KindScore(kind),
		})
	}
	sqliteSortSuggestions(suggestions, req.SQL[start:req.CursorOffset])
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

func (d *sqliteDriver) CompletionVocabulary() completer.Vocabulary {
	sqliteVocabularyOnce.Do(func() {
		var items []completer.Suggestion
		for _, keyword := range coresqlite.Keywords() {
			items = append(items, completer.Suggestion{Label: keyword, Kind: "keyword", Score: 40})
		}
		for _, function := range coresqlite.Functions() {
			items = append(items, completer.Suggestion{Label: function, Kind: "function", Score: 60})
		}
		for _, affinity := range coresqlite.TypeAffinities() {
			items = append(items, completer.Suggestion{Label: affinity, Kind: "type", Score: 35})
		}
		sqliteVocabulary = completer.NewVocabulary("sqlite", items)
	})
	return sqliteVocabulary
}

func sqliteCompletionReplaceStart(sql string, cursor int) int {
	start := cursor
	for start > 0 {
		c := sql[start-1]
		if c == '_' || (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			start--
			continue
		}
		break
	}
	if start > 0 && sql[start-1] == '"' {
		start--
	}
	return start
}

func sqliteCoreCandidateKind(candidateType completioncore.CandidateType) string {
	switch candidateType {
	case completioncore.CandidateColumn:
		return "column"
	case completioncore.CandidateTable:
		return "table"
	case completioncore.CandidateView:
		return "view"
	case completioncore.CandidateDatabase:
		return "database"
	case completioncore.CandidateKeyword:
		return "keyword"
	case completioncore.CandidateFunction:
		return "function"
	default:
		return "text"
	}
}

func sqliteSortSuggestions(suggestions []completer.Suggestion, prefix string) {
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
