// Package server provides the Unix socket server for golocated.
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/internal/socket"
	contentpkg "github.com/RelicOfTesla/golocate/pkg/content"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/message"
	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
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
	
	// Index status tracking
	isBuilding      bool      // 是否正在构建索引
	buildStartTime  time.Time // 索引构建开始时间
	lastBuildTime   time.Time // 最后索引时间
	indexedFileCount int      // 已索引文件数
	databasePath    string    // 数据库路径
	configPath      string    // 配置文件路径
	
	// Config (for get-config command)
	config *config.Config
	
	// ========== 新增：Message 接口组件 ==========
	parser message.MessageParser
	worker message.MessageWorker
}

// New creates a new server instance.
func New(idx *index.Index) *Server {
	return &Server{
		socketPath:    config.GetDefaultSocketPath(),
		index:         idx,
		maxConns:      config.DefaultMaxConns,
		connTimeout:   config.DefaultTimeout,
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
	
	// ========== 初始化 MessageParser 和 MessageWorker ==========
	
	// 1. 创建 MessageParser
	s.parser = message.NewMessageParser()
	
	// 2. 创建 MessageWorker
	s.worker = message.NewMessageWorker()
	
	// 3. 注册方法处理器
	s.registerMethodHandlers()
	
	// 4. 启动 Worker
	if err := s.worker.Start(); err != nil {
		return fmt.Errorf("failed to start message worker: %w", err)
	}
	
	// ========== 启动监听 ==========
	
	// Create listener using cross-platform socket package
	listener, err := socket.CreateListener(s.socketPath)
	if err != nil {
		s.worker.Stop()
		return err
	}
	
	s.listener = listener
	s.running = true
	
	log.Printf("Server listening on %s", s.socketPath)
	
	// Start accepting connections
	go s.acceptLoop()
	
	return nil
}

// registerMethodHandlers 注册所有方法处理器
func (s *Server) registerMethodHandlers() {
	// 注册 search 方法处理器
	s.worker.RegisterMethod("search", message.MethodHandlerFunc(s.handleSearchHandler))
	
	// 注册 status 方法处理器
	s.worker.RegisterMethod("status", message.MethodHandlerFunc(s.handleStatusHandler))
	
	// 注册 get-config 方法处理器
	s.worker.RegisterMethod("get-config", message.MethodHandlerFunc(s.handleGetConfigHandler))
	
	// 注册 set-config 方法处理器
	s.worker.RegisterMethod("set-config", message.MethodHandlerFunc(s.handleSetConfigHandler))
	
	// 注册 build 方法处理器
	s.worker.RegisterMethod("build", message.MethodHandlerFunc(s.handleBuildHandler))
	
	// 注册 stop 方法处理器
	s.worker.RegisterMethod("stop", message.MethodHandlerFunc(s.handleStopHandler))
}

// Stop stops the Unix socket server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return nil
	}
	
	s.running = false
	
	// 停止 MessageWorker
	if s.worker != nil {
		if err := s.worker.Stop(); err != nil {
			log.Printf("warning: failed to stop message worker: %v", err)
		}
	}
	
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}
	
	// Remove socket file only if no other server is using it
	// Try to connect to the socket to check if another server is listening
	if conn, err := net.DialTimeout("unix", s.socketPath, 5*time.Second); err == nil {
		// Connection succeeded, another server is using this socket
		conn.Close()
		log.Printf("Socket file %s is still in use by another server, not removing", s.socketPath)
	} else {
		// Connection failed, no server is using this socket, safe to remove
		if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: failed to remove socket file: %v", err)
		}
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
	// 创建连接级别的 WaitGroup，跟踪该连接的所有消息
	var connWg sync.WaitGroup
	
	defer func() {
		// 等待所有消息处理完成后再关闭连接
		connWg.Wait()
		
		conn.Close()
		s.mu.Lock()
		s.currentConns--
		s.mu.Unlock()
	}()
	
	log.Printf("[Server] Handling new connection from %s", conn.RemoteAddr())
	
	// Update deadline before reading
	if s.connTimeout > 0 {
		conn.SetDeadline(time.Now().Add(s.connTimeout))
	}
	
	// Read request using MessageParser
	reader := bufio.NewReader(conn)
	
	log.Printf("[Server] Parsing request using MessageParser...")
	
	// 使用 MessageParser 解析消息
	msg, remainder, err := s.parser.ParseMessage(conn, reader)
	if err != nil {
		log.Printf("[Server] Failed to parse message: %v", err)
		// 尝试使用旧方式发送错误响应（向后兼容）
		s.sendLegacyError(conn, fmt.Sprintf("failed to parse message: %v", err))
		return
	}
	
	log.Printf("[Server] Message parsed: id=%s, method=%s", msg.ID(), msg.Method())
	
	// 处理粘包：如果有剩余数据，记录日志
	if len(remainder) > 0 {
		log.Printf("[Server] Detected sticky packet, remainder: %d bytes", len(remainder))
		// 注意：当前实现不处理粘包的后续部分
		// 未来可以在这里实现粘包的递归处理
	}
	
	// 为消息设置完成回调
	connWg.Add(1)
	msg.SetOnComplete(func() {
		connWg.Done()
	})
	
	// 使用 MessageWorker 异步处理消息
	if err := s.worker.Handle(msg); err != nil {
		log.Printf("[Server] Failed to handle message: %v", err)
		connWg.Done() // 处理失败，也要 Done，避免死锁
		return
	}
}

