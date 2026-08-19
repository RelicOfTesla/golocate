package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/internal/testutil"
	"github.com/RelicOfTesla/golocate/pkg/config"
	contentpkg "github.com/RelicOfTesla/golocate/pkg/content"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startContentIndexServer builds a server whose content search uses a
// pre-built content token index.
func startContentIndexServer(t *testing.T, dir string, buildIndex bool) *server.Server {
	t.Helper()

	builder := index.NewBuilder(index.BuilderOptions{WorkerCount: 1})
	require.NoError(t, builder.Build(context.Background(), []string{dir}))
	idx := builder.Index()

	var ci *contentpkg.Index
	if buildIndex {
		ci = contentpkg.NewIndex(0)
		for _, e := range idx.GetAllEntries() {
			if !e.IsDir {
				ci.AddFile(e.Path)
			}
		}
	}

	srv := server.New(idx)
	socket := testutil.GetTestSocketPath("contentindex")
	srv.SetSocketPath(socket)
	srv.SetConfig(&config.Config{Directories: []string{dir}})
	srv.SetContentIndex(ci)
	require.NoError(t, srv.Start())
	t.Cleanup(func() { srv.Stop() })
	return srv
}

// TestContentIndex_PreciseCandidate verifies that a single-word content
// search hits a file that is NOT among the first candidates by mtime (the
// content index provides the precise candidate).
func TestContentIndex_PreciseCandidate(t *testing.T) {
	dir := t.TempDir()
	// Many NEW files without the keyword + one OLD file with it: mtime order
	// would scan the keyword file last (it would be capped away without the
	// index, since it is the only file with the token and sits at the end).
	for i := 0; i < 200; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "filler"+string(rune('a'+i%26))+string(rune('0'+i%10))+".txt"), []byte("filler"), 0o644))
	}
	old := filepath.Join(dir, "old_key.txt")
	require.NoError(t, os.WriteFile(old, []byte("precisetoken marker"), 0o644))
	// Set the keyword file's mtime far in the past.
	oldInfo, err := os.Stat(old)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(old, oldInfo.ModTime(), oldInfo.ModTime().AddDate(-5, 0, 0)))

	_ = startContentIndexServer(t, dir, true)
	c := getTestClient(t)
	c.SetSocketPath(testutil.GetTestSocketPath("contentindex"))

	res, err := c.SearchContent("", "precisetoken", index.SearchOptions{Limit: 50})
	require.NoError(t, err)
	require.Greater(t, len(res.Matches), 0, "content index should find the old keyword file")
	for _, m := range res.Matches {
		assert.Equal(t, old, m.Path)
	}
}

// TestContentIndex_SubstringFallback verifies that when the keyword is not a
// whole token (e.g. "needle" inside token "needleword"), the server falls
// back to the full scan and still finds substring matches.
func TestContentIndex_SubstringFallback(t *testing.T) {
	dir := t.TempDir()
	// "needleword" is the only token; the query "needle" is a substring of it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needleword here\n"), 0o644))

	_ = startContentIndexServer(t, dir, true)
	c := getTestClient(t)
	c.SetSocketPath(testutil.GetTestSocketPath("contentindex"))

	res, err := c.SearchContent("", "needle", index.SearchOptions{Limit: 50})
	require.NoError(t, err)
	require.Greater(t, len(res.Matches), 0, "substring match must survive the fallback scan")
	assert.Equal(t, "needleword here", res.Matches[0].Line)
}

// TestContentIndex_Disabled verifies that without an index the server still
// serves content searches (regression guard for the nil-index path).
func TestContentIndex_Disabled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("plainword content\n"), 0o644))

	_ = startContentIndexServer(t, dir, false)
	c := getTestClient(t)
	c.SetSocketPath(testutil.GetTestSocketPath("contentindex"))

	res, err := c.SearchContent("", "plainword", index.SearchOptions{Limit: 50})
	require.NoError(t, err)
	require.Greater(t, len(res.Matches), 0, "content search must work with a nil content index")
}
