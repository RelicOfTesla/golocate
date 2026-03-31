// Package protocol provides generic interfaces for request/response handling.
// This file defines universal interfaces that abstract away protocol-specific details,
// making it easier to work with different protocols in a unified way.
package protocol

import (
	"bufio"
	"context"
	"errors"
	"io"
)

// ========== 错误定义 ==========

var (
	// ErrInvalidRequestType indicates the request type is not supported.
	ErrInvalidRequestType = errors.New("invalid request type")

	// ErrInvalidResponseType indicates the response type is not supported.
	ErrInvalidResponseType = errors.New("invalid response type")
)

// ========== 通用写入接口 ==========

// ResponseWriter provides a generic interface for writing responses.
// This interface abstracts away protocol-specific serialization details.
//
// Design Principles:
// - Universal: Works with any protocol (JSON-RPC, Fast, etc.)
// - Simple: External code should not care about protocol details
// - Efficient: Minimal overhead and allocations
// - Extensible: Easy to add new protocols
//
// Usage:
//
//	writer := NewResponseWriter(conn, protocol)
//	err := writer.WriteResponse(ctx, response)
type ResponseWriter interface {
	// WriteResponse writes a response to the underlying writer.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - resp: Response to write (can be *Response, map, or any serializable type)
	//
	// Returns:
	//   - error: Writing failed
	//
	// Note: This method handles protocol-specific serialization automatically.
	WriteResponse(ctx context.Context, resp any) error
}

// RequestWriter provides a generic interface for writing requests.
// This interface abstracts away protocol-specific serialization details.
//
// Usage:
//
//	writer := NewRequestWriter(conn, protocol)
//	err := writer.WriteRequest(ctx, request)
type RequestWriter interface {
	// WriteRequest writes a request to the underlying writer.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - req: Request to write (can be *Request, map, or any serializable type)
	//
	// Returns:
	//   - error: Writing failed
	//
	// Note: This method handles protocol-specific serialization automatically.
	WriteRequest(ctx context.Context, req any) error
}

// ========== 通用读取接口 ==========

// ResponseReader provides a generic interface for reading responses.
// This interface abstracts away protocol-specific deserialization details.
//
// Usage:
//
//	reader := NewResponseReader(conn)
//	resp, err := reader.ReadResponse(ctx)
type ResponseReader interface {
	// ReadResponse reads a response from the underlying reader.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns:
	//   - *Response: The read response
	//   - error: Reading failed
	//
	// Note: This method automatically detects the protocol format.
	ReadResponse(ctx context.Context) (*Response, error)
}

// RequestReader provides a generic interface for reading requests.
// This interface abstracts away protocol-specific deserialization details.
//
// Usage:
//
//	reader := NewRequestReader(conn)
//	req, err := reader.ReadRequest(ctx)
type RequestReader interface {
	// ReadRequest reads a request from the underlying reader.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns:
	//   - *Request: The read request
	//   - error: Reading failed
	//
	// Note: This method automatically detects the protocol format.
	ReadRequest(ctx context.Context) (*Request, error)
}

// ========== 组合接口 ==========

// MessageWriter combines ResponseWriter and RequestWriter.
type MessageWriter interface {
	ResponseWriter
	RequestWriter
}

// MessageReader combines ResponseReader and RequestReader.
type MessageReader interface {
	ResponseReader
	RequestReader
}

// MessageIO combines MessageWriter and MessageReader.
type MessageIO interface {
	MessageWriter
	MessageReader
}

// ========== 默认实现 ==========

// defaultResponseWriter implements ResponseWriter interface.
type defaultResponseWriter struct {
	writer   *bufio.Writer
	protocol Protocol
}

// NewResponseWriter creates a new ResponseWriter.
//
// Parameters:
//   - w: The underlying writer (can be net.Conn, io.Writer, etc.)
//   - proto: The protocol to use for serialization
//
// Returns:
//   - ResponseWriter: The created writer
//
// Note: If proto is nil, Fast protocol is used by default.
func NewResponseWriter(w io.Writer, proto Protocol) ResponseWriter {
	if proto == nil {
		proto = NewFastProtocol()
	}
	return &defaultResponseWriter{
		writer:   bufio.NewWriter(w),
		protocol: proto,
	}
}

// WriteResponse implements ResponseWriter interface.
func (w *defaultResponseWriter) WriteResponse(ctx context.Context, resp any) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Convert response to *Response
	var protoResp *Response
	switch r := resp.(type) {
	case *Response:
		protoResp = r
	case map[string]any:
		protoResp = mapToResponse(r)
	default:
		// Try to use as-is (will be handled by protocol implementation)
		protoResp = &Response{
			Result: resp,
		}
	}

	// Use protocol to write response
	if err := w.protocol.WriteResponse(w.writer, protoResp); err != nil {
		return err
	}

	return w.writer.Flush()
}

