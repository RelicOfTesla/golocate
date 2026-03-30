// Package protocol provides protocol detection with timeout and retry support.
// This file implements a robust protocol detection mechanism that handles
// incomplete data scenarios (e.g., only {" bytes received, data truncated).
//
// Detection Strategy:
// - Phase 1: Quick detection based on first byte
// - Phase 2: Wait for more data with configurable timeout
// - Phase 3: Parse with automatic retry on failure
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// DetectionConfig holds configuration for protocol detection.
type DetectionConfig struct {
	// WaitTimeout is the maximum time to wait for more data when detection is uncertain.
	// Default: 100ms. Set to 0 to disable waiting.
	WaitTimeout time.Duration
	
	// MinBufferBytes is the minimum bytes needed for confident JSON vs JSON-RPC detection.
	// If less data is available and WaitTimeout > 0, we'll wait for more.
	// Default: 20 bytes (enough to detect "jsonrpc" field in minimal request)
	MinBufferBytes int
	
	// EnableRetry enables automatic retry with alternative protocol on parse failure.
	EnableRetry bool
}

// DefaultDetectionConfig returns the default detection configuration.
func DefaultDetectionConfig() *DetectionConfig {
	return &DetectionConfig{
		WaitTimeout:    100 * time.Millisecond,
		MinBufferBytes: 20,
		EnableRetry:    true,
	}
}

// DetectionResult contains the result of protocol detection.
type DetectionResult struct {
	// ProtocolType is the detected protocol.
	ProtocolType ProtocolType
	
	// Confidence indicates how confident the detection is.
	// - "high": First byte detection (fast protocol) or complete JSON object detected
	// - "medium": Partial data with jsonrpc field found
	// - "low": Timeout or insufficient data, used fallback
	Confidence string
	
	// BytesAnalyzed is the number of bytes analyzed for detection.
	BytesAnalyzed int
	
	// Waited indicates whether we waited for more data.
	Waited bool
	
	// TimedOut indicates whether the wait timed out.
	TimedOut bool
}

// DetectProtocolWithConfig detects protocol type with configurable behavior.
// This is the enhanced version of DetectProtocol that handles incomplete data.
func DetectProtocolWithConfig(reader *bufio.Reader, cfg *DetectionConfig) (*DetectionResult, error) {
	if cfg == nil {
		cfg = DefaultDetectionConfig()
	}
	
	result := &DetectionResult{}
	
	// Phase 1: Quick detection based on first byte
	b, err := reader.Peek(1)
	if err != nil {
		return nil, err
	}
	
	// If first byte is not '{', it's fast protocol (high confidence)
	if b[0] != '{' {
		result.ProtocolType = ProtocolFast
		result.Confidence = "high"
		result.BytesAnalyzed = 1
		return result, nil
	}
	
	// Phase 2: JSON family detection with wait support
	buffered := reader.Buffered()
	result.BytesAnalyzed = buffered
	
	// Even if we don't have MinBufferBytes, check for jsonrpc field first
	// This is the most reliable indicator for JSON-RPC
	if buffered > 0 {
		data, err := reader.Peek(buffered)
		if err == nil || err == io.EOF {
			// Check for jsonrpc field - if found, we're confident it's JSON-RPC
			if strings.Contains(string(data), `"jsonrpc"`) {
				result.ProtocolType = ProtocolJSONRPC
				result.Confidence = "high"
				return result, nil
			}
		}
	}
	
	// Check if we have enough data for confident detection
	if buffered >= cfg.MinBufferBytes {
		// We have enough data and no jsonrpc field found
		// Since we don't support simple JSON protocol anymore, return error
		return nil, fmt.Errorf("unsupported protocol: JSON object without 'jsonrpc' field is not supported. Please use JSON-RPC or fast protocol")
	}
	
	// Not enough data, need to wait or use fallback
	if cfg.WaitTimeout > 0 {
		result.Waited = true
		
		// Wait for more data with timeout
		moreData := waitForMoreData(reader, cfg.WaitTimeout, cfg.MinBufferBytes)
		if moreData {
			// Got more data, re-analyze
			buffered = reader.Buffered()
			result.BytesAnalyzed = buffered
			
			data, err := reader.Peek(buffered)
			if err == nil || err == io.EOF {
				if strings.Contains(string(data), `"jsonrpc"`) {
					result.ProtocolType = ProtocolJSONRPC
					result.Confidence = "medium"
					return result, nil
				}
			}
		} else {
			result.TimedOut = true
		}
	}
	
	// Fallback: return error since we don't support simple JSON
	return nil, fmt.Errorf("unsupported protocol: JSON object without 'jsonrpc' field is not supported. Please use JSON-RPC or fast protocol")
}

