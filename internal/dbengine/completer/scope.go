package completer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sqlwarden/internal/dbengine/schema"
)

// ScopedColumn is a schema column together with the visible relation name that
// owns it. Owner is an alias when one exists.
type ScopedColumn struct {
	Name         string
	DataType     string
	Owner        string
	Qualified    bool
	DisplayLabel string
}

type relationRef struct {
	namespace string
	table     string
	alias     string
	depth     int
}

type sqlToken struct {
	text  string
	lower string
	depth int
	start int
	end   int
}

// ScopedColumns resolves columns using the relation aliases visible around the
// cursor. It is deliberately a compatibility boundary around Omni completion:
// once Omni provides equivalent scope-aware candidates, callers can remove it.
//
// TODO: Replace this compatibility resolver when Omni's PostgreSQL and MySQL
// completion roadmap provides tested alias- and query-scope-aware completion.
func ScopedColumns(sqlText string, cursor int, objects []schema.Object, dialect string) ([]ScopedColumn, bool) {
	if cursor < 0 || cursor > len(sqlText) || len(objects) == 0 {
		return nil, false
	}
	tokens := tokenizeSQL(sqlText)
	depth := depthAt(tokens, cursor)
	qualifier := qualifierAt(tokens, cursor)
	refs := visibleRelationRefs(tokens, depth)
	if len(refs) == 0 {
		return nil, qualifier != ""
	}

	if qualifier != "" {
		for _, ref := range refs {
			visible := ref.table
			if ref.alias != "" {
				visible = ref.alias
			}
			if !identifierEqual(visible, qualifier, dialect) {
				continue
			}
			object, ok := findRelationObject(objects, ref, dialect)
			if !ok || object.Relational == nil {
				return nil, true
			}
			result := make([]ScopedColumn, 0, len(object.Relational.Columns))
			for _, column := range object.Relational.Columns {
				result = append(result, ScopedColumn{
					Name: column.Name, DataType: column.DataType, Owner: visible,
				})
			}
			return result, true
		}
		// A syntactically qualified reference must not leak columns from other
		// relations when its owner cannot be resolved.
		return nil, true
	}

	type ownedColumn struct {
		name, dataType, owner string
	}
	var all []ownedColumn
	counts := make(map[string]int)
	for _, ref := range refs {
		object, ok := findRelationObject(objects, ref, dialect)
		if !ok || object.Relational == nil {
			continue
		}
		owner := ref.table
		if ref.alias != "" {
			owner = ref.alias
		}
		for _, column := range object.Relational.Columns {
			key := identifierKey(column.Name, dialect)
			counts[key]++
			all = append(all, ownedColumn{name: column.Name, dataType: column.DataType, owner: owner})
		}
	}
	if len(all) == 0 {
		return nil, false
	}
	result := make([]ScopedColumn, 0, len(all))
	seen := make(map[string]bool)
	for _, column := range all {
		key := identifierKey(column.name, dialect)
		if counts[key] == 1 {
			result = append(result, ScopedColumn{Name: column.name, DataType: column.dataType, Owner: column.owner})
			continue
		}
		if column.owner != "" {
			result = append(result, ScopedColumn{
				Name: column.name, DataType: column.dataType, Owner: column.owner, Qualified: true,
			})
			continue
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, ScopedColumn{
				Name: column.name, DataType: column.dataType,
				DisplayLabel: column.name + " (" + smallInt(counts[key]) + ")",
			})
		}
	}
	return result, true
}

func qualifierAt(tokens []sqlToken, cursor int) string {
	var before []sqlToken
	for _, token := range tokens {
		if token.start >= cursor {
			break
		}
		if token.end <= cursor {
			before = append(before, token)
		}
	}
	if len(before) < 2 {
		return ""
	}
	last := len(before) - 1
	if before[last].text == "." && last > 0 {
		return before[last-1].text
	}
	if last >= 2 && before[last-1].text == "." && before[last].start <= cursor {
		return before[last-2].text
	}
	return ""
}

func visibleRelationRefs(tokens []sqlToken, cursorDepth int) []relationRef {
	var refs []relationRef
	for _, wantedDepth := range []int{cursorDepth, cursorDepth - 1, cursorDepth - 2} {
		if wantedDepth < 0 {
			continue
		}
		for i := 0; i < len(tokens); i++ {
			token := tokens[i]
			if token.depth != wantedDepth || !isRelationIntroducer(token.lower) {
				continue
			}
			ref, next, ok := relationAfter(tokens, i+1, wantedDepth)
			if ok {
				refs = append(refs, ref)
				i = next - 1
			}
		}
	}
	return dedupRefs(refs)
}

