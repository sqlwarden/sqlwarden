package postgres

import (
	"fmt"

	"github.com/sqlwarden/internal/engine/statement"
)

var _ statement.Generator = (*Driver)(nil)

var postgresStatementSpec = statement.Spec{Objects: []statement.ObjectSpec{
	{Kind: "table", Operations: []statement.Operation{statement.OperationSelect, statement.OperationInsert, statement.OperationUpdate, statement.OperationDelete}},
	{Kind: "view", Operations: []statement.Operation{statement.OperationSelect}},
	{Kind: "materialized_view", Operations: []statement.Operation{statement.OperationSelect}},
}}

func (*Driver) StatementSpec() statement.Spec { return postgresStatementSpec }

func (*Driver) Generate(request statement.Request) (string, error) {
	if err := statement.Validate(request, postgresStatementSpec); err != nil {
		return "", err
	}
	qualified := postgresDDLQualified(request.Object.Ref.Scope.Name("schema"), request.Object.Ref.Name)
	columns := request.Object.Relational.Columns
	quoted := make([]string, len(columns))
	values := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = pgQuoteIdent(column.Name)
		values[index] = fmt.Sprintf("$%d", index+1)
	}
	return statement.Build(request.Operation, qualified, quoted, values)
}
