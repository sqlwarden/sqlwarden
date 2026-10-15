package query

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/safety"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/pkg/result"
)

const actionQueryExecuted = "query.executed"

// ExecuteInput is one statement submitted on a live session.
type ExecuteInput struct {
	SessionID execution.SessionID
	SQL       string
	// Explain is empty, "plain" or "analyze".
	Explain string
	// PageSize and UseCursor apply to DQL only. A nil UseCursor means true.
	PageSize      *int
	UseCursor     *bool
	ConfirmUnsafe bool
}

type ExecuteOutput struct {
	Result      *result.ResultSet
	Transaction execution.TxStatus
}

// Execute classifies the statement, enforces the permission its class
// requires, and runs it on the principal's session. Runtime failures are
// returned unchanged for the caller to map.
func (s *Service) Execute(ctx context.Context, p access.Principal, ref access.ConnRef, in ExecuteInput) (ExecuteOutput, error) {
	conn, ws, err := s.runtimeConnection(ctx, p, ref)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if err := validateExecute(in); err != nil {
		return ExecuteOutput{}, err
	}
	if in.SessionID == "" {
		return ExecuteOutput{}, ErrSessionRequired
	}
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return ExecuteOutput{}, err
	}
	sc := scope(p, ref)
	session, err := s.deps.Runtime.Get(ctx, sc, in.SessionID)
	if err != nil {
		return ExecuteOutput{}, err
	}

	hasBroadExecute := s.can(ctx, p, ref, ws, conn, access.PermConnExecute)
	classification, err := s.deps.Analyzers.Classifier(conn.Driver).Classify(ctx, classifier.Request{SQL: in.SQL})
	if err != nil {
		return ExecuteOutput{}, err
	}
	logAttrs := queryLogAttrs(p, ref, ws, conn, classification)
	if classification.Kind == classifier.KindUnknown {
		s.deps.Logger.WarnContext(ctx, "query classification unknown", logAttrs...)
	} else {
		s.deps.Logger.DebugContext(ctx, "query classified", logAttrs...)
	}

	// Permission and safety decisions always use the classification of the
	// caller's original SQL; EXPLAIN never changes what permission a statement
	// requires or whether it needs unsafe confirmation.
	explainMode := explain.Mode(in.Explain)
	var plan *explain.Plan
	if in.Explain != "" {
		explainer, ok := s.deps.Analyzers.Explainer(conn.Driver)
		if !ok {
			return ExecuteOutput{}, ErrExplainUnsupported
		}
		if explainMode == explain.ModeAnalyze && !explainer.ExplainSpec().SupportsAnalyze {
			return ExecuteOutput{}, ErrExplainAnalyzeUnsupported
		}
		built, explainErr := explainer.Explain(in.SQL, explainMode)
		if explainErr != nil {
			switch {
			case errors.Is(explainErr, explain.ErrMultipleStatements):
				s.deps.Logger.WarnContext(ctx, "explain refused for multi-statement input", logAttrs...)
				return ExecuteOutput{}, ErrExplainMultiple
			case errors.Is(explainErr, explain.ErrAlreadyExplained):
				return ExecuteOutput{}, ErrExplainAlready
			default:
				return ExecuteOutput{}, explainErr
			}
		}
		plan = &built
	}

	required, allowed := s.kindPermission(ctx, p, ref, ws, conn, classification.Kind, hasBroadExecute)
	if !allowed {
		s.deps.Logger.WarnContext(ctx, "query permission denied", append(logAttrs, "required_permission", required)...)
		return ExecuteOutput{}, ErrForbidden
	}
	// Plain EXPLAIN only plans the statement and never runs it, so the
	// no-WHERE confirmation gate, which exists to stop real mutations, does
	// not apply. EXPLAIN ANALYZE runs the statement and stays gated.
	if classification.Kind == classifier.KindDML && !in.ConfirmUnsafe && explainMode != explain.ModePlain {
		checked, safetyErr := s.deps.Analyzers.SafetyChecker(conn.Driver).Check(ctx, safety.Request{SQL: in.SQL})
		if safetyErr != nil {
			return ExecuteOutput{}, safetyErr
		}
		if checked.Unsafe {
			s.deps.Logger.WarnContext(ctx, "unsafe query refused pending confirmation", append(logAttrs, "unsafe_statement_count", len(checked.Statements))...)
			return ExecuteOutput{}, &UnsafeError{Statements: checked.Statements}
		}
	}

	start := time.Now()
	lim := limits(policy)
	var out ExecuteOutput
	switch {
	case plan != nil:
		// The plan statement always yields the plan as a result set, whatever
		// the underlying statement's class, and runs as an execution because
		// ANALYZE carries out the statement it plans.
		res, runErr := s.deps.Runtime.Execute(ctx, sc, execution.ExecuteRequest{SessionID: session.ID, Limits: lim, Explain: plan})
		out, err = ExecuteOutput{Result: res.Result, Transaction: res.Transaction}, runErr
	case classification.Kind == classifier.KindDQL:
		res, runErr := s.deps.Runtime.Query(ctx, sc, execution.QueryRequest{
			SessionID: session.ID,
			SQL:       in.SQL,
			Limits:    lim,
			UseCursor: in.UseCursor == nil || *in.UseCursor,
			PageSize:  cursorPageSize(in.PageSize, policy),
		})
		out, err = ExecuteOutput{Result: res.Result, Transaction: res.Transaction}, runErr
	default:
		res, runErr := s.deps.Runtime.Execute(ctx, sc, execution.ExecuteRequest{SessionID: session.ID, SQL: in.SQL, Limits: lim})
		out, err = ExecuteOutput{Result: res.Result, Transaction: res.Transaction}, runErr
	}
	if err != nil {
		var failure *execution.Failure
		if (errors.As(err, &failure) && failure.Code == execution.FailureExecutionOutcomeUnknown) || isCanceled(ctx, err) {
			s.deps.Logger.WarnContext(ctx, "query cancelled", append(logAttrs, "duration_ms", time.Since(start).Milliseconds())...)
		} else {
			s.deps.Logger.WarnContext(ctx, "query execution failed", append(logAttrs, "duration_ms", time.Since(start).Milliseconds(), "error", err.Error())...)
		}
		return ExecuteOutput{}, err
	}
	if out.Result == nil {
		out.Result = &result.ResultSet{}
	}
	if classification.Kind != classifier.KindDML {
		out.Result.RowsAffected = nil
	}
	out.Result.DurationMs = time.Since(start).Milliseconds()
	s.deps.Logger.InfoContext(ctx, "query executed", append(logAttrs,
		"duration_ms", out.Result.DurationMs,
		slog.Group("result", "rows", len(out.Result.Rows), "columns", len(out.Result.Columns)),
		slog.String("query_cursor_id", out.Result.QueryCursorID),
	)...)
	if classification.Kind != classifier.KindDQL && explainMode != explain.ModePlain {
		s.emitExecuted(ctx, p, ref, conn, classification.Kind)
	}
	return out, nil
}

