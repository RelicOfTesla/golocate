// Package protocol provides JSON-RPC protocol implementation.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
	Jsonrpc string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  jsonrpcParams   `json:"params"`
}

// jsonrpcParams represents JSON-RPC params.
type jsonrpcParams struct {
	Content     string `json:"content"`
	IgnoreCase  bool   `json:"ignore_case"`
	Limit       int    `json:"limit"`
	Mode        string `json:"mode,omitempty"`
	Path        string `json:"path,omitempty"`
}

// jsonrpcResponse represents a JSON-RPC response.
type jsonrpcResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *jsonrpcError `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC error.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p *jsonrpcProtocol) ParseRequest(reader *bufio.Reader) (*Request, error) {
	// Read a line (JSON-RPC data followed by newline)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	
	// Trim whitespace
	data := []byte(strings.TrimSpace(line))
	if len(data) == 0 {
		return nil, fmt.Errorf("empty request")
	}
	
	// Parse JSON-RPC
	var req jsonrpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	
	// Convert to unified request
	return &Request{
		Method:     req.Method,
		Content:    req.Params.Content,
		IgnoreCase: req.Params.IgnoreCase,
		Limit:      req.Params.Limit,
		Mode:       req.Params.Mode,
		Path:       req.Params.Path,
	}, nil
}

func (p *jsonrpcProtocol) WriteRequest(writer *bufio.Writer, req *Request) error {
	// Convert to JSON-RPC request
	jsonrpcReq := jsonrpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  req.Method,
		Params: jsonrpcParams{
			Content:     req.Content,
			IgnoreCase:  req.IgnoreCase,
			Limit:       req.Limit,
			Mode:        req.Mode,
			Path:        req.Path,
		},
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
	
	return writer.Flush()
}

func (p *jsonrpcProtocol) ParseResponse(reader *bufio.Reader) (*Response, error) {
	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	
	// Parse JSON-RPC
	var resp jsonrpcResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	
	// Handle error
	if resp.Error != nil {
		return &Response{
			Error: resp.Error.Message,
		}, nil
	}
	
	// Handle result
	if resp.Result != nil {
		// Try to parse as response with count and paths
		if resultMap, ok := resp.Result.(map[string]interface{}); ok {
			count := 0
			if c, ok := resultMap["count"].(float64); ok {
				count = int(c)
			}
			
			var paths []string
			if p, ok := resultMap["paths"].([]interface{}); ok {
				for _, path := range p {
					if pathStr, ok := path.(string); ok {
						paths = append(paths, pathStr)
					}
				}
			}
			
			return &Response{
				Count: count,
				Paths: paths,
			}, nil
		}
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
		jsonrpcResp.Result = map[string]interface{}{
			"count": resp.Count,
			"paths": resp.Paths,
		}
	}
	
	// Encode JSON
	data, err := json.Marshal(jsonrpcResp)
	if err != nil {
		return err
	}
	
	// Write to writer
	if _, err := writer.Write(data); err != nil {
		return err
	}
	
	return writer.Flush()
}

func (p *jsonrpcProtocol) Name() string {
	return "json-rpc"
}
