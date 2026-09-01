// Package mysql adapts Bytebase/Omni MySQL completion to SQLWarden's metadata
// boundary. Omni does not yet export a MySQL scope snapshot, so reference
// collection remains isolated here for eventual replacement.
package mysql

import (
	"context"
	"slices"
	"strings"
	"unicode"

	"github.com/bytebase/omni/mysql/catalog"
	omnicompletion "github.com/bytebase/omni/mysql/completion"
	"github.com/bytebase/omni/mysql/parser"

	"github.com/sqlwarden/internal/engine/completioncore"
)

type reference struct {
	database string
	table    string
	alias    string
	columns  []string
	virtual  bool
	depth    int
}

type token struct {
	parser.Token
	depth int
}

// Complete combines Omni's grammar candidates with Bytebase-style visible
// reference resolution.
func Complete(
	ctx context.Context,
	sql string,
	cursor int,
	nativeCatalog *catalog.Catalog,
	metadata completioncore.MetadataResolver,
) ([]completioncore.Candidate, completioncore.Context, error) {
	if err := completioncore.CheckContext(ctx); err != nil {
		return nil, completioncore.Context{}, err
	}
	native := omnicompletion.Complete(sql, cursor, nativeCatalog)
	candidateSet := parser.Collect(sql, cursor)
	qualifier := qualifierAt(sql, cursor)
	valueContext := isInsertValueContext(sql, cursor)
	columnContext := !valueContext && (qualifier != "" || hasRule(candidateSet, "columnref") ||
		isDMLColumnContext(sql, cursor) || isSelectProjectionContext(sql, cursor))
	suppressNativeColumns := columnContext || valueContext

	result := make([]completioncore.Candidate, 0, len(native)+32)
	for _, candidate := range native {
		if suppressNativeColumns && candidate.Type == omnicompletion.CandidateColumn {
			continue
		}
		result = append(result, convertNative(candidate))
	}
	if columnContext && metadata != nil {
		result = append(result, resolveColumns(sql, cursor, qualifier, metadata)...)
	}
	result = append(result, cteRelationCandidates(sql, cursor)...)
	result = append(result, selectAliasCandidates(sql, cursor)...)
	continuation, isRelationContinuation := relationContinuationAt(sql, cursor)
	if isRelationContinuation {
		result = relationContinuationCandidates(continuation)
	}
	final := filterByPrefix(deduplicate(result), prefixAt(sql, cursor))

	position := completioncore.PositionAny
	switch {
	case valueContext:
		position = completioncore.PositionValue
	case isRelationContinuation:
		// A completed, unqualified relation reference: only trailing
		// keywords (AS, WHERE, JOIN, ...) are valid next.
		position = completioncore.PositionKeyword
	case columnContext:
		position = completioncore.PositionColumn
	case hasCandidateOfKind(final, relationCandidateKinds) && !hasCandidateOfKind(final, columnCandidateKinds):
		position = completioncore.PositionRelation
	case onlyKeywordCandidates(final):
		position = completioncore.PositionKeyword
	}

	return final, completioncore.Context{Position: position}, completioncore.CheckContext(ctx)
}

var relationCandidateKinds = []completioncore.CandidateType{
	completioncore.CandidateTable, completioncore.CandidateView,
	completioncore.CandidateDatabase,
}

var columnCandidateKinds = []completioncore.CandidateType{completioncore.CandidateColumn}

func hasCandidateOfKind(candidates []completioncore.Candidate, kinds []completioncore.CandidateType) bool {
	for _, candidate := range candidates {
		if slices.Contains(kinds, candidate.Type) {
			return true
		}
	}
	return false
}

// onlyKeywordCandidates reports whether every candidate is a bare keyword, so
// the cursor position can be classified without dialect-specific grammar
// inspection — a request with schema/object candidates never counts.
func onlyKeywordCandidates(candidates []completioncore.Candidate) bool {
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if candidate.Type != completioncore.CandidateKeyword {
			return false
		}
	}
	return true
}

