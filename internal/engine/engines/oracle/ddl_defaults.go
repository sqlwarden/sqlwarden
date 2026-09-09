package oracle

import (
	"fmt"
	"strings"

	oracleast "github.com/bytebase/omni/oracle/ast"
	oracleparser "github.com/bytebase/omni/oracle/parser"
	"github.com/sqlwarden/internal/engine/ddl"
)

func validateOracleDefaults(request ddl.Request) error {
	var values []*string
	switch request.Operation {
	case ddl.OperationCreateTable:
		for _, column := range request.Columns {
			values = append(values, column.Default)
		}
	case ddl.OperationAddColumn:
		values = append(values, request.Column.Default)
	case ddl.OperationAlterColumn:
		values = append(values, request.Changes.Default)
	}
	for _, value := range values {
		if value != nil {
			if err := validateOracleDefault(*value); err != nil {
				return err
			}
		}
	}
	return nil
}

// A default is embedded inside parentheses, so its token stream must never
// escape that boundary. Token spans distinguish punctuation in literals from
// SQL punctuation. Parsing then requires a single scalar expression.
func validateOracleDefault(value string) error {
	invalid := fmt.Errorf("default must be a single SQL expression without comments, bind variables, or subqueries")
	lexer := oracleparser.NewLexer(value)
	depth, end, count := 0, 0, 0
	for {
		token := lexer.NextToken()
		if token.Type == 0 {
			break
		}
		if strings.TrimSpace(value[end:token.Loc]) != "" {
			return invalid
		}
		raw := value[token.Loc:token.End]
		if raw == ";" || strings.HasPrefix(raw, ":") || strings.HasPrefix(raw, "/*") || token.Type == oracleparser.SELECT {
			return invalid
		}
		switch raw {
		case "(":
			depth++
		case ")":
			depth--
			if depth < 0 {
				return invalid
			}
		case ",":
			if depth == 0 {
				return invalid
			}
		}
		end = token.End
		count++
	}
	if lexer.Err != nil || depth != 0 || count == 0 || strings.TrimSpace(value[end:]) != "" {
		return invalid
	}
	parsed, err := oracleparser.Parse("SELECT (" + value + ") FROM DUAL")
	if err != nil || parsed == nil || parsed.Len() != 1 {
		return invalid
	}
	raw, ok := parsed.Items[0].(*oracleast.RawStmt)
	if !ok {
		return invalid
	}
	query, ok := raw.Stmt.(*oracleast.SelectStmt)
	if !ok || query.TargetList.Len() != 1 {
		return invalid
	}
	return nil
}
