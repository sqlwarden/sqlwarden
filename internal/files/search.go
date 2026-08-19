package files

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

const (
	minSearchQueryLength    = 2
	maxSnippetExcerptLength = 200
)

var (
	maxSearchFilesScanned    = 500
	maxSearchBytesPerFile    = 2 * 1024 * 1024
	maxSearchResults         = 50
	maxSearchSnippetsPerFile = 3
)

// SearchSnippet is one matched line within a searched file, carrying enough
// position information for the editor to jump to it.
type SearchSnippet struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Excerpt string `json:"excerpt"`
}

// scanContentForMatches counts case-insensitive occurrences of lowerQuery in
// content and returns up to maxSnippets snippets in first-match order, each
// carrying its 1-based line/column and a single-line excerpt. Both content
// and lowerQuery are lowercased internally, so the caller does not need to
// pre-lowercase either (callers should still pre-lowercase the query once
// per search rather than once per file, since re-lowercasing it here per
// candidate would otherwise repeat identical work).
func scanContentForMatches(content []byte, lowerQuery string, maxSnippets int) (int, []SearchSnippet) {
	if lowerQuery == "" {
		return 0, nil
	}
	lower := bytes.ToLower(content)
	queryBytes := bytes.ToLower([]byte(lowerQuery))

	matchCount := 0
	var snippets []SearchSnippet
	line := 1
	lineStart := 0
	cursor := 0
	searchFrom := 0

	for {
		idx := bytes.Index(lower[searchFrom:], queryBytes)
		if idx == -1 {
			break
		}
		matchPos := searchFrom + idx

		for ; cursor < matchPos; cursor++ {
			if content[cursor] == '\n' {
				line++
				lineStart = cursor + 1
			}
		}

		matchCount++
		if len(snippets) < maxSnippets {
			snippets = append(snippets, SearchSnippet{
				Line:    line,
				Column:  matchPos - lineStart + 1,
				Excerpt: excerptFromLine(content, lineStart),
			})
		}
		searchFrom = matchPos + 1
	}

	return matchCount, snippets
}

// excerptFromLine returns the single line starting at lineStart, truncated
// to maxSnippetExcerptLength bytes with an ellipsis marker.
func excerptFromLine(content []byte, lineStart int) string {
	end := lineStart
	for end < len(content) && content[end] != '\n' {
		end++
	}
	line := content[lineStart:end]
	if len(line) > maxSnippetExcerptLength {
		truncated := make([]byte, 0, maxSnippetExcerptLength+3)
		truncated = append(truncated, line[:maxSnippetExcerptLength]...)
		truncated = append(truncated, "..."...)
		return string(truncated)
	}
	return string(line)
}

// SearchInput is the domain input for a workspace file content search.
type SearchInput struct {
	Query string
}

// SearchFileResult is one matched file within a content search, carrying its
// breadcrumb path (a sibling of File, matching BrowserResult's shape) and up
// to maxSearchSnippetsPerFile matched lines.
type SearchFileResult struct {
	File       database.WorkspaceFile `json:"file"`
	Path       []PathSegment          `json:"path"`
	MatchCount int                    `json:"match_count"`
	Snippets   []SearchSnippet        `json:"snippets"`
}

// SearchResult is the full response for a workspace file content search.
type SearchResult struct {
	Query        string             `json:"query"`
	Results      []SearchFileResult `json:"results"`
	FilesScanned int                `json:"files_scanned"`
	Truncated    bool               `json:"truncated"`
}

// Search scans the authorized file tree's text-like files for a
// case-insensitive substring match and returns per-file snippets with jump
// targets. A single broken file mid-scan (missing content, unavailable
// storage backend, or a read error) is skipped rather than failing the
// whole request, since one bad blob should not take down search for the
// rest of the workspace.
func (s *Service) Search(ctx context.Context, scope Scope, input SearchInput) (SearchResult, error) {
	query := strings.TrimSpace(input.Query)
	if len(query) < minSearchQueryLength {
		return SearchResult{}, ErrInvalidSearchQuery
	}
	ownerID, err := s.authorizeTree(ctx, scope, access.PermWsFileRead)
	if err != nil {
		return SearchResult{}, err
	}

	all, err := s.db.ListAllWorkspaceFiles(ctx, scope.Workspace.ID, scope.Visibility, ownerID)
	if err != nil {
		return SearchResult{}, err
	}
	candidates := all[:0]
	for _, file := range all {
		if isTextLikeFile(file) {
			candidates = append(candidates, file)
		}
	}

	truncated := false
	if len(candidates) > maxSearchFilesScanned {
		candidates = candidates[:maxSearchFilesScanned]
		truncated = true
	}

	lowerQuery := strings.ToLower(query)
	results := make([]SearchFileResult, 0, len(candidates))
	for _, file := range candidates {
		content, found, err := s.db.CurrentWorkspaceFileContent(ctx, file)
		if err != nil || !found {
			continue
		}
		store, err := s.storeForContent(ctx, content)
		if err != nil {
			continue
		}
		reader, _, err := store.Get(ctx, content.StorageKey)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, int64(maxSearchBytesPerFile)))
		reader.Close()
		if readErr != nil {
			continue
		}

		matchCount, snippets := scanContentForMatches(data, lowerQuery, maxSearchSnippetsPerFile)
		if matchCount == 0 {
			continue
		}
		path, err := s.pathSegments(ctx, file)
		if err != nil {
			continue
		}
		results = append(results, SearchFileResult{
			File:       file,
			Path:       path,
			MatchCount: matchCount,
			Snippets:   snippets,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].MatchCount != results[j].MatchCount {
			return results[i].MatchCount > results[j].MatchCount
		}
		return results[i].File.Name < results[j].File.Name
	})
	if len(results) > maxSearchResults {
		results = results[:maxSearchResults]
	}

	return SearchResult{
		Query:        query,
		Results:      results,
		FilesScanned: len(candidates),
		Truncated:    truncated,
	}, nil
}
