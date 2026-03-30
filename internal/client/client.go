// Package client provides the Unix socket client for golocate.
package client

import (
	"context"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/config"
	errpkg "github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
)

// Client represents the Unix socket client.
type Client struct {
	socketPath string
	timeout    time.Duration
	retryCount int           // 重试次数
	retryDelay time.Duration // 重试间隔
	requestID  int64         // 请求 ID 计数器（原子操作）
}

// New creates a new client instance.
func New() *Client {
	return &Client{
		socketPath: config.DefaultSocketPath,
		timeout:    config.DefaultTimeout,
		retryCount: config.DefaultRetryCount,
		retryDelay: config.DefaultRetryDelay,
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

// nextRequestID 生成下一个请求 ID（线程安全）
func (c *Client) nextRequestID() int {
	return int(atomic.AddInt64(&c.requestID, 1))
}

// ========== 核心方法：统一的请求发送和响应接收 ==========

// doRequest 发送请求并接收响应（统一方法）
//
// 这是一个通用的方法，用于所有需要发送请求并接收响应的操作。
// 它使用通用的 ResponseWriter 和 RequestWriter 接口，自动处理协议检测和转换。
//
// 参数：
//   - req: 请求参数（可以是 *protocol.Request 或 map[string]any）
//
// 返回：
//   - *protocol.Response: 响应
//   - error: 错误
//
// 使用示例：
//
//	resp, err := c.doRequest(&protocol.Request{
//	    Method: "search",
//	    Content: "pattern",
//	    Path: "/path",
//	})
func (c *Client) doRequest(req any) (*protocol.Response, error) {
	// 连接服务器
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// 生成请求 ID
	requestID := c.nextRequestID()

	// 确保 req 是 *protocol.Request 类型
	var protoReq *protocol.Request
	switch r := req.(type) {
	case *protocol.Request:
		protoReq = r
	case map[string]any:
		protoReq = mapToProtocolRequest(r)
	default:
		return nil, fmt.Errorf("invalid request type: %T", req)
	}

	// 设置请求 ID
	protoReq.ID = requestID

	log.Printf("[Client] Sending request: id=%d, method=%s", requestID, protoReq.Method)

	// 根据请求方法选择协议
	// status、get-config、set-config 等命令需要 JSON-RPC 协议（支持 Result 字段）
	// search 等命令使用 Fast 协议（更快）
	var proto protocol.Protocol
	switch protoReq.Method {
	case "status", "get-config", "set-config":
		proto = protocol.NewJSONRPCProtocol()
	default:
		proto = protocol.NewFastProtocol()
	}

	// 使用通用的 RequestWriter 发送请求
	requestWriter := protocol.NewRequestWriter(conn, proto)
	if err := requestWriter.WriteRequest(context.Background(), protoReq); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	log.Printf("[Client] Request sent, receiving response...")

	// 使用通用的 ResponseReader 接收响应
	responseReader := protocol.NewResponseReader(conn)
	resp, err := responseReader.ReadResponse(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 验证响应 ID
	c.verifyResponseID(resp, requestID)

	// 处理错误响应
	if resp.Error != "" {
		return nil, fmt.Errorf("server error: %s", resp.Error)
	}

	log.Printf("[Client] Response received: id=%v, count=%d, total=%d", resp.ID, resp.Count, resp.Total)

	return resp, nil
}

// verifyResponseID 验证响应 ID 是否匹配请求 ID
func (c *Client) verifyResponseID(resp *protocol.Response, requestID int) {
	if resp.ID == nil {
		return
	}

	var respID int
	switch v := resp.ID.(type) {
	case int:
		respID = v
	case float64:
		respID = int(v)
	case string:
		if _, err := fmt.Sscanf(v, "%d", &respID); err != nil {
			log.Printf("[Client] Warning: response ID is not a number: %v", resp.ID)
			return
		}
	default:
		log.Printf("[Client] Warning: unexpected response ID type: %T", resp.ID)
		return
	}

	if respID != requestID {
		log.Printf("[Client] Warning: response ID (%d) does not match request ID (%d)", respID, requestID)
	}
}

// ========== 业务方法：使用统一的 doRequest ==========

// Search sends a search request to the server (JSON format, backward compatible).
// pattern is the content search query (optional, can be empty)
// opts.Path is the path filter (required, cannot be empty)
// opts.Offset and opts.Limit are for pagination
func (c *Client) Search(pattern string, opts index.SearchOptions) ([]*index.Entry, error) {
	result, err := c.SearchWithTotal(pattern, opts)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// SearchWithTotal sends a search request and returns results with total count.
func (c *Client) SearchWithTotal(pattern string, opts index.SearchOptions) (*SearchResult, error) {
	// 将 pattern 设置到 opts.Pattern
	opts.Pattern = pattern
	
	// 构建请求
	req := &protocol.Request{
		Method:      "search",
		Content:     opts.Pattern,
		Path:        opts.Path,
		IgnoreCase:  opts.IgnoreCase,
		Basename:    opts.Basename,
		Limit:       opts.Limit,
		Offset:      opts.Offset,
		Regex:       opts.Regex,
		ExtendedRegex: opts.ExtendedRegex,
		SortField:   opts.SortField,
		SortOrder:   opts.SortOrder,
	}

	// 使用统一方法发送请求
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	// 转换响应为 SearchResult
	return c.responseToSearchResult(resp), nil
}

// SearchFast sends a search request using the fast protocol.
// This method is kept for backward compatibility.
func (c *Client) SearchFast(pattern string, opts index.SearchOptions) (*SearchResult, error) {
	// Fast protocol is now the default
	return c.SearchWithTotal(pattern, opts)
}

// Status gets the server status.
func (c *Client) Status() (map[string]any, error) {
	req := &protocol.Request{
		Method: "status",
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if result, ok := resp.Result.(map[string]any); ok {
		return result, nil
	}

	return nil, fmt.Errorf("invalid response type")
}

// GetConfig gets the server configuration (including default values).
func (c *Client) GetConfig() (map[string]any, error) {
	req := &protocol.Request{
		Method: "get-config",
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if result, ok := resp.Result.(map[string]any); ok {
		return result, nil
	}

	return nil, fmt.Errorf("invalid response type")
}

// SetConfig sets the server configuration from YAML content.
func (c *Client) SetConfig(yamlContent string) error {
	req := &protocol.Request{
		Method:  "set-config",
		Content: yamlContent,
	}

	_, err := c.doRequest(req)
	return err
}

// Build sends a build request to the server.
func (c *Client) Build() error {
	req := &protocol.Request{
		Method: "build",
	}

	_, err := c.doRequest(req)
	return err
}

// ReloadConfig sends a reload-config request to the server.
func (c *Client) ReloadConfig() error {
	req := &protocol.Request{
		Method: "reload-config",
	}

	_, err := c.doRequest(req)
	return err
}

// UpdateDB is an alias for Build (compatibility with locate command).
func (c *Client) UpdateDB() error {
	return c.Build()
}

// ========== 流式搜索（保留但简化） ==========

// SearchStream sends a search request and streams results.
// Note: This method is kept for compatibility but currently returns all results at once.
// Future versions may implement true streaming.
func (c *Client) SearchStream(pattern string, opts index.SearchOptions, callback func(*index.Entry) bool) error {
	result, err := c.SearchWithTotal(pattern, opts)
	if err != nil {
		return err
	}

	for _, entry := range result.Entries {
		if !callback(entry) {
			break
		}
	}

	return nil
}

// ========== 辅助方法 ==========

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

	// Wrap the error with a friendly message
	return nil, errpkg.WrapConnectError(lastErr, c.socketPath)
}

// responseToSearchResult 将响应转换为搜索结果
func (c *Client) responseToSearchResult(resp *protocol.Response) *SearchResult {
	results := make([]*index.Entry, len(resp.Paths))
	for i, path := range resp.Paths {
		results[i] = &index.Entry{
			Path: path,
			Name: filepath.Base(path), // Extract filename from path
		}
	}

	return &SearchResult{
		Entries: results,
		Count:   resp.Count,
		Total:   resp.Total,
	}
}

// mapToProtocolRequest 将 map 转换为 protocol.Request
func mapToProtocolRequest(m map[string]any) *protocol.Request {
	req := &protocol.Request{
		Limit: 100, // 默认值
	}

	if method, ok := m["method"].(string); ok {
		req.Method = method
	}

	if id, ok := m["id"]; ok {
		req.ID = id
	}

	if content, ok := m["content"].(string); ok {
		req.Content = content
	}

	if path, ok := m["path"].(string); ok {
		req.Path = path
	}

	if ignoreCase, ok := m["ignore_case"].(bool); ok {
		req.IgnoreCase = ignoreCase
	}

	if limit, ok := m["limit"].(int); ok {
		req.Limit = limit
	}

	if offset, ok := m["offset"].(int64); ok {
		req.Offset = offset
	}

	if basename, ok := m["basename"].(bool); ok {
		req.Basename = basename
	}

	if regex, ok := m["regex"].(bool); ok {
		req.Regex = regex
	}

	if extendedRegex, ok := m["extended_regex"].(bool); ok {
		req.ExtendedRegex = extendedRegex
	}

	if sortField, ok := m["sort_field"].(string); ok {
		req.SortField = sortField
	}

	if sortOrder, ok := m["sort_order"].(string); ok {
		req.SortOrder = sortOrder
	}

	return req
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

// ========== 类型定义 ==========

// SearchResult represents a search result with pagination info.
type SearchResult struct {
	Entries []*index.Entry
	Count   int // Current page count
	Total   int // Total results count
}

// Request represents a client request.
type Request struct {
	ID                   any    `json:"id,omitempty"`                      // Request ID for async response support
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
	ID     any    `json:"id,omitempty"`     // Request ID for async response support
	Type   string `json:"type"`
	Path   string `json:"path,omitempty"`
	Name   string `json:"name,omitempty"`
	Size   int64   `json:"size,omitempty"`
	Count  int     `json:"count,omitempty"`
	Total  int     `json:"total,omitempty"` // Total results count (for pagination)
	Paths  any     `json:"paths,omitempty"` // Paths for search results
	Error  string  `json:"error,omitempty"`
	Result any     `json:"result,omitempty"`
}
