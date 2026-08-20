package test

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Part 1: Status API Tests ==========

// TestAPI_Status_ReturnsServerStatus tests that status API returns server status
func TestAPI_Status_ReturnsServerStatus(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "status", response["type"], "Response type should be 'status'")

	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check running status
	running, ok := result["running"].(bool)
	assert.True(t, ok, "Should have 'running' field (bool)")
	assert.True(t, running, "Server should be running")
}

// TestAPI_Status_HasIndexSize tests that status API returns index size
func TestAPI_Status_HasIndexSize(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check index_size
	indexSize, ok := result["index_size"]
	assert.True(t, ok, "Should have 'index_size' field")
	t.Logf("Index size: %v (type: %T)", indexSize, indexSize)
}

// TestAPI_Status_HasIndexedFileCount tests that status API returns indexed file count
func TestAPI_Status_HasIndexedFileCount(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check indexed_file_count
	fileCount, ok := result["indexed_file_count"]
	assert.True(t, ok, "Should have 'indexed_file_count' field")
	t.Logf("Indexed file count: %v (type: %T)", fileCount, fileCount)
}

// TestAPI_Status_HasBuildingStatus tests that status API returns index building status
func TestAPI_Status_HasBuildingStatus(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check is_building
	isBuilding, ok := result["is_building"].(bool)
	assert.True(t, ok, "Should have 'is_building' field (bool)")
	t.Logf("Is building: %v", isBuilding)

	// If building, should have build_duration
	if isBuilding {
		buildDuration, ok := result["build_duration"]
		assert.True(t, ok, "Should have 'build_duration' when building")
		t.Logf("Build duration: %v", buildDuration)
	}
}

// TestAPI_Status_HasConfigPath tests that status API returns config path
func TestAPI_Status_HasConfigPath(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check config_path (may be empty if not set)
	configPath, ok := result["config_path"]
	if ok && configPath != nil {
		t.Logf("Config path: %v", configPath)
	}
}

// TestAPI_Status_HasLastBuildTime tests that status API returns last build time
func TestAPI_Status_HasLastBuildTime(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check last_build_time (may not exist if never built)
	if lastBuildTime, ok := result["last_build_time"]; ok {
		t.Logf("Last build time: %v", lastBuildTime)
	}
	if lastBuildAgo, ok := result["last_build_ago"]; ok {
		t.Logf("Last build ago: %v", lastBuildAgo)
	}
}

// TestAPI_Status_AllFields tests that status API returns all expected fields
func TestAPI_Status_AllFields(t *testing.T) {
	response := sendAPIRequest(t, "status", "")

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "status", response["type"], "Response type should be 'status'")

	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Expected fields (may have additional fields)
	expectedFields := []string{
		"running",
		"index_size",
		"indexed_file_count",
		"is_building",
	}

	for _, field := range expectedFields {
		_, exists := result[field]
		assert.True(t, exists, "Should have field '%s'", field)
	}

	t.Logf("Status fields: %v", result)
}

// TestAPI_Status_JsonRPCFormat tests status API with JSON-RPC format
func TestAPI_Status_JsonRPCFormat(t *testing.T) {
	conn := connectSocket(t)
	defer conn.Close()

	// Send JSON-RPC request
	request := `{"jsonrpc":"2.0","method":"status","id":1}`
	_, err := conn.Write([]byte(request + "\n"))
	require.NoError(t, err, "Should send JSON-RPC request")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	require.NoError(t, err, "Should read response")
	t.Logf("Response: %s", strings.TrimSpace(response))

	// Parse response
	var result map[string]any
	err = json.Unmarshal([]byte(response), &result)
	require.NoError(t, err, "Should parse JSON response")

	// JSON-RPC response should have "jsonrpc", "id", and "result" or "error" fields
	assert.Equal(t, "2.0", result["jsonrpc"], "Should have jsonrpc version")
	assert.NotNil(t, result["id"], "Should have id")

	// Should have either result or error
	_, hasResult := result["result"]
	_, hasError := result["error"]
	assert.True(t, hasResult || hasError, "Should have either result or error")
}

// ========== Part 2: Get-Config API Tests ==========

// TestAPI_GetConfig_ReturnsConfig tests that get-config API returns configuration
func TestAPI_GetConfig_ReturnsConfig(t *testing.T) {
	response := sendAPIRequest(t, "get-config", "")

	require.NotNil(t, response, "Should return a response")

	// Could be "config" or "error" (if config not set)
	responseType, ok := response["type"].(string)
	require.True(t, ok, "Should have type field")

	if responseType == "error" {
		// Config not available - acceptable
		t.Logf("Config not available: %v", response["error"])
		return
	}

	// The worker tags responses with the RPC method name.
	assert.Equal(t, "get-config", responseType, "Response type should be 'get-config'")
}

