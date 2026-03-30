// Package protocol provides protocol abstraction for golocate.
package protocol

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

// TestDetectProtocol_FastProtocol tests fast protocol detection
func TestDetectProtocol_FastProtocol(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ProtocolType
	}{
		{
			name:     "simple text command",
			input:    "test\n",
			expected: ProtocolFast,
		},
		{
			name:     "search command",
			input:    "search\n",
			expected: ProtocolFast,
		},
		{
			name:     "text without newline",
			input:    "hello world",
			expected: ProtocolFast,
		},
		{
			name:     "single character",
			input:    "a",
			expected: ProtocolFast,
		},
		{
			name:     "whitespace",
			input:    "   test\n",
			expected: ProtocolFast,
		},
		{
			name:     "number",
			input:    "123\n",
			expected: ProtocolFast,
		},
		{
			name:     "special characters",
			input:    "!@#$%\n",
			expected: ProtocolFast,
		},
		{
			name:     "newlines",
			input:    "\n\n\n",
			expected: ProtocolFast,
		},
		{
			name:     "tab character",
			input:    "\ttest\n",
			expected: ProtocolFast,
		},
		{
			name:     "empty string after brace",
			input:    "}",
			expected: ProtocolFast,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			proto, err := DetectProtocol(reader)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if proto != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, proto)
			}
		})
	}
}

