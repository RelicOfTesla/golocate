package test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sync/atomic"

	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/internal/testutil"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Terms mode (combined query syntax) ==========

// TestAPI_TermsMode_AndAndExclude tests "foo -bar" semantics: every positive
// term must match, negative terms must not.
func TestAPI_TermsMode_AndAndExclude(t *testing.T) {
	c := getTestClient(t)

	// "server -test": paths containing "server" but not "test"
	results, err := c.Search("server -test", index.SearchOptions{
		PatternMode: index.PatternModeTerms,
		Limit:       50,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "Should find server files")

	for _, r := range results {
		lower := strings.ToLower(r.Path)
		assert.Contains(t, lower, "server", "Each result should contain every positive term")
		assert.NotContains(t, lower, "test", "Excluded term must not appear")
	}
}

// TestAPI_TermsMode_MultipleAnd tests multiple positive terms (AND semantics).
func TestAPI_TermsMode_MultipleAnd(t *testing.T) {
	c := getTestClient(t)

	results, err := c.Search("internal client", index.SearchOptions{
		PatternMode: index.PatternModeTerms,
		Limit:       50,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "Should find internal/client files")

	for _, r := range results {
		lower := strings.ToLower(r.Path)
		assert.Contains(t, lower, "internal")
		assert.Contains(t, lower, "client")
	}
}

// ========== Scope (directory restriction) ==========

// TestAPI_Scope_RestrictsResults tests that scope limits results to a directory.
func TestAPI_Scope_RestrictsResults(t *testing.T) {
	c := getTestClient(t)

	// Scope resolves relative to the server's working directory, so use the
	// absolute repo path for a deterministic expectation.
	scope := filepath.Join(repoRoot(), "internal")

	results, err := c.Search("go", index.SearchOptions{
		Limit: 100,
		Scope: scope,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "Should find .go files under internal/")

	scopePrefix := scope + string(filepath.Separator)
	for _, r := range results {
		assert.True(t,
			strings.HasPrefix(r.Path, scopePrefix) || r.Path == scope,
			"Result %q should be inside scope %s", r.Path, scope)
	}
}

// ========== Exclude (query-time glob exclusion) ==========

// TestAPI_Exclude_DropsMatchingPaths tests that exclude globs drop results.
func TestAPI_Exclude_DropsMatchingPaths(t *testing.T) {
	c := getTestClient(t)

	results, err := c.Search("test", index.SearchOptions{
		Limit:   100,
		Exclude: []string{"*_test.go"},
	})
	require.NoError(t, err)

	// Search without exclusion should find *_test.go files...
	all, err := c.Search("test", index.SearchOptions{Limit: 100})
	require.NoError(t, err)
	require.NotEmpty(t, all, "Sanity: should find test files")

	for _, r := range results {
		assert.False(t, strings.HasSuffix(r.Path, "_test.go"),
			"Excluded pattern *_test.go should not appear in results")
	}
}

// ========== stop method ==========

// TestAPI_Stop_StopsServer tests the stop RPC against a dedicated server.
func TestAPI_Stop_StopsServer(t *testing.T) {
	idx := index.NewIndex()
	idx.Add(&index.Entry{Name: "a.txt", Path: "a.txt"})

	srv := server.New(idx)
	socket := testutil.GetTestSocketPath("stop")
	srv.SetSocketPath(socket)
	require.NoError(t, srv.Start())
	t.Cleanup(func() { srv.Stop() })

	c := getTestClient(t)
	c.SetSocketPath(socket)
	require.True(t, c.IsServerRunning(), "Server should be running before stop")

	require.NoError(t, c.Stop(), "Stop request should succeed")

	// The handler stops asynchronously; give it a moment.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !srv.IsRunning() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not stop after stop request")
}

// ========== Search hook (boot-scan throttle lift) ==========

// TestAPI_SearchHook_InvokedOnSearch verifies every search request invokes
// the server's search hook (the mechanism that lifts boot-scan throttling).
func TestAPI_SearchHook_InvokedOnSearch(t *testing.T) {
	idx := index.NewIndex()
	idx.Add(&index.Entry{Name: "hook.txt", Path: "hook.txt"})

	srv := server.New(idx)
	socket := testutil.GetTestSocketPath("searchhook")
	srv.SetSocketPath(socket)
	require.NoError(t, srv.Start())
	t.Cleanup(func() { srv.Stop() })

	var calls atomic.Int32
	srv.SetSearchHook(func() { calls.Add(1) })

	c := getTestClient(t)
	c.SetSocketPath(socket)

	_, err := c.Search("hook", index.SearchOptions{Limit: 10})
	require.NoError(t, err)
	_, err = c.Search("hook", index.SearchOptions{Limit: 10})
	require.NoError(t, err)

	assert.Equal(t, int32(2), calls.Load(), "each search request should invoke the hook")
}

// ========== Metadata filters (type / size / mtime / hidden) ==========

// TestAPI_MetadataFilters_TypeAndSize tests extension and size filters over
// the socket protocol.
func TestAPI_MetadataFilters_TypeAndSize(t *testing.T) {
	c := getTestClient(t)

	// Only .go files, at least 1 byte (directories excluded by type filter).
	results, err := c.Search("", index.SearchOptions{
		Limit:   100,
		Types:   []string{"go"},
		MinSize: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "Should find .go files")

	for _, r := range results {
		assert.False(t, r.IsDir, "directories should be excluded with a type filter")
		assert.Equal(t, "go", strings.TrimPrefix(filepath.Ext(r.Name), "."), "extension should match filter")
	}
}

// TestAPI_MetadataFilters_NoHidden tests the exclude-hidden filter.
func TestAPI_MetadataFilters_NoHidden(t *testing.T) {
	c := getTestClient(t)

	// Sanity: without the filter, some hidden path exists in the repo index.
	all, err := c.Search("", index.SearchOptions{Limit: 1000})
	require.NoError(t, err)
	hasHidden := false
	for _, r := range all {
		if strings.HasPrefix(filepath.Base(r.Path), ".") {
			hasHidden = true
			break
		}
	}
	if !hasHidden {
		t.Skip("repo index has no hidden files to filter")
	}

	results, err := c.Search("", index.SearchOptions{Limit: 1000, ExcludeHidden: true})
	require.NoError(t, err)
	for _, r := range results {
		assert.False(t, strings.HasPrefix(filepath.Base(r.Path), "."),
			"hidden file %s should be excluded", r.Path)
	}
}
