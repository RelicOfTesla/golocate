// Package server provides the Unix socket server for golocated.
package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/constants"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/protocol"
	"github.com/RelicOfTesla/golocate/pkg/security"
)

// Server represents the Unix socket server.
type Server struct {
	socketPath    string
	listener      net.Listener
	index         *index.Index
	mu            sync.Mutex
	running       bool
	maxConns      int           // 最大连接数
	currentConns  int          // 当前连接数
	connTimeout   time.Duration // 连接超时
	pathValidator *security.PathValidator // 路径验证器
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

// New creates a new server instance.
func New(idx *index.Index) *Server {
	return &Server{
		socketPath:    constants.DefaultSocketPath,
		index:         idx,
		maxConns:      constants.DefaultMaxConns,
		connTimeout:   constants.DefaultTimeout,
		pathValidator: security.NewPathValidator(nil), // TODO: configure allowed directories
	}
}

// Start starts the server.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.running {
		return fmt.Errorf("server already running")
	}
	
	// Create listener using cross-platform socket package
	listener, err := socket.CreateListener(s.socketPath)
	if err != nil {
		return err
	}
	
	s.listener = listener
	s.running = true
	
	log.Printf("Server listening on %s", s.socketPath)
	
	// Start accepting connections
	go s.acceptLoop()
	
	return nil
}

// Stop stops the Unix socket server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return nil
	}
	
	s.running = false
	
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}
	
	// Remove socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to remove socket file: %v", err)
	}
	
	log.Println("Server stopped")
	return nil
}

// acceptLoop accepts incoming connections.
func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				log.Printf("accept error: %v", err)
			}
			return
		}
		
		// 检查最大连接数限制
		s.mu.Lock()
		if s.maxConns > 0 && s.currentConns >= s.maxConns {
			s.mu.Unlock()
			log.Printf("max connections reached (%d), rejecting new connection", s.maxConns)
			conn.Close()
			continue
		}
		s.currentConns++
		s.mu.Unlock()
		
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single client connection.
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		s.currentConns--
		s.mu.Unlock()
	}()
	
	// 设置连接超时
	if s.connTimeout > 0 {
		conn.SetDeadline(time.Now().Add(s.connTimeout))
	}
	
	// Read request with protocol detection
	reader := bufio.NewReader(conn)
	
	// Detect protocol type
	protoType, err := protocol.DetectProtocol(reader)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to detect protocol: %v", err))
		return
	}
	
	// Get protocol implementation
	proto := protocol.GetProtocol(protoType)
	
	// Parse request using protocol
	protoReq, err := proto.ParseRequest(reader)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("invalid protocol request: %v", err))
		return
	}
	
	// Convert protocol.Request to server.Request
	var req Request
	req.Method = protoReq.Method
	req.Content = protoReq.Content
	req.AcceptResponseFormat = protoReq.AcceptResponseFormat
	req.IgnoreCase = protoReq.IgnoreCase
	req.Limit = protoReq.Limit
	req.Mode = protoReq.Mode
	req.Path = protoReq.Path
	
	// Handle request
	switch req.Method {
	case "search":
		s.handleSearch(conn, &req, protoType)
	case "status":
		s.handleStatus(conn, &req)
	case "build":
		s.handleBuild(conn, &req)
	case "stop":
		s.handleStop(conn, &req)
	default:
		s.sendError(conn, fmt.Sprintf("unknown action: %s", req.Method))
	}
}

