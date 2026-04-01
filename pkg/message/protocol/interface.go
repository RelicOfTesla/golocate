// Package protocol provides protocol abstraction for golocate.
// This package defines the protocol interface and implementations for JSON, JSON-RPC, and fast protocol.
//
// Protocol Separation:
// - requestProtocol: The protocol type detected from the incoming request
// - responseProtocol: The protocol type used for sending responses
//
// Default Behavior:
// - If AcceptResponseFormat is empty, responseProtocol = requestProtocol
// - If AcceptResponseFormat is specified, responseProtocol = specified protocol
//
// Business Layer Transparency:
// - Business logic only deals with unified Request and Response structures
// - Protocol adapters handle protocol-specific details automatically
package protocol

import (
	"bufio"
)

// ============================================================================
// Field Name Constants
// ============================================================================

// Field name constants for JSON and Fast protocol field names.
// Use these constants instead of hardcoded strings to ensure consistency.
const (
	// Request field names
	FieldMethod               = "method"
	FieldID                   = "id"
	FieldJSONRPC              = "jsonrpc"
	FieldParams               = "params"
	FieldAcceptResponseFormat = "accept_response_format"
	
	// Search parameter field names
	FieldPattern     = "pattern"
	FieldPatternMode = "pattern_mode"
	FieldContent     = "content"
	FieldIgnoreCase  = "ignore_case"
	FieldBasename    = "basename"
	FieldLimit       = "limit"
	FieldOffset      = "offset"
	FieldSortField   = "sort_field"
	FieldSortOrder   = "sort_order"
	
	// Legacy/deprecated field names (for backward compatibility)
	FieldMode          = "mode"
	FieldRegex         = "regex"
	FieldExtendedRegex = "extended_regex"
	
	// Config parameter field names
	FieldConfig = "config"
	
	// Response field names
	FieldResult = "result"
	FieldError  = "error"
	FieldCount  = "count"
	FieldTotal  = "total"
	FieldPaths  = "paths"
	
	// Method names
	MethodSearch    = "search"
	MethodStatus    = "status"
	MethodGetConfig = "get-config"
	MethodSetConfig = "set-config"
)

// ============================================================================
// Base Types (Shared Fields)
// ============================================================================

// BaseRequest contains fields shared by all request types.
type BaseRequest struct {
	ID                   any    `json:"id,omitempty"`                    // Request ID for async response support
	Method               string `json:"method"`                          // The action to perform (e.g., "search", "status")
	AcceptResponseFormat string `json:"accept_response_format,omitempty"` // Desired response format: "json-rpc", "fast", or empty (same as request)
}

// ============================================================================
// Command-Specific Parameter Types
// ============================================================================

// SearchParams contains parameters specific to the search command.
type SearchParams struct {
	Pattern     string `json:"pattern"`               // Search pattern (required)
	PatternMode string `json:"pattern_mode"`          // Pattern mode: "normal", "regex", "extended_regex", "wildcard"
	Content     string `json:"content,omitempty"`     // Search file content (optional)
	IgnoreCase  bool   `json:"ignore_case"`           // Case-insensitive search
	Basename    bool   `json:"basename"`              // Search by basename only
	Limit       int    `json:"limit"`                 // Maximum number of results (default: 100)
	Offset      int64  `json:"offset,omitempty"`      // Offset for pagination
	SortField   string `json:"sort_field,omitempty"`  // Sort field: "name", "size", "time", "path"
	SortOrder   string `json:"sort_order,omitempty"`  // Sort order: "asc", "desc"
}

// StatusParams contains parameters specific to the status command.
type StatusParams struct {
	// No parameters for status command
}

// GetConfigParams contains parameters specific to the get-config command.
type GetConfigParams struct {
	// No parameters for get-config command
}

// SetConfigParams contains parameters specific to the set-config command.
type SetConfigParams struct {
	Config string `json:"config"` // Configuration content (YAML format)
}

// ============================================================================
// Command-Specific Request Types
// ============================================================================

// SearchRequest represents a search request.
type SearchRequest struct {
	BaseRequest
	SearchParams
}

// StatusRequest represents a status request.
type StatusRequest struct {
	BaseRequest
	StatusParams
}

// GetConfigRequest represents a get-config request.
type GetConfigRequest struct {
	BaseRequest
	GetConfigParams
}

// SetConfigRequest represents a set-config request.
type SetConfigRequest struct {
	BaseRequest
	SetConfigParams
}

// ============================================================================
// Unified Request Type (Protocol Layer)
// ============================================================================