func cteRelationCandidates(sql string, cursor int) []completioncore.Candidate {
	tokens := completionTokens(parser.Tokenize(sql))
	if len(tokens) == 0 {
		return nil
	}
	start, _ := statementBounds(tokens, cursor)
	cursorDepth := depthAt(tokens, cursor)
	selectIndex, selectDepth := activeSelect(tokens, start, cursor, cursorDepth)
	if selectIndex < 0 || !mysqlRelationPosition(sql, tokens, selectIndex, cursor, cursorDepth) {
		return nil
	}
	withIndex := -1
	for i := selectIndex - 1; i >= start; i-- {
		if tokens[i].depth == selectDepth && tokenIs(tokens[i], "WITH") {
			withIndex = i
			break
		}
	}
	if withIndex < 0 {
		return nil
	}
	names := mysqlCTENames(tokens, withIndex, selectIndex, selectDepth)
	result := make([]completioncore.Candidate, 0, len(names))
	for _, name := range names {
		result = append(result, completioncore.Candidate{Text: name, Type: completioncore.CandidateTable})
	}
	return result
}

func mysqlRelationPosition(sql string, tokens []token, selectIndex, cursor, depth int) bool {
	indices := make([]int, 0, 2)
	for i := selectIndex + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if tokens[i].depth == depth {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return false
	}
	last := tokens[indices[len(indices)-1]]
	if last.Type == parser.FROM || last.Type == parser.JOIN {
		return true
	}
	// A trailing comma opens the next slot of a relation list, so long as the
	// clause the cursor sits in is a FROM/JOIN and not, say, the SELECT list.
	if last.Type == ',' {
		return slices.ContainsFunc(indices, func(index int) bool {
			return tokens[index].Type == parser.FROM || tokens[index].Type == parser.JOIN
		})
	}
	if len(indices) < 2 || prefixAt(sql, cursor) == "" {
		return false
	}
	previous := tokens[indices[len(indices)-2]]
	return previous.Type == parser.FROM || previous.Type == parser.JOIN
}

func mysqlCTENames(tokens []token, withIndex, end, depth int) []string {
	i := withIndex + 1
	if i < end && tokenIs(tokens[i], "RECURSIVE") {
		i++
	}
	var result []string
	for i < end && tokens[i].depth == depth && parser.IsIdentTokenType(tokens[i].Type) {
		name := unquote(tokens[i].Str)
		i++
		if i < end && tokens[i].depth == depth && tokens[i].Type == '(' {
			close := matchingParen(tokens, i, end)
			if close < 0 {
				break
			}
			i = close + 1
		}
		if i < end && tokens[i].depth == depth && tokens[i].Type == parser.AS {
			i++
		}
		if i >= end || tokens[i].depth != depth || tokens[i].Type != '(' {
			break
		}
		close := matchingParen(tokens, i, end)
		if close < 0 {
			break
		}
		result = append(result, name)
		i = close + 1
		if i >= end || tokens[i].depth != depth || tokens[i].Type != ',' {
			break
		}
		i++
	}
	return result
}

// selectAliasCandidates returns output aliases from the SELECT owning the
// cursor. MySQL permits those aliases in GROUP BY, HAVING, and ORDER BY.
func selectAliasCandidates(sql string, cursor int) []completioncore.Candidate {
	tokens := completionTokens(parser.Tokenize(sql))
	if len(tokens) == 0 {
		return nil
	}
	start, _ := statementBounds(tokens, cursor)
	cursorDepth := depthAt(tokens, cursor)
	selectIndex, selectDepth := activeSelect(tokens, start, cursor, cursorDepth)
	if selectIndex < 0 || !mysqlAliasClause(tokens, selectIndex, cursor, selectDepth) {
		return nil
	}

	aliases := explicitProjectionAliases(tokens, selectIndex, selectDepth)
	result := make([]completioncore.Candidate, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, completioncore.Candidate{Text: alias, Type: completioncore.CandidateColumn})
	}
	return result
}

func activeSelect(tokens []token, start, cursor, cursorDepth int) (int, int) {
	index, depth := -1, -1
	for i := start; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if tokens[i].Type != parser.SELECT || tokens[i].depth > cursorDepth {
			continue
		}
		if tokens[i].depth > depth || tokens[i].depth == depth {
			index, depth = i, tokens[i].depth
		}
	}
	return index, depth
}