// sendLegacyError sends an error response in legacy format for backward compatibility.
// This function is kept for clients that expect the old error format during protocol parsing failures.
// Consider removing in future versions after migrating all clients to use the new error handling mechanism.
func (s *Server) sendLegacyError(conn net.Conn, errMsg string) {
	// 尝试使用 fast 协议发送错误响应
	proto := protocol.NewFastProtocol()
	writer := bufio.NewWriter(conn)
	
	resp := &protocol.Response{
		Error: errMsg,
	}
	
	if err := proto.WriteResponse(writer, resp); err != nil {
		log.Printf("[Server] Failed to send error response: %v", err)
		return
	}
	
	writer.Flush()
}

// ========== 方法处理器（实现 MethodHandler 接口） ==========

// handleSearchHandler 处理搜索请求（Message 接口）
func (s *Server) handleSearchHandler(ctx context.Context, msg message.Message) (any, error) {
	// 1. 从 Message 中解析请求参数
	var req protocol.SearchRequest
	
	if err := json.Unmarshal(msg.Payload(), &req); err != nil {
		return nil, fmt.Errorf("invalid request format: %w", err)
	}
	
	// 2. 输入验证
	
	// Pattern 是路径（必选项）
	pattern := req.Pattern
	
	// 验证 Pattern 不能只包含空白字符
	if strings.TrimSpace(pattern) == "" && pattern != "" {
		return nil, fmt.Errorf("invalid parameter: pattern cannot be only whitespace")
	}
	
	// 如果 Pattern 为空，使用默认值 "*"（搜索所有目录）
	if pattern == "" {
		pattern = "*"
	}
	
	// Content 是文件内容搜索（可选项）
	content := req.Content
	if content != "" && strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("invalid parameter: content cannot be only whitespace")
	}
	
	// 验证 Limit 不能为负数
	if req.Limit < 0 {
		return nil, fmt.Errorf("invalid parameter: limit cannot be negative")
	}
	
	// 验证 Offset 不能为负数
	if req.Offset < 0 {
		return nil, fmt.Errorf("invalid parameter: offset cannot be negative")
	}
	
	// 3. 解析搜索选项
	opts := index.SearchOptions{
		IgnoreCase:     req.IgnoreCase,
		Basename:       req.Basename,
		Limit:          req.Limit,
		Offset:         req.Offset,
		PatternMode:    index.PatternMode(req.PatternMode),
		SortField:      req.SortField,
		SortOrder:      req.SortOrder,
	}
	
	// 3.1 设置默认 PatternMode 为通配符模式
	if opts.PatternMode == "" {
		opts.PatternMode = index.PatternModeWildcard
	}
	
	// 3.5 验证正则表达式（如果启用了正则模式）
	if opts.PatternMode == index.PatternModeRegex || opts.PatternMode == index.PatternModeExtendedRegex {
		// 尝试编译正则表达式，验证其有效性
		var re *regexp.Regexp
		var err error
		
		if opts.PatternMode == index.PatternModeExtendedRegex {
			// 扩展正则（ERE）
			if opts.IgnoreCase {
				re, err = regexp.Compile("(?i)" + pattern)
			} else {
				re, err = regexp.Compile(pattern)
			}
		} else {
			// 基本正则（BRE）- 使用 POSIX
			if opts.IgnoreCase {
				re, err = regexp.CompilePOSIX("(?i)" + pattern)
			} else {
				re, err = regexp.CompilePOSIX(pattern)
			}
		}
		
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		_ = re // 验证通过，不需要使用
	}
	
	// 4. 执行路径搜索（Pattern 用于文件路径匹配）
	opts.Pattern = pattern
	log.Printf("[handleSearchHandler] Searching for pattern=%q, opts.PatternMode=%s, opts.IgnoreCase=%v, opts.Basename=%v", opts.Pattern, opts.PatternMode, opts.IgnoreCase, opts.Basename)
	results := s.index.Search(opts)
	log.Printf("[handleSearchHandler] Found %d results from path search", len(results))
	
	// 5. 如果有 Content 参数，进行文件内容搜索
	if content != "" {
		log.Printf("[handleSearchHandler] Performing content search for %q", content)
		
		// 获取 MaxFileSize，如果 config 为 nil 则使用默认值
		maxFileSize := int64(10 * 1024 * 1024) // 默认 10MB
		if s.config != nil && s.config.MaxContentFileSize > 0 {
			maxFileSize = s.config.MaxContentFileSize
		}
		
		// 创建内容搜索器
		contentSearcher, err := contentpkg.NewSearcher(contentpkg.SearchOptions{
			Pattern:       content,
			IgnoreCase:    req.IgnoreCase,
			Regex:         opts.PatternMode == index.PatternModeRegex,
			ExtendedRegex: opts.PatternMode == index.PatternModeExtendedRegex,
			MaxFileSize:   maxFileSize,
			MaxResults:    req.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create content searcher: %w", err)
		}
		
		// 提取文件路径列表
		filePaths := make([]string, len(results))
		for i, entry := range results {
			filePaths[i] = entry.Path
		}
		
		// 执行内容搜索
		contentResults, err := contentSearcher.Search(ctx, filePaths)
		if err != nil {
			return nil, fmt.Errorf("content search failed: %w", err)
		}
		
		log.Printf("[handleSearchHandler] Found %d content results", len(contentResults))
		
		// 转换内容搜索结果为响应格式
		resultMap := make(map[string]interface{})
		resultMap["results"] = contentResults
		resultMap["count"] = len(contentResults)
		resultMap["total"] = len(contentResults)
		
		return resultMap, nil
	}
	
	// 6. 获取总数（用于分页）
	totalCount := s.index.Count(pattern, index.SearchOptions{
		IgnoreCase:     opts.IgnoreCase,
		Basename:       opts.Basename,
		PatternMode:    opts.PatternMode,
	})
	
	// 7. 安全过滤
	if s.pathValidator != nil {
		filteredResults := make([]*index.Entry, 0, len(results))
		for _, entry := range results {
			if s.pathValidator.IsPathAllowed(entry.Path) {
				filteredResults = append(filteredResults, entry)
			}
		}
		results = filteredResults
	}
	
	// 8. 转换为响应格式
	resultMap := make(map[string]interface{})
	paths := make([]string, len(results))
	for i, entry := range results {
		paths[i] = entry.Path
	}
	resultMap["paths"] = paths
	resultMap["count"] = len(results)
	resultMap["total"] = totalCount
	
	return resultMap, nil
}

