package sqlserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/completioncore"
	"github.com/sqlwarden/internal/engine/completioncore/mssql"
)

func (d *Driver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("sqlserver completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var resolver *completioncore.SchemaResolver
	if req.Metadata != nil {
		resolver = completioncore.NewSchemaResolver(req.Metadata, req.Metadata.DefaultScope().Name("schema"))
	}
	candidates, cursorContext, err := mssql.Complete(ctx, req.SQL, req.CursorOffset, resolver)
	if err != nil {
		return completer.Result{}, err
	}

	start, end, prefix, _ := mssql.CursorWord(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind := string(candidate.Type)
		insertText := candidate.InsertText
		if insertText == "" {
			insertText = candidate.Text
			if candidate.Type != completioncore.CandidateKeyword && candidate.Type != completioncore.CandidateTypeName {
				insertText = mssql.QuoteIdentifier(candidate.Text)
			}
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			DisplayLabel: candidate.DisplayText,
			Kind:         kind,
			Detail:       sqlServerFirstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   end,
			Score:        completer.KindScore(kind),
		})
	}
	sqlServerSortSuggestions(suggestions, prefix)

	position := cursorContext.Position
	if position == "" {
		position = completioncore.PositionAny
	}
	result := completer.Result{Suggestions: suggestions, Context: position}
	if resolver != nil {
		result.Demands = resolver.Demands()
	}
	return result, ctx.Err()
}

func sqlServerFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sqlServerSortSuggestions(suggestions []completer.Suggestion, prefix string) {
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

var _ completer.Completer = (*Driver)(nil)