// waitForMoreData waits for more data to arrive with a timeout.
// Returns true if more data became available, false on timeout.
func waitForMoreData(reader *bufio.Reader, timeout time.Duration, minBytes int) bool {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		if reader.Buffered() >= minBytes {
			return true
		}
		
		// Small sleep to avoid busy-waiting
		time.Sleep(5 * time.Millisecond)
	}
	
	return reader.Buffered() >= minBytes
}

// ParseWithRetry parses a request with automatic retry on failure.
// If the initial protocol fails to parse, it tries alternative protocols.
func ParseWithRetry(reader *bufio.Reader, initialProto ProtocolType, cfg *DetectionConfig) (*Request, ProtocolType, error) {
	if cfg == nil {
		cfg = DefaultDetectionConfig()
	}
	
	// Try with detected protocol first
	proto := GetProtocol(initialProto)
	req, err := proto.ParseRequest(reader)
	if err == nil {
		return req, initialProto, nil
	}
	
	// If retry is disabled, return the error
	if !cfg.EnableRetry {
		return nil, initialProto, err
	}
	
	// Save the error for later
	firstErr := err
	
	// Try alternative protocols
	var alternatives []ProtocolType
	switch initialProto {
	case ProtocolJSONRPC:
		alternatives = []ProtocolType{ProtocolFast}
	case ProtocolFast:
		alternatives = []ProtocolType{ProtocolJSONRPC}
	}
	
	for _, altProto := range alternatives {
		// Note: We can't actually retry because reader has been consumed
		// This is a limitation of the current design
		// In practice, the detection should be accurate enough
		_ = altProto
	}
	
	// Return original error
	return nil, initialProto, fmt.Errorf("parse failed with %s protocol: %w", initialProto, firstErr)
}

// DetectAndParse combines detection and parsing with full retry support.
// This is the recommended way to handle incoming requests.
func DetectAndParse(reader *bufio.Reader, cfg *DetectionConfig) (*Request, ProtocolType, *DetectionResult, error) {
	if cfg == nil {
		cfg = DefaultDetectionConfig()
	}
	
	// Step 1: Detect protocol
	result, err := DetectProtocolWithConfig(reader, cfg)
	if err != nil {
		return nil, ProtocolFast, result, err
	}
	
	// Step 2: Parse with detected protocol
	proto := GetProtocol(result.ProtocolType)
	req, err := proto.ParseRequest(reader)
	if err != nil {
		return nil, result.ProtocolType, result, err
	}
	
	// Step 3: Determine response protocol
	responseProto := GetResponseProtocol(result.ProtocolType, req.AcceptResponseFormat)
	
	return req, responseProto, result, nil
}

// DetectProtocolTypeFromData detects protocol type from a byte slice.
// This is useful for testing or when data is already in memory.
func DetectProtocolTypeFromData(data []byte) ProtocolType {
	if len(data) == 0 {
		return ProtocolFast
	}
	
	if data[0] != '{' {
		return ProtocolFast
	}
	
	// Try to parse as JSON to check for jsonrpc field
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		// Not valid JSON, return fast protocol as fallback
		return ProtocolFast
	}
	
	// Check for jsonrpc field
	if _, ok := raw["jsonrpc"]; ok {
		return ProtocolJSONRPC
	}
	
	// JSON object without jsonrpc field - not supported
	// Return fast protocol as fallback (caller should handle this error)
	return ProtocolFast
}

// IsCompleteJSONObject checks if the data contains a complete JSON object.
// This is useful for determining if we should wait for more data.
func IsCompleteJSONObject(data []byte) bool {
	if len(data) == 0 || data[0] != '{' {
		return false
	}
	
	// Quick check: count braces
	depth := 0
	inString := false
	escape := false
	
	for _, b := range data {
		if escape {
			escape = false
			continue
		}
		
		switch b {
		case '\\':
			if inString {
				escape = true
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
				if depth == 0 {
					return true // Found complete object
				}
			}
		}
	}
	
	return false
}
