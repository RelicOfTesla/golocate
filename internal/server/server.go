// Package server provides the Unix socket server for golocated.
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/config"
	contentpkg "github.com/RelicOfTesla/golocate/pkg/content"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/message"
	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
	"github.com/RelicOfTesla/golocate/pkg/security"
)

// perDirCount holds the file/dir totals for one indexed root directory.
type PerDirCount struct {
	Files int64
	Dirs  int64
}

// BuildHistoryMax is how many recent builds are kept in status.
const BuildHistoryMax = 10

// BuildRecord is one entry of the recent-build history.
type BuildRecord struct {
	Time    time.Time
	Files   int64
	Dirs    int64
	Elapsed time.Duration
}

// RecordBuild appends a completed build to the history (newest first,
// capped at BuildHistoryMax) and bumps the build counter.
func (s *Server) RecordBuild(files, dirs int64, elapsed time.Duration) {
	s.buildCount.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := BuildRecord{Time: time.Now(), Files: files, Dirs: dirs, Elapsed: elapsed}
	s.buildHistory = append([]BuildRecord{rec}, s.buildHistory...)
	if len(s.buildHistory) > BuildHistoryMax {
		s.buildHistory = s.buildHistory[:BuildHistoryMax]
	}
}

// Server represents the Unix socket server.
type Server struct {
	socketPath    string
	listener      net.Listener
	index         *index.Index
	mu            sync.Mutex
	running       bool
	maxConns      int                     // 最大连接数
	currentConns  int                     // 当前连接数
	connTimeout   time.Duration           // 连接超时
	pathValidator *security.PathValidator // 路径验证器

	// Server lifetime context: cancelled in Stop() so in-flight index
	// builds can abort cleanly.
	ctx    context.Context
	cancel context.CancelFunc

	// Index status tracking
	isBuilding       bool                   // 是否正在构建索引
	buildStartTime   time.Time              // 索引构建开始时间
	buildScanned     int64                  // 当前构建已扫描条目数（构建进度）
	lastBuildTime    time.Time              // 最后索引时间
	indexedFileCount int                    // 已索引文件数
	lastBuildFiles   int64                  // 最近一次构建统计：文件数
	lastBuildDirs    int64                  // 最近一次构建统计：目录数
	lastBuildPerDir  map[string]PerDirCount // 最近一次构建统计：各根目录
	buildHistory     []BuildRecord          // 最近几次构建记录（最新在前）
	databasePath     string                 // 数据库路径
	configPath       string                 // 配置文件路径
	startTime        time.Time              // 服务启动时间

	// Config (for get-config command)
	config *config.Config

	// contentIndex is the optional in-memory content token index built when
	// config content_index: true. Content searches use it for precise
	// single-word candidates; nil disables it.
	contentIndex *contentpkg.Index

	// Request counters (exposed via status "stats" for light metrics).
	searchCount        atomic.Int64
	contentSearchCount atomic.Int64
	openCount          atomic.Int64
	buildCount         atomic.Int64

	// Runtime hooks (registered by svc) — called WITHOUT s.mu held.
	indexBuiltHook    func(*index.Index)   // after an index build completes and the live index is swapped
	configChangedHook func(*config.Config) // after set-config / reload-config is applied
	searchHook        func()               // on every search request (used to lift boot-time scan throttling)

	// ========== 新增：Message 接口组件 ==========
	parser message.MessageParser
	worker message.MessageWorker
}

