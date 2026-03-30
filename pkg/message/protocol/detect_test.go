// Package protocol provides tests for protocol detection with timeout and retry support.
package protocol

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Phase 1: First Byte Detection Tests ==========

func TestDetectProtocolWithConfig_FastProtocol(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected ProtocolType
	}{
		{
			name:     "Fast protocol request",
			data:     "method=search\npath=*\n\n",
			expected: ProtocolFast,
		},
		{
			name:     "Fast protocol with content",
			data:     "method=search\ncontent=test\n\n",
			expected: ProtocolFast,
		},
		{
			name:     "Plain text (not JSON)",
			data:     "hello world",
			expected: ProtocolFast,
		},
		{
			name:     "Empty line",
			data:     "\n",
			expected: ProtocolFast,
		},
	}

	cfg := DefaultDetectionConfig()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.data))
			result, err := DetectProtocolWithConfig(reader, cfg)
			
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ProtocolType)
			assert.Equal(t, "high", result.Confidence, "Fast protocol should have high confidence")
		})
	}
}

// ========== Phase 2: JSON-RPC Detection Tests ==========

func TestDetectProtocolWithConfig_JSONRPC(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected ProtocolType
	}{
		{
			name:     "JSON-RPC request",
			data:     `{"jsonrpc":"2.0","method":"search","params":{"content":"test"},"id":1}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with path",
			data:     `{"jsonrpc":"2.0","method":"search","params":{"path":"*.go"},"id":2}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC notification (no id)",
			data:     `{"jsonrpc":"2.0","method":"status"}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON-RPC with whitespace",
			data:     `{ "jsonrpc": "2.0", "method": "search", "params": {}, "id": 1 }`,
			expected: ProtocolJSONRPC,
		},
	}

	cfg := DefaultDetectionConfig()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.data))
			result, err := DetectProtocolWithConfig(reader, cfg)
			
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ProtocolType)
		})
	}
}

// ========== Incomplete Data Tests (Core Problem) ==========

func TestDetectProtocolWithConfig_IncompleteData(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		expectedError  bool
		expected       ProtocolType
		expectedConf   string
		waitTimeout    time.Duration
		minBufferBytes int
	}{
		{
			name:           "Only opening brace - no wait",
			data:           `{`,
			expectedError:  true, // No jsonrpc field, should return error
			waitTimeout:    0,    // No wait
			minBufferBytes: 20,
		},
		{
			name:           "Only two bytes - with wait timeout",
			data:           `{"`,
			expectedError:  true, // No jsonrpc field, should return error
			waitTimeout:    10 * time.Millisecond, // Short timeout for test
			minBufferBytes: 20,
		},
		{
			name:           "Partial JSON without jsonrpc field",
			data:           `{"method":"search","con`,
			expectedError:  true, // No jsonrpc field, should return error
			waitTimeout:    0,
			minBufferBytes: 5, // Lower threshold
		},
		{
			name:           "Partial JSON-RPC with jsonrpc field visible",
			data:           `{"jsonrpc":"2.0","method":"sea`,
			expected:       ProtocolJSONRPC, // Should detect jsonrpc field
			expectedConf:   "high",
			waitTimeout:    0,
			minBufferBytes: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &DetectionConfig{
				WaitTimeout:    tt.waitTimeout,
				MinBufferBytes: tt.minBufferBytes,
				EnableRetry:    true,
			}
			
			reader := bufio.NewReader(strings.NewReader(tt.data))
			result, err := DetectProtocolWithConfig(reader, cfg)
			
			if tt.expectedError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported protocol")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result.ProtocolType)
				assert.Equal(t, tt.expectedConf, result.Confidence)
			}
		})
	}
}

// ========== Configuration Tests ==========

func TestDetectionConfig_Defaults(t *testing.T) {
	cfg := DefaultDetectionConfig()
	
	assert.Equal(t, 100*time.Millisecond, cfg.WaitTimeout)
	assert.Equal(t, 20, cfg.MinBufferBytes)
	assert.True(t, cfg.EnableRetry)
}

func TestDetectProtocolWithConfig_NoWait(t *testing.T) {
	// Test with waiting disabled
	cfg := &DetectionConfig{
		WaitTimeout:    0,
		MinBufferBytes: 20,
		EnableRetry:    true,
	}
	
	// Only two bytes, no jsonrpc field
	reader := bufio.NewReader(strings.NewReader(`{"`))
	result, err := DetectProtocolWithConfig(reader, cfg)
	
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol")
	assert.Nil(t, result)
}

// ========== Utility Function Tests ==========