// defaultRequestWriter implements RequestWriter interface.
type defaultRequestWriter struct {
	writer   *bufio.Writer
	protocol Protocol
}

// NewRequestWriter creates a new RequestWriter.
//
// Parameters:
//   - w: The underlying writer (can be net.Conn, io.Writer, etc.)
//   - proto: The protocol to use for serialization
//
// Returns:
//   - RequestWriter: The created writer
//
// Note: If proto is nil, Fast protocol is used by default.
func NewRequestWriter(w io.Writer, proto Protocol) RequestWriter {
	if proto == nil {
		proto = NewFastProtocol()
	}
	return &defaultRequestWriter{
		writer:   bufio.NewWriter(w),
		protocol: proto,
	}
}

// WriteRequest implements RequestWriter interface.
func (w *defaultRequestWriter) WriteRequest(ctx context.Context, req any) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Convert request to *Request
	var protoReq *Request
	switch r := req.(type) {
	case *Request:
		protoReq = r
	case map[string]any:
		protoReq = mapToRequest(r)
	default:
		// Try to use as-is (will be handled by protocol implementation)
		return ErrInvalidRequestType
	}

	// Use protocol to write request
	if err := w.protocol.WriteRequest(w.writer, protoReq); err != nil {
		return err
	}

	return w.writer.Flush()
}

// defaultResponseReader implements ResponseReader interface.
type defaultResponseReader struct {
	reader *bufio.Reader
}

// NewResponseReader creates a new ResponseReader.
//
// Parameters:
//   - r: The underlying reader (can be net.Conn, io.Reader, etc.)
//
// Returns:
//   - ResponseReader: The created reader
//
// Note: The reader automatically detects the protocol format.
func NewResponseReader(r io.Reader) ResponseReader {
	return &defaultResponseReader{
		reader: bufio.NewReader(r),
	}
}

// ReadResponse implements ResponseReader interface.
func (r *defaultResponseReader) ReadResponse(ctx context.Context) (*Response, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Detect protocol
	protoType, err := DetectProtocol(r.reader)
	if err != nil {
		return nil, err
	}

	// Get protocol implementation
	proto := GetProtocol(protoType)

	// Use protocol to read response
	return proto.ParseResponse(r.reader)
}

// defaultRequestReader implements RequestReader interface.
type defaultRequestReader struct {
	reader *bufio.Reader
}

// NewRequestReader creates a new RequestReader.
//
// Parameters:
//   - r: The underlying reader (can be net.Conn, io.Reader, etc.)
//
// Returns:
//   - RequestReader: The created reader
//
// Note: The reader automatically detects the protocol format.
func NewRequestReader(r io.Reader) RequestReader {
	return &defaultRequestReader{
		reader: bufio.NewReader(r),
	}
}

// ReadRequest implements RequestReader interface.
func (r *defaultRequestReader) ReadRequest(ctx context.Context) (*Request, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Detect protocol
	protoType, err := DetectProtocol(r.reader)
	if err != nil {
		return nil, err
	}

	// Get protocol implementation
	proto := GetProtocol(protoType)

	// Use protocol to read request
	return proto.ParseRequest(r.reader)
}

// ========== 辅助函数 ==========

// mapToResponse converts a map to *Response.
func mapToResponse(m map[string]any) *Response {
	resp := &Response{}

	if id, ok := m["id"]; ok {
		resp.ID = id
	}

	if count, ok := m["count"].(int); ok {
		resp.Count = count
	}

	if total, ok := m["total"].(int); ok {
		resp.Total = total
	}

	if paths, ok := m["paths"].([]string); ok {
		resp.Paths = paths
	}

	if errMsg, ok := m["error"].(string); ok {
		resp.Error = errMsg
	}

	// Store the complete map in Result field for commands like status, get-config
	resp.Result = m

	return resp
}

// mapToRequest converts a map to *Request.
func mapToRequest(m map[string]any) *Request {
	req := &Request{}

	if method, ok := m["method"].(string); ok {
		req.Method = method
	}

	if id, ok := m["id"]; ok {
		req.ID = id
	}

	if content, ok := m["content"].(string); ok {
		req.Content = content
	}

	// path is deprecated, pattern is the path

	if ignoreCase, ok := m["ignore_case"].(bool); ok {
		req.IgnoreCase = ignoreCase
	}

	if limit, ok := m["limit"].(int); ok {
		req.Limit = limit
	}

	// Copy all fields for extensibility
	// Additional fields will be handled by protocol implementation

	return req
}
