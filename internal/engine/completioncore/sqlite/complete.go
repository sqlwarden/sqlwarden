package sqlite

import (
	"context"
	"strings"

	rqlitesql "github.com/rqlite/sql"

	"github.com/sqlwarden/internal/engine/completioncore"
)

type tokenInfo struct {
	tok    rqlitesql.Token
	lit    string
	offset int
}

// Complete returns cursor-aware candidates for in-progress SQLite SQL. Intent
// (columns / relations / keywords) is derived from the significant tokens
// preceding the cursor; names come only from meta.
func Complete(
	ctx context.Context,
	sql string,
	cursor int,
	meta completioncore.MetadataResolver,
) ([]completioncore.Candidate, completioncore.Context, error) {
	if err := completioncore.CheckContext(ctx); err != nil {
		return nil, completioncore.Context{}, err
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(sql) {
		cursor = len(sql)
	}

	prefix, qualifier := prefixAndQualifier(sql, cursor)
	toks := scanBefore(sql, cursor-len(prefix)-len(qualifier))
	intent := classifyIntent(toks, qualifier != "")

	// Relation extraction needs the whole statement, not just the tokens
	// before the cursor (the FROM clause often follows the SELECT list).
	allToks := scanBefore(sql, len(sql))

	var out []completioncore.Candidate
	if meta != nil {
		switch {
		case intent == intentColumn:
			out = append(out, columnCandidates(allToks, qualifier, meta)...)
		case intent == intentRelation:
			out = append(out, relationCandidates(meta)...)
			for _, db := range meta.DatabaseNames() {
				out = append(out, completioncore.Candidate{Text: db, Type: completioncore.CandidateDatabase})
			}
		}
	}

	// SQL keywords are valid in every non-qualified position, and scalar/
	// aggregate functions belong anywhere a column expression is expected.
	// A qualified prefix (`t.`) only ever resolves to that relation's columns.
	if qualifier == "" {
		for _, kw := range keywords {
			out = append(out, completioncore.Candidate{Text: kw, Type: completioncore.CandidateKeyword})
		}
		if intent == intentColumn || intent == intentUnknown {
			for _, fn := range functions {
				out = append(out, completioncore.Candidate{Text: fn, Type: completioncore.CandidateFunction})
			}
		}
	}

	out = filterByPrefix(dedupe(out), prefix)
	return out, completioncore.Context{Position: positionFor(intent, qualifier)}, completioncore.CheckContext(ctx)
}

type intent int

const (
	intentUnknown intent = iota
	intentKeyword
	intentRelation
	intentColumn
)

// scanBefore tokenizes sql[:end] and returns the significant (non-COMMENT)
// tokens in order.
func scanBefore(sql string, end int) []tokenInfo {
	if end < 0 {
		end = 0
	}
	if end > len(sql) {
		end = len(sql)
	}
	s := rqlitesql.NewScanner(strings.NewReader(sql[:end]))
	var toks []tokenInfo
	for {
		pos, tok, lit := s.Scan()
		if tok == rqlitesql.EOF {
			return toks
		}
		if tok == rqlitesql.COMMENT {
			continue
		}
		toks = append(toks, tokenInfo{tok: tok, lit: lit, offset: pos.Offset})
	}
}

// classifyIntent walks the token list backward to the nearest clause keyword
// that fixes what may follow it.
func classifyIntent(toks []tokenInfo, qualified bool) intent {
	if qualified {
		return intentColumn
	}
	for i := len(toks) - 1; i >= 0; i-- {
		switch toks[i].tok {
		case rqlitesql.FROM, rqlitesql.JOIN, rqlitesql.INTO, rqlitesql.UPDATE, rqlitesql.TABLE:
			return intentRelation
		case rqlitesql.SELECT, rqlitesql.WHERE, rqlitesql.ON, rqlitesql.HAVING, rqlitesql.SET, rqlitesql.BY:
			return intentColumn
		case rqlitesql.COMMA, rqlitesql.LP:
			continue // stay in the enclosing clause
		case rqlitesql.SEMI:
			return intentKeyword
		}
	}
	if len(toks) == 0 {
		return intentKeyword
	}
	return intentUnknown
}

func positionFor(in intent, _ string) string {
	switch in {
	case intentColumn:
		return completioncore.PositionColumn
	case intentRelation:
		return completioncore.PositionRelation
	case intentKeyword:
		return completioncore.PositionKeyword
	default:
		return completioncore.PositionAny
	}
}

// prefixAndQualifier splits the text immediately left of the cursor into an
// optional `qualifier.` and the trailing identifier prefix being typed.
func prefixAndQualifier(sql string, cursor int) (prefix, qualifier string) {
	i := cursor
	for i > 0 && isIdentByte(sql[i-1]) {
		i--
	}
	prefix = sql[i:cursor]
	if i > 0 && sql[i-1] == '.' {
		j := i - 1
		for j > 0 && isIdentByte(sql[j-1]) {
			j--
		}
		qualifier = sql[j : i-1]
	}
	return prefix, qualifier
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// relationCandidates returns every visible table/view in the default database.
func relationCandidates(meta completioncore.MetadataResolver) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, r := range meta.Relations(meta.DefaultDatabase(), meta.DefaultSchema()) {
		out = append(out, completioncore.Candidate{Text: r.Name, Type: r.Kind})
	}
	return out
}

// columnCandidates resolves columns. With a qualifier, it maps an alias to its
// relation via the FROM sources in toks; without one, it unions the columns of
// every relation referenced in the statement.
func columnCandidates(toks []tokenInfo, qualifier string, meta completioncore.MetadataResolver) []completioncore.Candidate {
	relations := fromRelations(toks)
	var out []completioncore.Candidate
	seen := map[string]bool{}
	add := func(rel string) {
		r, ok := meta.FindRelation(meta.DefaultDatabase(), meta.DefaultSchema(), rel)
		if !ok {
			return
		}
		for _, c := range r.Columns {
			if !seen[c.Name] {
				seen[c.Name] = true
				out = append(out, completioncore.Candidate{Text: c.Name, Type: completioncore.CandidateColumn})
			}
		}
	}
	if qualifier != "" {
		for _, fr := range relations {
			if strings.EqualFold(fr.alias, qualifier) || strings.EqualFold(fr.name, qualifier) {
				add(fr.name)
			}
		}
		if len(out) == 0 {
			add(qualifier) // qualifier may itself be a table name not aliased
		}
		return out
	}
	for _, fr := range relations {
		add(fr.name)
	}
	return out
}

type fromRelation struct{ alias, name string }

// fromRelations extracts `name [AS] alias` pairs from FROM/JOIN/UPDATE/INTO
// clauses in the token list.
func fromRelations(toks []tokenInfo) []fromRelation {
	var out []fromRelation
	for i := 0; i < len(toks); i++ {
		switch toks[i].tok {
		case rqlitesql.FROM, rqlitesql.JOIN, rqlitesql.INTO, rqlitesql.UPDATE:
			if i+1 < len(toks) && isIdentTok(toks[i+1].tok) {
				fr := fromRelation{name: toks[i+1].lit}
				j := i + 2
				if j < len(toks) && toks[j].tok == rqlitesql.AS {
					j++
				}
				if j < len(toks) && isIdentTok(toks[j].tok) && !isClauseKeyword(toks[j].tok) {
					fr.alias = toks[j].lit
				}
				out = append(out, fr)
			}
		}
	}
	return out
}

func isIdentTok(t rqlitesql.Token) bool {
	return t == rqlitesql.IDENT || t == rqlitesql.QIDENT
}

func isClauseKeyword(t rqlitesql.Token) bool {
	switch t {
	case rqlitesql.WHERE, rqlitesql.GROUP, rqlitesql.ORDER, rqlitesql.LIMIT,
		rqlitesql.JOIN, rqlitesql.ON, rqlitesql.LEFT, rqlitesql.INNER,
		rqlitesql.CROSS, rqlitesql.SET, rqlitesql.HAVING, rqlitesql.WINDOW:
		return true
	}
	return false
}

func filterByPrefix(cs []completioncore.Candidate, prefix string) []completioncore.Candidate {
	if prefix == "" {
		return cs
	}
	lp := strings.ToLower(prefix)
	var out []completioncore.Candidate
	for _, c := range cs {
		if strings.HasPrefix(strings.ToLower(c.Text), lp) {
			out = append(out, c)
		}
	}
	return out
}

func dedupe(cs []completioncore.Candidate) []completioncore.Candidate {
	seen := map[string]bool{}
	var out []completioncore.Candidate
	for _, c := range cs {
		k := string(c.Type) + "\x00" + strings.ToLower(c.Text)
		if c.Text == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}
