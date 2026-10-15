package query

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
	"github.com/sqlwarden/internal/execution"
)

// fakeCompleter records what the service hands the completion engine and
// returns a scripted result.
type fakeCompleter struct {
	calls    int
	driver   string
	req      completer.Request
	live     bool
	view     *metadata.CompletionView
	result   completer.Result
	outcome  completion.Outcome
	err      error
	vocab    completer.Vocabulary
	vocabErr error
}

func (f *fakeCompleter) CompleteWithMetadata(ctx context.Context, driver string, req completer.Request, loader completion.Loader) (completer.Result, completion.Outcome, error) {
	f.calls++
	f.driver = driver
	f.req = req
	f.live = loader.Live()
	f.view, _ = loader.View(ctx)
	return f.result, f.outcome, f.err
}

func (f *fakeCompleter) Vocabulary(string) (completer.Vocabulary, error) {
	return f.vocab, f.vocabErr
}

func newCompletionHarness(driver string) (schemaHarness, *fakeCompleter) {
	h := newSchemaHarness(driver)
	fake := &fakeCompleter{outcome: completion.Outcome{Status: completion.MetadataReady, Available: true}}
	h.svc.deps.Completion = fake
	return h, fake
}

func completionCalls(svc *Service, p access.Principal, ref access.ConnRef, id execution.SessionID) map[string]error {
	ctx := context.Background()
	out := map[string]error{}
	_, out["complete"] = svc.Complete(ctx, p, ref, id, CompleteInput{SQL: "SELECT ", CursorOffset: 7})
	_, out["index"] = svc.CompletionIndex(ctx, p, ref, id)
	return out
}

func TestCompletionDeniesBeforeRuntime(t *testing.T) {
	tests := []struct {
		name string
		p    access.Principal
		ref  access.ConnRef
		want error
	}{
		{name: "non-member", p: principal(outsider), ref: connRef(), want: ErrForbidden},
		{name: "org member without connection permission", p: principal(orgOnlyUser), ref: connRef(), want: ErrForbidden},
		{name: "unknown connection", p: principal(userA), ref: access.ConnRef{OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: 404}, want: ErrNotFound},
		{name: "connection in another workspace", p: principal(userA), ref: access.ConnRef{OrgID: testOrg, WorkspaceID: otherWorkspace, ConnectionID: testConn}, want: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, fake := newCompletionHarness(navigatorDriver)
			for op, err := range completionCalls(h.svc, tt.p, tt.ref, ownerSID) {
				accesstest.AssertDenies(t, err, tt.want, op)
			}
			if fake.calls != 0 || h.runtime.capsCalled != 0 || h.runtime.metaCalls != 0 {
				t.Fatalf("completer calls = %d, capabilities calls = %d, metadata calls = %d, want none on denial", fake.calls, h.runtime.capsCalled, h.runtime.metaCalls)
			}
		})
	}
}

func TestCompletionInvalidInputDeniesFirst(t *testing.T) {
	h, fake := newCompletionHarness(navigatorDriver)
	_, err := h.svc.Complete(context.Background(), principal(outsider), connRef(), "", CompleteInput{SQL: "SELECT", CursorOffset: -1})
	accesstest.AssertForbidden(t, err, "complete")
	if fake.calls != 0 {
		t.Fatalf("completer calls = %d, want none", fake.calls)
	}
}

func TestCompletionPerPermission(t *testing.T) {
	for _, account := range []int64{userA, dqlUser, dmlUser, ddlUser} {
		t.Run(fmt.Sprintf("account-%d", account), func(t *testing.T) {
			h, _ := newCompletionHarness(navigatorDriver)
			id := h.addSession(account)
			for op, err := range completionCalls(h.svc, principal(account), connRef(), id) {
				if err != nil {
					t.Fatalf("account %d %s: %v", account, op, err)
				}
			}
		})
	}
}

