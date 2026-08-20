// Package protocol provides decoder implementations for golocate.
// This file implements a simplified decoder design that directly reads from connections.
//
// Design Principles:
// 1. No protocol detection functions needed
// 2. Decoders read directly from io.Reader (more generic than net.Conn)
// 3. Simple first-byte check: '{' → JSON decoder, others → Fast decoder
// 4. Business layer doesn't need to know protocol type
// 5. Streaming processing without knowing data size in advance
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/RelicOfTesla/golocate/pkg/index"
)

// Decoder defines the interface for decoding requests.
// This is the simplified design that removes the need for protocol detection.
// It accepts io.Reader instead of net.Conn for more flexibility.
type Decoder interface {
	// Decode reads a request from the reader.
	// It does not need to know the data size in advance.
	// It reads data in a streaming manner until a complete message is received.
	Decode(reader io.Reader) (*Request, error)

	// DecodeWithRemainder reads a request from the reader and returns remaining data.
	// This is used for handling TCP sticky packets.
	DecodeWithRemainder(reader *bufio.Reader) (*Request, []byte, error)
}

// FastDecoder implements the Decoder interface for fast text protocol.
// It reads data line by line until it encounters an empty line (\n\n).
type FastDecoder struct{}

// NewFastDecoder creates a new FastDecoder instance.
func NewFastDecoder() *FastDecoder {
	return &FastDecoder{}
}

// Decode implements the Decoder interface.
// It reads lines from the reader until it finds an empty line (end of request).
// Each line is a key=value pair.
func (d *FastDecoder) Decode(reader io.Reader) (*Request, error) {
	// Create a buffered reader if not already
	bufReader, ok := reader.(*bufio.Reader)
	if !ok {
		bufReader = bufio.NewReader(reader)
	}

	req, _, err := d.DecodeWithRemainder(bufReader)
	return req, err
}

// DecodeWithRemainder implements the Decoder interface.
// It decodes a request and returns remaining data for sticky packet handling.
func (d *FastDecoder) DecodeWithRemainder(reader *bufio.Reader) (*Request, []byte, error) {
	req := &Request{
		Limit: 100, // default
	}

	// Read lines until we find an empty line (end of request)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// EOF reached, stop reading
				break
			}
			return nil, nil, fmt.Errorf("failed to read line: %w", err)
		}

		// Remove trailing newline
		line = strings.TrimRight(line, "\n")

		// Empty line marks end of request
		if line == "" {
			break
		}

		// Parse key=value pair
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // skip invalid lines
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		// Parse field
		parseRequestField(req, key, value)
	}

	// Check for remaining data (sticky packet handling)
	var remainder []byte
	if reader.Buffered() > 0 {
		remainingData := make([]byte, reader.Buffered())
		n, err := reader.Read(remainingData)
		if err != nil && err != io.EOF {
			// Log error but don't fail
		} else if n > 0 {
			remainder = remainingData[:n]
		}
	}

	return req, remainder, nil
}

// JSONDecoder implements the Decoder interface for JSON-RPC protocol.
// It uses json.NewDecoder for streaming decoding.
type JSONDecoder struct{}

// NewJSONDecoder creates a new JSONDecoder instance.
func NewJSONDecoder() *JSONDecoder {
	return &JSONDecoder{}
}

// Decode implements the Decoder interface.
// It uses json.NewDecoder to decode a JSON-RPC request from the reader.
func (d *JSONDecoder) Decode(reader io.Reader) (*Request, error) {
	// Create a buffered reader if not already
	bufReader, ok := reader.(*bufio.Reader)
	if !ok {
		bufReader = bufio.NewReader(reader)
	}

	req, _, err := d.DecodeWithRemainder(bufReader)
	return req, err
}

// DecodeWithRemainder implements the Decoder interface.
// It decodes a request and returns remaining data for sticky packet handling.
func (d *JSONDecoder) DecodeWithRemainder(reader *bufio.Reader) (*Request, []byte, error) {
	// jsonrpcRequest represents a JSON-RPC request. The embedded Request
	// makes TOP-LEVEL search fields work too (e.g. {"method":"search",
	// "content":"test"}), while an explicit "params" object (JSON-RPC style)
	// takes precedence when both are present. Note: embedded fields shadowed
	// by the explicit Jsonrpc/ID/Method/Params declarations are not parsed
	// from the top level, which is the intended behavior.
	type jsonrpcRequest struct {
		Jsonrpc string   `json:"jsonrpc"`
		ID      any      `json:"id"`
		Method  string   `json:"method"`
		Params  *Request `json:"params,omitempty"`
		Request
	}

	// Use json.NewDecoder for streaming decode
	var req jsonrpcRequest
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&req); err != nil {
		return nil, nil, fmt.Errorf("failed to decode JSON-RPC: %w", err)
	}

	// Convert to unified request: top-level search fields first, explicit
	// params object (JSON-RPC style) takes precedence.
	params := req.Request
	if req.Params != nil {
		params = *req.Params
	}

	// Get remaining data from json.Decoder.Buffered()
	var remainder []byte
	if buffered := decoder.Buffered(); buffered != nil {
		// Read all remaining data
		remainingData, err := io.ReadAll(buffered)
		if err != nil {
			// Ignore error, just don't return remainder
		} else if len(remainingData) > 0 {
			remainder = remainingData
		}
	}

	return &Request{
		Method:               req.Method,
		ID:                   req.ID,
		Pattern:              params.Pattern,
		Content:              params.Content,
		IgnoreCase:           params.IgnoreCase,
		Limit:                params.Limit,
		PatternMode:          params.PatternMode,
		Basename:             params.Basename,
		Offset:               params.Offset,
		SortField:            params.SortField,
		SortOrder:            params.SortOrder,
		Scope:                params.Scope,
		Exclude:              params.Exclude,
		Types:                params.Types,
		MinSize:              params.MinSize,
		MaxSize:              params.MaxSize,
		MtimeAfter:           params.MtimeAfter,
		MtimeBefore:          params.MtimeBefore,
		ExcludeHidden:        params.ExcludeHidden,
		Dedupe:               params.Dedupe,
		AcceptResponseFormat: params.AcceptResponseFormat,
		Config:               params.Config,
	}, remainder, nil
}