func validateExecute(in ExecuteInput) error {
	fields := map[string]string{}
	if in.SQL == "" {
		fields["sql"] = "SQL is required."
	}
	if in.Explain != "" && in.Explain != string(explain.ModePlain) && in.Explain != string(explain.ModeAnalyze) {
		fields["explain"] = `Explain must be "plain" or "analyze".`
	}
	if in.PageSize != nil && *in.PageSize <= 0 {
		fields["page_size"] = "Page size must be greater than 0."
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// kindPermission reports the permission a statement class requires and
// whether the principal holds it. conn:execute covers every class; a class
// the classifier cannot determine requires it.
func (s *Service) kindPermission(ctx context.Context, p access.Principal, ref access.ConnRef, ws access.Workspace, conn access.Connection, kind classifier.Kind, hasBroadExecute bool) (string, bool) {
	required := access.PermConnExecute
	switch kind {
	case classifier.KindDQL:
		required = access.PermConnDQL
	case classifier.KindDML:
		required = access.PermConnDML
	case classifier.KindDDL:
		required = access.PermConnDDL
	}
	if hasBroadExecute {
		return required, true
	}
	if required == access.PermConnExecute {
		return required, false
	}
	return required, s.can(ctx, p, ref, ws, conn, required)
}

func (s *Service) can(ctx context.Context, p access.Principal, ref access.ConnRef, ws access.Workspace, conn access.Connection, permission string) bool {
	return s.deps.Requirer.Can(ctx, p, ref.OrgID, ws.OwnerType, "connection", conn.ID, permission)
}

// emitExecuted records a statement that may have changed the target. The
// record carries identifiers and the statement class only, never SQL. It is
// fail-open: the statement has already run, so an audit failure must not
// report the request as failed.
func (s *Service) emitExecuted(ctx context.Context, p access.Principal, ref access.ConnRef, conn access.Connection, kind classifier.Kind) {
	orgID := ref.OrgID
	err := audit.Emit(ctx, s.deps.Audit, audit.Event{
		OrgID:      &orgID,
		Actor:      audit.ActorFromPrincipal(p),
		Action:     actionQueryExecuted,
		Resource:   "connection",
		ResourceID: strconv.FormatInt(conn.ID, 10),
		Outcome:    audit.OutcomeSuccess,
		Metadata: map[string]string{
			"workspace_id": strconv.FormatInt(ref.WorkspaceID, 10),
			"driver":       conn.Driver,
			"kind":         string(kind),
		},
	})
	if err != nil {
		s.deps.Logger.WarnContext(ctx, "query audit event failed",
			slog.Int64("connection_id", conn.ID),
			slog.String("query_kind", string(kind)),
			slog.String("error", err.Error()),
		)
	}
}

func queryLogAttrs(p access.Principal, ref access.ConnRef, ws access.Workspace, conn access.Connection, classification classifier.Result) []any {
	return []any{
		slog.Group("account", "id", p.Subject.ID),
		slog.Group("org", "id", ref.OrgID),
		slog.Group("workspace", "id", ws.ID, "owner_type", ws.OwnerType),
		slog.Group("connection", "id", conn.ID, "driver", conn.Driver),
		slog.Group("query", "kind", classification.Kind, "classifier", classification.Source),
	}
}

func isCanceled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil
}
