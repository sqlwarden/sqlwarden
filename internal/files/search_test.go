package files

import (
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/database"
)

func TestScanContentForMatches(t *testing.T) {
	content := []byte("select * from orders\nwhere orders.id = 1\n-- orders report\nselect orders_alias")

	count, snippets := scanContentForMatches(content, "orders", 3)
	if count != 4 {
		t.Fatalf("match count = %d, want 4", count)
	}
	if len(snippets) != 3 {
		t.Fatalf("snippets = %+v, want 3 (capped)", snippets)
	}
	want := []SearchSnippet{
		{Line: 1, Column: 15, Excerpt: "select * from orders"},
		{Line: 2, Column: 7, Excerpt: "where orders.id = 1"},
		{Line: 3, Column: 4, Excerpt: "-- orders report"},
	}
	for i, snippet := range want {
		if snippets[i] != snippet {
			t.Fatalf("snippet[%d] = %+v, want %+v", i, snippets[i], snippet)
		}
	}

	count, snippets = scanContentForMatches(content, "nomatch", 3)
	if count != 0 || len(snippets) != 0 {
		t.Fatalf("no-match scan = count %d, snippets %+v, want 0 and empty", count, snippets)
	}

	count, snippets = scanContentForMatches(content, "ORDERS", 3)
	if count != 4 {
		t.Fatalf("uppercase query match count = %d, want 4 (case-insensitive)", count)
	}
	_ = snippets

	long := []byte(strings.Repeat("x", 250) + "needle")
	_, longSnippets := scanContentForMatches(long, "needle", 1)
	if len(longSnippets) != 1 {
		t.Fatalf("long line snippets = %+v, want 1", longSnippets)
	}
	if !strings.HasSuffix(longSnippets[0].Excerpt, "...") || len(longSnippets[0].Excerpt) != maxSnippetExcerptLength+3 {
		t.Fatalf("long line excerpt = %q (len %d), want %d chars plus ellipsis", longSnippets[0].Excerpt, len(longSnippets[0].Excerpt), maxSnippetExcerptLength)
	}

	emptyCount, emptySnippets := scanContentForMatches(content, "", 3)
	if emptyCount != 0 || len(emptySnippets) != 0 {
		t.Fatalf("empty query scan = count %d, snippets %+v, want 0 and empty", emptyCount, emptySnippets)
	}
}

