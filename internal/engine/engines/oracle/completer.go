package oracle

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	oracleparser "github.com/bytebase/omni/oracle/parser"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/completioncore"
	oraclecompletion "github.com/sqlwarden/internal/engine/completioncore/oracle"
)

var (
	_ completer.Completer          = (*oracleDriver)(nil)
	_ completer.VocabularyProvider = (*oracleDriver)(nil)

	oracleVocabularyOnce sync.Once
	oracleVocabulary     completer.Vocabulary
)

func (d *oracleDriver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("oracle completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var resolver *completioncore.SchemaResolver
	var metadataResolver completioncore.MetadataResolver
	if req.Metadata != nil {
		resolver = completioncore.NewSchemaResolver(req.Metadata, req.Metadata.DefaultScope().Name("schema"))
		metadataResolver = resolver
	}

	candidates, cursorContext, err := oraclecompletion.Complete(ctx, req.SQL, req.CursorOffset, metadataResolver)
	if err != nil {
		return completer.Result{}, err
	}

	start, end, prefix, _ := oraclecompletion.CursorWord(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind := oracleCandidateKind(candidate.Type)
		insertText := candidate.Text
		if candidate.InsertText != "" {
			insertText = candidate.InsertText
		} else if kind != "keyword" && !isSafeOracleIdentifier(candidate.Text) {
			insertText = oracleQuoteIdent(candidate.Text)
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			DisplayLabel: candidate.DisplayText,
			Kind:         kind,
			Detail:       firstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   end,
			Score:        completer.KindScore(kind),
		})
	}
	oracleSortSuggestions(suggestions, prefix)

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

func (d *oracleDriver) CompletionVocabulary() completer.Vocabulary {
	oracleVocabularyOnce.Do(func() {
		var items []completer.Suggestion
		seen := map[string]bool{}
		for token := 0; token < 20000; token++ {
			name := oracleparser.TokenName(token)
			if name == "" || !isAlphaWord(name) {
				continue
			}
			upper := strings.ToUpper(name)
			if seen[upper] {
				continue
			}
			seen[upper] = true
			items = append(items, completer.Suggestion{Label: upper, Kind: "keyword", Score: 40})
		}
		for _, name := range strings.Fields(
			"NUMBER VARCHAR2 NVARCHAR2 CHAR NCHAR CLOB NCLOB BLOB RAW DATE TIMESTAMP " +
				"BINARY_FLOAT BINARY_DOUBLE FLOAT INTERVAL",
		) {
			items = append(items, completer.Suggestion{Label: name, Kind: "type", Score: 35})
		}
		for _, function := range oraclecompletion.BuiltinFunctions() {
			items = append(items, completer.Suggestion{Label: function.Name, InsertText: function.Name, Detail: function.Detail, Kind: "function", Score: completer.KindScore("function")})
		}
		oracleVocabulary = completer.NewVocabulary("oracle", items)
	})
	return oracleVocabulary
}

func oracleCandidateKind(t completioncore.CandidateType) string {
	switch t {
	case completioncore.CandidateColumn:
		return "column"
	case completioncore.CandidateTable:
		return "table"
	case completioncore.CandidateView:
		return "view"
	case completioncore.CandidateMaterializedView:
		return "materialized_view"
	case completioncore.CandidateSchema:
		return "schema"
	case completioncore.CandidateSequence:
		return "sequence"
	case completioncore.CandidateFunction:
		return "function"
	case completioncore.CandidateProcedure:
		return "procedure"
	case completioncore.CandidateKeyword:
		return "keyword"
	default:
		return "text"
	}
}

func isSafeOracleIdentifier(identifier string) bool {
	if identifier == "" || oracleparser.IsReservedKeyword(identifier) {
		return false
	}
	// Bare Oracle identifiers are folded to upper case; only emit unquoted when
	// already all-upper and lexically simple.
	if identifier != strings.ToUpper(identifier) {
		return false
	}
	for i := 0; i < len(identifier); i++ {
		c := identifier[i]
		alpha := c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if i == 0 && !alpha {
			return false
		}
		if !alpha && !digit && c != '_' && c != '$' && c != '#' {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func isAlphaWord(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			return false
		}
	}
	return s != ""
}

func oracleSortSuggestions(suggestions []completer.Suggestion, prefix string) {
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