// handleStatusHandler 处理状态请求（Message 接口）
func (s *Server) handleStatusHandler(ctx context.Context, msg message.Message) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	result := map[string]any{
		"running":    s.running,
		"index_size": s.index.Len(),
	}
	
	// Add config file path (special value for status command)
	if s.configPath != "" {
		result["config_path"] = s.configPath
	}
	
	// Add index building status
	result["is_building"] = s.isBuilding
	if s.isBuilding && !s.buildStartTime.IsZero() {
		result["build_duration"] = time.Since(s.buildStartTime).String()
	}
	
	// Add last build time if available
	if !s.lastBuildTime.IsZero() {
		result["last_build_time"] = s.lastBuildTime.Format(time.RFC3339)
		result["last_build_ago"] = time.Since(s.lastBuildTime).String()
	}
	
	// Add indexed file count
	result["indexed_file_count"] = s.indexedFileCount
	
	return result, nil
}

// handleGetConfigHandler 处理获取配置请求（Message 接口）
func (s *Server) handleGetConfigHandler(ctx context.Context, msg message.Message) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// If config is not set, return error
	if s.config == nil {
		return nil, fmt.Errorf("config not available")
	}
	
	// Return complete config (including default values)
	result := map[string]any{
		"socket_path":         s.config.SocketPath,
		"directories":         s.config.Directories,
		"database_path":       s.config.DatabasePath,
		"ignore_patterns":     s.config.IgnorePatterns,
		"pid_file":            s.config.PIDFile,
		"log_file":            s.config.LogFile,
		"follow_symlinks":     s.config.FollowSymlinks,
		"worker_count":        s.config.WorkerCount,
		"content_search":      s.config.ContentSearch,
		"max_content_file_size": s.config.MaxContentFileSize,
		"index_interval":      s.config.IndexInterval,
		"throttle_index":      s.config.ThrottleIndex,
		"index_strategy":      s.config.IndexStrategy,
	}
	
	return result, nil
}

