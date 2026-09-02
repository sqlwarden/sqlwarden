package oracle

import (
	"context"
	"database/sql/driver"
	"testing"
)

type fakeConn struct {
	execed []string
	closed bool
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *fakeConn) Close() error                        { c.closed = true; return nil }
func (c *fakeConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c *fakeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.execed = append(c.execed, query)
	return driver.RowsAffected(0), nil
}

type fakeConnector struct {
	conn *fakeConn
}

func (c *fakeConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c *fakeConnector) Driver() driver.Driver                        { return nil }

func TestSchemaConnectorAppliesCurrentSchema(t *testing.T) {
	conn := &fakeConn{}
	sc := schemaConnector{inner: &fakeConnector{conn: conn}, quotedSchema: oracleQuoteIdent("HR")}

	got, err := sc.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if got != conn {
		t.Fatalf("Connect returned unexpected conn")
	}
	if len(conn.execed) != 1 || conn.execed[0] != `ALTER SESSION SET CURRENT_SCHEMA = "HR"` {
		t.Fatalf("unexpected session init statements: %#v", conn.execed)
	}
}

func TestSchemaConnectorNoSchemaIsPassthrough(t *testing.T) {
	conn := &fakeConn{}
	sc := schemaConnector{inner: &fakeConnector{conn: conn}}

	if _, err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if len(conn.execed) != 0 {
		t.Fatalf("expected no session init statements, got %#v", conn.execed)
	}
}
