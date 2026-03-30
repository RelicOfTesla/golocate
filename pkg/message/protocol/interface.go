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

// Request represents a unified request structure.
// Only Method is required. All other fields are optional.
type Request struct {
	// Required fields
	Method string `json:"method"` // The action to perform (e.g., "search", "status")

	// Optional fields - protocol layer
	ID                   any    `json:"id,omitempty"`                    // Request ID for async response support (similar to json-rpc id)
	AcceptResponseFormat string `json:"accept_response_format,omitempty"` // Desired response format: "json-rpc", "fast", or empty (same as request)

	// Optional fields - search parameters
	Content       string `json:"content,omitempty"`        // Search content/pattern (optional, for search)
	Path          string `json:"path,omitempty"`           // Path filter (optional, for search)
	IgnoreCase    bool   `json:"ignore_case"`              // Case-insensitive search (optional, for search)
	Mode          string `json:"mode,omitempty"`           // Search mode: "regex", "extended_regex", etc. (optional, for search)
	Limit         int    `json:"limit"`                    // Maximum number of results (optional, for search, default: 100)
	Basename      bool   `json:"basename"`                 // Search by basename only (optional, for search)
	Regex         bool   `json:"regex"`                    // Enable regex search mode (optional, for search)
	ExtendedRegex bool   `json:"extended_regex"`           // Enable extended regex mode (optional, for search)
	Offset        int64  `json:"offset,omitempty"`         // Offset for pagination (optional, for search)
	SortField     string `json:"sort_field,omitempty"`     // Sort field: "name", "size", "time", "path" (optional, for search)
	SortOrder     string `json:"sort_order,omitempty"`     // Sort order: "asc", "desc" (optional, for search)
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

// DetectProtocol detects the protocol type from the first byte.
// It reads all available buffered data to detect JSON-RPC fields,
// supporting detection even when the "jsonrpc" field appears after large content.
//
// This function uses the default detection configuration, which waits up to 100ms
// for more data when the buffered data is insufficient for confident detection.
// For custom detection behavior, use DetectProtocolWithConfig.
func DetectProtocol(reader *bufio.Reader) (ProtocolType, error) {
	result, err := DetectProtocolWithConfig(reader, DefaultDetectionConfig())
	if err != nil {
		return ProtocolFast, err
	}
	return result.ProtocolType, nil
}

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

// ProtocolPair represents a pair of request and response protocols.
// This is used to make the protocol separation explicit and clear.
type ProtocolPair struct {
	RequestProtocol  ProtocolType // Protocol detected from request
	ResponseProtocol ProtocolType // Protocol to use for response
}

// DetectProtocolPair detects both request and response protocols from a request.
// This is a convenience function that combines DetectProtocol and GetResponseProtocol.
//
// Parameters:
//   - reader: The buffered reader to detect protocol from
//
// Returns:
//   - ProtocolPair containing both request and response protocols
//   - error if detection fails
//
// Note: This function peeks at the reader but does not consume data.
// The caller should use the returned requestProtocol to parse the request.
func DetectProtocolPair(reader *bufio.Reader) (*ProtocolPair, error) {
	// Detect request protocol
	requestProto, err := DetectProtocol(reader)
	if err != nil {
		return nil, err
	}

	// For now, we can't determine AcceptResponseFormat without parsing the request
	// So we return the request protocol as both request and response protocol
	// The caller should call GetResponseProtocol after parsing the request
	return &ProtocolPair{
		RequestProtocol:  requestProto,
		ResponseProtocol: requestProto, // Default: same as request
	}, nil
}
