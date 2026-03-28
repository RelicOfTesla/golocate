// Package protocol provides JSON protocol implementation.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

// jsonProtocol implements the Protocol interface for JSON.
type jsonProtocol struct{}

// NewJSONProtocol creates a new JSON protocol instance.
func NewJSONProtocol() Protocol {
	return &jsonProtocol{}
}

// jsonRequest represents a JSON request.
type jsonRequest struct {
	Method               string `json:"method"`
	Content              string `json:"content"`
	IgnoreCase           bool   `json:"ignore_case"`
	Limit                int    `json:"limit"`
	Mode                 string `json:"mode,omitempty"`
	Path                 string `json:"path,omitempty"`
	AcceptResponseFormat string `json:"accept_response_format,omitempty"`
}

// jsonResponse represents a JSON response.
type jsonResponse struct {
	Count int      `json:"count,omitempty"`
	Paths []string `json:"paths,omitempty"`
	Error string   `json:"error,omitempty"`
}

func (p *jsonProtocol) ParseRequest(reader *bufio.Reader) (*Request, error) {
	// Read a line (JSON data followed by newline)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	
	// Trim whitespace (including the newline)
	data := []byte(strings.TrimSpace(line))
	if len(data) == 0 {
		return nil, fmt.Errorf("empty request")
	}
	
	// Log the received data for debugging
	log.Printf("[JSON Protocol] Received: %s", string(data))
	
	// Parse JSON
	var req jsonRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("[JSON Protocol] Parse error: %v", err)
		return nil, err
	}
	
	// Log the parsed request
	log.Printf("[JSON Protocol] Parsed: method=%s, content=%s", req.Method, req.Content)
	
	// Convert to unified request
	return &Request{
		Method:               req.Method,
		Content:              req.Content,
		IgnoreCase:           req.IgnoreCase,
		Limit:                req.Limit,
		Mode:                 req.Mode,
		Path:                 req.Path,
		AcceptResponseFormat: req.AcceptResponseFormat,
	}, nil
}

func (p *jsonProtocol) WriteRequest(writer *bufio.Writer, req *Request) error {
	// Convert to JSON request
	jsonReq := jsonRequest{
		Method:               req.Method,
		Content:              req.Content,
		IgnoreCase:           req.IgnoreCase,
		Limit:                req.Limit,
		Mode:                 req.Mode,
		Path:                 req.Path,
		AcceptResponseFormat: req.AcceptResponseFormat,
	}
	
	// Encode JSON
	data, err := json.Marshal(jsonReq)
	if err != nil {
		return err
	}
	
	// Write to writer
	if _, err := writer.Write(data); err != nil {
		return err
	}
	
	return writer.Flush()
}

func (p *jsonProtocol) ParseResponse(reader *bufio.Reader) (*Response, error) {
	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	
	// Parse JSON
	var resp jsonResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	
	// Convert to unified response
	return &Response{
		Count: resp.Count,
		Paths: resp.Paths,
		Error: resp.Error,
	}, nil
}

func (p *jsonProtocol) WriteResponse(writer *bufio.Writer, resp *Response) error {
	// Convert to JSON response
	jsonResp := jsonResponse{
		Count: resp.Count,
		Paths: resp.Paths,
		Error: resp.Error,
	}
	
	// Encode JSON
	data, err := json.Marshal(jsonResp)
	if err != nil {
		return err
	}
	
	// Write to writer
	if _, err := writer.Write(data); err != nil {
		return err
	}
	
	return writer.Flush()
}

func (p *jsonProtocol) Name() string {
	return "json"
}
