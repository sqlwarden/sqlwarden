package mysql

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	mysqlcatalog "github.com/bytebase/omni/mysql/catalog"
	mysqlcompletion "github.com/bytebase/omni/mysql/completion"
	mysqlparser "github.com/bytebase/omni/mysql/parser"

	"github.com/sqlwarden/internal/dbengine/completer"
	"github.com/sqlwarden/internal/dbengine/schema"
)

const preparedCompletionCatalogs = 32

var (
	_                           completer.Completer          = (*mysqlDriver)(nil)
	_                           completer.CatalogInvalidator = (*mysqlDriver)(nil)
	_                           completer.VocabularyProvider = (*mysqlDriver)(nil)
	mysqlCompletionCatalogCache                              = completer.NewPreparedCache[*mysqlcatalog.Catalog](preparedCompletionCatalogs)
	mysqlVocabularyOnce         sync.Once
	mysqlVocabulary             completer.Vocabulary
	mysqlSafeType               = regexp.MustCompile(`^[A-Za-z0-9_ (),.'"]+$`)
)

func (d *mysqlDriver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("mysql completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var catalog *mysqlcatalog.Catalog
	if req.Catalog != nil {
		key := mysqlCompletionCatalogKey(req.ConnectionID, req.CatalogVersion)
		var err error
		if key == "" {
			catalog, err = buildMySQLCompletionCatalog(req.Catalog, req.Objects)
		} else {
			catalog, err = mysqlCompletionCatalogCache.GetOrBuild(ctx, key, func() (*mysqlcatalog.Catalog, error) {
				return buildMySQLCompletionCatalog(req.Catalog, req.Objects)
			})
		}
		if err != nil {
			return completer.Result{}, err
		}
	}

	candidates := mysqlcompletion.Complete(req.SQL, req.CursorOffset, catalog)
	start := mysqlCompletionReplaceStart(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind, score := mysqlCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" && kind != "type" && kind != "engine" && kind != "charset" {
			insertText = mysqlQuoteCompletionIdentifier(candidate.Text)
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			Kind:         kind,
			Detail:       mysqlFirstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        score,
		})
	}
	suggestions = mysqlScopedSuggestions(req, suggestions, start)
	if req.TriggerKind == completer.TriggerAutomatic && isMySQLBareSelect(req.SQL, req.CursorOffset) {
		suggestions = mysqlCuratedSelectSuggestions(start, req.CursorOffset)
	}
	mysqlSortSuggestions(suggestions)
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}
	return completer.Result{Suggestions: suggestions}, nil
}

func (d *mysqlDriver) CompletionVocabulary() completer.Vocabulary {
	mysqlVocabularyOnce.Do(func() {
		var items []completer.Suggestion
		for token := 0; token < 10000; token++ {
			if name := mysqlparser.TokenName(token); name != "" {
				items = append(items, completer.Suggestion{Label: strings.ToUpper(name), Kind: "keyword", Score: 40})
			}
		}
		for _, candidate := range mysqlcompletion.Complete("SELECT ", len("SELECT "), mysqlcatalog.New()) {
			kind, score := mysqlCandidateKind(candidate.Type)
			if kind == "function" || kind == "type" || kind == "charset" || kind == "engine" {
				items = append(items, completer.Suggestion{Label: candidate.Text, Kind: kind, Score: score})
			}
		}
		for _, name := range strings.Fields("bigint binary bit blob boolean char date datetime decimal double enum float int integer json mediumint numeric real set smallint text time timestamp tinyint varbinary varchar year") {
			items = append(items, completer.Suggestion{Label: name, Kind: "type", Score: 35})
		}
		mysqlVocabulary = completer.NewVocabulary("mysql", items)
	})
	return mysqlVocabulary
}