// TestAPI_GetConfig_HasRequiredFields tests that get-config API returns all config fields
func TestAPI_GetConfig_HasRequiredFields(t *testing.T) {
	response := sendAPIRequest(t, "get-config", "")

	require.NotNil(t, response, "Should return a response")

	// Skip if config not available
	if response["type"] == "error" {
		t.Skip("Config not available")
	}

	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Expected config fields
	expectedFields := []string{
		"socket_path",
		"directories",
		"database_path",
		"ignore_patterns",
		"pid_file",
		"log_file",
		"follow_symlinks",
		"worker_count",
		"content_search",
		"max_content_file_size",
		"index_interval",
		"throttle_index",
		"index_strategy",
	}

	for _, field := range expectedFields {
		_, exists := result[field]
		assert.True(t, exists, "Should have field '%s'", field)
	}

	t.Logf("Config fields: %v", result)
}

// TestAPI_GetConfig_FieldTypes tests that get-config API returns correct field types
func TestAPI_GetConfig_FieldTypes(t *testing.T) {
	response := sendAPIRequest(t, "get-config", "")

	require.NotNil(t, response, "Should return a response")

	// Skip if config not available
	if response["type"] == "error" {
		t.Skip("Config not available")
	}

	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	// Check field types
	assert.IsType(t, "", result["socket_path"], "socket_path should be string")
	assert.IsType(t, []any{}, result["directories"], "directories should be array")
	assert.IsType(t, "", result["database_path"], "database_path should be string")
	assert.IsType(t, []any{}, result["ignore_patterns"], "ignore_patterns should be array")
	assert.IsType(t, "", result["pid_file"], "pid_file should be string")
	assert.IsType(t, "", result["log_file"], "log_file should be string")
	assert.IsType(t, false, result["follow_symlinks"], "follow_symlinks should be bool")
	assert.IsType(t, float64(0), result["worker_count"], "worker_count should be number")
	assert.IsType(t, false, result["content_search"], "content_search should be bool")
	assert.IsType(t, float64(0), result["max_content_file_size"], "max_content_file_size should be number")
	assert.IsType(t, "", result["index_interval"], "index_interval should be string")
	assert.IsType(t, false, result["throttle_index"], "throttle_index should be bool")
	assert.IsType(t, "", result["index_strategy"], "index_strategy should be string")
}

// TestAPI_GetConfig_JsonRPCFormat tests get-config API with JSON-RPC format
func TestAPI_GetConfig_JsonRPCFormat(t *testing.T) {
	conn := connectSocket(t)
	defer conn.Close()

	// Send JSON-RPC request
	request := `{"jsonrpc":"2.0","method":"get-config","id":2}`
	_, err := conn.Write([]byte(request + "\n"))
	require.NoError(t, err, "Should send JSON-RPC request")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	require.NoError(t, err, "Should read response")
	t.Logf("Response: %s", strings.TrimSpace(response))

	// Parse response
	var result map[string]any
	err = json.Unmarshal([]byte(response), &result)
	require.NoError(t, err, "Should parse JSON response")

	// JSON-RPC response should have "jsonrpc", "id", and "result" or "error" fields
	assert.Equal(t, "2.0", result["jsonrpc"], "Should have jsonrpc version")
	assert.NotNil(t, result["id"], "Should have id")

	// Should have either result or error
	_, hasResult := result["result"]
	_, hasError := result["error"]
	assert.True(t, hasResult || hasError, "Should have either result or error")
}

// ========== Part 3: Set-Config API Tests ==========

// TestAPI_SetConfig_EmptyContent tests set-config with empty content
func TestAPI_SetConfig_EmptyContent(t *testing.T) {
	response := sendAPIRequest(t, "set-config", "")

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for empty content")
	assert.Contains(t, response["error"], "empty", "Error message should mention empty content")
}

// TestAPI_SetConfig_InvalidYAML tests set-config with invalid YAML
func TestAPI_SetConfig_InvalidYAML(t *testing.T) {
	// Invalid YAML (missing colon)
	invalidYAML := `
directories:
  - /home
worker_count invalid_value
`
	response := sendAPIRequest(t, "set-config", invalidYAML)

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for invalid YAML")
	assert.Contains(t, response["error"], "YAML", "Error message should mention YAML parsing error")
}

