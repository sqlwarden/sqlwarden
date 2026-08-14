// Package postgres adapts Bytebase/Omni PostgreSQL completion to SQLWarden's
// metadata boundary.
package postgres

import (
	"context"
	"strings"
	"unicode"

	omnipg "github.com/bytebase/omni/pg"
	"github.com/bytebase/omni/pg/catalog"
	omnicompletion "github.com/bytebase/omni/pg/completion"
	"github.com/bytebase/omni/pg/parser"

	"github.com/sqlwarden/internal/engine/completioncore"
)

// Complete combines Omni's grammar candidates with parser-native scope
// resolution. nativeCatalog is retained only for Omni candidate resolution;
// semantic resolution reads the dialect-neutral SQLWarden catalog.
func Complete(
	ctx context.Context,
	sql string,
	cursor int,
	nativeCatalog *catalog.Catalog,
	metadata completioncore.MetadataResolver,
) ([]completioncore.Candidate, error) {
	if err := completioncore.CheckContext(ctx); err != nil {
		return nil, err
	}
	native := omnicompletion.Complete(sql, cursor, nativeCatalog)
	completion := omnipg.CollectCompletion(sql, cursor)
	qualifier := qualifierAt(sql, cursor)
	valueContext := isInsertValueContext(sql, cursor)
	columnContext := !valueContext && (qualifier != "" ||
		hasRule(completion.Candidates, "columnref") ||
		hasRule(completion.Candidates, "any_name") ||
		isDMLColumnContext(sql, cursor))
	suppressNativeColumns := columnContext || valueContext

	result := make([]completioncore.Candidate, 0, len(native)+32)
	for _, candidate := range native {
		if !presentableNativeCandidate(candidate) {
			continue
		}
		if suppressNativeColumns && candidate.Type == omnicompletion.CandidateColumn {
			continue
		}
		result = append(result, convertNative(candidate))
	}
	if columnContext && metadata != nil {
		result = append(result, resolveColumns(sql, cursor, completion, metadata)...)
	}
	result = append(result, cteRelationCandidates(sql, cursor)...)
	result = append(result, selectAliasCandidates(sql, cursor)...)
	if continuation, ok := relationContinuationAt(sql, cursor); ok {
		result = relationContinuationCandidates(continuation)
	}
	return filterByPrefix(deduplicate(result), prefixAt(sql, cursor)), completioncore.CheckContext(ctx)
}

func cteRelationCandidates(sql string, cursor int) []completioncore.Candidate {
	tokens := parser.Tokenize(sql)
	if len(tokens) == 0 {
		return nil
	}
	depths, cursorDepth := postgresTokenDepths(tokens, cursor)
	start := 0
	for i, token := range tokens {
		if token.Loc >= cursor {
			break
		}
		if token.Type == ';' && depths[i] == 0 {
			start = i + 1
		}
	}
	selectIndex, selectDepth := activeSelect(tokens, depths, start, cursor, cursorDepth)
	if selectIndex < 0 || !postgresRelationPosition(sql, tokens, depths, selectIndex, cursor, cursorDepth) {
		return nil
	}
	withIndex := -1
	for i := selectIndex - 1; i >= start; i-- {
		if depths[i] == selectDepth && tokens[i].Type == parser.WITH {
			withIndex = i
			break
		}
	}
	if withIndex < 0 {
		return nil
	}
	names := postgresCTENamesAt(tokens, depths, withIndex, selectIndex, selectDepth)
	result := make([]completioncore.Candidate, 0, len(names))
	for _, name := range names {
		result = append(result, completioncore.Candidate{Text: name, Type: completioncore.CandidateTable})
	}
	return result
}

