package test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/internal/client"
	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/internal/testutil"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"

	"github.com/stretchr/testify/require"
)

var testServer *server.Server

// repoRoot returns the golocate repository root (parent of this test package).
func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(thisFile))
}

// TestMain sets up the test server before all tests and tears it down after
func TestMain(m *testing.M) {
	// Build index from the repository root (this package lives in test/,
	// so the working directory is not the repo root).
	// This ensures search tests have data to work with.
	ctx := context.Background()
	builder := index.NewBuilder(index.BuilderOptions{
		WorkerCount: 1,
		// Exclude VCS metadata and Go build caches so content-search tests
		// only scan real project files (and stay fast/deterministic).
		IgnorePatterns: []string{".git", ".gocache", ".gopath"},
	})

	// Build index from the repository root
	if err := builder.Build(ctx, []string{repoRoot()}); err != nil {
		// Log warning but don't fail - tests can still run with empty index
		println("Warning: Failed to build test index:", err.Error())
	}

	// Get the built index
	idx := builder.Index()

	// Create and start test server
	testServer = server.New(idx)
	testServer.SetSocketPath(socketPath)

	// Set a temporary config path for set-config tests
	tempConfigPath := filepath.Join(os.TempDir(), "golocate-test-config.yaml")
	testServer.SetConfigPath(tempConfigPath)

	// Set the config so socket-triggered builds rebuild the test index
	// (without it runBuild would fall back to scanning "/"), and so
	// get-config tests return real configuration.
	testServer.SetConfig(&config.Config{
		Directories:    []string{repoRoot()},
		WorkerCount:    1,
		IgnorePatterns: []string{".git", ".gocache", ".gopath"},
	})

	if err := testServer.Start(); err != nil {
		panic("Failed to start test server: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Stop server
	testServer.Stop()

	os.Exit(code)
}

// getTestClient creates a test client connected to golocated.
// This function is only used in test files and should not be used in production code.
func getTestClient(t *testing.T) *client.Client {
	c := client.New()
	c.SetSocketPath(socketPath)
	return c
}

// getSocketPath returns the test socket path for the current platform.
func getSocketPath() string {
	return testutil.GetTestSocketPath("base")
}

var socketPath = getSocketPath() // Use unique socket path for testing to avoid conflict with main server

// connectSocket creates a direct connection to golocated Unix socket.
// This function is only used in test files and should not be used in production code.
func connectSocket(t *testing.T) net.Conn {
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err, "Should connect to Unix socket")

	// Set 15s timeout for read/write operations
	deadline := time.Now().Add(15 * time.Second)
	conn.SetDeadline(deadline)

	return conn
}

// sendRawBin sends raw data to socket and returns response.
// This function is only used in test files and should not be used in production code.
func sendRawBin(t *testing.T, data []byte) string {
	conn := connectSocket(t)
	defer conn.Close()

	_, err := conn.Write(data)
	require.NoError(t, err, "Should send data")

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return response
}

// sendRawMsg sends raw data to socket and returns response.
// This function is only used in test files and should not be used in production code.
func sendRawMsg(t *testing.T, data string) string {
	return sendRawBin(t, []byte(data+"\n"))
}

var sendRawData = sendRawMsg

// ========== Helper Functions for Test Validation ==========

// responseContainsError checks if response contains an error.
// This function is only used in test files and should not be used in production code.
func responseContainsError(response string) bool {
	return strings.Contains(response, `"error"`) ||
		strings.Contains(response, "error=") ||
		strings.Contains(response, `"type":"error"`)
}

// responseHasResults checks if response contains results.
// This function is only used in test files and should not be used in production code.
func responseHasResults(response string) bool {
	return strings.Contains(response, `"type":"result"`) ||
		strings.Contains(response, "count=") ||
		strings.Contains(response, `"count"`) ||
		strings.Contains(response, `"result"`) || // JSON-RPC 响应包含 result 字段
		strings.Contains(response, "result=") // Fast 协议响应包含 result= 字段
}

// ========== API Response Structures ==========

// StatusResponse represents the response from status API
type StatusResponse struct {
	Type   string         `json:"type"`
	Result map[string]any `json:"result"`
}

// ConfigResponse represents the response from get-config/set-config API
type ConfigResponse struct {
	Type   string         `json:"type"`
	Result map[string]any `json:"result"`
	Error  string         `json:"error,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// ========== Helper Functions ==========

// sendAPIRequest sends a JSON-RPC request to the server and returns the result.
// This function is only used in test files and should not be used in production code.
func sendAPIRequest(t *testing.T, method string, content string) map[string]any {
	conn := connectSocket(t)
	defer conn.Close()

	// Prepare JSON-RPC request
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      1,
		"params":  map[string]any{},
	}
	if content != "" {
		req["params"].(map[string]any)["content"] = content
	}

	// Send request
	encoder := json.NewEncoder(conn)
	err := encoder.Encode(req)
	require.NoError(t, err, "Should send request")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Logf("Failed to read response: %v", err)
		return nil
	}

	t.Logf("Response: %s", strings.TrimSpace(response))

	// Parse JSON-RPC response
	var jsonrpcResp struct {
		Jsonrpc string         `json:"jsonrpc"`
		ID      any            `json:"id"`
		Result  map[string]any `json:"result"`
		Error   any            `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &jsonrpcResp); err != nil {
		t.Logf("Failed to parse response: %v", err)
		return nil
	}

	// Check for JSON-RPC error
	if jsonrpcResp.Error != nil {
		t.Logf("JSON-RPC error: %v", jsonrpcResp.Error)
		// Convert error to string for easier testing
		var errMsg string
		if errMap, ok := jsonrpcResp.Error.(map[string]any); ok {
			if msg, ok := errMap["message"].(string); ok {
				errMsg = msg
			} else {
				errMsg = fmt.Sprintf("%v", jsonrpcResp.Error)
			}
		} else {
			errMsg = fmt.Sprintf("%v", jsonrpcResp.Error)
		}
		return map[string]any{
			"type":  "error",
			"error": errMsg,
		}
	}

	// Return response in expected format: {"type": method, "result": {...}}
	result := jsonrpcResp.Result
	if result == nil {
		result = make(map[string]any)
	}

	return map[string]any{
		"type":   method,
		"result": result,
	}
}