func mysqlAliasClause(tokens []token, selectIndex, cursor, depth int) bool {
	clause, pending := 0, 0
	for i := selectIndex + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if tokens[i].depth != depth {
			continue
		}
		switch tokens[i].Type {
		case parser.GROUP, parser.ORDER:
			pending = tokens[i].Type
			clause = 0
		case parser.HAVING:
			clause = tokens[i].Type
		case parser.WHERE, parser.LIMIT, parser.UNION:
			clause, pending = 0, 0
		default:
			if tokenIs(tokens[i], "BY") {
				if pending == parser.GROUP || pending == parser.ORDER {
					clause = pending
				}
				pending = 0
			}
		}
	}
	return clause == parser.GROUP || clause == parser.HAVING || clause == parser.ORDER
}

func explicitProjectionAliases(tokens []token, selectIndex, depth int) []string {
	end := len(tokens)
	for i := selectIndex + 1; i < len(tokens); i++ {
		if tokens[i].depth == depth && tokens[i].Type == parser.FROM {
			end = i
			break
		}
	}
	var result []string
	itemStart := selectIndex + 1
	for i := itemStart; i <= end; i++ {
		if i != end && (tokens[i].depth != depth || tokens[i].Type != ',') {
			continue
		}
		if alias := explicitProjectionAlias(tokens[itemStart:i], depth); alias != "" {
			result = append(result, alias)
		}
		itemStart = i + 1
	}
	return result
}

func explicitProjectionAlias(tokens []token, depth int) string {
	for i := len(tokens) - 2; i >= 0; i-- {
		if tokens[i].depth == depth && tokens[i].Type == parser.AS &&
			tokens[i+1].depth == depth && parser.IsIdentTokenType(tokens[i+1].Type) {
			return unquote(tokens[i+1].Str)
		}
	}
	if len(tokens) < 2 {
		return ""
	}
	last := tokens[len(tokens)-1]
	previous := tokens[len(tokens)-2]
	if last.depth == depth && parser.IsIdentTokenType(last.Type) &&
		!(previous.depth == depth && previous.Type == '.') {
		for _, item := range tokens[:len(tokens)-1] {
			if item.depth == depth {
				return unquote(last.Str)
			}
		}
	}
	return ""
}

type relationContinuation struct {
	afterJoin bool
	hasAlias  bool
}

func relationContinuationAt(sql string, cursor int) (relationContinuation, bool) {
	if cursor <= 0 || cursor > len(sql) || !unicode.IsSpace(rune(sql[cursor-1])) {
		return relationContinuation{}, false
	}
	tokens := completionTokens(parser.Tokenize(sql[:cursor]))
	if len(tokens) == 0 || tokens[len(tokens)-1].Type == parser.AS {
		return relationContinuation{}, false
	}

	start, end := statementBounds(tokens, cursor)
	cursorDepth := depthAt(tokens, cursor)
	ctes := collectCTEs(tokens, start, end)
	for i := end - 1; i >= start; i-- {
		if tokens[i].depth != cursorDepth ||
			(tokens[i].Type != parser.FROM && tokens[i].Type != parser.JOIN) {
			continue
		}
		ref, consumed, ok := parseReference(tokens, i+1, end, cursorDepth, ctes)
		if !ok || consumed != end {
			return relationContinuation{}, false
		}
		return relationContinuation{
			afterJoin: tokens[i].Type == parser.JOIN,
			hasAlias:  ref.alias != "",
		}, true
	}
	return relationContinuation{}, false
}

func relationContinuationCandidates(context relationContinuation) []completioncore.Candidate {
	labels := make([]string, 0, 20)
	if !context.hasAlias {
		labels = append(labels, "AS")
	}
	if context.afterJoin {
		labels = append(labels, "ON", "USING")
	}
	labels = append(labels,
		"JOIN", "INNER", "LEFT", "RIGHT", "CROSS", "NATURAL", "STRAIGHT_JOIN",
		"WHERE", "GROUP", "HAVING", "WINDOW",
		"UNION", "ORDER", "LIMIT", "FOR",
	)
	result := make([]completioncore.Candidate, 0, len(labels))
	for _, label := range labels {
		result = append(result, completioncore.Candidate{
			Text: label,
			Type: completioncore.CandidateKeyword,
		})
	}
	return result
}

