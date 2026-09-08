package sqlserver

import (
	"fmt"

	"github.com/sqlwarden/internal/engine/statement"
)

var _ statement.Generator = (*Driver)(nil)

var sqlServerStatementSpec = statement.Spec{Objects: []statement.ObjectSpec{
	{Kind: "table", Operations: []statement.Operation{statement.OperationSelect, statement.OperationInsert, statement.OperationUpdate, statement.OperationDelete}},
	{Kind: "view", Operations: []statement.Operation{statement.OperationSelect}},
}}

func (*Driver) StatementSpec() statement.Spec { return sqlServerStatementSpec }

// Generate builds INSERT/UPDATE/DELETE/SELECT templates using bracket
// quoting and named @p1, @p2, ... parameters — go-mssqldb does not translate
// "?" placeholders into T-SQL's @paramN form the way lib/pq translates "?"
// into "$1" for Postgres, so a "?" template would be invalid SQL Server
// syntax if ever executed as-is.
func (*Driver) Generate(request statement.Request) (string, error) {
	if err := statement.Validate(request, sqlServerStatementSpec); err != nil {
		return "", err
	}
	qualified := sqlServerQuoteQualified(request.Object.Ref.Scope.Name("schema"), request.Object.Ref.Name)
	columns := request.Object.Relational.Columns
	quoted := make([]string, len(columns))
	values := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = sqlServerQuoteIdent(column.Name)
		values[index] = fmt.Sprintf("@p%d", index+1)
	}
	return statement.Build(request.Operation, qualified, quoted, values)
}