// New creates a new server instance.
func New(idx *index.Index) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		socketPath:    config.GetDefaultSocketPath(),
		index:         idx,
		maxConns:      config.DefaultMaxConns,
		connTimeout:   config.DefaultTimeout,
		pathValidator: security.NewPathValidator(nil), // Will be updated when config is set
		ctx:           ctx,
		cancel:        cancel,
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
	s.startTime = time.Now()

	slog.Info("server listening", "path", s.socketPath)

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

	// 注册 reload-config 方法处理器
	s.worker.RegisterMethod("reload-config", message.MethodHandlerFunc(s.handleReloadConfigHandler))

	// 注册 open 方法处理器（浏览器端 H5 通过它打开本地文件/目录）
	s.worker.RegisterMethod("open", message.MethodHandlerFunc(s.handleOpenHandler))

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

	// Cancel any in-flight index build
	if s.cancel != nil {
		s.cancel()
	}

	// 停止 MessageWorker
	if s.worker != nil {
		if err := s.worker.Stop(); err != nil {
			slog.Warn("failed to stop message worker", "error", err)
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
		slog.Info("socket file still in use by another server, not removing", "path", s.socketPath)
	} else {
		// Connection failed, no server is using this socket, safe to remove
		if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove socket file", "error", err)
		}
	}

	slog.Info("server stopped")
	return nil
}

// acceptLoop accepts incoming connections.
func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				slog.Error("accept error", "error", err)
			}
			return
		}

		// 检查最大连接数限制
		s.mu.Lock()
		if s.maxConns > 0 && s.currentConns >= s.maxConns {
			s.mu.Unlock()
			slog.Info("max connections reached, rejecting new connection", "max_conns", s.maxConns)
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

	slog.Debug("handling new connection", "remote_addr", conn.RemoteAddr())

	// Update deadline before reading
	if s.connTimeout > 0 {
		conn.SetDeadline(time.Now().Add(s.connTimeout))
	}

	// Read request using MessageParser
	reader := bufio.NewReader(conn)

	slog.Debug("parsing request using MessageParser")

	// 使用 MessageParser 解析消息
	msg, remainder, err := s.parser.ParseMessage(conn, reader)
	if err != nil {
		slog.Error("failed to parse message", "error", err)
		// 尝试使用旧方式发送错误响应（向后兼容）
		s.sendLegacyError(conn, fmt.Sprintf("failed to parse message: %v", err))
		return
	}

	slog.Debug("message parsed", "id", msg.ID(), "method", msg.Method())

	// 处理粘包：如果有剩余数据，记录日志
	if len(remainder) > 0 {
		slog.Debug("detected sticky packet", "remainder_bytes", len(remainder))
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
		slog.Error("failed to handle message", "error", err)
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
		slog.Error("failed to send error response", "error", err)
		return
	}

	writer.Flush()
}

// ========== 方法处理器（实现 MethodHandler 接口） ==========