func convertNative(candidate omnicompletion.Candidate) completioncore.Candidate {
	kind := completioncore.CandidateKeyword
	switch candidate.Type {
	case omnicompletion.CandidateDatabase:
		kind = completioncore.CandidateDatabase
	case omnicompletion.CandidateTable:
		kind = completioncore.CandidateTable
	case omnicompletion.CandidateView:
		kind = completioncore.CandidateView
	case omnicompletion.CandidateColumn:
		kind = completioncore.CandidateColumn
	case omnicompletion.CandidateFunction:
		kind = completioncore.CandidateFunction
	case omnicompletion.CandidateProcedure:
		kind = completioncore.CandidateProcedure
	case omnicompletion.CandidateIndex:
		kind = completioncore.CandidateIndex
	case omnicompletion.CandidateTrigger:
		kind = completioncore.CandidateTrigger
	case omnicompletion.CandidateEvent:
		kind = completioncore.CandidateEvent
	case omnicompletion.CandidateVariable:
		kind = completioncore.CandidateVariable
	case omnicompletion.CandidateCharset:
		kind = completioncore.CandidateCharset
	case omnicompletion.CandidateEngine:
		kind = completioncore.CandidateEngine
	case omnicompletion.CandidateType_:
		kind = completioncore.CandidateTypeName
	}
	return completioncore.Candidate{
		Text: candidate.Text, Type: kind, Definition: candidate.Definition, Comment: candidate.Comment,
	}
}

func resolveColumns(sql string, cursor int, qualifier string, metadata completioncore.MetadataResolver) []completioncore.Candidate {
	refs := visibleReferences(sql, cursor)
	if qualifier != "" {
		for _, ref := range refs {
			if !strings.EqualFold(visibleName(ref), qualifier) {
				continue
			}
			return columnsForReference(ref, metadata)
		}
		return nil
	}

	type ownedCandidate struct {
		candidate completioncore.Candidate
		owner     string
	}
	var owned []ownedCandidate
	counts := make(map[string]int)
	for _, ref := range refs {
		for _, candidate := range columnsForReference(ref, metadata) {
			counts[strings.ToLower(candidate.Text)]++
			owned = append(owned, ownedCandidate{candidate: candidate, owner: visibleName(ref)})
		}
	}
	result := make([]completioncore.Candidate, 0, len(owned))
	for _, item := range owned {
		if counts[strings.ToLower(item.candidate.Text)] > 1 && item.owner != "" {
			item.candidate.DisplayText = item.owner + "." + item.candidate.Text
			item.candidate.Text = item.owner + "." + item.candidate.Text
		}
		result = append(result, item.candidate)
	}
	return result
}

func columnsForReference(ref reference, metadata completioncore.MetadataResolver) []completioncore.Candidate {
	if ref.virtual {
		result := make([]completioncore.Candidate, 0, len(ref.columns))
		for _, name := range ref.columns {
			if name != "" && name != "*" {
				result = append(result, completioncore.Candidate{Text: name, Type: completioncore.CandidateColumn})
			}
		}
		return result
	}
	database := ref.database
	if database == "" {
		database = metadata.DefaultDatabase()
	}
	relation, ok := metadata.FindRelation(database, "", ref.table)
	if !ok {
		return nil
	}
	result := make([]completioncore.Candidate, 0, len(relation.Columns))
	for _, column := range relation.Columns {
		result = append(result, completioncore.Candidate{
			Text: column.Name, Type: completioncore.CandidateColumn,
			Definition: completioncore.ColumnDefinition(relation, column), Comment: column.Comment,
		})
	}
	return result
}

