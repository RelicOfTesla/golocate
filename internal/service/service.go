// Package service provides the daemon service implementation.
package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/watcher"
)

// Service represents the daemon service.
type Service struct {
	cfg      *config.Config
	db       *database.DB
	watcher  watcher.Watcher
	updater  *index.Updater
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// Request represents a client request.
type Request struct {
	// Action is the action to perform
	Action string `json:"action"`
	// Query is the search query
	Query string `json:"query"`
	// Options are search options
	Options index.SearchOptions `json:"options"`
}

// Response represents a server response.
type Response struct {
	// Success indicates if the request was successful
	Success bool `json:"success"`
	// Error is the error message if failed
	Error string `json:"error,omitempty"`
	// Results are the search results
	Results []*index.Entry `json:"results,omitempty"`
	// Count is the result count
	Count int `json:"count,omitempty"`
	// Stats are database statistics
	Stats map[string]interface{} `json:"stats,omitempty"`
}

const (
	ActionSearch = "search"
	ActionCount  = "count"
	ActionStats  = "stats"
	ActionPing   = "ping"
)

// NewService creates a new daemon service.
func NewService(cfg *config.Config) (*Service, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Open database
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create watcher
	w, err := watcher.NewWatcher(ctx, &watcher.Config{
		Directories:    cfg.Directories,
		IgnorePatterns: cfg.IgnorePatterns,
		Recursive:      true,
	})
	if err != nil {
		db.Close()
		cancel()
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	// Build initial index
	builder := index.NewBuilder(index.BuilderOptions{
		IgnorePatterns: cfg.IgnorePatterns,
		WorkerCount:    cfg.WorkerCount,
	})

	if err := builder.Build(ctx, cfg.Directories); err != nil {
		w.Close()
		db.Close()
		cancel()
		return nil, fmt.Errorf("failed to build index: %w", err)
	}

	// Save initial index to database
	idx := builder.Index()
	// Note: We would iterate through index entries and save to DB
	// For now, we keep it in memory and use DB for persistence in future

	log.Printf("indexed %d entries", idx.Len())

	return &Service{
		cfg:     cfg,
		db:      db,
		watcher: w,
		updater: index.NewUpdater(idx),
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// Start starts the daemon service.
func (s *Service) Start() error {
	// Remove existing socket
	socketPath := s.cfg.SocketPath
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	s.listener = listener

	// Set socket permissions
	if err := os.Chmod(socketPath, 0600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Write PID file
	if err := s.writePIDFile(); err != nil {
		log.Printf("warning: failed to write PID file: %v", err)
	}

	log.Printf("golocate daemon started on %s", socketPath)
	log.Printf("watcher type: %s", watcher.GetWatcherType())

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptLoop()

	// Start watching file changes
	s.wg.Add(1)
	go s.watchLoop()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("shutting down...")
	s.Stop()

	return nil
}

// Stop stops the daemon service.
func (s *Service) Stop() {
	s.cancel()

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Close watcher
	if s.watcher != nil {
		s.watcher.Close()
	}

	// Close database
	if s.db != nil {
		s.db.Close()
	}

	// Remove PID file
	if s.cfg.PIDFile != "" {
		os.Remove(s.cfg.PIDFile)
	}

	// Remove socket file
	if s.cfg.SocketPath != "" {
		os.Remove(s.cfg.SocketPath)
	}

	s.wg.Wait()
	log.Println("daemon stopped")
}

// acceptLoop accepts incoming connections.
func (s *Service) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			log.Printf("accept error: %v", err)
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a client connection.
func (s *Service) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Read request
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(writer, "invalid request format")
			continue
		}

		// Handle request
		resp := s.handleRequest(&req)

		// Send response
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("failed to marshal response: %v", err)
			continue
		}

		writer.WriteString(string(data) + "\n")
		writer.Flush()
	}
}

// handleRequest handles a client request.
func (s *Service) handleRequest(req *Request) *Response {
	switch req.Action {
	case ActionSearch:
		return s.handleSearch(req)
	case ActionCount:
		return s.handleCount(req)
	case ActionStats:
		return s.handleStats(req)
	case ActionPing:
		return &Response{Success: true}
	default:
		return &Response{
			Success: false,
			Error:   fmt.Sprintf("unknown action: %s", req.Action),
		}
	}
}

// handleSearch handles a search request.
func (s *Service) handleSearch(req *Request) *Response {
	// Search from database
	results, err := s.db.Search(req.Query, req.Options)
	if err != nil {
		return &Response{
			Success: false,
			Error:   err.Error(),
		}
	}
	return &Response{
		Success: true,
		Results: results,
	}
}

// handleCount handles a count request.
func (s *Service) handleCount(req *Request) *Response {
	count, err := s.db.Count(req.Query, req.Options)
	if err != nil {
		return &Response{
			Success: false,
			Error:   err.Error(),
		}
	}
	return &Response{
		Success: true,
		Count:   count,
	}
}

// handleStats handles a stats request.
func (s *Service) handleStats(req *Request) *Response {
	stats, err := s.db.GetStats()
	if err != nil {
		return &Response{
			Success: false,
			Error:   err.Error(),
		}
	}
	return &Response{
		Success: true,
		Stats:   stats,
	}
}

// watchLoop watches for file system changes.
func (s *Service) watchLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case event := <-s.watcher.Events():
			s.updater.HandleEvent(event)
		case err := <-s.watcher.Errors():
			log.Printf("watcher error: %v", err)
		}
	}
}

// sendError sends an error response.
func (s *Service) sendError(writer *bufio.Writer, msg string) {
	resp := &Response{
		Success: false,
		Error:   msg,
	}
	data, _ := json.Marshal(resp)
	writer.WriteString(string(data) + "\n")
	writer.Flush()
}

// writePIDFile writes the PID file.
func (s *Service) writePIDFile() error {
	if s.cfg.PIDFile == "" {
		return nil
	}

	dir := filepath.Dir(s.cfg.PIDFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	pid := os.Getpid()
	return os.WriteFile(s.cfg.PIDFile, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}
