package sqlite

import (
	"github.com/sqlwarden/internal/engine/statement"
)

var _ statement.Generator = (*sqliteDriver)(nil)

var sqliteStatementSpec = statement.Spec{Objects: []statement.ObjectSpec{
	{Kind: "table", Operations: []statement.Operation{statement.OperationSelect, statement.OperationInsert, statement.OperationUpdate, statement.OperationDelete}},
	{Kind: "view", Operations: []statement.Operation{statement.OperationSelect}},
}}

func (*sqliteDriver) StatementSpec() statement.Spec { return sqliteStatementSpec }

func (*sqliteDriver) Generate(request statement.Request) (string, error) {
	if err := statement.Validate(request, sqliteStatementSpec); err != nil {
		return "", err
	}
	qualified := sqliteQualify(request.Object.Ref.Scope.Name("database"), request.Object.Ref.Name)
	columns := request.Object.Relational.Columns
	quoted := make([]string, len(columns))
	values := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = sqliteQuoteIdent(column.Name)
		values[index] = "?"
	}
	return statement.Build(request.Operation, qualified, quoted, values)
}
