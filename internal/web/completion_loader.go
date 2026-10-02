package web

import (
	"context"
	"log/slog"
	"net/http"

	completionapp "github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine/metadata"
	schemaapp "github.com/sqlwarden/internal/schema"
)

// navigatorCompletionLoader adapts the schema navigator to the completion
// loader hook. A nil live inspector means there is no usable session, so
// Ensure never queries the target.
type navigatorCompletionLoader struct {
	app       *application
	request   *http.Request
	navigator *schemaapp.Navigator
	conn      schemaapp.Connection
	driver    string
	tree      metadata.Tree
	live      metadata.SchemaInspector
}

func (l navigatorCompletionLoader) View(ctx context.Context) (*metadata.CompletionView, error) {
	view, err := l.navigator.CompletionView(ctx, l.conn, l.tree, l.live)
	if err != nil {
		l.app.logWarn(l.request, "completion metadata unavailable",
			slog.Int64("connection_id", l.conn.ID),
			slog.String("driver", l.driver),
			slog.Any("error", err),
		)
	}
	return view, err
}

func (l navigatorCompletionLoader) Live() bool { return l.live != nil }

func (l navigatorCompletionLoader) Ensure(ctx context.Context, demands []metadata.Demand) completionapp.LoadReport {
	report := l.navigator.EnsureForCompletion(ctx, l.conn, l.tree, l.live, demands)
	if report.Failed > 0 {
		l.app.logWarn(l.request, "completion metadata loads failed",
			slog.Int64("connection_id", l.conn.ID),
			slog.String("driver", l.driver),
			slog.Int("loads_failed", report.Failed),
		)
	}
	return completionapp.LoadReport(report)
}

type staticCompletionLoader struct{ view *metadata.CompletionView }

func (l staticCompletionLoader) View(context.Context) (*metadata.CompletionView, error) {
	return l.view, nil
}
func (staticCompletionLoader) Live() bool { return false }
func (staticCompletionLoader) Ensure(context.Context, []metadata.Demand) completionapp.LoadReport {
	return completionapp.LoadReport{}
}
