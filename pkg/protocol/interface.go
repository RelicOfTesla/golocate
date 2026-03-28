// Package protocol provides protocol abstraction for golocate.
// This package defines the protocol interface and implementations for JSON, JSON-RPC, and fast protocol.
package protocol

import (
	"bufio"
	"io"
)

// Request represents a unified request structure.
type Request struct {
	Method               string
	Mode                 string
	Path                 string
	Content              string
	IgnoreCase           bool
	Limit                int
	AcceptResponseFormat string // json, json-rpc, or empty (fast protocol)
}

// Response represents a unified response structure.
type Response struct {
	Count int
	Paths []string
	Error string
}

// Protocol defines the interface for protocol handling.
type Protocol interface {
	// ParseRequest parses a request from the reader.
	ParseRequest(reader *bufio.Reader) (*Request, error)
	
	// WriteRequest writes a request to the writer.
	WriteRequest(writer *bufio.Writer, req *Request) error
	
	// ParseResponse parses a response from the reader.
	ParseResponse(reader *bufio.Reader) (*Response, error)
	
	// WriteResponse writes a response to the writer.
	WriteResponse(writer *bufio.Writer, resp *Response) error
	
	// Name returns the protocol name.
	Name() string
}

// ProtocolType represents the protocol type.
type ProtocolType string

const (
	ProtocolFast    ProtocolType = "fast"     // Fast text protocol
	ProtocolJSON    ProtocolType = "json"     // JSON protocol
	ProtocolJSONRPC ProtocolType = "json-rpc" // JSON-RPC protocol
)

// DetectProtocol detects the protocol type from the first byte.
func DetectProtocol(reader *bufio.Reader) (ProtocolType, error) {
	// Peek at the first byte
	b, err := reader.Peek(1)
	if err != nil {
		return ProtocolFast, err
	}
	
	// If the first byte is '{', it's JSON or JSON-RPC
	if b[0] == '{' {
		// Peek more bytes to detect JSON-RPC (use smaller buffer to avoid blocking)
		data, err := reader.Peek(50)
		if err != nil && err != io.EOF {
			// If we can't peek more, just assume JSON
			return ProtocolJSON, nil
		}
		
		// Check for JSON-RPC version field
		for i := 0; i < len(data); i++ {
			if data[i] == 'j' && i+13 < len(data) {
				if string(data[i:i+14]) == `"jsonrpc":"2.0"` {
					return ProtocolJSONRPC, nil
				}
			}
		}
		
		return ProtocolJSON, nil
	}
	
	// Otherwise, it's fast protocol
	return ProtocolFast, nil
}

// GetProtocol returns the protocol implementation for the given type.
func GetProtocol(protoType ProtocolType) Protocol {
	switch protoType {
	case ProtocolJSON:
		return NewJSONProtocol()
	case ProtocolJSONRPC:
		return NewJSONRPCProtocol()
	default:
		return NewFastProtocol()
	}
}
