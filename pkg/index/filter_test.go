package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testFilterIndex() *Index {
	idx := NewIndex()
	idx.Add(&Entry{Name: "a.go", Path: "/tmp/pkg/a.go", IsDir: false, Size: 100, ModTime: time.Unix(1_700_000_000, 0)})
	idx.Add(&Entry{Name: "b.md", Path: "/tmp/pkg/b.md", IsDir: false, Size: 5000, ModTime: time.Unix(1_700_100_000, 0)})
	idx.Add(&Entry{Name: "c.txt", Path: "/tmp/pkg/c.txt", IsDir: false, Size: 20000, ModTime: time.Unix(1_700_200_000, 0)})
	idx.Add(&Entry{Name: "pkg", Path: "/tmp/pkg", IsDir: true})
	idx.Add(&Entry{Name: ".hidden.go", Path: "/tmp/pkg/.hidden.go", IsDir: false, Size: 50})
	idx.Add(&Entry{Name: "d", Path: "/tmp/.dot/d", IsDir: true})
	return idx
}

// TestFilterMetadata_ByType verifies extension filtering and directory exclusion.
func TestFilterMetadata_ByType(t *testing.T) {
	idx := testFilterIndex()
	results := idx.Search(SearchOptions{Pattern: "", PatternMode: PatternModeNormal, Types: []string{"go"}})

	var paths []string
	for _, r := range results {
		paths = append(paths, r.Path)
	}
	assert.Contains(t, paths, "/tmp/pkg/a.go")
	assert.Contains(t, paths, "/tmp/pkg/.hidden.go")
	assert.NotContains(t, paths, "/tmp/pkg/b.md")
	assert.NotContains(t, paths, "/tmp/pkg", "directories excluded when type filter set")
	assert.Len(t, paths, 2)
}

// TestFilterMetadata_BySize verifies size range filtering.
func TestFilterMetadata_BySize(t *testing.T) {
	idx := testFilterIndex()

	results := idx.Search(SearchOptions{Pattern: "", MinSize: 1000, MaxSize: 10000})
	var paths []string
	for _, r := range results {
		paths = append(paths, r.Path)
	}
	assert.Contains(t, paths, "/tmp/pkg/b.md")
	assert.NotContains(t, paths, "/tmp/pkg/a.go")  // 100 < 1000
	assert.NotContains(t, paths, "/tmp/pkg/c.txt") // 20000 > 10000

	// Count should agree with Search.
	assert.Equal(t, len(results), idx.Count("", SearchOptions{MinSize: 1000, MaxSize: 10000}))
}

// TestFilterMetadata_ByMtime verifies modification time filtering.
func TestFilterMetadata_ByMtime(t *testing.T) {
	idx := testFilterIndex()

	results := idx.Search(SearchOptions{Pattern: "", MtimeAfter: 1_700_050_000})
	var paths []string
	for _, r := range results {
		paths = append(paths, r.Path)
	}
	assert.Contains(t, paths, "/tmp/pkg/b.md")
	assert.Contains(t, paths, "/tmp/pkg/c.txt")
	assert.NotContains(t, paths, "/tmp/pkg/a.go") // older than threshold
}

// TestFilterMetadata_ExcludeHidden verifies dotfile exclusion.
func TestFilterMetadata_ExcludeHidden(t *testing.T) {
	idx := testFilterIndex()

	results := idx.Search(SearchOptions{Pattern: "", ExcludeHidden: true})
	for _, r := range results {
		assert.False(t, isHiddenPath(r.Path), "%s should be excluded", r.Path)
	}

	// Without the filter hidden entries are present.
	all := idx.Search(SearchOptions{Pattern: ""})
	found := false
	for _, r := range all {
		if isHiddenPath(r.Path) {
			found = true
			break
		}
	}
	assert.True(t, found, "hidden entries exist without the filter")
}

// TestFilterMetadata_Combined verifies filters compose with scope/exclude.
func TestFilterMetadata_Combined(t *testing.T) {
	idx := testFilterIndex()
	opts := SearchOptions{
		Types:   []string{"go", "md"},
		MinSize: 60,
		Scope:   "/tmp/pkg",
		Exclude: []string{"*.hidden*"},
		Limit:   100,
	}
	results := idx.Search(opts)
	require.NotEmpty(t, results)
	for _, r := range results {
		assert.False(t, isExcludedPath(r.Path, opts.Exclude))
		assert.Equal(t, "/tmp/pkg", r.Path[:len("/tmp/pkg")])
		assert.False(t, r.IsDir)
		assert.GreaterOrEqual(t, r.Size, int64(60))
	}
}