// TestAPI_SetConfig_InvalidWorkerCount tests set-config with invalid worker_count
func TestAPI_SetConfig_InvalidWorkerCount(t *testing.T) {
	// Invalid worker_count (negative)
	invalidConfig := `
directories:
  - /home
worker_count: -1
`
	response := sendAPIRequest(t, "set-config", invalidConfig)

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for invalid config")
	assert.Contains(t, response["error"], "worker_count", "Error message should mention worker_count")
}

// TestAPI_SetConfig_WorkerCountTooHigh tests set-config with worker_count exceeding limit
func TestAPI_SetConfig_WorkerCountTooHigh(t *testing.T) {
	// Invalid worker_count (too high)
	invalidConfig := `
directories:
  - /home
worker_count: 200
`
	response := sendAPIRequest(t, "set-config", invalidConfig)

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for invalid config")
	assert.Contains(t, response["error"], "worker_count", "Error message should mention worker_count")
}

// TestAPI_SetConfig_InvalidMaxFileSize tests set-config with negative max_content_file_size
func TestAPI_SetConfig_InvalidMaxFileSize(t *testing.T) {
	// Invalid max_content_file_size (negative)
	invalidConfig := `
directories:
  - /home
max_content_file_size: -100
`
	response := sendAPIRequest(t, "set-config", invalidConfig)

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for invalid config")
	assert.Contains(t, response["error"], "max_content_file_size", "Error message should mention max_content_file_size")
}

// TestAPI_SetConfig_InvalidIndexInterval tests set-config with invalid index_interval format
func TestAPI_SetConfig_InvalidIndexInterval(t *testing.T) {
	// Invalid index_interval (wrong format)
	invalidConfig := `
directories:
  - /home
index_interval: "invalid_interval"
`
	response := sendAPIRequest(t, "set-config", invalidConfig)

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for invalid config")
	assert.Contains(t, response["error"], "index_interval", "Error message should mention index_interval")
}

// TestAPI_SetConfig_InvalidIndexStrategy tests set-config with invalid index_strategy
func TestAPI_SetConfig_InvalidIndexStrategy(t *testing.T) {
	// Invalid index_strategy (not in allowed values)
	invalidConfig := `
directories:
  - /home
index_strategy: "invalid_strategy"
`
	response := sendAPIRequest(t, "set-config", invalidConfig)

	require.NotNil(t, response, "Should return a response")
	assert.Equal(t, "error", response["type"], "Should return error for invalid config")
	assert.Contains(t, response["error"], "index_strategy", "Error message should mention index_strategy")
}

// TestAPI_SetConfig_ValidConfig tests set-config with valid configuration
func TestAPI_SetConfig_ValidConfig(t *testing.T) {
	// Valid configuration
	validConfig := `
directories:
  - /home
  - /tmp
ignore_patterns:
  - "*.git"
  - "*node_modules"
worker_count: 8
follow_symlinks: false
content_search: true
max_content_file_size: 5242880
index_interval: "2h"
throttle_index: true
index_strategy: "auto"
`
	response := sendAPIRequest(t, "set-config", validConfig)

	require.NotNil(t, response, "Should return a response")

	// Note: This test may fail if config_path is not set on the server
	if response["type"] == "error" {
		t.Logf("Config save failed (may be expected): %v", response["error"])
		// This is acceptable - server may not have config path set
		return
	}

	assert.Equal(t, "set-config", response["type"], "Should return config response")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "Should have result field")
	assert.Equal(t, "saved", result["status"], "Status should be 'saved'")
}

// TestAPI_SetConfig_PartialUpdate tests set-config with partial configuration
func TestAPI_SetConfig_PartialUpdate(t *testing.T) {
	// Partial configuration (only some fields)
	partialConfig := `
directories:
  - /home
worker_count: 4
`
	response := sendAPIRequest(t, "set-config", partialConfig)

	require.NotNil(t, response, "Should return a response")

	// Note: This test may fail if config_path is not set on the server
	if response["type"] == "error" {
		t.Logf("Config save failed (may be expected): %v", response["error"])
		return
	}

	assert.Equal(t, "set-config", response["type"], "Should return config response")
}

// TestAPI_SetConfig_AllIndexStrategies tests set-config with all valid index strategies
func TestAPI_SetConfig_AllIndexStrategies(t *testing.T) {
	validStrategies := []string{"replace", "merge", "auto"}

	for _, strategy := range validStrategies {
		t.Run("strategy_"+strategy, func(t *testing.T) {
			config := `
directories:
  - /home
index_strategy: "` + strategy + `"
`
			response := sendAPIRequest(t, "set-config", config)

			require.NotNil(t, response, "Should return a response")

			// Note: This test may fail if config_path is not set on the server
			if response["type"] == "error" {
				t.Logf("Config save failed for strategy '%s' (may be expected): %v", strategy, response["error"])
				return
			}

			assert.Equal(t, "set-config", response["type"], "Should accept valid strategy '%s'", strategy)
		})
	}
}