// SelectDecoder selects a decoder based on the first non-whitespace byte of the data.
// This is the simple protocol selection logic:
// - First non-whitespace byte is '{' → JSONDecoder
// - Otherwise → FastDecoder
//
// This function skips leading whitespace (spaces, tabs, newlines, carriage returns)
// and then peeks the first non-whitespace byte to determine the decoder.
// It returns a bufio.Reader that includes all data (so it can be used for decoding).
func SelectDecoder(reader io.Reader) (Decoder, *bufio.Reader, error) {
	// Create a buffered reader
	bufReader, ok := reader.(*bufio.Reader)
	if !ok {
		bufReader = bufio.NewReader(reader)
	}

	// Skip leading whitespace
	for {
		b, err := bufReader.Peek(1)
		if err != nil {
			return nil, bufReader, fmt.Errorf("failed to peek byte: %w", err)
		}

		// Check if it's whitespace
		if b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r' {
			// Consume the whitespace
			bufReader.ReadByte()
			continue
		}

		// Non-whitespace byte found
		break
	}

	// Peek the first non-whitespace byte
	b, err := bufReader.Peek(1)
	if err != nil {
		return nil, bufReader, fmt.Errorf("failed to peek first byte: %w", err)
	}

	slog.Debug("First non-whitespace byte", "char", string(b[0]), "hex", fmt.Sprintf("0x%02x", b[0]))

	// Select decoder based on first non-whitespace byte
	if b[0] == '{' {
		slog.Debug("Selected JSONDecoder")
		return NewJSONDecoder(), bufReader, nil
	}
	slog.Debug("Selected FastDecoder")
	return NewFastDecoder(), bufReader, nil
}

// DecodeRequest is a convenience function that selects a decoder and decodes the request.
// This is the main entry point for decoding requests.
// It accepts any io.Reader, including net.Conn.
func DecodeRequest(reader io.Reader) (*Request, error) {
	decoder, bufReader, err := SelectDecoder(reader)
	if err != nil {
		return nil, err
	}

	return decoder.Decode(bufReader)
}

// DecodeRequestFromConn is a convenience function for decoding from net.Conn.
// It's equivalent to DecodeRequest(conn) but makes the intent clearer.
func DecodeRequestFromConn(conn net.Conn) (*Request, error) {
	return DecodeRequest(conn)
}

// parseRequestField parses a key=value pair and sets the field in the request.
// This is a helper function used by FastDecoder.
func parseRequestField(req *Request, key, value string) {
	switch key {
	case FieldMethod:
		req.Method = value
	case FieldID:
		// Try to parse as integer first, then as string
		if n, err := strconv.Atoi(value); err == nil {
			req.ID = n
		} else {
			req.ID = value
		}
	case FieldMode:
		// Deprecated: mode field is no longer used
	case FieldPattern:
		req.Pattern = value
	case FieldContent:
		req.Content = value
	case FieldAcceptResponseFormat:
		req.AcceptResponseFormat = value
	case FieldIgnoreCase:
		req.IgnoreCase = value == "true"
	case FieldLimit:
		if n, err := strconv.Atoi(value); err == nil {
			req.Limit = n
		}
	case FieldBasename:
		req.Basename = value == "true"
	case FieldRegex:
		if value == "true" {
			req.PatternMode = string(index.PatternModeRegex)
		}
	case FieldExtendedRegex:
		if value == "true" {
			req.PatternMode = string(index.PatternModeExtendedRegex)
		}
	case FieldPatternMode:
		req.PatternMode = value
	case FieldOffset:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			req.Offset = n
		}
	case FieldSortField:
		req.SortField = value
	case FieldSortOrder:
		req.SortOrder = value
	case FieldScope:
		req.Scope = value
	case FieldExclude:
		req.Exclude = append(req.Exclude, value)
	case FieldTypes:
		req.Types = append(req.Types, value)
	case FieldMinSize:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			req.MinSize = n
		}
	case FieldMaxSize:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			req.MaxSize = n
		}
	case FieldMtimeAfter:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			req.MtimeAfter = n
		}
	case FieldMtimeBefore:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			req.MtimeBefore = n
		}
	case FieldExcludeHidden:
		req.ExcludeHidden = value == "true"
	case FieldDedupe:
		req.Dedupe = value == "true"
	case FieldConfig:
		req.Config = value
	}
}
