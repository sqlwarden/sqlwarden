package mysql

import (
	"github.com/sqlwarden/internal/engine/statement"
)

var _ statement.Generator = (*mysqlDriver)(nil)

var mysqlStatementSpec = statement.Spec{Objects: []statement.ObjectSpec{
	{Kind: "table", Operations: []statement.Operation{statement.OperationSelect, statement.OperationInsert, statement.OperationUpdate, statement.OperationDelete}},
	{Kind: "view", Operations: []statement.Operation{statement.OperationSelect}},
}}

func (*mysqlDriver) StatementSpec() statement.Spec { return mysqlStatementSpec }

func (*mysqlDriver) Generate(request statement.Request) (string, error) {
	if err := statement.Validate(request, mysqlStatementSpec); err != nil {
		return "", err
	}
	qualified := mysqlQuoteQualified(request.Object.Ref.Scope.Name("database"), request.Object.Ref.Name)
	columns := request.Object.Relational.Columns
	quoted := make([]string, len(columns))
	values := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = mysqlQuoteIdent(column.Name)
		values[index] = "?"
	}
	return statement.Build(request.Operation, qualified, quoted, values)
}
