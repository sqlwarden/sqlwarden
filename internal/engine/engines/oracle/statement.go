package oracle

import (
	"strconv"

	"github.com/sqlwarden/internal/engine/statement"
)

var _ statement.Generator = (*oracleDriver)(nil)

var oracleStatementSpec = statement.Spec{Objects: []statement.ObjectSpec{
	{Kind: "table", Operations: []statement.Operation{
		statement.OperationSelect, statement.OperationInsert,
		statement.OperationUpdate, statement.OperationDelete,
	}},
	{Kind: "view", Operations: []statement.Operation{statement.OperationSelect}},
	{Kind: "materialized_view", Operations: []statement.Operation{statement.OperationSelect}},
}}

func (*oracleDriver) StatementSpec() statement.Spec { return oracleStatementSpec }

func (*oracleDriver) Generate(request statement.Request) (string, error) {
	if err := statement.Validate(request, oracleStatementSpec); err != nil {
		return "", err
	}
	qualified := oracleQualified(request.Object.Ref.Scope.Name("schema"), request.Object.Ref.Name)
	columns := request.Object.Relational.Columns
	quoted := make([]string, len(columns))
	values := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = oracleQuoteIdent(column.Name)
		values[i] = ":" + strconv.Itoa(i+1)
	}
	return statement.Build(request.Operation, qualified, quoted, values)
}