func TestSearchMatchesTextFilesAndRanksByCount(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	if _, err := f.service.Search(f.ctx, scope, SearchInput{Query: "o"}); !errors.Is(err, ErrInvalidSearchQuery) {
		t.Fatalf("short query error = %v, want %v", err, ErrInvalidSearchQuery)
	}

	folder, err := f.service.Create(f.ctx, scope, CreateInput{Name: "reports", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	orders, err := f.service.Create(f.ctx, scope, CreateInput{Name: "orders.sql", ParentID: &folder.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, orders.ID, "", strings.NewReader(
		"select * from orders\nwhere orders.id = 1\n-- orders report\nselect orders_alias")); err != nil {
		t.Fatal(err)
	}
	customers, err := f.service.Create(f.ctx, scope, CreateInput{Name: "customers.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, customers.ID, "", strings.NewReader("select * from customers")); err != nil {
		t.Fatal(err)
	}
	binary, err := f.service.Create(f.ctx, scope, CreateInput{Name: "orders.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, binary.ID, "", strings.NewReader("orders binary payload")); err != nil {
		t.Fatal(err)
	}

	sharedScope := f.sharedScope(f.member)
	if _, err := f.service.Search(f.ctx, sharedScope, SearchInput{Query: "orders"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("shared search without permission error = %v, want %v", err, ErrForbidden)
	}

	result, err := f.service.Search(f.ctx, scope, SearchInput{Query: "ORDERS"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Query != "ORDERS" || result.FilesScanned != 2 || result.Truncated {
		t.Fatalf("search result summary = %+v, want query ORDERS, 2 scanned, not truncated", result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results = %+v, want only orders.sql (binary and no-match files excluded)", result.Results)
	}
	match := result.Results[0]
	if match.File.ID != orders.ID || match.MatchCount != 4 {
		t.Fatalf("match = %+v, want orders.sql with 4 matches", match)
	}
	if len(match.Path) != 2 || match.Path[0].Name != "reports" || match.Path[1].Name != "orders.sql" {
		t.Fatalf("match path = %+v, want [reports orders.sql]", match.Path)
	}
	if len(match.Snippets) != 3 {
		t.Fatalf("snippets = %+v, want 3 (capped)", match.Snippets)
	}
	first := match.Snippets[0]
	if first.Line != 1 || first.Column != 15 || first.Excerpt != "select * from orders" {
		t.Fatalf("first snippet = %+v, want line 1 column 15 %q", first, "select * from orders")
	}
	for i := 1; i < len(match.Snippets); i++ {
		if match.Snippets[i].Line <= match.Snippets[i-1].Line {
			t.Fatalf("snippets not in increasing line order: %+v", match.Snippets)
		}
	}
}

func TestSearchDoesNotLeakPrivateFilesAcrossAccounts(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})

	ownerScope := f.privateScope(f.owner)
	memberScope := f.privateScope(f.member)

	ownerFile, err := f.service.Create(f.ctx, ownerScope, CreateInput{Name: "owner-orders.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, ownerScope, ownerFile.ID, "", strings.NewReader("select * from orders")); err != nil {
		t.Fatal(err)
	}

	memberFile, err := f.service.Create(f.ctx, memberScope, CreateInput{Name: "member-orders.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, memberScope, memberFile.ID, "", strings.NewReader("select * from orders")); err != nil {
		t.Fatal(err)
	}

	memberResult, err := f.service.Search(f.ctx, memberScope, SearchInput{Query: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(memberResult.Results) != 1 || memberResult.Results[0].File.ID != memberFile.ID {
		t.Fatalf("member search results = %+v, want only member-orders.sql (owner's private file must not leak)", memberResult.Results)
	}

	ownerResult, err := f.service.Search(f.ctx, ownerScope, SearchInput{Query: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerResult.Results) != 1 || ownerResult.Results[0].File.ID != ownerFile.ID {
		t.Fatalf("owner search results = %+v, want only owner-orders.sql (member's private file must not leak)", ownerResult.Results)
	}
}

func TestSearchDoesNotLeakFilesAcrossWorkspaces(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})

	otherWs, err := f.db.InsertWorkspace(f.ctx, &f.org.ID, "org", f.org.ID, "Other Workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddWorkspaceMember(f.ctx, otherWs.ID, f.owner.ID, nil); err != nil {
		t.Fatal(err)
	}

	scope := f.privateScope(f.owner)
	otherScope := scope
	otherScope.Workspace = otherWs

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "orders.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select * from orders")); err != nil {
		t.Fatal(err)
	}

	otherFile, err := f.service.Create(f.ctx, otherScope, CreateInput{Name: "other-orders.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, otherScope, otherFile.ID, "", strings.NewReader("select * from orders")); err != nil {
		t.Fatal(err)
	}

	result, err := f.service.Search(f.ctx, scope, SearchInput{Query: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].File.ID != file.ID {
		t.Fatalf("workspace search results = %+v, want only orders.sql from this workspace (other workspace's file must not leak)", result.Results)
	}
}

func TestSearchTruncatesAtMaxFilesScanned(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	originalMax := maxSearchFilesScanned
	maxSearchFilesScanned = 1
	t.Cleanup(func() { maxSearchFilesScanned = originalMax })

	first, err := f.service.Create(f.ctx, scope, CreateInput{Name: "a.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, first.ID, "", strings.NewReader("select 1")); err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Create(f.ctx, scope, CreateInput{Name: "b.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, second.ID, "", strings.NewReader("select 1")); err != nil {
		t.Fatal(err)
	}

	result, err := f.service.Search(f.ctx, scope, SearchInput{Query: "select"})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 1 || !result.Truncated {
		t.Fatalf("truncated search result = %+v, want 1 scanned and truncated", result)
	}
	if len(result.Results) != 1 || result.Results[0].File.ID != first.ID {
		t.Fatalf("truncated results = %+v, want only a.sql (name-ordered)", result.Results)
	}
}