func visibleReferences(sql string, cursor int) []reference {
	tokens := completionTokens(parser.Tokenize(sql))
	if len(tokens) == 0 {
		return nil
	}
	stmtStart, stmtEnd := statementBounds(tokens, cursor)
	cursorDepth := depthAt(tokens, cursor)
	ctes := collectCTEs(tokens, stmtStart, stmtEnd)

	var result []reference
	minDepth := 0
	if !outerScopeAllowed(tokens, cursor, cursorDepth) {
		minDepth = cursorDepth
	}
	for depth := cursorDepth; depth >= minDepth; depth-- {
		selectIndex := lastTokenAtDepth(tokens, stmtStart, cursor, depth, parser.SELECT)
		if selectIndex < 0 {
			continue
		}
		result = append(result, collectSelectReferences(tokens, selectIndex, stmtEnd, depth, ctes)...)
	}
	if len(result) == 0 {
		result = append(result, collectDMLReferences(tokens, stmtStart, stmtEnd, 0, ctes)...)
	}
	return deduplicateReferences(result)
}

func outerScopeAllowed(tokens []token, cursor, cursorDepth int) bool {
	if cursorDepth <= 0 {
		return true
	}
	open := -1
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Loc >= cursor {
			continue
		}
		if tokens[i].Type == '(' && tokens[i].depth == cursorDepth-1 {
			open = i
			break
		}
	}
	if open <= 0 {
		return true
	}
	previous := tokens[open-1]
	if tokenIs(previous, "LATERAL") {
		return true
	}
	return previous.Type != parser.FROM && previous.Type != parser.JOIN && previous.Type != ','
}