// handleSearch handles a search request.
func (s *Server) handleSearch(conn net.Conn, req *Request, protoType protocol.ProtocolType) {
	// ========== 输入验证 ==========
	
	// 验证 Path 不能为空（必选项）
	if req.Path == "" || strings.TrimSpace(req.Path) == "" {
		s.sendError(conn, "invalid parameter: path is required and cannot be empty")
		return
	}
	
	// 验证 Limit 不能为负数
	if req.Limit < 0 {
		s.sendError(conn, "invalid parameter: limit cannot be negative")
		return
	}
	
	// 验证 Offset 不能为负数
	if req.Offset < 0 {
		s.sendError(conn, "invalid parameter: offset cannot be negative")
		return
	}
	
	// 验证 Content：如果非空但只包含空白字符，返回错误
	if req.Content != "" && strings.TrimSpace(req.Content) == "" {
		s.sendError(conn, "invalid parameter: content cannot be only whitespace")
		return
	}
	
	// ========== 解析搜索选项 ==========
	
	// Parse options from flattened fields
	opts := index.SearchOptions{
		IgnoreCase:     req.IgnoreCase,
		Basename:       req.Basename,
		Limit:          req.Limit,
		Offset:         req.Offset,
		Regex:          req.Regex,
		ExtendedRegex:  req.ExtendedRegex,
		SortField:      req.SortField,
		SortOrder:      req.SortOrder,
		Path:           req.Path,
	}
	
	// 根据定义：PATH 是必选项（路径过滤），CONTENT 是可选项（文件内容搜索）
	// - 如果只有 PATH，则只进行路径过滤
	// - 如果 PATH 和 CONTENT 都有，则进行路径过滤 + 文件内容搜索
	
	query := req.Content
	// 如果 Content 为空，则只进行路径过滤（搜索路径包含 Path 的所有文件）
	if query == "" {
		query = req.Path
		opts.Path = "" // 避免重复过滤
	}
	
	// Search (server handles all filtering, regex, sorting logic)
	results := s.index.Search(query, opts)
	
	// Filter results by path validator (security check)
	if s.pathValidator != nil {
		filteredResults := make([]*index.Entry, 0, len(results))
		for _, entry := range results {
			if s.pathValidator.IsPathAllowed(entry.Path) {
				filteredResults = append(filteredResults, entry)
			}
		}
		results = filteredResults
	}
	
	// Send results based on AcceptResponseFormat or protocol type
	writer := bufio.NewWriter(conn)
	
	// Determine response format
	var responseProtoType protocol.ProtocolType
	if req.AcceptResponseFormat == "json" {
		responseProtoType = protocol.ProtocolJSON
	} else if req.AcceptResponseFormat == "json-rpc" {
		responseProtoType = protocol.ProtocolJSONRPC
	} else if protoType == protocol.ProtocolJSON {
		responseProtoType = protocol.ProtocolJSON
	} else if protoType == protocol.ProtocolJSONRPC {
		responseProtoType = protocol.ProtocolJSONRPC
	} else {
		responseProtoType = protocol.ProtocolFast
	}
	
	// Send results based on response format
	if responseProtoType == protocol.ProtocolJSON || responseProtoType == protocol.ProtocolJSONRPC {
		// Use JSON or JSON-RPC encoding
		encoder := json.NewEncoder(writer)
		for _, entry := range results {
			resp := Response{
				Type: "result",
				Path: entry.Path,
				Name: entry.Name,
				Size: entry.Size,
			}
			if err := encoder.Encode(resp); err != nil {
				log.Printf("error sending result: %v", err)
				return
			}
			writer.Flush()
		}
		done := Response{
			Type:  "done",
			Count: len(results),
		}
		if err := encoder.Encode(done); err != nil {
			log.Printf("error sending done: %v", err)
		}
		writer.Flush()
	} else {
		// Use fast protocol
		paths := make([]string, len(results))
		for i, entry := range results {
			paths[i] = entry.Path
		}
		resp := &protocol.Response{
			Count: len(results),
			Paths: paths,
		}
		
		// Get protocol and write response
		proto := protocol.GetProtocol(responseProtoType)
		if err := proto.WriteResponse(writer, resp); err != nil {
			log.Printf("error sending response: %v", err)
		}
		writer.Flush()
	}
}

// handleStatus handles a status request.
func (s *Server) handleStatus(conn net.Conn, req *Request) {
	resp := Response{
		Type: "status",
		Result: map[string]interface{}{
			"index_size": s.index.Len(),
			"running":    s.running,
		},
	}
	
	encoder := json.NewEncoder(conn)
	encoder.Encode(resp)
}

// handleBuild handles a build request.
func (s *Server) handleBuild(conn net.Conn, req *Request) {
	// TODO: Implement index building
	resp := Response{
		Type:  "result",
		Error: "build not implemented yet",
	}
	
	encoder := json.NewEncoder(conn)
	encoder.Encode(resp)
}

// handleStop handles a stop request.
func (s *Server) handleStop(conn net.Conn, req *Request) {
	resp := Response{
		Type:   "result",
		Result: "stopping server",
	}
	
	encoder := json.NewEncoder(conn)
	encoder.Encode(resp)
	
	// Stop server asynchronously
	go s.Stop()
}

// sendError sends an error response.
func (s *Server) sendError(conn net.Conn, msg string) {
	resp := Response{
		Type:  "error",
		Error: msg,
	}
	
	encoder := json.NewEncoder(conn)
	encoder.Encode(resp)
}

// SetIndex sets the index for the server.
func (s *Server) SetIndex(idx *index.Index) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index = idx
}

// SetSocketPath sets the socket path for the server.
func (s *Server) SetSocketPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.socketPath = path
}

// IsRunning returns whether the server is running.
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
