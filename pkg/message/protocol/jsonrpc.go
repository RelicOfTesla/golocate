// Package protocol provides JSON-RPC protocol implementation.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

// jsonrpcProtocol implements the Protocol interface for JSON-RPC.
type jsonrpcProtocol struct{}

// NewJSONRPCProtocol creates a new JSON-RPC protocol instance.
func NewJSONRPCProtocol() Protocol {
	return &jsonrpcProtocol{}
}

// jsonrpcRequest represents a JSON-RPC request.
type jsonrpcRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Method  string        `json:"method"`
	Params  Request       `json:"params"` // Embed protocol.Request directly
}

// jsonrpcResponse represents a JSON-RPC response.
type jsonrpcResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      any `json:"id"`
	Result  any `json:"result,omitempty"`
	Error   *jsonrpcError `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC error.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p *jsonrpcProtocol) ParseRequest(reader *bufio.Reader) (*Request, error) {
	req, _, err := p.ParseRequestWithRemainder(reader)
	return req, err
}

// ParseRequestWithRemainder parses a JSON-RPC request and returns remaining data (for sticky packet handling).
func (p *jsonrpcProtocol) ParseRequestWithRemainder(reader *bufio.Reader) (*Request, []byte, error) {
	// Read a line (JSON-RPC data followed by newline)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, nil, err
	}
	
	// Trim whitespace
	data := []byte(strings.TrimSpace(line))
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("empty request")
	}
	
	// Log the received data for debugging
	log.Printf("[JSON-RPC Protocol] Received: %s", string(data))
	
	// Handle TCP sticky packets: split concatenated JSON messages
	messages, jsonRemainder := SplitJSONMessagesWithRemainder(data)
	
	// If no complete JSON message found, return error
	if len(messages) == 0 {
		return nil, jsonRemainder, fmt.Errorf("no complete JSON message found")
	}
	
	// Parse the first JSON message
	firstMsg := messages[0]
	log.Printf("[JSON-RPC Protocol] Parsing first message: %s", string(firstMsg))
	
	// Parse JSON-RPC
	var req jsonrpcRequest
	if err := json.Unmarshal(firstMsg, &req); err != nil {
		log.Printf("[JSON-RPC Protocol] Parse error: %v", err)
		return nil, jsonRemainder, err
	}
	
	// Log the parsed request
	log.Printf("[JSON-RPC Protocol] Parsed: method=%s, content=%s, pattern_mode=%s, ignore_case=%v, limit=%d", 
		req.Method, req.Params.Content, req.Params.PatternMode, req.Params.IgnoreCase, req.Params.Limit)
	
	// Check for more data in the reader (mixed protocol sticky packet)
	var remainder []byte
	if len(jsonRemainder) > 0 {
		// There's remaining JSON data
		remainder = jsonRemainder
	} else if reader.Buffered() > 0 {
		// There's more data in the buffer (could be another protocol)
		remainingData, err := io.ReadAll(reader)
		if err != nil {
			log.Printf("[JSON-RPC Protocol] Error reading remaining data: %v", err)
		} else if len(remainingData) > 0 {
			remainder = remainingData
			log.Printf("[JSON-RPC Protocol] Detected mixed protocol sticky packet, remainder: %d bytes", len(remainder))
		}
	}
	
	// Convert to unified request
	return &Request{
		Method:        req.Method,
		ID:            req.ID,
		Pattern:       req.Params.Pattern,     // Search pattern (path)
		Content:       req.Params.Content,     // File content search
		IgnoreCase:    req.Params.IgnoreCase,
		Limit:         req.Params.Limit,
		PatternMode:   req.Params.PatternMode,
		Basename:      req.Params.Basename,
		Offset:        req.Params.Offset,
		SortField:     req.Params.SortField,
		SortOrder:     req.Params.SortOrder,
	}, remainder, nil
}

func (p *jsonrpcProtocol) WriteRequest(writer *bufio.Writer, req *Request) error {
	// Convert to JSON-RPC request
	jsonrpcReq := jsonrpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  req.Method,
		Params:  *req, // Embed protocol.Request directly
	}
	
	// Encode JSON
	data, err := json.Marshal(jsonrpcReq)
	if err != nil {
		return err
	}
	
	// Write to writer
	if _, err := writer.Write(data); err != nil {
		return err
	}
	
	// Add newline delimiter for message framing
	if _, err := writer.Write([]byte("\n")); err != nil {
		return err
	}
	
	return writer.Flush()
}

func (p *jsonrpcProtocol) ParseResponse(reader *bufio.Reader) (*Response, error) {
	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	
	log.Printf("[JSON-RPC Protocol] ParseResponse received data: %s", string(data))
	
	// Parse JSON-RPC
	var resp jsonrpcResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	
	log.Printf("[JSON-RPC Protocol] ParseResponse parsed: ID=%v, Result=%v, Error=%v", resp.ID, resp.Result, resp.Error)
	
	// Handle error
	if resp.Error != nil {
		return &Response{
			Error: resp.Error.Message,
		}, nil
	}
	
	// Handle result
	if resp.Result != nil {
		// Try to parse as response with count and paths
		if resultMap, ok := resp.Result.(map[string]any); ok {
			log.Printf("[JSON-RPC Protocol] ParseResponse resultMap: %v", resultMap)
			
			count := 0
			if c, ok := resultMap["count"].(float64); ok {
				count = int(c)
			}
			
			total := 0
			if t, ok := resultMap["total"].(float64); ok {
				total = int(t)
			}
			
			var paths []string
			if p, ok := resultMap["paths"].([]any); ok {
				for _, path := range p {
					if pathStr, ok := path.(string); ok {
						paths = append(paths, pathStr)
					}
				}
			}
			
			result := &Response{
				Count:  count,
				Total:  total,
				Paths:  paths,
				Result: resultMap, // Keep original result for status, get-config, etc.
			}
			
			log.Printf("[JSON-RPC Protocol] ParseResponse returning: ID=%v, Count=%d, Total=%d, Result=%v", result.ID, result.Count, result.Total, result.Result)
			
			return result, nil
		}
		
		// For non-map results, keep the original result
		return &Response{
			Result: resp.Result,
		}, nil
	}
	
	return &Response{}, nil
}

func (p *jsonrpcProtocol) WriteResponse(writer *bufio.Writer, resp *Response) error {
	// Create JSON-RPC response
	jsonrpcResp := jsonrpcResponse{
		Jsonrpc: "2.0",
		ID:      1,
	}
	
	// Handle error
	if resp.Error != "" {
		jsonrpcResp.Error = &jsonrpcError{
			Code:    -1,
			Message: resp.Error,
		}
	} else {
		// 优先使用 resp.Result（用于 status、get-config 等命令）
		if resp.Result != nil {
			jsonrpcResp.Result = resp.Result
		} else {
			// 否则使用 count、total、paths（用于 search 命令）
			jsonrpcResp.Result = map[string]any{
				"count": resp.Count,
				"total": resp.Total,
				"paths": resp.Paths,
			}
		}
	}
	
	// Encode JSON
	data, err := json.Marshal(jsonrpcResp)
	if err != nil {
		return err
	}
	
	log.Printf("[JSON-RPC Protocol] WriteResponse sending JSON: %s", string(data))
	
	// Write to writer with newline delimiter
	if _, err := writer.Write(data); err != nil {
		return err
	}
	
	// Add newline delimiter for message framing
	if _, err := writer.Write([]byte("\n")); err != nil {
		return err
	}
	
	return writer.Flush()
}

func (p *jsonrpcProtocol) Name() string {
	return "json-rpc"
}

// NewStreamMsg creates a new stream message (reserved for future streaming support).
// Currently returns nil, error as streaming is not yet implemented.
func (p *jsonrpcProtocol) NewStreamMsg() (any, error) {
	return nil, fmt.Errorf("streaming not implemented for json-rpc protocol")
}