func postgresRelationPosition(sql string, tokens []parser.Token, depths []int, selectIndex, cursor, depth int) bool {
	indices := make([]int, 0, 2)
	for i := selectIndex + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if depths[i] == depth {
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
	if len(indices) < 2 || prefixAt(sql, cursor) == "" {
		return false
	}
	previous := tokens[indices[len(indices)-2]]
	return previous.Type == parser.FROM || previous.Type == parser.JOIN
}

func postgresCTENamesAt(tokens []parser.Token, depths []int, withIndex, end, depth int) []string {
	i := withIndex + 1
	if i < end && tokens[i].Type == parser.RECURSIVE {
		i++
	}
	var result []string
	for i < end && depths[i] == depth && parser.IsIdentifierTokenType(tokens[i].Type) {
		name := normalizeIdentifier(tokens[i].Str)
		i++
		if i < end && depths[i] == depth && tokens[i].Type == '(' {
			close := matchingPGParen(tokens, i, end)
			if close < 0 {
				break
			}
			i = close + 1
		}
		if i < end && depths[i] == depth && tokens[i].Type == parser.AS {
			i++
		}
		if i >= end || depths[i] != depth || tokens[i].Type != '(' {
			break
		}
		close := matchingPGParen(tokens, i, end)
		if close < 0 {
			break
		}
		result = append(result, name)
		i = close + 1
		if i >= end || depths[i] != depth || tokens[i].Type != ',' {
			break
		}
		i++
	}
	return result
}

// selectAliasCandidates returns output aliases from the SELECT owning the
// cursor. PostgreSQL permits those aliases in GROUP BY and ORDER BY, but not
// in HAVING or WHERE.
func selectAliasCandidates(sql string, cursor int) []completioncore.Candidate {
	tokens := parser.Tokenize(sql)
	if len(tokens) == 0 {
		return nil
	}
	depths, cursorDepth := postgresTokenDepths(tokens, cursor)
	start := 0
	for i, token := range tokens {
		if token.Loc >= cursor {
			break
		}
		if token.Type == ';' && depths[i] == 0 {
			start = i + 1
		}
	}
	selectIndex, selectDepth := activeSelect(tokens, depths, start, cursor, cursorDepth)
	if selectIndex < 0 || !postgresAliasClause(tokens, depths, selectIndex, cursor, selectDepth) {
		return nil
	}

	aliases := explicitProjectionAliases(tokens, depths, selectIndex, selectDepth)
	result := make([]completioncore.Candidate, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, completioncore.Candidate{Text: alias, Type: completioncore.CandidateColumn})
	}
	return result
}

func postgresTokenDepths(tokens []parser.Token, cursor int) ([]int, int) {
	depth := 0
	depths := make([]int, len(tokens))
	for i, token := range tokens {
		depths[i] = depth
		if token.Loc >= cursor {
			continue
		}
		switch token.Type {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depths, depth
}

func activeSelect(tokens []parser.Token, depths []int, start, cursor, cursorDepth int) (int, int) {
	index, depth := -1, -1
	for i := start; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if tokens[i].Type != parser.SELECT || depths[i] > cursorDepth {
			continue
		}
		if depths[i] > depth || depths[i] == depth {
			index, depth = i, depths[i]
		}
	}
	return index, depth
}

func postgresAliasClause(tokens []parser.Token, depths []int, selectIndex, cursor, depth int) bool {
	clause, pending := 0, 0
	for i := selectIndex + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
		if depths[i] != depth {
			continue
		}
		switch tokens[i].Type {
		case parser.GROUP_P, parser.ORDER:
			pending = tokens[i].Type
			clause = 0
		case parser.BY:
			if pending == parser.GROUP_P || pending == parser.ORDER {
				clause = pending
			}
			pending = 0
		case parser.WHERE, parser.HAVING, parser.LIMIT, parser.UNION:
			clause, pending = 0, 0
		}
	}
	return clause == parser.GROUP_P || clause == parser.ORDER
}

func explicitProjectionAliases(tokens []parser.Token, depths []int, selectIndex, depth int) []string {
	end := len(tokens)
	for i := selectIndex + 1; i < len(tokens); i++ {
		if depths[i] == depth && tokens[i].Type == parser.FROM {
			end = i
			break
		}
	}
	var result []string
	itemStart := selectIndex + 1
	for i := itemStart; i <= end; i++ {
		if i != end && (depths[i] != depth || tokens[i].Type != ',') {
			continue
		}
		if alias := explicitProjectionAlias(tokens[itemStart:i], depths[itemStart:i], depth); alias != "" {
			result = append(result, alias)
		}
		itemStart = i + 1
	}
	return result
}