// TestDetectProtocol_JSONProtocol tests JSON protocol detection
func TestDetectProtocol_JSONRPCProtocol(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    ProtocolType
		expectError bool
	}{
		// ===== Standard JSON-RPC 2.0 tests =====
		{
			name:     "basic JSON-RPC request",
			input:    `{"jsonrpc":"2.0","id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with method",
			input:    `{"jsonrpc":"2.0","method":"search","id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with params object",
			input:    `{"jsonrpc":"2.0","method":"search","params":{"query":"test"},"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with params array",
			input:    `{"jsonrpc":"2.0","method":"search","params":["query"],"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC without newline",
			input:    `{"jsonrpc":"2.0","id":1}`,
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC with whitespace =====
		{
			name:     "JSON-RPC with spaces around values",
			input:    `{ "jsonrpc": "2.0", "id": 1 }` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with whitespace before jsonrpc",
			input:    `{  "jsonrpc":"2.0"}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with newlines",
			input:    "{\n\"jsonrpc\":\"2.0\",\n\"id\":1\n}" + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with tabs",
			input:    "{\t\"jsonrpc\":\"2.0\",\t\"id\":1}" + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC notification (no id) =====
		{
			name:     "JSON-RPC notification without id",
			input:    `{"jsonrpc":"2.0","method":"update"}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC notification with params",
			input:    `{"jsonrpc":"2.0","method":"update","params":{"count":5}}` + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC field order variations =====
		{
			name:     "JSON-RPC with id first",
			input:    `{"id":1,"jsonrpc":"2.0","method":"search"}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with method first",
			input:    `{"method":"search","jsonrpc":"2.0","id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with params first",
			input:    `{"params":{},"jsonrpc":"2.0","method":"search","id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC batch requests =====
		{
			name:     "JSON-RPC batch request array",
			input:    `[{"jsonrpc":"2.0","method":"search","id":1}]` + "\n",
			expected: ProtocolFast, // Batch starts with '[', not '{'
		},
		{
			name:     "JSON-RPC batch with multiple requests",
			input:    `[{"jsonrpc":"2.0","method":"search","id":1},{"jsonrpc":"2.0","method":"find","id":2}]` + "\n",
			expected: ProtocolFast, // Batch starts with '[', not '{'
		},

		// ===== JSON-RPC version variations =====
		{
			name:     "JSON-RPC with version 1.0",
			input:    `{"jsonrpc":"1.0","id":1}` + "\n",
			expected: ProtocolJSONRPC, // Has "jsonrpc" field
		},
		{
			name:     "JSON-RPC with version 2.0",
			input:    `{"jsonrpc":"2.0","id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with custom version string",
			input:    `{"jsonrpc":"2.1","id":1}` + "\n",
			expected: ProtocolJSONRPC, // Has "jsonrpc" field
		},

		// ===== JSON-RPC with different id types =====
		{
			name:     "JSON-RPC with string id",
			input:    `{"jsonrpc":"2.0","id":"abc123"}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with numeric id",
			input:    `{"jsonrpc":"2.0","id":12345}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with negative id",
			input:    `{"jsonrpc":"2.0","id":-1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with null id",
			input:    `{"jsonrpc":"2.0","id":null}` + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC with complex params =====
		{
			name:     "JSON-RPC with nested params",
			input:    `{"jsonrpc":"2.0","method":"search","params":{"filter":{"path":"/test"}},"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with array params",
			input:    `{"jsonrpc":"2.0","method":"search","params":["test",1,true],"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with empty params object",
			input:    `{"jsonrpc":"2.0","method":"search","params":{},"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with empty params array",
			input:    `{"jsonrpc":"2.0","method":"search","params":[],"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC with special characters =====
		{
			name:     "JSON-RPC with unicode in method name",
			input:    `{"jsonrpc":"2.0","method":"搜索","id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with special characters in params",
			input:    `{"jsonrpc":"2.0","method":"search","params":{"query":"test\"with\"quotes"},"id":1}` + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== JSON-RPC malformed =====
		{
			name:     "JSON-RPC missing closing brace",
			input:    `{"jsonrpc":"2.0","id":1` + "\n",
			expected: ProtocolJSONRPC,
		},
		{
			name:        "JSON-RPC missing quotes around jsonrpc (invalid JSON, should return error)",
			input:       `{jsonrpc:"2.0","id":1}` + "\n",
			expected:    ProtocolFast,
			expectError: true, // No quotes around jsonrpc, so string search won't find "jsonrpc", should return error
		},
		{
			name:     "JSON-RPC-like string in value (detected as JSON-RPC due to simple string match)",
			input:    `{"method":"jsonrpc"}` + "\n",
			expected: ProtocolJSONRPC, // Simple string match finds "jsonrpc" in value
		},
		{
			name:     "JSON-RPC field in nested object (detected as JSON-RPC due to simple string match)",
			input:    `{"data":{"jsonrpc":"fake"}}` + "\n",
			expected: ProtocolJSONRPC, // Simple string match finds "jsonrpc" in nested object
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			proto, err := DetectProtocol(reader)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if proto != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, proto)
			}
		})
	}
}

// TestDetectProtocol_EdgeCases tests edge cases
func TestDetectProtocol_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    ProtocolType
		expectError bool
	}{
		// ===== Empty and minimal input =====
		{
			name:        "empty input",
			input:       "",
			expected:    ProtocolFast,
			expectError: true, // Peek on empty reader returns EOF
		},
		{
			name:        "single brace",
			input:       "{",
			expected:    ProtocolFast,
			expectError: true, // Incomplete JSON without jsonrpc field
		},
		{
			name:        "brace with newline",
			input:       "{\n",
			expected:    ProtocolFast,
			expectError: true, // Incomplete JSON without jsonrpc field
		},
		{
			name:        "partial JSON - only opening brace",
			input:       "{ ",
			expected:    ProtocolFast,
			expectError: true, // Incomplete JSON without jsonrpc field
		},
		{
			name:        "partial JSON - unclosed string",
			input:       `{"test`,
			expected:    ProtocolFast,
			expectError: true, // Incomplete JSON without jsonrpc field
		},
		{
			name:        "partial JSON - unclosed object",
			input:       `{"test":`,
			expected:    ProtocolFast,
			expectError: true, // Incomplete JSON without jsonrpc field
		},

		// ===== JSON/JSON-RPC boundary =====
		// Note: Current implementation uses simple string match for "jsonrpc"
		// With 4096-byte peek limit, we can detect jsonrpc field even if it appears after long content
		{
			name:     "JSON-RPC with jsonrpc at 128 byte boundary",
			input:    `{"` + strings.Repeat("a", 120) + `":"x","jsonrpc":"2.0"}` + "\n",
			expected: ProtocolJSONRPC, // jsonrpc detected within 4096-byte peek limit
		},
		{
			name:     "JSON-RPC with jsonrpc within old 128-byte limit",
			input:    `{"` + strings.Repeat("a", 110) + `":"x","jsonrpc":"2.0"}` + "\n",
			expected: ProtocolJSONRPC, // jsonrpc starts at byte 112, well within limit
		},
		{
			name:     "JSON-RPC with jsonrpc at 200 bytes",
			input:    `{"` + strings.Repeat("a", 200) + `":"x","jsonrpc":"2.0"}` + "\n",
			expected: ProtocolJSONRPC, // jsonrpc detected within 4096-byte peek limit
		},
		{
			name:     "JSON-RPC-like string in value (simple match detects it)",
			input:    `{"method":"jsonrpc"}` + "\n",
			expected: ProtocolJSONRPC, // Simple string match finds "jsonrpc"
		},
		{
			name:     "JSON-RPC field in nested object (simple match detects it)",
			input:    `{"data":{"jsonrpc":"fake"}}` + "\n",
			expected: ProtocolJSONRPC, // Simple string match finds "jsonrpc"
		},

		// ===== Multiple braces =====
		{
			name:        "multiple braces",
			input:       "{{}}",
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "brace followed by text",
			input:       "{not valid json",
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},

		// ===== Special characters =====
		{
			name:        "binary data after brace",
			input:       "{\x00\x01\x02\x03",
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "unicode after brace",
			input:       `{中文测试}`,
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},

		// ===== Quote variations =====
		{
			name:        "JSON with single quotes (invalid JSON but starts with brace)",
			input:       `{'key':'value'}` + "\n",
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},

		// ===== Multiple objects =====
		{
			name:        "multiple JSON objects concatenated",
			input:       `{"a":1}{"b":2}` + "\n",
			expected:    ProtocolFast,
			expectError: true, // Multiple JSON objects without jsonrpc field
		},

		// ===== Leading characters =====
		{
			name:     "leading space before brace",
			input:    ` {"key":"value"}` + "\n",
			expected: ProtocolFast, // Leading space makes it fast protocol
		},
		{
			name:     "leading newline before brace",
			input:    "\n{\"key\":\"value\"}\n",
			expected: ProtocolFast, // Leading newline makes it fast protocol
		},
		{
			name:     "leading tab before brace",
			input:    "\t{\"key\":\"value\"}\n",
			expected: ProtocolFast, // Leading tab makes it fast protocol
		},

		// ===== JSON-RPC special cases =====
		{
			name:     "JSON-RPC with version as integer",
			input:    `{"jsonrpc":2,"id":1}` + "\n",
			expected: ProtocolJSONRPC, // Has "jsonrpc" field
		},
		{
			name:     "JSON-RPC with version as number",
			input:    `{"jsonrpc":2.0,"id":1}` + "\n",
			expected: ProtocolJSONRPC, // Has "jsonrpc" field
		},
		{
			name:     "JSON-RPC with null version",
			input:    `{"jsonrpc":null,"id":1}` + "\n",
			expected: ProtocolJSONRPC, // Has "jsonrpc" field
		},

		// ===== Minimal valid structures =====
		{
			name:        "minimal JSON object",
			input:       `{}` + "\n",
			expected:    ProtocolFast,
			expectError: true, // Empty JSON object without jsonrpc field
		},
		{
			name:     "minimal JSON-RPC",
			input:    `{"jsonrpc":"2.0"}` + "\n",
			expected: ProtocolJSONRPC,
		},

		// ===== Comment-like content =====
		{
			name:     "JSON with comment-like content",
			input:    `{"jsonrpc":"2.0","method":"search","comment":"this is a comment"}` + "\n",
			expected: ProtocolJSONRPC,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			proto, err := DetectProtocol(reader)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if proto != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, proto)
			}
		})
	}
}

// TestDetectProtocol_LongInput tests with long input
func TestDetectProtocol_LongInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    ProtocolType
		bufSize     int // buffer size for bufio.Reader, 0 means default
		expectError bool
	}{
		{
			name:     "long fast protocol command",
			input:    strings.Repeat("a", 10000) + "\n",
			expected: ProtocolFast,
		},
		{
			name:        "long JSON object",
			input:       `{"data":"`+strings.Repeat("x", 10000)+`"}`,
			expected:    ProtocolFast,
			expectError: true, // JSON without jsonrpc field
		},
		{
			name:     "long JSON-RPC with large params",
			input:    `{"jsonrpc":"2.0","method":"search","params":{"data":"`+strings.Repeat("x", 10000)+`"},"id":1}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC field after 200 bytes",
			input:    `{"`+strings.Repeat("a", 200)+`":"value","jsonrpc":"2.0"}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC field at beginning",
			input:    `{"jsonrpc":"2.0","`+strings.Repeat("a", 200)+`":"value"}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC field after 3000 bytes",
			input:    `{"`+strings.Repeat("a", 3000)+`":"value","jsonrpc":"2.0"}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:        "JSON-RPC field after 5000 bytes with default buffer",
			input:       `{"`+strings.Repeat("a", 5000)+`":"value","jsonrpc":"2.0"}`,
			expected:    ProtocolFast,
			expectError: true, // default buffer (4096) can't see jsonrpc field
		},
		{
			name:     "JSON-RPC field after 5000 bytes with large buffer",
			input:    `{"`+strings.Repeat("a", 5000)+`":"value","jsonrpc":"2.0"}`,
			expected: ProtocolJSONRPC,
			bufSize:  8192, // larger buffer can hold all data
		},
		{
			name:        "JSON-RPC field after 10000 bytes with default buffer",
			input:       `{"`+strings.Repeat("a", 10000)+`":"value","jsonrpc":"2.0"}`,
			expected:    ProtocolFast,
			expectError: true, // default buffer (4096) can't see jsonrpc field
		},
		{
			name:     "JSON-RPC field after 10000 bytes with large buffer",
			input:    `{"`+strings.Repeat("a", 10000)+`":"value","jsonrpc":"2.0"}`,
			expected: ProtocolJSONRPC,
			bufSize:  16384, // larger buffer can hold all data
		},
		{
			name:     "JSON-RPC with 196kb data before jsonrpc field",
			input:    `{"data":"`+strings.Repeat("x", 196*1024)+`","jsonrpc":"2.0"}`,
			expected: ProtocolJSONRPC,
			bufSize:  256*1024, // 256KB buffer for 196KB data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader *bufio.Reader
			if tt.bufSize > 0 {
				reader = bufio.NewReaderSize(strings.NewReader(tt.input), tt.bufSize)
			} else {
				reader = bufio.NewReader(strings.NewReader(tt.input))
			}
			proto, err := DetectProtocol(reader)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if proto != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, proto)
			}
		})
	}
}

