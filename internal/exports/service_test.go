package exports

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/pkg/result"
)

type fakeDriver struct {
	pages []*result.ResultSet
}

func (d *fakeDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (d *fakeDriver) Ping(context.Context) error                             { return nil }
func (d *fakeDriver) Close() error                                           { return nil }
func (d *fakeDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return nil, errors.New("buffered query should not be used")
}
func (d *fakeDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return nil, errors.New("execute should not be used")
}
func (d *fakeDriver) Dialect() engine.Dialect { return engine.DialectSQLite }
func (d *fakeDriver) StartQuery(context.Context, cursor.QueryRequest) (cursor.QueryCursor, error) {
	return &fakeCursor{pages: d.pages}, nil
}

type fakeCursor struct {
	pages  []*result.ResultSet
	index  int
	closed bool
}

func (c *fakeCursor) Columns() []result.Column { return c.pages[0].Columns }
func (c *fakeCursor) Close() error {
	c.closed = true
	return nil
}
func (c *fakeCursor) Fetch(context.Context, cursor.ScanOptions) (*result.ResultSet, cursor.QueryCursorState, error) {
	if c.index >= len(c.pages) {
		return &result.ResultSet{Columns: c.Columns()}, cursor.QueryCursorState{Exhausted: true}, nil
	}
	page := c.pages[c.index]
	c.index++
	return page, cursor.QueryCursorState{RowsReturned: len(page.Rows), BytesReturned: page.BytesReturned, Exhausted: c.index >= len(c.pages)}, nil
}

func TestServiceStreamsCursorPages(t *testing.T) {
	driver := &fakeDriver{pages: []*result.ResultSet{
		{
			Columns: []result.Column{{Name: "id"}, {Name: "name"}},
			Rows: []result.Row{
				{{Type: result.ValueTypeInteger, Integer: 1}, {Type: result.ValueTypeText, Text: "Ada"}},
			},
		},
		{
			Columns: []result.Column{{Name: "id"}, {Name: "name"}},
			Rows: []result.Row{
				{{Type: result.ValueTypeInteger, Integer: 2}, {Type: result.ValueTypeText, Text: "Grace"}},
			},
		},
	}}
	var buf bytes.Buffer
	got, err := NewService().Stream(context.Background(), driver, &buf, StreamOptions{Format: FormatCSV, SQL: "select * from users", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows != 2 {
		t.Fatalf("rows = %d, want 2", got.Rows)
	}
	if want := "id,name\n1,Ada\n2,Grace\n"; buf.String() != want {
		t.Fatalf("csv = %q, want %q", buf.String(), want)
	}
}

func TestServiceEnforcesByteLimit(t *testing.T) {
	driver := &fakeDriver{pages: []*result.ResultSet{{
		Columns: []result.Column{{Name: "name"}},
		Rows:    []result.Row{{{Type: result.ValueTypeText, Text: "long value"}}},
	}}}
	_, err := NewService().Stream(context.Background(), driver, io.Discard, StreamOptions{Format: FormatCSV, SQL: "select name", MaxBytes: 5})
	if !errors.Is(err, ErrByteLimitExceeded) {
		t.Fatalf("err = %v, want ErrByteLimitExceeded", err)
	}
}