func TestIsCompleteJSONObject(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{
			name:     "Complete simple object",
			data:     `{}`,
			expected: true,
		},
		{
			name:     "Complete object with fields",
			data:     `{"method":"search"}`,
			expected: true,
		},
		{
			name:     "Complete nested object",
			data:     `{"method":"search","params":{"path":"*.go"}}`,
			expected: true,
		},
		{
			name:     "Incomplete object - missing closing brace",
			data:     `{"method":"search"`,
			expected: false,
		},
		{
			name:     "Incomplete object - only opening",
			data:     `{`,
			expected: false,
		},
		{
			name:     "Incomplete with nested object",
			data:     `{"method":"search","params":{`,
			expected: false,
		},
		{
			name:     "Empty string",
			data:     ``,
			expected: false,
		},
		{
			name:     "Not JSON",
			data:     `hello`,
			expected: false,
		},
		{
			name:     "JSON with string containing braces",
			data:     `{"content":"{not an object}"}`,
			expected: true,
		},
		{
			name:     "JSON with escaped quotes",
			data:     `{"content":"he said \"hello\""}`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCompleteJSONObject([]byte(tt.data))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectProtocolTypeFromData(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected ProtocolType
	}{
		{
			name:     "JSON-RPC with jsonrpc field",
			data:     `{"jsonrpc":"2.0","method":"search"}`,
			expected: ProtocolJSONRPC,
		},
		{
			name:     "JSON without jsonrpc field",
			data:     `{"method":"search","content":"test"}`,
			expected: ProtocolFast, // Returns fast as fallback
		},
		{
			name:     "Fast protocol",
			data:     `method=search`,
			expected: ProtocolFast,
		},
		{
			name:     "Empty data",
			data:     ``,
			expected: ProtocolFast,
		},
		{
			name:     "Invalid JSON starting with brace",
			data:     `{invalid json}`,
			expected: ProtocolFast, // Returns fast as fallback
		},
		{
			name:     "JSON-RPC with extra whitespace",
			data:     `{ "jsonrpc" : "2.0" }`,
			expected: ProtocolJSONRPC,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectProtocolTypeFromData([]byte(tt.data))
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ========== Integration Tests ==========

func TestDetectAndParse_Complete(t *testing.T) {
	tests := []struct {
		name             string
		data             string
		expectedProtocol ProtocolType
		expectedMethod   string
	}{
		{
			name:             "JSON-RPC request",
			data:             `{"jsonrpc":"2.0","method":"search","params":{"content":"test"},"id":1}`,
			expectedProtocol: ProtocolJSONRPC,
			expectedMethod:   "search",
		},
		{
			name:             "Fast protocol request",
			data:             "method=search\npath=*\ncontent=test\n\n",
			expectedProtocol: ProtocolFast,
			expectedMethod:   "search",
		},
	}

	cfg := DefaultDetectionConfig()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.data))
			req, proto, result, err := DetectAndParse(reader, cfg)
			
			require.NoError(t, err)
			assert.Equal(t, tt.expectedProtocol, proto)
			assert.Equal(t, tt.expectedMethod, req.Method)
			assert.NotNil(t, result)
		})
	}
}

// ========== Benchmark Tests ==========

func BenchmarkDetectProtocolWithConfig_JSON(b *testing.B) {
	data := `{"method":"search","content":"test","path":"*.go","limit":100}`
	cfg := DefaultDetectionConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(data))
		_, _ = DetectProtocolWithConfig(reader, cfg)
	}
}

func BenchmarkDetectProtocolWithConfig_JSONRPC(b *testing.B) {
	data := `{"jsonrpc":"2.0","method":"search","params":{"content":"test"},"id":1}`
	cfg := DefaultDetectionConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(data))
		_, _ = DetectProtocolWithConfig(reader, cfg)
	}
}

func BenchmarkDetectProtocolWithConfig_Fast(b *testing.B) {
	data := "method=search\npath=*\ncontent=test\n\n"
	cfg := DefaultDetectionConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(data))
		_, _ = DetectProtocolWithConfig(reader, cfg)
	}
}

// ========== Edge Case Tests ==========

func TestDetectProtocolWithConfig_LargeJSON(t *testing.T) {
	// Create a large JSON object without jsonrpc field
	var buf bytes.Buffer
	buf.WriteString(`{"method":"search","content":"`)
	buf.WriteString(strings.Repeat("a", 10000))
	buf.WriteString(`","limit":10}`)
	
	data := buf.String()
	cfg := DefaultDetectionConfig()
	
	reader := bufio.NewReader(strings.NewReader(data))
	result, err := DetectProtocolWithConfig(reader, cfg)
	
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol")
	assert.Nil(t, result)
}

func TestDetectProtocolWithConfig_JSONRPCWithLargeContent(t *testing.T) {
	// JSON-RPC with jsonrpc field at the beginning, large params
	var buf bytes.Buffer
	buf.WriteString(`{"jsonrpc":"2.0","method":"search","params":{"content":"`)
	buf.WriteString(strings.Repeat("a", 10000))
	buf.WriteString(`"},"id":1}`)
	
	data := buf.String()
	cfg := DefaultDetectionConfig()
	
	reader := bufio.NewReader(strings.NewReader(data))
	result, err := DetectProtocolWithConfig(reader, cfg)
	
	require.NoError(t, err)
	assert.Equal(t, ProtocolJSONRPC, result.ProtocolType)
	assert.Equal(t, "high", result.Confidence)
}

func TestDetectProtocolWithConfig_JSONRPCWithJSONRPCAtEnd(t *testing.T) {
	// JSON-RPC with jsonrpc field at the end (unusual but valid)
	data := `{"method":"search","params":{"content":"test"},"jsonrpc":"2.0","id":1}`
	cfg := DefaultDetectionConfig()
	
	reader := bufio.NewReader(strings.NewReader(data))
	result, err := DetectProtocolWithConfig(reader, cfg)
	
	require.NoError(t, err)
	assert.Equal(t, ProtocolJSONRPC, result.ProtocolType, "Should detect JSON-RPC even with jsonrpc field at end")
}

func TestDetectProtocolWithConfig_InvalidJSON(t *testing.T) {
	// Invalid JSON (missing closing brace) without jsonrpc field
	data := `{"method":"search","content":"test"`
	cfg := DefaultDetectionConfig()
	
	reader := bufio.NewReader(strings.NewReader(data))
	result, err := DetectProtocolWithConfig(reader, cfg)
	
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol")
	assert.Nil(t, result)
}
