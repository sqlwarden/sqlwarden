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

	"github.com/sqlwarden/internal/dbengine/completer"
	"github.com/sqlwarden/internal/dbengine/completioncore"
	corepostgres "github.com/sqlwarden/internal/dbengine/completioncore/postgres"
	"github.com/sqlwarden/internal/dbengine/schema"
)

const preparedCompletionCatalogs = 32

var (
	_                               completer.Completer          = (*postgresDriver)(nil)
	_                               completer.CatalogInvalidator = (*postgresDriver)(nil)
	_                               completer.VocabularyProvider = (*postgresDriver)(nil)
	pgCompletionCatalogCache                                     = completer.NewPreparedCache[*pgcatalog.Catalog](preparedCompletionCatalogs)
	pgSchemaIndexCache                                           = completer.NewPreparedCache[*schema.Index](preparedCompletionCatalogs)
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
	var metadata completioncore.MetadataResolver
	if req.Schema != nil && req.Schema.Catalog != nil {
		key := completionCatalogKey(req.ConnectionID, req.Schema.Version)
		var err error
		defaultSchema := postgresCompletionDefaultSchema(req.Schema.Catalog)
		var index *schema.Index
		if key == "" {
			catalog, err = buildPostgresCompletionCatalog(req.Schema.Catalog, req.Schema.Objects)
			index = schema.NewIndex(*req.Schema)
		} else {
			catalog, err = pgCompletionCatalogCache.GetOrBuild(ctx, key, func() (*pgcatalog.Catalog, error) {
				return buildPostgresCompletionCatalog(req.Schema.Catalog, req.Schema.Objects)
			})
			if err == nil {
				index, err = pgSchemaIndexCache.GetOrBuild(ctx, key, func() (*schema.Index, error) {
					return schema.NewIndex(*req.Schema), nil
				})
			}
		}
		if err != nil {
			return completer.Result{}, err
		}
		metadata = completioncore.NewSchemaResolver(index, defaultSchema)
	}

	candidates, err := corepostgres.Complete(ctx, req.SQL, req.CursorOffset, catalog, metadata)
	if err != nil {
		return completer.Result{}, err
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
	sortSuggestions(suggestions)
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}
	return completer.Result{Suggestions: suggestions}, nil
}

func postgresCompletionDefaultSchema(catalog *schema.Catalog) string {
	if catalog == nil {
		return "public"
	}
	if catalog.DefaultNamespace != "" {
		return catalog.DefaultNamespace
	}
	for _, namespace := range catalog.Namespaces {
		if namespace.Name == "public" {
			return "public"
		}
	}
	if len(catalog.Namespaces) == 1 {
		return catalog.Namespaces[0].Name
	}
	return "public"
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

func buildPostgresCompletionCatalog(catalog *schema.Catalog, objects []schema.Object) (*pgcatalog.Catalog, error) {
	native := pgcatalog.New()
	for _, namespace := range catalog.Namespaces {
		if namespace.Name == "" || namespace.Name == "public" || namespace.Name == "pg_catalog" {
			continue
		}
		if err := execPostgresCatalog(native, "CREATE SCHEMA "+pgQuoteIdent(namespace.Name)); err != nil {
			return nil, fmt.Errorf("prepare postgres schema %q: %w", namespace.Name, err)
		}
	}
	for _, object := range objects {
		statement := postgresCompletionDDL(object, false)
		if statement == "" {
			continue
		}
		if err := execPostgresCatalog(native, statement); err != nil && object.Relational != nil {
			if fallbackErr := execPostgresCatalog(native, postgresCompletionDDL(object, true)); fallbackErr != nil {
				return nil, fmt.Errorf("prepare postgres completion object %s.%s: %w", object.Ref.Namespace, object.Ref.Name, fallbackErr)
			}
		}
	}
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

func postgresCompletionDDL(object schema.Object, fallbackTypes bool) string {
	namespace := object.Ref.Namespace
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

func postgresCompletionColumns(object schema.Object, fallbackTypes bool) string {
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

func postgresCompletionSelectColumns(object schema.Object) string {
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
	case completioncore.CandidateSchema:
		return "schema", 90
	case completioncore.CandidateTable:
		return "table", 80
	case completioncore.CandidateView:
		return "view", 75
	case completioncore.CandidateMaterializedView:
		return "materialized_view", 74
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

func sortSuggestions(suggestions []completer.Suggestion) {
	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		if suggestions[i].Kind != suggestions[j].Kind {
			return suggestions[i].Kind < suggestions[j].Kind
		}
		return strings.ToLower(suggestions[i].Label) < strings.ToLower(suggestions[j].Label)
	})
}
