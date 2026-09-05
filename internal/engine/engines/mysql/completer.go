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

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/completioncore"
	coremysql "github.com/sqlwarden/internal/engine/completioncore/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

const preparedCompletionCatalogs = 32

var (
	_                           completer.Completer          = (*Driver)(nil)
	_                           completer.CatalogInvalidator = (*Driver)(nil)
	_                           completer.VocabularyProvider = (*Driver)(nil)
	mysqlCompletionCatalogCache                              = completer.NewPreparedCache[*mysqlcatalog.Catalog](preparedCompletionCatalogs)
	mysqlSchemaIndexCache                                    = completer.NewPreparedCache[*metadata.Index](preparedCompletionCatalogs)
	mysqlVocabularyOnce         sync.Once
	mysqlVocabulary             completer.Vocabulary
	mysqlSafeType               = regexp.MustCompile(`^[A-Za-z0-9_ (),.'"]+$`)
)

func (d *Driver) Complete(ctx context.Context, req completer.Request) (completer.Result, error) {
	if req.CursorOffset < 0 || req.CursorOffset > len(req.SQL) {
		return completer.Result{}, fmt.Errorf("mysql completion cursor offset %d is out of range", req.CursorOffset)
	}
	if err := ctx.Err(); err != nil {
		return completer.Result{}, err
	}

	var catalog *mysqlcatalog.Catalog
	var resolver completioncore.MetadataResolver
	if req.Schema != nil && req.Schema.Directory != nil {
		key := mysqlCompletionCatalogKey(req.ConnectionID, req.Schema.Version)
		var err error
		var index *metadata.Index
		if key == "" {
			catalog, err = buildMySQLCompletionCatalog(req.Schema.Directory, req.Schema.Objects)
			index = metadata.NewIndex(*req.Schema)
		} else {
			catalog, err = mysqlCompletionCatalogCache.GetOrBuild(ctx, key, func() (*mysqlcatalog.Catalog, error) {
				return buildMySQLCompletionCatalog(req.Schema.Directory, req.Schema.Objects)
			})
			if err == nil {
				index, err = mysqlSchemaIndexCache.GetOrBuild(ctx, key, func() (*metadata.Index, error) {
					return metadata.NewIndex(*req.Schema), nil
				})
			}
		}
		if err != nil {
			return completer.Result{}, err
		}
		resolver = completioncore.NewSchemaResolver(index, "")
	}

	candidates, cursorContext, err := coremysql.Complete(ctx, req.SQL, req.CursorOffset, catalog, resolver)
	if err != nil {
		return completer.Result{}, err
	}
	start := mysqlCompletionReplaceStart(req.SQL, req.CursorOffset)
	suggestions := make([]completer.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		kind, score := mysqlCoreCandidateKind(candidate.Type)
		insertText := candidate.Text
		if kind != "keyword" && kind != "type" && kind != "engine" && kind != "charset" {
			insertText = mysqlQuoteCompletionPath(candidate.Text)
		}
		suggestions = append(suggestions, completer.Suggestion{
			Label:        candidate.Text,
			DisplayLabel: candidate.DisplayText,
			Kind:         kind,
			Detail:       mysqlFirstNonEmpty(candidate.Definition, candidate.Comment),
			InsertText:   insertText,
			ReplaceStart: start,
			ReplaceEnd:   req.CursorOffset,
			Score:        score,
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
	return completer.Result{Suggestions: suggestions, Context: position}, nil
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

func (d *Driver) InvalidateCompletionCatalog(connectionID string) {
	mysqlCompletionCatalogCache.InvalidatePrefix(connectionID + ":")
	mysqlSchemaIndexCache.InvalidatePrefix(connectionID + ":")
}

func buildMySQLCompletionCatalog(directory *metadata.Directory, objects []metadata.Object) (*mysqlcatalog.Catalog, error) {
	native := mysqlcatalog.New()
	created := make(map[string]bool)
	for _, node := range directory.ScopeNodes() {
		database := node.Path.Name("database")
		if database == "" || created[database] {
			continue
		}
		if err := execMySQLCatalog(native, "CREATE DATABASE "+mysqlCompletionQuoteIdent(database)); err != nil {
			return nil, fmt.Errorf("prepare mysql database %q: %w", database, err)
		}
		created[database] = true
	}
	for _, object := range objects {
		databaseName := object.Ref.Scope.Name("database")
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
				return nil, fmt.Errorf("prepare mysql completion object %s.%s: %w", databaseName, object.Ref.Name, fallbackErr)
			}
		}
	}
	current := directory.DefaultScope.Name("database")
	if current == "" {
		for database := range created {
			current = database
			break
		}
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

func mysqlCompletionDDL(databaseName string, object metadata.Object, fallbackTypes bool) string {
	qualified := mysqlCompletionQuoteIdent(databaseName) + "." + mysqlCompletionQuoteIdent(object.Ref.Name)
	switch object.Ref.Kind {
	case "table":
		return "CREATE TABLE " + qualified + " (" + mysqlCompletionColumns(object, fallbackTypes) + ")"
	case "view":
		return "CREATE VIEW " + qualified + " AS SELECT " + mysqlCompletionSelectColumns(object)
	}
	return ""
}

func mysqlCompletionColumns(object metadata.Object, fallbackTypes bool) string {
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

func mysqlCompletionSelectColumns(object metadata.Object) string {
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

func mysqlCoreCandidateKind(candidateType completioncore.CandidateType) (string, int) {
	switch candidateType {
	case completioncore.CandidateColumn:
		return "column", 100
	case completioncore.CandidateTable:
		return "table", 90
	case completioncore.CandidateView:
		return "view", 85
	case completioncore.CandidateDatabase:
		// Prefer relations in ordinary unqualified slots. Database names remain
		// available for explicit qualification and prefix matching.
		return "database", 70
	case completioncore.CandidateFunction:
		return "function", 60
	case completioncore.CandidateProcedure:
		return "procedure", 58
	case completioncore.CandidateIndex:
		return "index", 55
	case completioncore.CandidateTrigger:
		return "trigger", 54
	case completioncore.CandidateEvent:
		return "event", 53
	case completioncore.CandidateEngine:
		return "engine", 45
	case completioncore.CandidateCharset:
		return "charset", 45
	case completioncore.CandidateTypeName:
		return "type", 35
	case completioncore.CandidateKeyword:
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