// handleSearchHandler 处理搜索请求（Message 接口）
func (s *Server) handleSearchHandler(ctx context.Context, msg message.Message) (any, error) {
	// Notify the daemon that a user is waiting for results, so a throttled
	// boot-time scan can be lifted to full speed (outside the lock).
	if h := s.searchHook; h != nil {
		h()
	}

	s.searchCount.Add(1)

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

	// Content 是文件内容搜索（可选项）
	content := req.Content
	if content != "" && strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("invalid parameter: content cannot be only whitespace")
	}
	if content != "" {
		s.contentSearchCount.Add(1)
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
		IgnoreCase:    req.IgnoreCase,
		Basename:      req.Basename,
		Limit:         req.Limit,
		Offset:        req.Offset,
		PatternMode:   index.PatternMode(req.PatternMode),
		SortField:     req.SortField,
		SortOrder:     req.SortOrder,
		Scope:         req.Scope,
		Exclude:       req.Exclude,
		Types:         req.Types,
		MinSize:       req.MinSize,
		MaxSize:       req.MaxSize,
		MtimeAfter:    req.MtimeAfter,
		MtimeBefore:   req.MtimeBefore,
		ExcludeHidden: req.ExcludeHidden,
		Dedupe:        req.Dedupe,
	}

	// 3.1 未显式指定模式时，按 pattern 是否含通配符元字符自动选择：
	// - 含 * ? [ → wildcard（glob 匹配，如 "test*" 匹配 "test.txt"）
	// - 否则 → normal（子串匹配，locate 核心语义，如 "main" 匹配 "main.go"）
	if opts.PatternMode == "" {
		if strings.ContainsAny(pattern, "*?[") {
			opts.PatternMode = index.PatternModeWildcard
		} else {
			opts.PatternMode = index.PatternModeNormal
		}
	}

	// 3.2 自动判断 Basename
	// 如果 pattern 不包含路径分隔符，则自动设置为 Basename=true（搜索文件名）
	// 这样用户搜索 "main.go" 时会自动搜索文件名，而不是完整路径。
	// 注意：内容搜索与 terms 多词模式不做该推断——路径过滤按全路径匹配
	// （例如 --content keyword internal 应匹配 internal/ 下的所有文件，
	//   --terms "internal client" 应匹配同时含两个词的完整路径）。
	if !opts.Basename && content == "" && opts.PatternMode != index.PatternModeTerms &&
		!strings.Contains(pattern, "/") && !strings.Contains(pattern, "\\") {
		opts.Basename = true
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
			// 注意：regexp.CompilePOSIX 不支持 (?i) 语法
			// 如果需要 IgnoreCase，使用 regexp.Compile 代替
			if opts.IgnoreCase {
				re, err = regexp.Compile("(?i)" + pattern)
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
	slog.Debug("searching for pattern", "pattern", opts.Pattern, "pattern_mode", opts.PatternMode, "ignore_case", opts.IgnoreCase, "basename", opts.Basename)

	// 内容搜索时的候选文件集合
	var candidates []*index.Entry
	if content != "" {
		// 内容搜索时 limit 作用于"内容匹配数"，而非路径候选数：
		// 候选搜索不截断，否则前面的候选不含关键词时结果会被错误地截成 0。
		const maxContentScanFiles = 5000
		if pattern == "" {
			// 纯内容搜索（无路径过滤）：从全量索引取候选，封顶避免扫描全盘。
			// 按修改时间降序排序，让"最近修改"的文件先被扫描——关键词最
			// 可能出现在最新文件中，命中率与直觉都更好。
			candidates = s.index.GetAllEntries()
			newestFirst(candidates)
			// 预建内容索引（content_index: true）：单 token 查询用精确候选。
			// 无命中时保持全量扫描——子串匹配（如 "hello" 命中 token
			// "helloworld"）仍可能成立，不能直接判 0。
			if s.contentIndex != nil && strings.TrimSpace(content) == content && !strings.ContainsAny(content, " \t\n") {
				if paths := s.contentIndex.Lookup(content); len(paths) > 0 {
					byPath := make(map[string]*index.Entry, len(paths))
					for _, e := range candidates {
						byPath[e.Path] = e
					}
					candidates = candidates[:0]
					for _, pth := range paths {
						if e, ok := byPath[pth]; ok {
							candidates = append(candidates, e)
						}
					}
				}
			}
		} else {
			candOpts := opts
			candOpts.Limit = 0
			candidates = s.index.Search(candOpts)
		}
		// 候选集应用 scope/exclude 与元数据过滤（与路径搜索语义一致）
		candidates = index.FilterScopeExclude(candidates, opts.Scope, opts.Exclude)
		candidates = index.FilterMetadata(candidates, opts)
		if len(candidates) > maxContentScanFiles {
			slog.Warn("content search candidate list capped", "total", len(candidates), "scanning", maxContentScanFiles)
			candidates = candidates[:maxContentScanFiles]
		}
	} else {
		candidates = s.index.Search(opts)
	}
	results := candidates
	slog.Debug("found results from path search", "count", len(results))

	// 5. 如果有 Content 参数，进行文件内容搜索
	if content != "" {
		slog.Debug("performing content search", "content", content)

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

		slog.Debug("found content results", "count", len(contentResults))

		// 转换内容搜索结果为响应格式
		resultMap := make(map[string]interface{})
		resultMap["results"] = contentResults
		resultMap["count"] = len(contentResults)
		resultMap["total"] = len(contentResults)

		return resultMap, nil
	}

	// 6. 获取总数（用于分页）
	totalCount := s.index.Count(pattern, index.SearchOptions{
		IgnoreCase:  opts.IgnoreCase,
		Basename:    opts.Basename,
		PatternMode: opts.PatternMode,
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

	// Platform "open" capability (xdg-open / open / rundll32 present) so
	// clients can hide open buttons when the default app is unavailable.
	result := map[string]any{
		"running":          s.running,
		"index_size":       s.index.Len(),
		"pid":              os.Getpid(),
		"protocol_version": protocol.ProtocolVersion,
		"open_supported":   openCommandSupported(),
	}
	if !s.startTime.IsZero() {
		result["uptime"] = time.Since(s.startTime).String()
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
	if s.isBuilding {
		result["build_scanned"] = s.buildScanned
	}

	// Add last build time if available
	if !s.lastBuildTime.IsZero() {
		result["last_build_time"] = s.lastBuildTime.Format(time.RFC3339)
		result["last_build_ago"] = time.Since(s.lastBuildTime).String()
	}
	if s.lastBuildFiles > 0 || s.lastBuildDirs > 0 {
		result["last_build_files"] = s.lastBuildFiles
		result["last_build_dirs"] = s.lastBuildDirs
	}
	if len(s.lastBuildPerDir) > 0 {
		perDir := make(map[string]any, len(s.lastBuildPerDir))
		for dir, c := range s.lastBuildPerDir {
			perDir[dir] = map[string]any{"files": c.Files, "dirs": c.Dirs}
		}
		result["last_build_per_dir"] = perDir
	}
	if len(s.buildHistory) > 0 {
		history := make([]map[string]any, 0, len(s.buildHistory))
		for _, rec := range s.buildHistory {
			history = append(history, map[string]any{
				"time":    rec.Time.Format(time.RFC3339),
				"files":   rec.Files,
				"dirs":    rec.Dirs,
				"elapsed": rec.Elapsed.String(),
			})
		}
		result["build_history"] = history
	}

	// Light request metrics.
	result["stats"] = map[string]any{
		"searches":         s.searchCount.Load(),
		"content_searches": s.contentSearchCount.Load(),
		"opens":            s.openCount.Load(),
		"builds":           s.buildCount.Load(),
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
		"socket_path":           s.config.SocketPath,
		"directories":           s.config.Directories,
		"database_path":         s.config.DatabasePath,
		"ignore_patterns":       s.config.IgnorePatterns,
		"pid_file":              s.config.PIDFile,
		"log_file":              s.config.LogFile,
		"follow_symlinks":       s.config.FollowSymlinks,
		"worker_count":          s.config.WorkerCount,
		"content_search":        s.config.ContentSearch,
		"max_content_file_size": s.config.MaxContentFileSize,
		"index_interval":        s.config.IndexInterval,
		"throttle_index":        s.config.ThrottleIndex,
		"index_strategy":        s.config.IndexStrategy,
		"content_index":         s.config.ContentIndex,
	}

	return result, nil
}

// handleSetConfigHandler 处理设置配置请求（Message 接口）
func (s *Server) handleSetConfigHandler(ctx context.Context, msg message.Message) (any, error) {
	// 解析请求
	var req struct {
		Content string `json:"content,omitempty"`
	}

	if err := json.Unmarshal(msg.Payload(), &req); err != nil {
		return nil, fmt.Errorf("invalid request format: %w", err)
	}

	s.mu.Lock()
	configPath := s.configPath
	s.mu.Unlock()

	// Check if config path is set
	if configPath == "" {
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
	if err := newCfg.Save(configPath); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// Update server config + path validator, then notify svc (outside the lock,
	// because the hook may call back into the server, e.g. StartBuild).
	s.mu.Lock()
	s.config = newCfg
	if len(newCfg.Directories) > 0 {
		s.pathValidator = security.NewPathValidator(newCfg.Directories)
	}
	hook := s.configChangedHook
	s.mu.Unlock()

	if hook != nil {
		hook(newCfg)
	}

	// Return success
	return map[string]any{"status": "saved"}, nil
}

// handleReloadConfigHandler 处理重新加载配置文件请求（Message 接口）
func (s *Server) handleReloadConfigHandler(ctx context.Context, msg message.Message) (any, error) {
	s.mu.Lock()
	configPath := s.configPath
	s.mu.Unlock()

	// Check if config path is set
	if configPath == "" {
		return nil, fmt.Errorf("config file path not set")
	}

	// Read config file from disk
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML config
	newCfg, err := config.LoadFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate config
	if err := newCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply config + notify svc (outside the lock)
	s.mu.Lock()
	s.config = newCfg
	if len(newCfg.Directories) > 0 {
		s.pathValidator = security.NewPathValidator(newCfg.Directories)
	}
	hook := s.configChangedHook
	s.mu.Unlock()

	if hook != nil {
		hook(newCfg)
	}

	return map[string]any{"status": "reloaded"}, nil
}

// StartBuild 启动一次异步索引重建（供 build 方法处理器与 svc 配置变更后调用）。
func (s *Server) StartBuild() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已经在构建中
	if s.isBuilding {
		return fmt.Errorf("index build already in progress")
	}

	// 设置构建状态
	s.isBuilding = true
	s.buildStartTime = time.Now()

	go s.runBuild()
	return nil
}

// runBuild 异步执行索引构建（StartBuild 的 goroutine 主体）。
func (s *Server) runBuild() {
	defer func() {
		s.mu.Lock()
		s.isBuilding = false
		s.mu.Unlock()
	}()

	// 获取配置
	var directories []string
	var workerCount int
	var ignorePatterns []string

	s.mu.Lock()
	if s.config != nil && len(s.config.Directories) > 0 {
		directories = s.config.Directories
		workerCount = s.config.WorkerCount
		ignorePatterns = s.config.IgnorePatterns
	} else {
		// 使用默认目录
		directories = []string{"/"}
		workerCount = 4
		slog.Warn("no directories configured, using default", "directories", directories)
	}
	s.mu.Unlock()

	// 创建 Builder
	opts := index.BuilderOptions{
		WorkerCount:    workerCount,
		IgnorePatterns: ignorePatterns,
	}
	builder := index.NewBuilder(opts)
	builder.SetProgressCallback(func(scanned int64) {
		s.SetBuildProgress(scanned)
	})

	// 构建索引（使用服务生命周期 context，Stop 时可取消）
	slog.Info("starting index build", "directories", directories)
	if err := builder.Build(s.ctx, directories); err != nil {
		if s.ctx.Err() != nil {
			slog.Info("index build cancelled")
		} else {
			slog.Error("index build failed", "error", err)
		}
		return
	}

	// 获取新索引
	newIndex := builder.Index()

	// 更新服务器的索引与状态
	s.mu.Lock()
	s.index = newIndex
	s.indexedFileCount = newIndex.Len()
	s.lastBuildTime = time.Now()
	if f, d := builder.Stats(); f > 0 || d > 0 {
		s.lastBuildFiles = f
		s.lastBuildDirs = d
	}
	if perDir := builder.PerDirStats(); len(perDir) > 0 {
		s.lastBuildPerDir = make(map[string]PerDirCount, len(perDir))
		for dir, c := range perDir {
			s.lastBuildPerDir[dir] = PerDirCount{Files: c.Files, Dirs: c.Dirs}
		}
	}
	hook := s.indexBuiltHook
	s.mu.Unlock()

	// Unify history + build counter through RecordBuild.
	s.RecordBuild(s.lastBuildFiles, s.lastBuildDirs, time.Since(s.buildStartTime))

	slog.Info("index build completed", "count", newIndex.Len())

	// 通知 svc 换掉 updater/持久化（hook 在锁外调用，避免死锁）
	if hook != nil {
		hook(newIndex)
	}
}

// handleBuildHandler 处理构建索引请求（Message 接口）
func (s *Server) handleBuildHandler(ctx context.Context, msg message.Message) (any, error) {
	if err := s.StartBuild(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "build started",
		"directories": func() []string {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.config != nil && len(s.config.Directories) > 0 {
				return s.config.Directories
			}
			return []string{"/"}
		}(),
	}, nil
}

// handleStopHandler 处理停止服务请求（Message 接口）
func (s *Server) handleStopHandler(ctx context.Context, msg message.Message) (any, error) {
	result := "stopping server"

	// 异步停止服务
	go s.Stop()

	return map[string]any{"status": result}, nil
}

// handleOpenHandler 处理打开文件/目录请求（Message 接口）。
// 供浏览器端 H5 使用：浏览器不能直接打开本地路径，由 daemon 代为调用
// 平台默认应用（xdg-open / open / explorer）。路径必须通过白名单校验。
func (s *Server) handleOpenHandler(ctx context.Context, msg message.Message) (any, error) {
	var req struct {
		Path string `json:"content,omitempty"` // fast/json 协议的 content 字段携带路径
	}
	if err := json.Unmarshal(msg.Payload(), &req); err != nil {
		return nil, fmt.Errorf("invalid request format: %w", err)
	}
	if req.Path == "" {
		return nil, fmt.Errorf("open: missing path")
	}

	s.openCount.Add(1)

	s.mu.Lock()
	validator := s.pathValidator
	s.mu.Unlock()
	// 白名单校验：越权路径（不在索引目录内）直接拒绝。
	if validator != nil && !validator.IsPathAllowed(req.Path) {
		slog.Warn("open request rejected by path validator", "path", req.Path)
		return nil, fmt.Errorf("path not allowed: %s", req.Path)
	}

	if _, err := os.Stat(req.Path); err != nil {
		return nil, fmt.Errorf("path does not exist: %s", req.Path)
	}

	if err := openPath(req.Path); err != nil {
		slog.Error("failed to open path", "path", req.Path, "error", err)
		return nil, fmt.Errorf("failed to open %s: %w", req.Path, err)
	}

	slog.Info("opened path via default application", "path", req.Path)
	return map[string]any{"status": "opened", "path": req.Path}, nil
}

// openCommand returns the platform default "open" command name.
func openCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "windows":
		return "rundll32"
	default:
		return "xdg-open"
	}
}

// openCommandSupported reports whether the platform default opener is
// installed on this host (used by the H5 UI to hide open buttons when it is
// not available).
func openCommandSupported() bool {
	_, err := exec.LookPath(openCommand())
	return err == nil
}

// openPath opens a file or directory with the platform's default application.
func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// newestFirst sorts entries by modification time, newest first. Entries with
// a zero ModTime (never stat'd) sort last.
func newestFirst(entries []*index.Entry) {
	sort.Slice(entries, func(i, j int) bool {
		ti, tj := entries[i].ModTime, entries[j].ModTime
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.After(tj)
	})
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
	if isBuilding {
		s.buildScanned = 0
	}
}

// SetBuildProgress updates the number of entries scanned by the in-flight
// index build. Safe to call from the build goroutine.
func (s *Server) SetBuildProgress(scanned int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buildScanned = scanned
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

// SetLastBuildStats records the file/dir totals of the most recent build.
func (s *Server) SetLastBuildStats(files, dirs int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastBuildFiles = files
	s.lastBuildDirs = dirs
}

// SetLastBuildPerDir records per-root-directory totals of the most recent
// build (keyed by the configured root directory).
func (s *Server) SetLastBuildPerDir(perDir map[string]PerDirCount) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastBuildPerDir = perDir
}

// SetConfig sets the config for the server (for get-config command).
func (s *Server) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg

	// Update pathValidator with allowed directories from config
	if cfg != nil && len(cfg.Directories) > 0 {
		s.pathValidator = security.NewPathValidator(cfg.Directories)
	}
}

// SetIndexBuiltHook registers a callback invoked after an index build completes
// and the live index has been swapped. The hook is called without s.mu held.
func (s *Server) SetIndexBuiltHook(fn func(*index.Index)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.indexBuiltHook = fn
}

// SetConfigChangedHook registers a callback invoked after set-config /
// reload-config has been applied. The hook is called without s.mu held.
func (s *Server) SetConfigChangedHook(fn func(*config.Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configChangedHook = fn
}

// SetSearchHook registers a callback invoked on every search request.
// The hook is called without s.mu held and must not block for long.
func (s *Server) SetSearchHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchHook = fn
}

// SetContentIndex installs (or clears, with nil) the content token index used
// for precise content-search candidates.
func (s *Server) SetContentIndex(ix *contentpkg.Index) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contentIndex = ix
}