// handleSetConfigHandler 处理设置配置请求（Message 接口）
func (s *Server) handleSetConfigHandler(ctx context.Context, msg message.Message) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 解析请求
	var req struct {
		Content string `json:"content,omitempty"`
	}
	
	if err := json.Unmarshal(msg.Payload(), &req); err != nil {
		return nil, fmt.Errorf("invalid request format: %w", err)
	}
	
	// Check if config path is set
	if s.configPath == "" {
		return nil, fmt.Errorf("config file path not set")
	}
	
	// Check if config content is provided
	if req.Content == "" {
		return nil, fmt.Errorf("config content is empty")
	}
	
	// Parse YAML config
	newCfg, err := config.LoadFromYAML([]byte(req.Content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	
	// Validate config
	if err := newCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	// Save config to file
	if err := newCfg.Save(s.configPath); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}
	
	// Update server config
	s.config = newCfg
	
	// Return success
	return map[string]any{"status": "saved"}, nil
}

// handleBuildHandler 处理构建索引请求（Message 接口）
func (s *Server) handleBuildHandler(ctx context.Context, msg message.Message) (any, error) {
	// TODO: Implement index building
	return nil, fmt.Errorf("build not implemented yet")
}

// handleStopHandler 处理停止服务请求（Message 接口）
func (s *Server) handleStopHandler(ctx context.Context, msg message.Message) (any, error) {
	result := "stopping server"
	
	// 异步停止服务
	go s.Stop()
	
	return map[string]any{"status": result}, nil
}

// ========== 设置方法（保持向后兼容） ==========

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

// SetDatabasePath sets the database path for status reporting.
func (s *Server) SetDatabasePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.databasePath = path
}

// SetConfigPath sets the config file path for status reporting.
func (s *Server) SetConfigPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configPath = path
}

// SetBuildingStatus sets the index building status.
func (s *Server) SetBuildingStatus(isBuilding bool, startTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isBuilding = isBuilding
	s.buildStartTime = startTime
}

// SetLastBuildTime sets the last build time.
func (s *Server) SetLastBuildTime(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastBuildTime = t
}

// SetIndexedFileCount sets the indexed file count.
func (s *Server) SetIndexedFileCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.indexedFileCount = count
}

// SetConfig sets the config for the server (for get-config command).
func (s *Server) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}