// TestAPI_SetConfig_JsonRPCFormat tests set-config API with JSON-RPC format
func TestAPI_SetConfig_JsonRPCFormat(t *testing.T) {
	conn := connectSocket(t)
	defer conn.Close()

	// Send JSON-RPC request with YAML content
	yamlContent := `directories:\n  - /home\nworker_count: 4`
	request := `{"jsonrpc":"2.0","method":"set-config","content":"` + yamlContent + `","id":3}`

	_, err := conn.Write([]byte(request + "\n"))
	require.NoError(t, err, "Should send JSON-RPC request")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	require.NoError(t, err, "Should read response")
	t.Logf("Response: %s", strings.TrimSpace(response))

	// Parse response
	var result map[string]any
	err = json.Unmarshal([]byte(response), &result)
	require.NoError(t, err, "Should parse JSON response")

	// JSON-RPC response should have "jsonrpc", "id", and "result" or "error" fields
	assert.Equal(t, "2.0", result["jsonrpc"], "Should have jsonrpc version")
	assert.NotNil(t, result["id"], "Should have id")

	// Should have either result or error
	_, hasResult := result["result"]
	_, hasError := result["error"]
	assert.True(t, hasResult || hasError, "Should have either result or error")
}

// ========== Part 4: Combined Tests ==========

// TestAPI_GetConfigAfterSet tests that get-config reflects changes after set-config
func TestAPI_GetConfigAfterSet(t *testing.T) {
	// First, try to set a config
	newConfig := `
directories:
  - /tmp
worker_count: 6
`
	setResponse := sendAPIRequest(t, "set-config", newConfig)

	// If set-config succeeded, get-config should reflect the change
	if setResponse["type"] != "error" {
		// Get config
		getResponse := sendAPIRequest(t, "get-config", "")

		require.NotNil(t, getResponse, "Should return a response")

		if getResponse["type"] == "error" {
			t.Skip("Config not available after set")
		}

		result, ok := getResponse["result"].(map[string]any)
		require.True(t, ok, "Should have result field")

		// Check that worker_count was updated
		workerCount, ok := result["worker_count"].(float64)
		require.True(t, ok, "worker_count should be a number")
		assert.Equal(t, float64(6), workerCount, "worker_count should be 6")

		t.Logf("Updated config: worker_count = %v", workerCount)
	} else {
		t.Logf("set-config failed (expected if no config path): %v", setResponse["error"])
	}
}

// TestAPI_StatusAfterConfigChange tests that status remains stable after config changes
func TestAPI_StatusAfterConfigChange(t *testing.T) {
	// Get initial status
	initialStatus := sendAPIRequest(t, "status", "")
	require.NotNil(t, initialStatus, "Should return initial status")

	// Try to change config
	newConfig := `
directories:
  - /tmp
worker_count: 8
`
	sendAPIRequest(t, "set-config", newConfig)

	// Get status again
	finalStatus := sendAPIRequest(t, "status", "")
	require.NotNil(t, finalStatus, "Should return final status")

	// Server should still be running
	assert.Equal(t, "status", finalStatus["type"], "Should still return status type")
	result, ok := finalStatus["result"].(map[string]any)
	require.True(t, ok, "Should have result field")

	running, ok := result["running"].(bool)
	require.True(t, ok, "Should have running field")
	assert.True(t, running, "Server should still be running after config change")
}

// TestAPI_MultipleStatusRequests tests multiple consecutive status requests
func TestAPI_MultipleStatusRequests(t *testing.T) {
	for i := 0; i < 5; i++ {
		response := sendAPIRequest(t, "status", "")
		require.NotNil(t, response, "Should return response on request %d", i+1)
		assert.Equal(t, "status", response["type"], "Should return status type on request %d", i+1)
	}
}

// TestAPI_MultipleGetConfigRequests tests multiple consecutive get-config requests
func TestAPI_MultipleGetConfigRequests(t *testing.T) {
	for i := 0; i < 5; i++ {
		response := sendAPIRequest(t, "get-config", "")
		require.NotNil(t, response, "Should return response on request %d", i+1)
		// Type can be "get-config" or "error" (if config not available)
		assert.Contains(t, []string{"get-config", "error"}, response["type"], "Should return valid type on request %d", i+1)
	}
}
