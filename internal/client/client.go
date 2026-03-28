// Package client provides the Unix socket client for golocate.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/constants"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/protocol"
)

// Client represents the Unix socket client.
type Client struct {
	socketPath string
	timeout    time.Duration
	retryCount int           // 重试次数
	retryDelay time.Duration // 重试间隔
}

// Request represents a client request.
type Request struct {
	Method               string `json:"method"`
	Content              string `json:"content,omitempty"`                // Search file content (optional)
	Path                 string `json:"path"`                             // Search/filter by path (required)
	AcceptResponseFormat string `json:"accept_response_format,omitempty"` // json, json-rpc, or empty (fast protocol)
	
	// Search options (flattened, aligned with fast protocol)
	IgnoreCase     bool   `json:"ignore_case,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Offset         int64  `json:"offset,omitempty"` // For pagination
	Basename       bool   `json:"basename,omitempty"`
	Regex          bool   `json:"regex,omitempty"`
	ExtendedRegex  bool   `json:"extended_regex,omitempty"`
	SortField      string `json:"sort_field,omitempty"`
	SortOrder      string `json:"sort_order,omitempty"`
}

// Response represents a server response.
type Response struct {
	Type   string      `json:"type"`
	Path   string      `json:"path,omitempty"`
	Name   string      `json:"name,omitempty"`
	Size   int64       `json:"size,omitempty"`
	Count  int         `json:"count,omitempty"`
	Error  string      `json:"error,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

// New creates a new client instance.
func New() *Client {
	return &Client{
		socketPath: constants.DefaultSocketPath,
		timeout:    constants.DefaultTimeout,
		retryCount: constants.DefaultRetryCount,
		retryDelay: constants.DefaultRetryDelay,
	}
}

// SetSocketPath sets the socket path.
func (c *Client) SetSocketPath(path string) {
	c.socketPath = path
}

// SetTimeout sets the connection timeout.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// Search sends a search request to the server (JSON format, backward compatible).
// pattern is the content search query (optional, can be empty)
// opts.Path is the path filter (required, cannot be empty)
// opts.Offset and opts.Limit are for pagination
func (c *Client) Search(pattern string, opts index.SearchOptions) ([]*index.Entry, error) {
	// Build request with flattened fields
	req := Request{
		Method:               "search",
		Content:              pattern,     // Content is optional (can be empty)
		Path:                 opts.Path,  // Path is required (must be set)
		AcceptResponseFormat: "json",     // Explicitly request JSON response
		IgnoreCase:           opts.IgnoreCase,
		Basename:             opts.Basename,
		Limit:                opts.Limit,
		Offset:               opts.Offset,
		Regex:                opts.Regex,
		ExtendedRegex:        opts.ExtendedRegex,
		SortField:            opts.SortField,
		SortOrder:            opts.SortOrder,
	}
	
	// Connect to server
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	
	// Send request
	if err := c.sendRequest(conn, &req); err != nil {
		return nil, err
	}
	
	// Receive results
	results := []*index.Entry{}
	decoder := json.NewDecoder(bufio.NewReader(conn))
	
	for {
		var resp Response
		if err := decoder.Decode(&resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		
		switch resp.Type {
		case "result":
			results = append(results, &index.Entry{
				Path: resp.Path,
				Name: resp.Name,
				Size: resp.Size,
			})
		case "done":
			return results, nil
		case "error":
			return nil, fmt.Errorf("server error: %s", resp.Error)
		}
	}
}

// SearchFast sends a search request using the fast protocol.
// pattern is the content search query (optional, can be empty)
// opts.Path is the path filter (required, cannot be empty)
// opts.Offset and opts.Limit are for pagination
func (c *Client) SearchFast(pattern string, opts index.SearchOptions) ([]*index.Entry, error) {
	// Connect to server
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	
	// Send request using fast protocol
	writer := bufio.NewWriter(conn)
	fmt.Fprintf(writer, "method=search\n")
	fmt.Fprintf(writer, "path=%s\n", opts.Path) // Path is required
	if pattern != "" {
		fmt.Fprintf(writer, "content=%s\n", pattern) // Content is optional
	}
	fmt.Fprintf(writer, "ignore_case=%v\n", opts.IgnoreCase)
	fmt.Fprintf(writer, "limit=%d\n", opts.Limit)
	fmt.Fprintf(writer, "offset=%d\n", opts.Offset) // Offset for pagination
	if opts.Regex {
		fmt.Fprintf(writer, "mode=regex\n")
	}
	fmt.Fprintf(writer, "\n") // Empty line to end request
	writer.Flush()
	
	// Receive response
	reader := bufio.NewReader(conn)
	
	// Peek first byte to detect protocol
	firstByte, err := reader.Peek(1)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if firstByte[0] == '{' {
		// JSON response (backward compatible)
		return c.receiveJSONResults(reader)
	}
	
	// Fast protocol response
	return c.receiveFastResults(reader)
}

// receiveJSONResults receives JSON format results.
func (c *Client) receiveJSONResults(reader *bufio.Reader) ([]*index.Entry, error) {
	results := []*index.Entry{}
	decoder := json.NewDecoder(reader)
	
	for {
		var resp Response
		if err := decoder.Decode(&resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		
		switch resp.Type {
		case "result":
			results = append(results, &index.Entry{
				Path: resp.Path,
				Name: resp.Name,
				Size: resp.Size,
			})
		case "done":
			return results, nil
		case "error":
			return nil, fmt.Errorf("server error: %s", resp.Error)
		}
	}
}

// receiveFastResults receives fast protocol results.
func (c *Client) receiveFastResults(reader *bufio.Reader) ([]*index.Entry, error) {
	// Get fast protocol implementation
	proto := protocol.GetProtocol(protocol.ProtocolFast)
	
	resp, err := proto.ParseResponse(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	results := make([]*index.Entry, len(resp.Paths))
	for i, path := range resp.Paths {
		results[i] = &index.Entry{
			Path: path,
			Name: path, // Will be extracted by caller if needed
		}
	}
	
	return results, nil
}

// SearchStream sends a search request and streams results.
func (c *Client) SearchStream(pattern string, opts index.SearchOptions, callback func(*index.Entry) bool) error {
	// Build request
	req := Request{
		Method:  "search",
		Content: pattern,
		
	}
	
	// Connect to server
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	
	// Send request
	if err := c.sendRequest(conn, &req); err != nil {
		return err
	}
	
	// Stream results
	decoder := json.NewDecoder(bufio.NewReader(conn))
	
	for {
		var resp Response
		if err := decoder.Decode(&resp); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		
		switch resp.Type {
		case "result":
			entry := &index.Entry{
				Path: resp.Path,
				Name: resp.Name,
				Size: resp.Size,
			}
			// Call callback, stop if it returns false
			if !callback(entry) {
				return nil
			}
		case "done":
			return nil
		case "error":
			return fmt.Errorf("server error: %s", resp.Error)
		}
	}
}

// Status gets the server status.
func (c *Client) Status() (map[string]interface{}, error) {
	req := Request{
		Method:  "status",
	}
	
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	
	if err := c.sendRequest(conn, &req); err != nil {
		return nil, err
	}
	
	var resp Response
	decoder := json.NewDecoder(bufio.NewReader(conn))
	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if resp.Type == "error" {
		return nil, fmt.Errorf("server error: %s", resp.Error)
	}
	
	if result, ok := resp.Result.(map[string]interface{}); ok {
		return result, nil
	}
	
	return nil, fmt.Errorf("invalid response type")
}

// Build sends a build request to the server.
func (c *Client) Build() error {
	req := Request{
		Method:  "build",
	}
	
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	
	if err := c.sendRequest(conn, &req); err != nil {
		return err
	}
	
	var resp Response
	decoder := json.NewDecoder(bufio.NewReader(conn))
	if err := decoder.Decode(&resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	
	if resp.Type == "error" {
		return fmt.Errorf("server error: %s", resp.Error)
	}
	
	return nil
}

// ReloadConfig sends a reload-config request to the server.
func (c *Client) ReloadConfig() error {
	req := Request{
		Method:  "reload-config",
	}
	
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	
	if err := c.sendRequest(conn, &req); err != nil {
		return err
	}
	
	var resp Response
	decoder := json.NewDecoder(bufio.NewReader(conn))
	if err := decoder.Decode(&resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	
	if resp.Type == "error" {
		return fmt.Errorf("server error: %s", resp.Error)
	}
	
	return nil
}

// UpdateDB is an alias for Build (compatibility with locate command).
func (c *Client) UpdateDB() error {
	return c.Build()
}

// connect connects to the Unix socket with retry.
func (c *Client) connect() (net.Conn, error) {
	var lastErr error
	
	for i := 0; i <= c.retryCount; i++ {
		conn, err := socket.Connect(c.socketPath)
		if err == nil {
			// Set timeout
			if c.timeout > 0 {
				conn.SetDeadline(time.Now().Add(c.timeout))
			}
			return conn, nil
		}
		lastErr = err
		
		// 如果不是最后一次尝试，等待后重试
		if i < c.retryCount {
			time.Sleep(c.retryDelay)
		}
	}
	
	return nil, fmt.Errorf("failed to connect to server after %d retries: %w", c.retryCount, lastErr)
}

// sendRequest sends a request to the server.
func (c *Client) sendRequest(conn net.Conn, req *Request) error {
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	return nil
}

// IsServerRunning checks if the server is running.
func (c *Client) IsServerRunning() bool {
	conn, err := net.DialTimeout("unix", c.socketPath, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