func mysqlScopedSuggestions(req completer.Request, suggestions []completer.Suggestion, start int) []completer.Suggestion {
	columns, handled := completer.ScopedColumns(req.SQL, req.CursorOffset, req.Objects, "mysql")
	if !handled {
		return suggestions
	}
	result := make([]completer.Suggestion, 0, len(suggestions)+len(columns))
	for _, suggestion := range suggestions {
		if suggestion.Kind != "column" {
			result = append(result, suggestion)
		}
	}
	for _, column := range columns {
		label := column.Name
		insertText := mysqlQuoteCompletionIdentifier(column.Name)
		if column.Qualified {
			label = column.Owner + "." + column.Name
			insertText = mysqlQuoteCompletionIdentifier(column.Owner) + "." + insertText
		}
		detail := column.DataType
		if column.Owner != "" {
			detail = column.Owner + " · " + detail
		}
		result = append(result, completer.Suggestion{
			Label: label, DisplayLabel: column.DisplayLabel, Kind: "column", Detail: strings.TrimSuffix(detail, " · "),
			InsertText: insertText, ReplaceStart: start, ReplaceEnd: req.CursorOffset, Score: 100,
		})
	}
	return result
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

func (d *mysqlDriver) InvalidateCompletionCatalog(connectionID string) {
	mysqlCompletionCatalogCache.InvalidatePrefix(connectionID + ":")
}

func buildMySQLCompletionCatalog(catalog *schema.Catalog, objects []schema.Object) (*mysqlcatalog.Catalog, error) {
	native := mysqlcatalog.New()
	created := make(map[string]bool)
	for _, namespace := range catalog.Namespaces {
		if namespace.Name == "" {
			continue
		}
		if err := execMySQLCatalog(native, "CREATE DATABASE "+mysqlCompletionQuoteIdent(namespace.Name)); err != nil {
			return nil, fmt.Errorf("prepare mysql database %q: %w", namespace.Name, err)
		}
		created[namespace.Name] = true
	}
	if catalog.Database != "" && !created[catalog.Database] {
		if err := execMySQLCatalog(native, "CREATE DATABASE "+mysqlCompletionQuoteIdent(catalog.Database)); err != nil {
			return nil, fmt.Errorf("prepare mysql database %q: %w", catalog.Database, err)
		}
		created[catalog.Database] = true
	}
	for _, object := range objects {
		databaseName := object.Ref.Namespace
		if databaseName == "" {
			databaseName = catalog.Database
		}
		if databaseName == "" {
			continue
		}
		if !created[databaseName] {
			if err := execMySQLCatalog(native, "CREATE DATABASE "+mysqlCompletionQuoteIdent(databaseName)); err != nil {
				return nil, fmt.Errorf("prepare mysql database %q: %w", databaseName, err)
			}
			created[databaseName] = true
		}
		native.SetCurrentDatabase(databaseName)
		statement := mysqlCompletionDDL(databaseName, object, false)
		if statement == "" {
			continue
		}
		if err := execMySQLCatalog(native, statement); err != nil && object.Relational != nil {
			if fallbackErr := execMySQLCatalog(native, mysqlCompletionDDL(databaseName, object, true)); fallbackErr != nil {
				return nil, fmt.Errorf("prepare mysql completion object %s.%s: %w", object.Ref.Namespace, object.Ref.Name, fallbackErr)
			}
		}
	}
	current := catalog.Database
	if current == "" && len(catalog.Namespaces) > 0 {
		current = catalog.Namespaces[0].Name
	}
	native.SetCurrentDatabase(current)
	return native, nil
}

func execMySQLCatalog(catalog *mysqlcatalog.Catalog, sql string) error {
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

func mysqlCompletionDDL(databaseName string, object schema.Object, fallbackTypes bool) string {
	qualified := mysqlCompletionQuoteIdent(databaseName) + "." + mysqlCompletionQuoteIdent(object.Ref.Name)
	switch object.Ref.Kind {
	case "table":
		return "CREATE TABLE " + qualified + " (" + mysqlCompletionColumns(object, fallbackTypes) + ")"
	case "view":
		return "CREATE VIEW " + qualified + " AS SELECT " + mysqlCompletionSelectColumns(object)
	}
	return ""
}

func mysqlCompletionColumns(object schema.Object, fallbackTypes bool) string {
	if object.Relational == nil || len(object.Relational.Columns) == 0 {
		return mysqlCompletionQuoteIdent("__sqlwarden_placeholder") + " TEXT"
	}
	columns := make([]string, 0, len(object.Relational.Columns))
	for _, column := range object.Relational.Columns {
		dataType := strings.TrimSpace(column.DataType)
		if fallbackTypes || dataType == "" || !mysqlSafeType.MatchString(dataType) {
			dataType = "TEXT"
		}
		columns = append(columns, mysqlCompletionQuoteIdent(column.Name)+" "+dataType)
	}
	return strings.Join(columns, ", ")
}

func mysqlCompletionSelectColumns(object schema.Object) string {
	if object.Relational == nil || len(object.Relational.Columns) == 0 {
		return "NULL AS " + mysqlCompletionQuoteIdent("__sqlwarden_placeholder")
	}
	columns := make([]string, 0, len(object.Relational.Columns))
	for _, column := range object.Relational.Columns {
		columns = append(columns, "NULL AS "+mysqlCompletionQuoteIdent(column.Name))
	}
	return strings.Join(columns, ", ")
}

func mysqlCandidateKind(candidateType mysqlcompletion.CandidateType) (string, int) {
	switch candidateType {
	case mysqlcompletion.CandidateColumn:
		return "column", 100
	case mysqlcompletion.CandidateDatabase:
		return "database", 90
	case mysqlcompletion.CandidateTable:
		return "table", 80
	case mysqlcompletion.CandidateView:
		return "view", 75
	case mysqlcompletion.CandidateFunction:
		return "function", 60
	case mysqlcompletion.CandidateProcedure:
		return "procedure", 58
	case mysqlcompletion.CandidateIndex:
		return "index", 55
	case mysqlcompletion.CandidateTrigger:
		return "trigger", 54
	case mysqlcompletion.CandidateEvent:
		return "event", 53
	case mysqlcompletion.CandidateEngine:
		return "engine", 45
	case mysqlcompletion.CandidateCharset:
		return "charset", 45
	case mysqlcompletion.CandidateType_:
		return "type", 35
	case mysqlcompletion.CandidateKeyword:
		return "keyword", 40
	default:
		return "text", 20
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

func mysqlCompletionCatalogKey(connectionID, version string) string {
	if connectionID == "" || version == "" {
		return ""
	}
	return connectionID + ":" + version
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

func mysqlSortSuggestions(suggestions []completer.Suggestion) {
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
