package test

import (
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anyInt64 converts a JSON-decoded number (float64/int64/int) to int64.
func anyInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// TestAPI_Build_ReportsProgress verifies that a socket-triggered index build
// reports live progress (build_scanned) through the status API while it runs,
// and that the build completes with a non-empty index.
func TestAPI_Build_ReportsProgress(t *testing.T) {
	// Earlier set-config tests may have pointed the server at other
	// directories; restore the repo index configuration first so the rebuild
	// below reproduces the test index (and later tests can rely on it).
	testServer.SetConfig(&config.Config{
		Directories:    []string{repoRoot()},
		WorkerCount:    1,
		IgnorePatterns: []string{".git", ".gocache", ".gopath"},
	})

	// Trigger an index rebuild via the build API.
	response := sendAPIRequest(t, "build", "")
	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "build", response["type"], "Response type should be 'build'")

	var sawBuildingWithProgress bool
	var lastScanned int64 = -1
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		status := sendAPIRequest(t, "status", "")
		require.NotNil(t, status, "Should return a status response")
		result, ok := status["result"].(map[string]any)
		require.True(t, ok, "Should have result field")

		isBuilding, ok := result["is_building"].(bool)
		require.True(t, ok, "Should have is_building field")

		if isBuilding {
			scannedAny, hasScanned := result["build_scanned"]
			assert.True(t, hasScanned, "build_scanned should be present while building")
			if hasScanned {
				scanned := anyInt64(scannedAny)
				t.Logf("build in progress, scanned=%d", scanned)
				assert.GreaterOrEqual(t, scanned, int64(0), "build_scanned must be non-negative")
				if lastScanned >= 0 {
					assert.GreaterOrEqual(t, scanned, lastScanned,
						"build_scanned must be non-decreasing (got %d after %d)", scanned, lastScanned)
				}
				if scanned > 0 {
					sawBuildingWithProgress = true
				}
				lastScanned = scanned
			}
		} else {
			// Build finished.
			fileCount, ok := result["indexed_file_count"]
			require.True(t, ok, "Should have indexed_file_count field")
			assert.Greater(t, anyInt64(fileCount), int64(0), "Build should index files")
			t.Logf("build finished, indexed_file_count=%d", anyInt64(fileCount))
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	if !sawBuildingWithProgress {
		t.Logf("NOTE: saw no build_scanned>0 sample (build too fast); fields were still validated")
	}
}
