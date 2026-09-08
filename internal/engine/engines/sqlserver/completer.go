package sqlserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	mssqlcompletion "github.com/bytebase/omni/mssql/completion"

	"github.com/sqlwarden/internal/engine/completer"
)

// Complete provides keyword/type-only SQL Server completion. The vendored
// github.com/bytebase/omni/mssql/completion package has no catalog-backed
// resolution today: its resolveCatalogRule (mssql/completion/resolve.go) is
// an explicit upstream TODO stub that returns nil for every catalog-shaped
// rule (table_ref, columnref, schema_ref, func_name, proc_ref, index_ref,
// trigger_ref, sequence_ref, login_ref, user_ref, role_ref), and omni has no
// mssql/catalog package to build a prepared native catalog from in the first
// place (unlike mysql/catalog, pg/catalog, mariadb/catalog, tidb/catalog).
// So req.Schema is intentionally unused here: there is nothing yet for it to
// feed. Table/column/schema-aware completion requires a future
// internal/engine/completioncore/mssql package (mirroring completioncore/
// mysql, completioncore/oracle, completioncore/postgres) that resolves
// candidates against SQLWarden's own metadata.Index; that is out of this
// task's scope.
func (d *Driver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("sqlserver completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	candidates := mssqlcompletion.Complete(req.SQL, req.CursorOffset, nil)
	start := sqlServerCompletionReplaceStart(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind, score := sqlServerCandidateKind(candidate.Type)
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			Kind:         kind,
			Detail:       sqlServerFirstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   candidate.Text,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        score,
		})
	}
	sqlServerSortSuggestions(suggestions, req.SQL[start:req.CursorOffset])
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}
	return completer.Result{Suggestions: suggestions}, nil
}

func sqlServerCandidateKind(candidateType mssqlcompletion.CandidateType) (string, int) {
	switch candidateType {
	case mssqlcompletion.CandidateType_:
		return "type", 35
	case mssqlcompletion.CandidateKeyword:
		return "keyword", 40
	default:
		// Catalog-shaped candidate types (schema/table/view/column/function/
		// procedure/index/trigger/sequence/login/user/role) are never
		// produced by the vendored completion package today — see the
		// Complete doc comment — but are handled defensively in case that
		// changes upstream.
		return "text", 20
	}
}

func sqlServerCompletionReplaceStart(sql string, cursor int) int {
	start := cursor
	for start > 0 {
		c := sql[start-1]
		if isSQLServerIdentifierByte(c) {
			start--
			continue
		}
		break
	}
	if start > 0 && sql[start-1] == '[' {
		start--
	}
	return start
}

func isSQLServerIdentifierByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
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