// Request represents a unified request structure.
// This is a union type that can hold any command-specific request.
// Business logic should use command-specific types (SearchRequest, StatusRequest, etc.)
// when possible for better type safety.
type Request struct {
	// Required fields
	Method string `json:"method"` // The action to perform (e.g., "search", "status")

	// Optional fields - protocol layer
	ID                   any    `json:"id,omitempty"`                    // Request ID for async response support (similar to json-rpc id)
	AcceptResponseFormat string `json:"accept_response_format,omitempty"` // Desired response format: "json-rpc", "fast", or empty (same as request)

	// Optional fields - search parameters
	Pattern     string `json:"pattern"`             // Search path/pattern (required for search, can be wildcard/regex/normal path)
	Content     string `json:"content,omitempty"`   // Search content/pattern (optional, for search)
	IgnoreCase  bool   `json:"ignore_case"`          // Case-insensitive search (optional, for search)
	PatternMode string `json:"pattern_mode"`         // Pattern mode: "normal", "regex", "extended_regex", "wildcard" (optional, for search)
	Basename    bool   `json:"basename"`             // Search by basename only (optional, for search)
	Limit       int    `json:"limit"`                // Maximum number of results (optional, for search, default: 100)
	Offset      int64  `json:"offset,omitempty"`     // Offset for pagination (optional, for search)
	SortField   string `json:"sort_field,omitempty"` // Sort field: "name", "size", "time", "path" (optional, for search)
	SortOrder   string `json:"sort_order,omitempty"` // Sort order: "asc", "desc" (optional, for search)
	
	// Optional fields - set-config parameters
	Config string `json:"config,omitempty"` // Configuration content (YAML format, for set-config)
}

// Response represents a unified response structure.
// No fields are required - the response structure depends on the operation.
type Response struct {
	ID    any // Request ID for async response support (similar to json-rpc id)
	Count int         // Number of results in current page
	Total int         // Total number of results (for pagination)
	Paths []string    // Result paths (for search)
	Error string      // Error message (if any)
	Result any // Generic result field (for status, get-config, etc.)
}

// Protocol defines the interface for protocol handling.
type Protocol interface {
	// ParseRequest parses a request from the reader.
	ParseRequest(reader *bufio.Reader) (*Request, error)
	
	// ParseRequestWithRemainder parses a request from the reader and returns remaining data.
	// This method handles TCP sticky packets by extracting the first complete request
	// and returning any remaining data for subsequent processing.
	//
	// Parameters:
	//   - reader: The buffered reader to read from
	//
	// Returns:
	//   - *Request: The parsed request
	//   - []byte: Remaining data (may be nil if no sticky packet)
	//   - error: Parsing error if any
	//
	// Example usage:
	//   req, remainder, err := proto.ParseRequestWithRemainder(reader)
	//   if err != nil {
	//       // handle error
	//   }
	//   // process request...
	//   if len(remainder) > 0 {
	//       // handle sticky packet: parse remainder as next request
	//   }
	ParseRequestWithRemainder(reader *bufio.Reader) (*Request, []byte, error)
	
	// WriteRequest writes a request to the writer.
	WriteRequest(writer *bufio.Writer, req *Request) error
	
	// ParseResponse parses a response from the reader.
	ParseResponse(reader *bufio.Reader) (*Response, error)
	
	// WriteResponse writes a response to the writer.
	WriteResponse(writer *bufio.Writer, resp *Response) error
	
	// Name returns the protocol name.
	Name() string
	
	// NewStreamMsg creates a new stream message (reserved for future streaming support).
	// Currently returns nil, error. In the future, this can be implemented to support
	// streaming responses for large result sets.
	NewStreamMsg() (any, error)
}

// ProtocolType represents the protocol type.
type ProtocolType string

const (
	ProtocolFast    ProtocolType = "fast"     // Fast text protocol
	ProtocolJSONRPC ProtocolType = "json-rpc" // JSON-RPC protocol
)

// GetProtocol returns the protocol implementation for the given type.
func GetProtocol(protoType ProtocolType) Protocol {
	switch protoType {
	case ProtocolJSONRPC:
		return NewJSONRPCProtocol()
	default:
		return NewFastProtocol()
	}
}

// GetResponseProtocol determines the response protocol based on request protocol and AcceptResponseFormat.
// This function implements the protocol separation logic:
// - If acceptFormat is empty, use the same protocol as the request
// - If acceptFormat is specified, use the specified protocol
//
// Parameters:
//   - requestProto: The protocol detected from the incoming request
//   - acceptFormat: The AcceptResponseFormat field from the request (can be empty)
//
// Returns:
//   - The protocol to use for sending responses
func GetResponseProtocol(requestProto ProtocolType, acceptFormat string) ProtocolType {
	switch acceptFormat {
	case "json-rpc":
		return ProtocolJSONRPC
	case "fast":
		return ProtocolFast
	default:
		// If not specified, use the same protocol as the request
		return requestProto
	}
}