func TestCompleteValidatesInput(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		in   CompleteInput
		want string
	}{
		{name: "negative cursor", in: CompleteInput{SQL: "SELECT", CursorOffset: -1}, want: "cursor_offset must not be negative"},
		{name: "cursor past text", in: CompleteInput{SQL: "SELECT", CursorOffset: 7}, want: "cursor_offset is outside SQL text"},
		{name: "cursor inside surrogate pair", in: CompleteInput{SQL: "\U0001F600", CursorOffset: 1}, want: "cursor_offset splits a UTF-16 surrogate pair"},
		{name: "unknown trigger kind", in: CompleteInput{SQL: "SELECT", CursorOffset: 0, TriggerKind: "typed"}, want: "trigger_kind must be invoked or automatic"},
		{name: "long trigger character", in: CompleteInput{SQL: "SELECT", CursorOffset: 0, TriggerChar: ".."}, want: "trigger_character must contain at most one character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, fake := newCompletionHarness(navigatorDriver)
			_, err := h.svc.Complete(ctx, principal(userA), connRef(), "", tt.in)
			var input *CompletionInputError
			if !errors.As(err, &input) || err.Error() != tt.want {
				t.Fatalf("err = %v, want input error %q", err, tt.want)
			}
			if fake.calls != 0 {
				t.Fatalf("completer calls = %d, want none for invalid input", fake.calls)
			}
		})
	}

	h, _ := newCompletionHarness(navigatorDriver)
	big := strings.Repeat("x", maxCompletionSQLBytes+1)
	if _, err := h.svc.Complete(ctx, principal(userA), connRef(), "", CompleteInput{SQL: big}); !errors.Is(err, ErrCompletionTooLarge) {
		t.Fatalf("oversized SQL err = %v, want ErrCompletionTooLarge", err)
	}
}