func explicitProjectionAlias(tokens []parser.Token, depths []int, depth int) string {
	for i := len(tokens) - 2; i >= 0; i-- {
		if depths[i] == depth && tokens[i].Type == parser.AS &&
			depths[i+1] == depth && parser.IsIdentifierTokenType(tokens[i+1].Type) {
			return normalizeIdentifier(tokens[i+1].Str)
		}
	}
	if len(tokens) < 2 {
		return ""
	}
	last := len(tokens) - 1
	if depths[last] == depth && parser.IsIdentifierTokenType(tokens[last].Type) &&
		!(depths[last-1] == depth && tokens[last-1].Type == '.') {
		for _, itemDepth := range depths[:last] {
			if itemDepth == depth {
				return normalizeIdentifier(tokens[last].Str)
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
	tokens := parser.Tokenize(sql[:cursor])
	if len(tokens) == 0 || tokens[len(tokens)-1].Type == parser.AS {
		return relationContinuation{}, false
	}

	start := 0
	depth := 0
	depths := make([]int, len(tokens))
	for i, token := range tokens {
		if token.Type == ')' && depth > 0 {
			depth--
		}
		depths[i] = depth
		switch token.Type {
		case '(':
			depth++
		case ';':
			if depth == 0 {
				start = i + 1
			}
		}
	}
	cursorDepth := depth
	for i := len(tokens) - 1; i >= start; i-- {
		if depths[i] != cursorDepth || (tokens[i].Type != parser.FROM && tokens[i].Type != parser.JOIN) {
			continue
		}
		ref, consumed, ok := parsePhysicalReference(tokens, i+1, len(tokens))
		if !ok || consumed != len(tokens) {
			return relationContinuation{}, false
		}
		return relationContinuation{
			afterJoin: tokens[i].Type == parser.JOIN,
			hasAlias:  ref.Alias != "",
		}, true
	}
	return relationContinuation{}, false
}

func relationContinuationCandidates(context relationContinuation) []completioncore.Candidate {
	labels := make([]string, 0, 24)
	if !context.hasAlias {
		labels = append(labels, "AS", "TABLESAMPLE")
	}
	if context.afterJoin {
		labels = append(labels, "ON", "USING")
	}
	labels = append(labels,
		"JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "NATURAL",
		"WHERE", "GROUP", "HAVING", "WINDOW",
		"UNION", "INTERSECT", "EXCEPT",
		"ORDER", "LIMIT", "OFFSET", "FETCH", "FOR",
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

func presentableNativeCandidate(candidate omnicompletion.Candidate) bool {
	// Omni patches incomplete relation references with this internal sentinel.
	// It can be returned as a table candidate even though it is not in the catalog.
	if candidate.Text == "_x" {
		return false
	}
	if candidate.Type != omnicompletion.CandidateKeyword {
		return true
	}

	// Grammar punctuation is useful to the parser but noisy in an editor menu.
	// Keep words (including one-character word-like tokens), while leaving
	// punctuation such as ",", ";", and "(" for the user to type directly.
	runes := []rune(candidate.Text)
	return len(runes) != 1 ||
		unicode.IsLetter(runes[0]) ||
		unicode.IsDigit(runes[0]) ||
		runes[0] == '_'
}

func convertNative(candidate omnicompletion.Candidate) completioncore.Candidate {
	kind := completioncore.CandidateKeyword
	switch candidate.Type {
	case omnicompletion.CandidateSchema:
		kind = completioncore.CandidateSchema
	case omnicompletion.CandidateTable:
		kind = completioncore.CandidateTable
	case omnicompletion.CandidateView:
		kind = completioncore.CandidateView
	case omnicompletion.CandidateMaterializedView:
		kind = completioncore.CandidateMaterializedView
	case omnicompletion.CandidateColumn:
		kind = completioncore.CandidateColumn
	case omnicompletion.CandidateFunction:
		kind = completioncore.CandidateFunction
	case omnicompletion.CandidateSequence:
		kind = completioncore.CandidateSequence
	case omnicompletion.CandidateIndex:
		kind = completioncore.CandidateIndex
	case omnicompletion.CandidateType_:
		kind = completioncore.CandidateTypeName
	case omnicompletion.CandidateTrigger:
		kind = completioncore.CandidateTrigger
	}
	return completioncore.Candidate{
		Text: candidate.Text, Type: kind, Definition: candidate.Definition, Comment: candidate.Comment,
	}
}

func resolveColumns(
	sql string,
	cursor int,
	completion *omnipg.CompletionContext,
	metadata completioncore.MetadataResolver,
) []completioncore.Candidate {
	if completion == nil {
		return nil
	}
	qualifier := qualifierAt(sql, cursor)
	qualifierQuoted := qualifierIsQuoted(sql, cursor)
	var refs []parser.RangeReference
	if completion.Scope != nil {
		refs = completion.Scope.References
	}
	if len(refs) == 0 {
		refs = collectDMLReferences(sql, cursor)
	}
	if qualifier != "" {
		for _, ref := range refs {
			if !identifierMatches(visibleName(ref), qualifier, qualifierQuoted) {
				continue
			}
			return columnsForReference(ref, completion, metadata, sql)
		}
		// A qualified expression is unambiguous. Never leak columns from other
		// visible relations when its owner cannot be resolved.
		return nil
	}

	type ownedCandidate struct {
		candidate completioncore.Candidate
		owner     string
	}
	var owned []ownedCandidate
	counts := make(map[string]int)
	for _, ref := range refs {
		for _, candidate := range columnsForReference(ref, completion, metadata, sql) {
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

func columnsForReference(
	ref parser.RangeReference,
	completion *omnipg.CompletionContext,
	metadata completioncore.MetadataResolver,
	sql string,
) []completioncore.Candidate {
	switch ref.Kind {
	case omnipg.RangeReferenceRelation:
		relation, ok := metadata.FindRelation(metadata.DefaultDatabase(), ref.Schema, ref.Name)
		if !ok {
			return nil
		}
		return relationColumns(relation)
	case omnipg.RangeReferenceCTE:
		return virtualColumns(ref, sql)
	case omnipg.RangeReferenceSubquery, omnipg.RangeReferenceJoinAlias, omnipg.RangeReferenceFunction:
		return virtualColumns(ref, sql)
	default:
		return nil
	}
}

func virtualColumns(ref parser.RangeReference, sql string) []completioncore.Candidate {
	names := append([]string(nil), ref.AliasColumns...)
	if len(names) == 0 && validLocation(ref.BodyLoc.Start, ref.BodyLoc.End, len(sql)) {
		names = projectedNames(sql[ref.BodyLoc.Start:ref.BodyLoc.End])
	}
	result := make([]completioncore.Candidate, 0, len(names))
	for _, name := range names {
		if name != "" && name != "*" {
			result = append(result, completioncore.Candidate{Text: name, Type: completioncore.CandidateColumn})
		}
	}
	return result
}

func relationColumns(relation completioncore.Relation) []completioncore.Candidate {
	result := make([]completioncore.Candidate, 0, len(relation.Columns))
	for _, column := range relation.Columns {
		result = append(result, completioncore.Candidate{
			Text: column.Name, Type: completioncore.CandidateColumn,
			Definition: completioncore.ColumnDefinition(relation, column), Comment: column.Comment,
		})
	}
	return result
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
	if operation == parser.WITH {
		depth := 0
		for i := start + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
			switch tokens[i].Type {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case parser.INSERT, parser.UPDATE, parser.DELETE_P:
				if depth == 0 {
					operation = tokens[i].Type
					i = len(tokens)
				}
			}
		}
	}
	switch operation {
	case parser.UPDATE, parser.DELETE_P:
		return true
	case parser.INSERT:
		for i := start + 1; i < len(tokens) && tokens[i].Loc < cursor; i++ {
			switch tokens[i].Type {
			case parser.VALUES, parser.SELECT:
				return false
			}
		}
		return true
	default:
		return false
	}
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
		if item.Type == parser.INSERT || strings.EqualFold(parser.TokenName(item.Type), "INSERT") {
			inInsert = true
			continue
		}
		if inInsert && (item.Type == parser.VALUES ||
			strings.EqualFold(parser.TokenName(item.Type), "VALUES")) {
			return true
		}
	}
	return false
}

func collectDMLReferences(sql string, cursor int) []parser.RangeReference {
	tokens := parser.Tokenize(sql)
	start, end := 0, len(tokens)
	for i, item := range tokens {
		if item.Type != ';' {
			continue
		}
		if item.Loc < cursor {
			start = i + 1
		} else {
			end = i
			break
		}
	}
	ctes := collectPGCTEs(tokens, start, end)
	var result []parser.RangeReference
	for i := start; i < end; i++ {
		next := i + 1
		switch tokens[i].Type {
		case parser.UPDATE:
		case parser.INSERT:
			if next < end && tokens[next].Type == parser.INTO {
				next++
			}
		case parser.DELETE_P:
			if next < end && tokens[next].Type == parser.FROM {
				next++
			}
		case parser.FROM, parser.JOIN:
		default:
			continue
		}
		ref, consumed, ok := parsePhysicalReference(tokens, next, end)
		if ok {
			if cte, exists := ctes[strings.ToLower(ref.Name)]; exists && ref.Schema == "" {
				cte.Alias = ref.Alias
				ref = cte
			}
			result = append(result, ref)
			i = consumed - 1
		}
	}
	return deduplicateReferences(result)
}

func collectPGCTEs(tokens []parser.Token, start, end int) map[string]parser.RangeReference {
	result := make(map[string]parser.RangeReference)
	if start >= end || tokens[start].Type != parser.WITH {
		return result
	}
	i := start + 1
	if i < end && tokens[i].Type == parser.RECURSIVE {
		i++
	}
	for i < end && parser.IsIdentifierTokenType(tokens[i].Type) {
		ref := parser.RangeReference{
			Kind: omnipg.RangeReferenceCTE,
			Name: normalizeIdentifier(tokens[i].Str),
		}
		i++
		if i < end && tokens[i].Type == '(' {
			close := matchingPGParen(tokens, i, end)
			if close < 0 {
				break
			}
			for j := i + 1; j < close; j++ {
				if parser.IsIdentifierTokenType(tokens[j].Type) {
					ref.AliasColumns = append(ref.AliasColumns, normalizeIdentifier(tokens[j].Str))
				}
			}
			i = close + 1
		}
		if i < end && tokens[i].Type == parser.AS {
			i++
		}
		if i >= end || tokens[i].Type != '(' {
			break
		}
		close := matchingPGParen(tokens, i, end)
		if close < 0 {
			break
		}
		ref.BodyLoc.Start = tokens[i].End
		ref.BodyLoc.End = tokens[close].Loc
		result[strings.ToLower(ref.Name)] = ref
		i = close + 1
		if i >= end || tokens[i].Type != ',' {
			break
		}
		i++
	}
	return result
}

func matchingPGParen(tokens []parser.Token, open, end int) int {
	depth := 0
	for i := open; i < end; i++ {
		switch tokens[i].Type {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parsePhysicalReference(tokens []parser.Token, start, end int) (parser.RangeReference, int, bool) {
	i := start
	if i < end && tokens[i].Type == parser.ONLY {
		i++
	}
	if i >= end || !parser.IsIdentifierTokenType(tokens[i].Type) {
		return parser.RangeReference{}, i, false
	}
	ref := parser.RangeReference{Kind: omnipg.RangeReferenceRelation, Name: normalizeIdentifier(tokens[i].Str)}
	i++
	if i+1 < end && tokens[i].Type == '.' && parser.IsIdentifierTokenType(tokens[i+1].Type) {
		ref.Schema, ref.Name = ref.Name, normalizeIdentifier(tokens[i+1].Str)
		i += 2
	}
	if i < end && tokens[i].Type == parser.AS {
		i++
	}
	if i < end && parser.IsIdentifierTokenType(tokens[i].Type) && !isPGClauseToken(tokens[i].Type) {
		ref.Alias = normalizeIdentifier(tokens[i].Str)
		i++
	}
	return ref, i, true
}

func isPGClauseToken(typ int) bool {
	switch typ {
	case parser.SET, parser.WHERE, parser.ON, parser.VALUES, parser.JOIN,
		parser.INNER_P, parser.LEFT, parser.RIGHT, parser.CROSS, parser.NATURAL,
		parser.ORDER, parser.GROUP_P, parser.HAVING, parser.LIMIT, parser.UNION,
		parser.FOR, parser.USING, parser.FROM, parser.INTO, parser.RETURNING,
		parser.SELECT, parser.INSERT, parser.UPDATE, parser.DELETE_P:
		return true
	default:
		return false
	}
}

func deduplicateReferences(refs []parser.RangeReference) []parser.RangeReference {
	seen := make(map[string]bool, len(refs))
	result := make([]parser.RangeReference, 0, len(refs))
	for _, ref := range refs {
		key := strings.ToLower(ref.Schema + "\x00" + ref.Name + "\x00" + ref.Alias)
		if ref.Name == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, ref)
	}
	return result
}

func visibleName(ref parser.RangeReference) string {
	if ref.Alias != "" {
		return normalizeIdentifier(ref.Alias)
	}
	return normalizeIdentifier(ref.Name)
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
	if i > 0 && sql[i-1] == '"' {
		i--
		for i > 0 {
			i--
			if sql[i] == '"' {
				return strings.ReplaceAll(sql[i+1:end-1], `""`, `"`)
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

func qualifierIsQuoted(sql string, cursor int) bool {
	i := cursor
	for i > 0 && isIdentRune(rune(sql[i-1])) {
		i--
	}
	if i == 0 || sql[i-1] != '.' {
		return false
	}
	i--
	for i > 0 && unicode.IsSpace(rune(sql[i-1])) {
		i--
	}
	return i > 0 && sql[i-1] == '"'
}

func identifierMatches(left, right string, quoted bool) bool {
	if quoted {
		return left == right
	}
	return strings.EqualFold(left, right)
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

func validLocation(start, end, length int) bool {
	return start >= 0 && end > start && end <= length
}

// projectedNames is a completion-oriented projection reader for virtual
// relations. Omni owns scope discovery; this small fallback only names direct
// SELECT outputs until Omni exposes semantic projected columns.
func projectedNames(sql string) []string {
	tokens := parser.Tokenize(sql)
	selectIndex := -1
	depth := 0
	selectDepth := 0
	for i, token := range tokens {
		switch token.Type {
		case '(':
			depth++
		case ')':
			depth--
		case parser.SELECT:
			if selectIndex < 0 {
				selectIndex, selectDepth = i, depth
			}
		}
	}
	if selectIndex < 0 {
		return nil
	}
	var names []string
	itemStart := selectIndex + 1
	depth = selectDepth
	for i := selectIndex + 1; i <= len(tokens); i++ {
		atEnd := i == len(tokens)
		if !atEnd {
			switch tokens[i].Type {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == selectDepth && tokens[i].Type == parser.FROM {
				atEnd = true
			}
		}
		if atEnd || (depth == selectDepth && tokens[i].Type == ',') {
			if name := projectionName(tokens[itemStart:i]); name != "" {
				names = append(names, name)
			}
			itemStart = i + 1
			if atEnd {
				break
			}
		}
	}
	return names
}

func projectionName(tokens []parser.Token) string {
	if len(tokens) == 0 {
		return ""
	}
	for i := len(tokens) - 2; i >= 0; i-- {
		if tokens[i].Type == parser.AS && i+1 < len(tokens) {
			return normalizeIdentifier(tokens[i+1].Str)
		}
	}
	last := tokens[len(tokens)-1]
	if parser.IsIdentifierTokenType(last.Type) {
		if len(tokens) == 1 || tokens[len(tokens)-2].Type == '.' {
			return normalizeIdentifier(last.Str)
		}
		// PostgreSQL allows a bare output alias.
		return normalizeIdentifier(last.Str)
	}
	return ""
}

func normalizeIdentifier(name string) string {
	if len(name) >= 2 && name[0] == '"' && name[len(name)-1] == '"' {
		return strings.ReplaceAll(name[1:len(name)-1], `""`, `"`)
	}
	return name
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
