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
	"github.com/sqlwarden/internal/engine/metadata"
)

const preparedCompletionIndexes = 32

var (
	_ completer.Completer          = (*oracleDriver)(nil)
	_ completer.VocabularyProvider = (*oracleDriver)(nil)
	_ completer.CatalogInvalidator = (*oracleDriver)(nil)

	oracleSchemaIndexCache = completer.NewPreparedCache[*metadata.Index](preparedCompletionIndexes)

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

	var resolver completioncore.MetadataResolver
	if req.Schema != nil && req.Schema.Directory != nil {
		var index *metadata.Index
		key := oracleCompletionKey(req.ConnectionID, req.Schema.Version)
		if key == "" {
			index = metadata.NewIndex(*req.Schema)
		} else {
			built, err := oracleSchemaIndexCache.GetOrBuild(ctx, key, func() (*metadata.Index, error) {
				return metadata.NewIndex(*req.Schema), nil
			})
			if err != nil {
				return completer.Result{}, err
			}
			index = built
		}
		resolver = completioncore.NewSchemaResolver(index, index.DefaultScope().Name("schema"))
	}

	candidates, cursorContext, err := oraclecompletion.Complete(ctx, req.SQL, req.CursorOffset, resolver)
	if err != nil {
		return completer.Result{}, err
	}

	start := oracleCompletionReplaceStart(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind, score := oracleCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" {
			insertText = oracleQuoteCompletionPath(candidate.Text)
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
	oracleSortSuggestions(suggestions, req.SQL[start:req.CursorOffset])

	position := cursorContext.Position
	if position == "" {
		position = completioncore.PositionAny
	}
	return completer.Result{Suggestions: suggestions, Context: position}, ctx.Err()
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
		for _, name := range strings.Fields(
			"COUNT SUM AVG MIN MAX NVL NVL2 COALESCE DECODE TO_CHAR TO_DATE TO_NUMBER " +
				"TRUNC ROUND SYSDATE SYSTIMESTAMP ROWNUM LISTAGG RANK ROW_NUMBER",
		) {
			items = append(items, completer.Suggestion{Label: name, Kind: "function", Score: 45})
		}
		oracleVocabulary = completer.NewVocabulary("oracle", items)
	})
	return oracleVocabulary
}

func (d *oracleDriver) InvalidateCompletionCatalog(connectionID string) {
	oracleSchemaIndexCache.InvalidatePrefix(connectionID + ":")
}

func oracleCompletionKey(connectionID, version string) string {
	if connectionID == "" || version == "" {
		return ""
	}
	return connectionID + ":" + version
}

func oracleCandidateKind(t completioncore.CandidateType) (string, int) {
	switch t {
	case completioncore.CandidateColumn:
		return "column", 100
	case completioncore.CandidateTable:
		return "table", 90
	case completioncore.CandidateView:
		return "view", 85
	case completioncore.CandidateMaterializedView:
		return "materialized_view", 84
	case completioncore.CandidateSchema:
		return "schema", 70
	case completioncore.CandidateSequence:
		return "sequence", 60
	case completioncore.CandidateFunction:
		return "function", 58
	case completioncore.CandidateProcedure:
		return "procedure", 57
	case completioncore.CandidateKeyword:
		return "keyword", 40
	default:
		return "text", 20
	}
}

func oracleCompletionReplaceStart(sql string, cursor int) int {
	start := cursor
	for start > 0 {
		c := sql[start-1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$' || c == '#' {
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

func oracleQuoteCompletionPath(identifier string) string {
	parts := strings.Split(identifier, ".")
	for i, part := range parts {
		if isSafeOracleIdentifier(part) {
			parts[i] = part
			continue
		}
		parts[i] = oracleQuoteIdent(part)
	}
	return strings.Join(parts, ".")
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