func TestCompleteUsesNavigatorWithLiveSession(t *testing.T) {
	ctx := context.Background()
	h, fake := newCompletionHarness(navigatorDriver)
	sql := "\U0001F600 SELECT u"
	fake.result = completer.Result{Suggestions: []completer.Suggestion{{Label: "users", ReplaceStart: len(sql) - 1, ReplaceEnd: len(sql)}}}

	out, err := h.svc.Complete(ctx, principal(userA), connRef(), ownerSID, CompleteInput{SQL: sql, CursorOffset: 11, TriggerKind: "automatic", TriggerChar: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if fake.driver != navigatorDriver || !fake.live || fake.view == nil {
		t.Fatalf("driver = %q live = %v view = %v, want the navigator loader with a live session", fake.driver, fake.live, fake.view)
	}
	if fake.req.CursorOffset != len(sql) || fake.req.ConnectionID != fmt.Sprint(testConn) || fake.req.TriggerKind != completer.TriggerAutomatic {
		t.Fatalf("request = %+v, want byte cursor %d on connection %d", fake.req, len(sql), testConn)
	}
	if len(out.Suggestions) != 1 || out.Suggestions[0].ReplaceStart != 10 || out.Suggestions[0].ReplaceEnd != 11 {
		t.Fatalf("suggestions = %+v, want UTF-16 range 10-11", out.Suggestions)
	}
	if out.Mode != "ephemeral" || out.MetadataStatus != completion.MetadataReady || !out.MetadataAvailable || out.Context != "any" {
		t.Fatalf("completion = %+v", out)
	}
}

func TestCompleteWithoutSessionIsNotLive(t *testing.T) {
	ctx := context.Background()
	for _, id := range []execution.SessionID{"", "b-session"} {
		h, fake := newCompletionHarness(navigatorDriver)
		if _, err := h.svc.Complete(ctx, principal(userA), connRef(), id, CompleteInput{SQL: "SELECT ", CursorOffset: 7}); err != nil {
			t.Fatalf("session %q: %v", id, err)
		}
		if fake.live || h.runtime.metaCalls != 0 {
			t.Fatalf("session %q: live = %v metadata calls = %d, want cached-only completion", id, fake.live, h.runtime.metaCalls)
		}
	}
}

func TestCompleteWithoutNavigatorTreeUsesStaticView(t *testing.T) {
	h, fake := newCompletionHarness(bareDriver)
	if _, err := h.svc.Complete(context.Background(), principal(userA), connRef(), ownerSID, CompleteInput{SQL: "SELECT ", CursorOffset: 7}); err != nil {
		t.Fatal(err)
	}
	if fake.live || fake.view == nil || fake.view.DefaultScope() != mainScope {
		t.Fatalf("live = %v view = %v, want a static view over the default scope", fake.live, fake.view)
	}
}

func TestCompleteTruncatesSuggestions(t *testing.T) {
	h, fake := newCompletionHarness(navigatorDriver)
	fake.result.Suggestions = make([]completer.Suggestion, maxCompletionSuggestions+5)
	out, err := h.svc.Complete(context.Background(), principal(userA), connRef(), "", CompleteInput{SQL: "SELECT ", CursorOffset: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Suggestions) != maxCompletionSuggestions {
		t.Fatalf("suggestions = %d, want %d", len(out.Suggestions), maxCompletionSuggestions)
	}
}

func TestCompleteMapsCompleterFailures(t *testing.T) {
	ctx := context.Background()
	h, fake := newCompletionHarness(navigatorDriver)
	fake.err = completion.ErrUnsupported
	if _, err := h.svc.Complete(ctx, principal(userA), connRef(), "", CompleteInput{SQL: "SELECT ", CursorOffset: 7}); !errors.Is(err, ErrCompletionUnsupported) {
		t.Fatalf("err = %v, want ErrCompletionUnsupported", err)
	}

	h, fake = newCompletionHarness(navigatorDriver)
	fake.outcome = completion.Outcome{Status: completion.MetadataDegraded, Available: true}
	out, err := h.svc.Complete(ctx, principal(userA), connRef(), "", CompleteInput{SQL: "SELECT ", CursorOffset: 7})
	if err != nil {
		t.Fatal(err)
	}
	if out.MetadataAvailable || out.MetadataStatus != completion.MetadataDegraded {
		t.Fatalf("completion = %+v, want degraded metadata reported unavailable", out)
	}
}

func TestCompletionIndexServesNavigatorAndStaticViews(t *testing.T) {
	ctx := context.Background()
	h, _ := newCompletionHarness(navigatorDriver)
	out, err := h.svc.CompletionIndex(ctx, principal(userA), connRef(), "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Version == "" || out.Objects == nil || out.Columns == nil || out.Schemas == nil {
		t.Fatalf("index = %+v, want versioned non-nil lists", out)
	}

	h, _ = newCompletionHarness(bareDriver)
	out, err = h.svc.CompletionIndex(ctx, principal(userA), connRef(), ownerSID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 0 || h.runtime.metaCalls != 0 {
		t.Fatalf("objects = %+v metadata calls = %d, want an empty static index", out.Objects, h.runtime.metaCalls)
	}
}

func TestCompletionVocabulary(t *testing.T) {
	ctx := context.Background()
	h, fake := newCompletionHarness(navigatorDriver)
	fake.vocab = completer.Vocabulary{Dialect: "fake"}

	if _, err := h.svc.CompletionVocabulary(ctx, access.Principal{}, navigatorDriver); !errors.Is(err, ErrForbidden) {
		t.Fatalf("anonymous err = %v, want ErrForbidden", err)
	}
	got, err := h.svc.CompletionVocabulary(ctx, principal(outsider), navigatorDriver)
	if err != nil || got.Dialect != "fake" {
		t.Fatalf("vocabulary = %+v err = %v, want any account to read it", got, err)
	}
	if _, err := h.svc.CompletionVocabulary(ctx, principal(userA), "unknown"); !errors.Is(err, ErrUnknownEngine) {
		t.Fatalf("unknown engine err = %v, want ErrUnknownEngine", err)
	}
	fake.vocabErr = completion.ErrUnsupported
	if _, err := h.svc.CompletionVocabulary(ctx, principal(userA), navigatorDriver); !errors.Is(err, ErrVocabularyUnsupported) {
		t.Fatalf("unsupported err = %v, want ErrVocabularyUnsupported", err)
	}
}

func TestUTF16Offsets(t *testing.T) {
	text := "a\U0001F600b"
	for utf16, want := range map[int]int{0: 0, 1: 1, 3: 5, 4: 6} {
		got, err := utf16OffsetToByteOffset(text, utf16)
		if err != nil || got != want {
			t.Fatalf("utf16 %d -> byte %d, %v; want %d", utf16, got, err, want)
		}
		if back := byteOffsetToUTF16Offset(text, want); back != utf16 {
			t.Fatalf("byte %d -> utf16 %d, want %d", want, back, utf16)
		}
	}
	if got := byteOffsetToUTF16Offset(text, 100); got != 4 {
		t.Fatalf("past-end byte offset -> %d, want clamp to 4", got)
	}
}

func TestProjectCompletionIndexFromView(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	public := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	users := metadata.ObjectRef{Scope: public, Kind: "table", Name: "users"}
	view := metadatatest.Build(metadatatest.Tree(true), metadatatest.Fixture{
		DefaultScope: public,
		Objects:      []metadata.Object{{Ref: users, Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "integer"}}}}},
	})
	out, err := projectCompletionIndex(view)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultSchema != "public" || len(out.Schemas) != 1 || out.Schemas[0] != "public" {
		t.Fatalf("schemas = %+v default %q", out.Schemas, out.DefaultSchema)
	}
	if len(out.Objects) != 1 || out.Objects[0] != (CompletionIndexObject{Schema: "public", Name: "users", Kind: "table", Score: completer.KindScore("table")}) {
		t.Fatalf("objects = %+v", out.Objects)
	}
	if len(out.Columns) != 1 || out.Columns[0].Name != "id" || out.Columns[0].Type != "integer" {
		t.Fatalf("columns = %+v", out.Columns)
	}
	again, _ := projectCompletionIndex(view)
	if out.Version == "" || out.Version != again.Version {
		t.Fatalf("version = %q / %q", out.Version, again.Version)
	}
}

func TestProjectCompletionIndexReportsDefaultScopeRelationCoverage(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	public := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	tree := metadatatest.Tree(true)

	listed := metadatatest.Build(tree, metadatatest.Fixture{DefaultScope: public})
	out, err := projectCompletionIndex(listed)
	if err != nil {
		t.Fatal(err)
	}
	if !out.DefaultScopeRelationsListed {
		t.Fatal("default scope with every folder listed must report relations listed")
	}
	if out.ColumnScore != completer.KindScore("column") {
		t.Fatalf("column score = %d", out.ColumnScore)
	}

	partial := metadata.NewCompletionView(tree, public, "", map[metadata.ListingKey][]metadata.Child{
		{Parent: public, Folder: "tables"}: {{Kind: "table", Name: "orders"}},
	}, nil)
	out, err = projectCompletionIndex(partial)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScopeRelationsListed {
		t.Fatal("unlisted view folders must not report relations listed")
	}

	noDefault := metadatatest.Build(tree, metadatatest.Fixture{Scopes: []metadata.ScopePath{public}})
	out, err = projectCompletionIndex(noDefault)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScopeRelationsListed {
		t.Fatal("no default scope must not report relations listed")
	}

	database := metadatatest.Build(tree, metadatatest.Fixture{DefaultScope: app})
	out, err = projectCompletionIndex(database)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScopeRelationsListed {
		t.Fatal("a default scope without relation folders must not report relations listed")
	}
}

func completionFallbackTree() metadata.Tree {
	tree := metadatatest.Tree(true)
	tree.FallbackScopes = []string{"dbo"}
	return tree
}

func relationFolderListings(scope metadata.ScopePath) map[metadata.ListingKey][]metadata.Child {
	return map[metadata.ListingKey][]metadata.Child{
		{Parent: scope, Folder: "tables"}:             {{Kind: "table", Name: "orders"}},
		{Parent: scope, Folder: "views"}:              {},
		{Parent: scope, Folder: "materialized_views"}: {},
		{Parent: scope, Folder: "foreign_tables"}:     {},
	}
}

func TestProjectCompletionIndexSearchSchemas(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	sales := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
	dbo := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	schemas := func(names ...string) []metadata.Child {
		out := []metadata.Child{}
		for _, name := range names {
			out = append(out, metadata.Child{Kind: "schema", Name: name})
		}
		return out
	}
	merge := func(parts ...map[metadata.ListingKey][]metadata.Child) map[metadata.ListingKey][]metadata.Child {
		out := map[metadata.ListingKey][]metadata.Child{}
		for _, part := range parts {
			for k, v := range part {
				out[k] = v
			}
		}
		return out
	}
	for _, tc := range []struct {
		name     string
		def      metadata.ScopePath
		listings map[metadata.ListingKey][]metadata.Child
		search   []string
		listed   bool
	}{
		{"default and dbo listed", sales, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales", "dbo")},
			relationFolderListings(sales), relationFolderListings(dbo),
		), []string{"sales", "dbo"}, true},
		{"dbo unlisted", sales, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales", "dbo")},
			relationFolderListings(sales),
		), []string{"sales", "dbo"}, false},
		{"dbo absent", sales, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales")},
			relationFolderListings(sales),
		), []string{"sales"}, true},
		{"database default searches dbo", app, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales", "dbo")},
			relationFolderListings(dbo),
		), []string{"dbo"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := projectCompletionIndex(metadata.NewCompletionView(completionFallbackTree(), tc.def, "", tc.listings, nil))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(out.SearchSchemas, tc.search) || out.DefaultScopeRelationsListed != tc.listed {
				t.Fatalf("search = %q listed %v, want %q %v", out.SearchSchemas, out.DefaultScopeRelationsListed, tc.search, tc.listed)
			}
		})
	}
}

func TestProjectCompletionIndexSearchSchemasFlatTree(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	out, err := projectCompletionIndex(metadatatest.Build(metadatatest.Tree(false), metadatatest.Fixture{DefaultScope: app}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.SearchSchemas, []string{""}) || !out.DefaultScopeRelationsListed {
		t.Fatalf("search = %q listed %v", out.SearchSchemas, out.DefaultScopeRelationsListed)
	}
}

func TestProjectCompletionIndexSearchSchemasEmptyWithoutDefault(t *testing.T) {
	out, err := projectCompletionIndex(metadata.NewCompletionView(metadata.Tree{}, "", "", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if out.SearchSchemas == nil || len(out.SearchSchemas) != 0 {
		t.Fatalf("search = %#v, want empty non-nil", out.SearchSchemas)
	}
}