func relationAfter(tokens []sqlToken, i, depth int) (relationRef, int, bool) {
	for i < len(tokens) && tokens[i].depth == depth && (tokens[i].lower == "only" || tokens[i].lower == "lateral") {
		i++
	}
	if i >= len(tokens) || tokens[i].depth != depth || !isIdentifierToken(tokens[i]) {
		return relationRef{}, i, false
	}
	ref := relationRef{table: tokens[i].text, depth: depth}
	i++
	if i+1 < len(tokens) && tokens[i].depth == depth && tokens[i].text == "." && isIdentifierToken(tokens[i+1]) {
		ref.namespace, ref.table = ref.table, tokens[i+1].text
		i += 2
	}
	if i < len(tokens) && tokens[i].depth == depth && tokens[i].lower == "as" {
		i++
		if i < len(tokens) && tokens[i].depth == depth && isIdentifierToken(tokens[i]) {
			ref.alias = tokens[i].text
			i++
		}
	} else if i < len(tokens) && tokens[i].depth == depth && isIdentifierToken(tokens[i]) && !isClauseKeyword(tokens[i].lower) {
		ref.alias = tokens[i].text
		i++
	}
	return ref, i, true
}

func findRelationObject(objects []schema.Object, ref relationRef, dialect string) (schema.Object, bool) {
	for _, object := range objects {
		if object.Ref.Kind != "table" && object.Ref.Kind != "view" && object.Ref.Kind != "materialized_view" && object.Ref.Kind != "foreign_table" {
			continue
		}
		if !identifierEqual(object.Ref.Name, ref.table, dialect) {
			continue
		}
		if ref.namespace != "" && !identifierEqual(object.Ref.Namespace, ref.namespace, dialect) {
			continue
		}
		return object, true
	}
	return schema.Object{}, false
}

func tokenizeSQL(text string) []sqlToken {
	var tokens []sqlToken
	depth := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if r == '-' && i+1 < len(text) && text[i+1] == '-' {
			i += 2
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		if r == '/' && i+1 < len(text) && text[i+1] == '*' {
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				i++
			}
			if i+1 < len(text) {
				i += 2
			}
			continue
		}
		start := i
		if r == '\'' {
			i += size
			for i < len(text) {
				if text[i] == '\'' {
					i++
					if i < len(text) && text[i] == '\'' {
						i++
						continue
					}
					break
				}
				_, n := utf8.DecodeRuneInString(text[i:])
				i += n
			}
			continue
		}
		if r == '"' || r == '`' {
			quote := byte(r)
			i += size
			var value strings.Builder
			for i < len(text) {
				if text[i] == quote {
					i++
					if i < len(text) && text[i] == quote {
						value.WriteByte(quote)
						i++
						continue
					}
					break
				}
				rr, n := utf8.DecodeRuneInString(text[i:])
				value.WriteRune(rr)
				i += n
			}
			tokens = append(tokens, sqlToken{text: value.String(), lower: strings.ToLower(value.String()), depth: depth, start: start, end: i})
			continue
		}
		if isIdentifierRune(r) {
			i += size
			for i < len(text) {
				rr, n := utf8.DecodeRuneInString(text[i:])
				if !isIdentifierRune(rr) {
					break
				}
				i += n
			}
			word := text[start:i]
			tokens = append(tokens, sqlToken{text: word, lower: strings.ToLower(word), depth: depth, start: start, end: i})
			continue
		}
		switch r {
		case '(':
			tokens = append(tokens, sqlToken{text: "(", lower: "(", depth: depth, start: start, end: start + size})
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
			tokens = append(tokens, sqlToken{text: ")", lower: ")", depth: depth, start: start, end: start + size})
		case '.', ',', ';':
			tokens = append(tokens, sqlToken{text: string(r), lower: string(r), depth: depth, start: start, end: start + size})
		}
		i += size
	}
	return tokens
}

func depthAt(tokens []sqlToken, cursor int) int {
	depth := 0
	for _, token := range tokens {
		if token.start >= cursor {
			break
		}
		if token.text == "(" {
			depth = token.depth + 1
		} else if token.text == ")" {
			depth = token.depth
		}
	}
	return depth
}

func isIdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

func isIdentifierToken(token sqlToken) bool {
	return token.text != "" && token.text != "." && token.text != "," && token.text != "(" && token.text != ")" && token.text != ";"
}

func isRelationIntroducer(s string) bool {
	return s == "from" || s == "join" || s == "update" || s == "into"
}

func isClauseKeyword(s string) bool {
	switch s {
	case "where", "join", "left", "right", "full", "inner", "outer", "cross", "natural",
		"on", "using", "group", "order", "having", "limit", "offset", "union", "except",
		"intersect", "window", "set", "values", "returning", "for", "as":
		return true
	default:
		return false
	}
}

func identifierEqual(a, b, dialect string) bool {
	if dialect == "mysql" {
		return strings.EqualFold(a, b)
	}
	return a == b || strings.EqualFold(a, b)
}

func identifierKey(value, dialect string) string {
	if dialect == "mysql" {
		return strings.ToLower(value)
	}
	return strings.ToLower(value)
}

func dedupRefs(refs []relationRef) []relationRef {
	seen := make(map[string]bool)
	result := make([]relationRef, 0, len(refs))
	for _, ref := range refs {
		key := strings.ToLower(ref.namespace + "\x00" + ref.table + "\x00" + ref.alias)
		if !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}
	return result
}

func smallInt(value int) string {
	if value < 10 {
		return string(rune('0' + value))
	}
	return "many"
}