func completionTokens(source []parser.Token) []token {
	depth := 0
	result := make([]token, 0, len(source))
	for _, item := range source {
		result = append(result, token{Token: item, depth: depth})
		switch item.Type {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return result
}

func statementBounds(tokens []token, cursor int) (int, int) {
	start, end := 0, len(tokens)
	for i, item := range tokens {
		if item.Type != ';' || item.depth != 0 {
			continue
		}
		if item.Loc < cursor {
			start = i + 1
		} else {
			end = i
			break
		}
	}
	return start, end
}

func depthAt(tokens []token, cursor int) int {
	depth := 0
	for _, item := range tokens {
		if item.Loc >= cursor {
			break
		}
		switch item.Type {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

func lastTokenAtDepth(tokens []token, start, cursor, depth, typ int) int {
	result := -1
	for i := start; i < len(tokens) && tokens[i].Loc <= cursor; i++ {
		if tokens[i].depth == depth && tokens[i].Type == typ {
			result = i
		}
	}
	return result
}

func collectSelectReferences(tokens []token, selectIndex, end, depth int, ctes map[string]reference) []reference {
	var result []reference
	for i := selectIndex + 1; i < end; i++ {
		item := tokens[i]
		if item.depth < depth {
			break
		}
		if item.depth != depth {
			continue
		}
		if item.Type != parser.FROM && item.Type != parser.JOIN {
			continue
		}
		ref, next, ok := parseReference(tokens, i+1, end, depth, ctes)
		if ok {
			result = append(result, ref)
			i = next - 1
		}
		if item.Type != parser.FROM {
			continue
		}
		for i+1 < end {
			nextToken := tokens[i+1]
			if nextToken.depth == depth && nextToken.Type == ',' {
				ref, next, ok = parseReference(tokens, i+2, end, depth, ctes)
				if ok {
					result = append(result, ref)
					i = next - 1
					continue
				}
			}
			break
		}
	}
	return result
}

func collectDMLReferences(tokens []token, start, end, depth int, ctes map[string]reference) []reference {
	var result []reference
	for i := start; i < end; i++ {
		if tokens[i].depth != depth {
			continue
		}
		next := i + 1
		switch tokens[i].Type {
		case parser.UPDATE:
		case parser.INSERT, parser.REPLACE:
			if next < end && tokens[next].Type == parser.INTO {
				next++
			}
		case parser.DELETE:
			if next < end && tokens[next].Type == parser.FROM {
				next++
			}
		case parser.FROM, parser.JOIN:
		default:
			continue
		}
		ref, _, ok := parseReference(tokens, next, end, depth, ctes)
		if ok {
			result = append(result, ref)
		}
	}
	return result
}

func parseReference(tokens []token, start, end, depth int, ctes map[string]reference) (reference, int, bool) {
	i := start
	for i < end && tokens[i].depth == depth &&
		(tokenIs(tokens[i], "LATERAL") || tokenIs(tokens[i], "ONLY")) {
		i++
	}
	if i >= end {
		return reference{}, i, false
	}
	if tokens[i].Type == '(' && tokens[i].depth == depth {
		close := matchingParen(tokens, i, end)
		if close < 0 {
			return reference{}, i, false
		}
		alias, columns, next := parseAlias(tokens, close+1, end, depth)
		if alias == "" {
			return reference{}, next, false
		}
		bodyStart, bodyEnd := tokens[i].End, tokens[close].Loc
		body := tokenSliceText(tokens, bodyStart, bodyEnd)
		if len(columns) == 0 {
			columns = projectedNames(body)
		}
		return reference{table: alias, alias: alias, columns: columns, virtual: true, depth: depth}, next, true
	}
	if tokens[i].depth != depth || !parser.IsIdentTokenType(tokens[i].Type) {
		return reference{}, i, false
	}
	ref := reference{table: unquote(tokens[i].Str), depth: depth}
	i++
	if i+1 < end && tokens[i].depth == depth && tokens[i].Type == '.' &&
		tokens[i+1].depth == depth && parser.IsIdentTokenType(tokens[i+1].Type) {
		ref.database, ref.table = ref.table, unquote(tokens[i+1].Str)
		i += 2
	}
	ref.alias, _, i = parseAlias(tokens, i, end, depth)
	if cte, ok := ctes[strings.ToLower(ref.table)]; ok && ref.database == "" {
		ref.virtual = true
		ref.columns = cte.columns
	}
	return ref, i, true
}

func parseAlias(tokens []token, start, end, depth int) (string, []string, int) {
	i := start
	if i < end && tokens[i].depth == depth && tokens[i].Type == parser.AS {
		i++
	}
	alias := ""
	if i < end && tokens[i].depth == depth && parser.IsIdentTokenType(tokens[i].Type) && !isClauseToken(tokens[i].Type) {
		alias = unquote(tokens[i].Str)
		i++
	}
	var columns []string
	if alias != "" && i < end && tokens[i].depth == depth && tokens[i].Type == '(' {
		close := matchingParen(tokens, i, end)
		if close >= 0 {
			for j := i + 1; j < close; j++ {
				if parser.IsIdentTokenType(tokens[j].Type) {
					columns = append(columns, unquote(tokens[j].Str))
				}
			}
			i = close + 1
		}
	}
	return alias, columns, i
}

func collectCTEs(tokens []token, start, end int) map[string]reference {
	result := make(map[string]reference)
	for i := start; i < end; i++ {
		if !tokenIs(tokens[i], "WITH") {
			continue
		}
		depth := tokens[i].depth
		i++
		if i < end && tokenIs(tokens[i], "RECURSIVE") {
			i++
		}
		for i < end && tokens[i].depth == depth && parser.IsIdentTokenType(tokens[i].Type) {
			name := unquote(tokens[i].Str)
			i++
			var columns []string
			if i < end && tokens[i].Type == '(' && tokens[i].depth == depth {
				close := matchingParen(tokens, i, end)
				if close < 0 {
					break
				}
				for j := i + 1; j < close; j++ {
					if parser.IsIdentTokenType(tokens[j].Type) {
						columns = append(columns, unquote(tokens[j].Str))
					}
				}
				i = close + 1
			}
			if i < end && tokens[i].Type == parser.AS {
				i++
			}
			if i >= end || tokens[i].Type != '(' || tokens[i].depth != depth {
				break
			}
			close := matchingParen(tokens, i, end)
			if close < 0 {
				break
			}
			if len(columns) == 0 {
				columns = projectedNames(tokenSliceText(tokens, tokens[i].End, tokens[close].Loc))
			}
			result[strings.ToLower(name)] = reference{
				table: name, columns: columns, virtual: true, depth: depth,
			}
			i = close + 1
			if i >= end || tokens[i].Type != ',' || tokens[i].depth != depth {
				break
			}
			i++
		}
	}
	return result
}

func projectedNames(sql string) []string {
	tokens := completionTokens(parser.Tokenize(sql))
	selectIndex := -1
	selectDepth := 0
	for i, item := range tokens {
		if item.Type == parser.SELECT {
			selectIndex, selectDepth = i, item.depth
			break
		}
	}
	if selectIndex < 0 {
		return nil
	}
	var names []string
	start := selectIndex + 1
	for i := start; i <= len(tokens); i++ {
		atEnd := i == len(tokens)
		if !atEnd && tokens[i].depth == selectDepth && tokens[i].Type == parser.FROM {
			atEnd = true
		}
		if atEnd || (tokens[i].depth == selectDepth && tokens[i].Type == ',') {
			if name := projectionName(tokens[start:i]); name != "" {
				names = append(names, name)
			}
			start = i + 1
			if atEnd {
				break
			}
		}
	}
	return names
}

func projectionName(tokens []token) string {
	if len(tokens) == 0 {
		return ""
	}
	for i := len(tokens) - 2; i >= 0; i-- {
		if tokens[i].Type == parser.AS && i+1 < len(tokens) {
			return unquote(tokens[i+1].Str)
		}
	}
	last := tokens[len(tokens)-1]
	if parser.IsIdentTokenType(last.Type) {
		return unquote(last.Str)
	}
	return ""
}

func matchingParen(tokens []token, open, end int) int {
	if open >= end || tokens[open].Type != '(' {
		return -1
	}
	depth := tokens[open].depth
	for i := open + 1; i < end; i++ {
		if tokens[i].Type == ')' && tokens[i].depth == depth+1 {
			return i
		}
	}
	return -1
}

func tokenSliceText(tokens []token, start, end int) string {
	var builder strings.Builder
	for _, item := range tokens {
		if item.Loc < start || item.End > end {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(item.Str)
	}
	return builder.String()
}

func hasRule(candidates *parser.CandidateSet, rule string) bool {
	if candidates == nil {
		return false
	}
	for _, candidate := range candidates.Rules {
		if candidate.Rule == rule {
			return true
		}
	}
	return false
}

func isDMLColumnContext(sql string, cursor int) bool {
	tokens := parser.Tokenize(sql)
	start := 0
	for i, item := range tokens {
		if item.Loc >= cursor {
			break
		}
		if item.Type == ';' {
			start = i + 1
		}
	}
	if start >= len(tokens) {
		return false
	}
	operation := tokens[start].Type
	if strings.EqualFold(tokens[start].Str, "WITH") {
		depth := 0
		for i := start + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
			switch tokens[i].Type {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case parser.INSERT, parser.REPLACE, parser.UPDATE, parser.DELETE:
				if depth == 0 {
					operation = tokens[i].Type
					i = len(tokens)
				}
			}
		}
	}
	switch operation {
	case parser.UPDATE, parser.DELETE:
		return true
	case parser.INSERT, parser.REPLACE:
		for i := start + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
			switch tokens[i].Type {
			case parser.VALUES:
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isSelectProjectionContext reports whether the cursor sits in a SELECT
// projection list, before that SELECT's own FROM/WHERE/GROUP/... clause. The
// grammar caret covers this when the projection parses, but a half-typed
// identifier immediately followed by a real FROM clause defeats error
// recovery, so the token scan keeps column resolution working there.
func isSelectProjectionContext(sql string, cursor int) bool {
	tokens := completionTokens(parser.Tokenize(sql))
	if len(tokens) == 0 {
		return false
	}
	start, _ := statementBounds(tokens, cursor)
	cursorDepth := depthAt(tokens, cursor)
	selectIndex, selectDepth := activeSelect(tokens, start, cursor, cursorDepth)
	if selectIndex < 0 || selectDepth != cursorDepth {
		return false
	}
	for i := selectIndex + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if tokens[i].depth != selectDepth {
			continue
		}
		switch tokens[i].Type {
		case parser.FROM, parser.WHERE, parser.GROUP, parser.HAVING,
			parser.ORDER, parser.LIMIT, parser.UNION, parser.INTO:
			return false
		}
	}
	return true
}

func isInsertValueContext(sql string, cursor int) bool {
	tokens := parser.Tokenize(sql)
	inInsert := false
	for _, item := range tokens {
		if item.Loc >= cursor {
			break
		}
		if item.Type == ';' {
			inInsert = false
			continue
		}
		if item.Type == parser.INSERT || item.Type == parser.REPLACE ||
			strings.EqualFold(item.Str, "INSERT") || strings.EqualFold(item.Str, "REPLACE") {
			inInsert = true
			continue
		}
		if inInsert && (item.Type == parser.VALUES ||
			strings.EqualFold(item.Str, "VALUES")) {
			return true
		}
	}
	return false
}

func visibleName(ref reference) string {
	if ref.alias != "" {
		return ref.alias
	}
	return ref.table
}

func qualifierAt(sql string, cursor int) string {
	if cursor < 0 || cursor > len(sql) {
		return ""
	}
	i := cursor
	for i > 0 && isIdentRune(rune(sql[i-1])) {
		i--
	}
	if i == 0 || sql[i-1] != '.' {
		return ""
	}
	i--
	for i > 0 && unicode.IsSpace(rune(sql[i-1])) {
		i--
	}
	end := i
	if i > 0 && sql[i-1] == '`' {
		i--
		for i > 0 {
			i--
			if sql[i] == '`' {
				return strings.ReplaceAll(sql[i+1:end-1], "``", "`")
			}
		}
		return ""
	}
	for i > 0 && isIdentRune(rune(sql[i-1])) {
		i--
	}
	return sql[i:end]
}

func prefixAt(sql string, cursor int) string {
	if cursor < 0 || cursor > len(sql) {
		return ""
	}
	start := cursor
	for start > 0 && isIdentRune(rune(sql[start-1])) {
		start--
	}
	return sql[start:cursor]
}

func filterByPrefix(candidates []completioncore.Candidate, prefix string) []completioncore.Candidate {
	if prefix == "" {
		return candidates
	}
	prefix = strings.ToLower(prefix)
	result := make([]completioncore.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		text := candidate.Text
		if dot := strings.LastIndexByte(text, '.'); dot >= 0 {
			text = text[dot+1:]
		}
		if strings.HasPrefix(strings.ToLower(text), prefix) {
			result = append(result, candidate)
		}
	}
	return result
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

func unquote(value string) string {
	if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
		return strings.ReplaceAll(value[1:len(value)-1], "``", "`")
	}
	return value
}

func isClauseToken(typ int) bool {
	switch typ {
	case parser.SET, parser.WHERE, parser.ON, parser.VALUES, parser.JOIN,
		parser.INNER, parser.LEFT, parser.RIGHT, parser.CROSS, parser.NATURAL,
		parser.ORDER, parser.GROUP, parser.HAVING, parser.LIMIT, parser.UNION,
		parser.FOR, parser.USING, parser.FROM, parser.INTO,
		parser.SELECT, parser.INSERT, parser.UPDATE, parser.DELETE:
		return true
	default:
		return false
	}
}

func tokenIs(item token, name string) bool {
	return strings.EqualFold(item.Str, name) || strings.EqualFold(parser.TokenName(item.Type), name)
}

func deduplicateReferences(refs []reference) []reference {
	seen := make(map[string]bool, len(refs))
	result := make([]reference, 0, len(refs))
	for _, ref := range refs {
		key := strings.ToLower(ref.database + "\x00" + ref.table + "\x00" + ref.alias)
		if ref.table == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, ref)
	}
	return result
}

func deduplicate(candidates []completioncore.Candidate) []completioncore.Candidate {
	seen := make(map[string]bool, len(candidates))
	result := make([]completioncore.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := string(candidate.Type) + "\x00" + strings.ToLower(candidate.Text)
		if candidate.Text == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, candidate)
	}
	return result
}
