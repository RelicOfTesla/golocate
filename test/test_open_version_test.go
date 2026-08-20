package test

import (
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPI_Open_RejectsOutOfScope verifies the open RPC refuses a path outside
// the test server's allowed directories (repoRoot).
func TestAPI_Open_RejectsOutOfScope(t *testing.T) {
	// Earlier reload/set-config tests may have widened the path validator
	// (e.g. a reload with default directories "/"); restore the repo scope
	// so this rejection test is deterministic.
	testServer.SetConfig(&config.Config{
		Directories:    []string{repoRoot()},
		WorkerCount:    1,
		IgnorePatterns: []string{".git", ".gocache", ".gopath"},
	})

	response := sendAPIRequest(t, "open", "/etc/passwd")
	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Out-of-scope path must be rejected")
	assert.Contains(t, response["error"], "path not allowed", "Error should mention path validation")
}

// TestAPI_Status_HasProtocolVersion verifies the status RPC carries the wire
// protocol version (for client/server compatibility checks).
func TestAPI_Status_HasProtocolVersion(t *testing.T) {
	response := sendAPIRequest(t, "status", "")
	require.NotNil(t, response, "Should return a response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	version, ok := result["protocol_version"]
	require.True(t, ok, "Should have protocol_version field")
	require.GreaterOrEqual(t, anyInt64(version), int64(1), "Protocol version should be >= 1")
}