// TestDetectProtocol_MalformedJSON tests with malformed JSON
func TestDetectProtocol_MalformedJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    ProtocolType
		expectError bool
	}{
		{
			name:        "missing closing brace",
			input:       `{"method":"search"`,
			expected:    ProtocolFast,
			expectError: true, // Incomplete JSON without jsonrpc field
		},
		{
			name:        "missing opening quote",
			input:       `{method":"search"}`,
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "missing closing quote",
			input:       `{"method:"search"}`,
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "extra comma",
			input:       `{"method":"search",}`,
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "trailing characters",
			input:       `{"method":"search"}garbage`,
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "jsonrpc with typo",
			input:       `{"jsonprc":"2.0","id":1}`,
			expected:    ProtocolFast,
			expectError: true, // Typo in field name, no jsonrpc field
		},
		{
			name:        "binary data after brace",
			input:       "{\x00\x01\x02\x03",
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
		{
			name:        "unicode after brace",
			input:       `{中文测试}`,
			expected:    ProtocolFast,
			expectError: true, // Invalid JSON without jsonrpc field
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			proto, err := DetectProtocol(reader)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if proto != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, proto)
			}
		})
	}
}

// TestDetectProtocol_Concurrent tests concurrent protocol detection
func TestDetectProtocol_Concurrent(t *testing.T) {
	// Run multiple goroutines to test thread safety
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)
	errorCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			var input string
			var expected ProtocolType
			expectError := false

			switch idx % 3 {
			case 0:
				input = "test\n"
				expected = ProtocolFast
			case 1:
				input = `{"method":"search"}` + "\n"
				expected = ProtocolFast
				expectError = true // JSON without jsonrpc field should return error
			case 2:
				input = `{"jsonrpc":"2.0","id":1}` + "\n"
				expected = ProtocolJSONRPC
			}

			reader := bufio.NewReader(strings.NewReader(input))
			proto, err := DetectProtocol(reader)

			if expectError {
				if err == nil {
					errorCh <- fmt.Errorf("expected error, got nil")
					done <- false
					return
				}
			} else {
				if err != nil {
					errorCh <- err
					done <- false
					return
				}
			}

			if proto != expected {
				errorCh <- fmt.Errorf("expected %s, got %s", expected, proto)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errorCh:
		if err != nil {
			t.Errorf("concurrent test error: %v", err)
		}
	default:
		// No errors, all tests passed
	}
}

