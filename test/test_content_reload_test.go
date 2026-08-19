package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cliclient "github.com/RelicOfTesla/golocate/pkg/cli"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Content Search API Tests ==========

// TestAPI_SearchContent_ReturnsMatches tests that a content search returns
// grep-style matches (path, line number, line text) instead of plain paths.
func TestAPI_SearchContent_ReturnsMatches(t *testing.T) {
	c := getTestClient(t)

	result, err := c.SearchContent("", "golocate", index.SearchOptions{
		Limit: 50,
	})
	require.NoError(t, err, "Content search should succeed")
	require.NotNil(t, result)
	require.Greater(t, len(result.Matches), 0, "Should find matches for 'golocate' in the repo")

	for _, m := range result.Matches {
		assert.NotEmpty(t, m.Path, "Match should have a path")
		assert.GreaterOrEqual(t, m.LineNum, 1, "Match should have a line number")
		assert.Contains(t, strings.ToLower(m.Line), "golocate", "Match line should contain the keyword")
	}
}

// TestAPI_SearchContent_Context tests that matches carry surrounding context
// lines (grep -C1 style).
func TestAPI_SearchContent_Context(t *testing.T) {
	c := getTestClient(t)

	// Restrict to README_CN.md, which has "golocate" mid-file (context on
	// both sides) and at line 1 (no before-context).
	result, err := c.SearchContent("README_CN", "golocate", index.SearchOptions{
		Limit: 100,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Greater(t, len(result.Matches), 0, "Should find matches in README_CN.md")

	hasBoth := false
	for _, m := range result.Matches {
		assert.Equal(t, "README_CN.md", filepath.Base(m.Path))
		// Context lines may legitimately be blank lines; only require presence.
		if len(m.Before) > 0 && len(m.After) > 0 {
			hasBoth = true
		}
	}
	assert.True(t, hasBoth, "At least one match should carry before+after context")
}

// TestAPI_SearchContent_WithPathPattern tests content search combined with a
// path pattern: results should be restricted to files matching the pattern.
func TestAPI_SearchContent_WithPathPattern(t *testing.T) {
	c := getTestClient(t)

	result, err := c.SearchContent("README", "golocate", index.SearchOptions{
		Limit: 20,
	})
	require.NoError(t, err, "Content search should succeed")
	require.NotNil(t, result)
	require.Greater(t, len(result.Matches), 0, "Should find matches in README files")

	for _, m := range result.Matches {
		assert.Contains(t, strings.ToLower(m.Path), "readme", "All matches should be in README files")
	}
}

// TestAPI_SearchContent_NoMatch tests content search with a keyword that
// does not exist anywhere. The keyword is generated at runtime so it can
// never appear in any indexed file (including this test file itself).
func TestAPI_SearchContent_NoMatch(t *testing.T) {
	c := getTestClient(t)

	keyword := fmt.Sprintf("golocate-no-such-keyword-%d", time.Now().UnixNano())
	result, err := c.SearchContent("", keyword, index.SearchOptions{
		Limit: 10,
	})
	require.NoError(t, err, "Content search should succeed")
	require.NotNil(t, result)
	assert.Len(t, result.Matches, 0, "Should find no matches")
	assert.Equal(t, 0, result.Count)
}

// ========== reload-config API Tests ==========

func testConfigPath() string {
	return filepath.Join(os.TempDir(), "golocate-test-config.yaml")
}

func writeTestConfig(t *testing.T, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(testConfigPath(), []byte(content), 0644))
}

// TestAPI_ReloadConfig_ValidConfig tests that reload-config reloads the config file.
func TestAPI_ReloadConfig_ValidConfig(t *testing.T) {
	writeTestConfig(t, "worker_count: 4\n")

	response := sendAPIRequest(t, "reload-config", "")
	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "reload-config", response["type"], "Response type should be 'reload-config'")

	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")
	assert.Equal(t, "reloaded", result["status"], "Should report reloaded status")
}

// TestAPI_ReloadConfig_InvalidConfig tests that reload-config reports invalid YAML.
func TestAPI_ReloadConfig_InvalidConfig(t *testing.T) {
	writeTestConfig(t, "worker_count: not-a-number\n")

	response := sendAPIRequest(t, "reload-config", "")
	require.NotNil(t, response, "Should return a response")

	// Invalid worker_count fails validation -> error response
	assert.Equal(t, "error", response["type"], "Invalid config should produce an error")

	// Restore a valid config so subsequent tests are unaffected
	writeTestConfig(t, "worker_count: 4\n")
}

// cliSearchContent exercises pkg/cli.Search with Content set (the same path
// the `golocate --content` command uses). Pattern restricts candidate files;
// content is the keyword.
func cliSearchContent(pattern, keyword string, limit int) (*cliclient.SearchResult, error) {
	return cliclient.Search(cliclient.SearchOptions{
		Pattern:    pattern,
		Content:    keyword,
		Limit:      limit,
		SocketPath: socketPath,
	})
}

// TestAPI_SearchContent_ViaCLI tests the CLI-level search options with content.
func TestAPI_SearchContent_ViaCLI(t *testing.T) {
	res, err := cliSearchContent("README", "golocate", 20)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Greater(t, len(res.Matches), 0, "CLI content search should find matches")
	for _, m := range res.Matches {
		assert.NotEmpty(t, m.Path)
		assert.GreaterOrEqual(t, m.LineNum, 1)
	}
}
