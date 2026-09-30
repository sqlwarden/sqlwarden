package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	completionapp "github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

const (
	maxCompletionSQLBytes    = 1 << 20
	maxCompletionSuggestions = 200
)

type completionRequest struct {
	SQL          string `json:"sql"`
	CursorOffset int    `json:"cursor_offset"`
	TriggerKind  string `json:"trigger_kind,omitempty"`
	TriggerChar  string `json:"trigger_character,omitempty"`
}

type completionResponse struct {
	Suggestions       []completer.Suggestion `json:"suggestions"`
	Mode              string                 `json:"mode"`
	MetadataAvailable bool                   `json:"metadata_available"`
	MetadataStatus    string                 `json:"metadata_status"`
	Context           string                 `json:"context,omitempty"`
}

func (app *application) completeConnectionSQL(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	// JSON escaping can expand a valid SQL string, so bound the transport above
	// the logical 1 MiB SQL limit while still preventing unbounded decoding.
	r.Body = http.MaxBytesReader(w, r.Body, 6*maxCompletionSQLBytes+4096)
	var input completionRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	if len(input.SQL) > maxCompletionSQLBytes {
		app.errorMessage(w, r, http.StatusRequestEntityTooLarge, "SQL exceeds the 1 MiB completion limit.", nil)
		return
	}
	byteOffset, err := utf16OffsetToByteOffset(input.SQL, input.CursorOffset)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}
	triggerKind := completer.TriggerKind(input.TriggerKind)
	if triggerKind == "" {
		triggerKind = completer.TriggerInvoked
	}
	if triggerKind != completer.TriggerInvoked && triggerKind != completer.TriggerAutomatic {
		app.badRequest(w, r, errors.New("trigger_kind must be invoked or automatic"))
		return
	}
	if utf8.RuneCountInString(input.TriggerChar) > 1 {
		app.badRequest(w, r, errors.New("trigger_character must contain at most one character"))
		return
	}

	conn := contextGetConnection(r)
	connID := strconv.FormatInt(conn.ID, 10)
	req := completer.Request{
		SQL: input.SQL, CursorOffset: byteOffset, ConnectionID: connID,
		TriggerKind: triggerKind, TriggerChar: input.TriggerChar,
	}
	out := completionResponse{
		Suggestions:    []completer.Suggestion{},
		Mode:           "ephemeral",
		MetadataStatus: "unavailable",
	}

	if _, ok := app.optionalSchemaSession(w, r); !ok {
		return
	}
	navConn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if navConn.Persistent {
		out.Mode = "persistent"
	}
	if tree, ok := app.optionalNavigatorTree(conn); ok {
		set, metaErr := app.schemaNavigator.CompletionMetadata(r.Context(), navConn, tree)
		if metaErr != nil {
			app.logWarn(r, "completion metadata unavailable", slog.Int64("connection_id", conn.ID), slog.Any("error", metaErr))
		} else if len(set.Directory.Roots) > 0 || len(set.Objects) > 0 {
			req.Schema = set
			out.MetadataAvailable, out.MetadataStatus = true, "ready"
		}
	}

	result, err := app.completionService.Complete(r.Context(), conn.Driver, req)
	if errors.Is(err, completionapp.ErrUnsupported) {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support SQL completion.", nil)
		return
	}
	if err != nil && req.Schema != nil {
		app.logWarn(r, "schema-aware SQL completion failed; retrying without metadata",
			slog.Int64("connection_id", conn.ID),
			slog.String("driver", conn.Driver),
			slog.String("mode", out.Mode),
			slog.String("error", err.Error()),
		)
		req.Schema = nil
		out.MetadataAvailable, out.MetadataStatus = false, "degraded"
		result, err = app.completionService.Complete(r.Context(), conn.Driver, req)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if len(result.Suggestions) > maxCompletionSuggestions {
		result.Suggestions = result.Suggestions[:maxCompletionSuggestions]
	}
	for i := range result.Suggestions {
		result.Suggestions[i].ReplaceStart = byteOffsetToUTF16Offset(input.SQL, result.Suggestions[i].ReplaceStart)
		result.Suggestions[i].ReplaceEnd = byteOffsetToUTF16Offset(input.SQL, result.Suggestions[i].ReplaceEnd)
	}
	out.Suggestions = result.Suggestions
	out.Context = result.Context
	if out.Context == "" {
		out.Context = "any"
	}
	app.logDebug(r, "SQL completion returned",
		slog.Int64("connection_id", conn.ID),
		slog.String("driver", conn.Driver),
		slog.String("mode", out.Mode),
		slog.String("trigger_kind", string(triggerKind)),
		slog.Bool("metadata_available", out.MetadataAvailable),
		slog.Int("suggestion_count", len(out.Suggestions)),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()),
	)
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func utf16OffsetToByteOffset(text string, offset int) (int, error) {
	if offset < 0 {
		return 0, errors.New("cursor_offset must not be negative")
	}
	units := 0
	for byteOffset, r := range text {
		if units == offset {
			return byteOffset, nil
		}
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > offset {
			return 0, errors.New("cursor_offset splits a UTF-16 surrogate pair")
		}
		units += width
	}
	if units == offset {
		return len(text), nil
	}
	return 0, errors.New("cursor_offset is outside SQL text")
}

func byteOffsetToUTF16Offset(text string, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(text) {
		byteOffset = len(text)
	}
	units := 0
	for i, r := range text {
		if i >= byteOffset {
			break
		}
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	if !utf8.ValidString(text[:byteOffset]) {
		return units
	}
	return units
}
