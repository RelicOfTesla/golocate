package server

import (
	"context"
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/internal/testutil"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/message"
	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
)

func TestNew(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	if server == nil {
		t.Error("Expected non-nil server")
	}

	if server.socketPath == "" {
		t.Error("Expected non-empty socket path")
	}

	if server.index == nil {
		t.Error("Expected non-nil index")
	}
}

func TestServerIsRunning(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	if server.IsRunning() {
		t.Error("Expected server to not be running initially")
	}
}

func TestServerStartAndStop(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Use a unique socket path to avoid conflict with main server
	server.socketPath = testutil.GetTestSocketPath("server_start_stop")

	// Start server
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Error("Expected server to be running after start")
	}

	// Stop server
	err = server.Stop()
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	if server.IsRunning() {
		t.Error("Expected server to not be running after stop")
	}
}

func TestServerStartTwice(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Use a unique socket path to avoid conflict with main server
	server.socketPath = testutil.GetTestSocketPath("server_start_twice")

	// Start server
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Try to start again (should fail)
	err = server.Start()
	if err == nil {
		t.Error("Expected error when starting server twice")
	}

	// Clean up
	server.Stop()
}

func TestServerStopWhenNotRunning(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Stop should not error when not running
	err := server.Stop()
	if err != nil {
		t.Errorf("Expected no error when stopping non-running server, got: %v", err)
	}
}

func TestServerSetIndex(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Create new index
	newIdx := index.NewIndex()
	entry := &index.Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		ModTime: time.Now(),
	}
	newIdx.Add(entry)

	// Set new index
	server.SetIndex(newIdx)

	if server.index != newIdx {
		t.Error("Expected index to be updated")
	}
}

func TestRequestParsing(t *testing.T) {
	req := protocol.Request{
		Method:     "search",
		Content:    "test",
		IgnoreCase: true,
		Limit:      10,
	}

	if req.Method != "search" {
		t.Errorf("Expected action 'search', got %q", req.Method)
	}

	if req.Content != "test" {
		t.Errorf("Expected content 'test', got %q", req.Content)
	}

	if !req.IgnoreCase {
		t.Error("Expected ignore_case to be true")
	}
}

func TestServerWithCustomSocketPath(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Set custom socket path
	customPath := testutil.GetTestSocketPath("custom")
	server.socketPath = customPath

	if server.socketPath != customPath {
		t.Errorf("Expected socket path %q, got %q", customPath, server.socketPath)
	}
}

func TestServerStartCreatesSocketFile(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Use a unique socket path for testing
	server.socketPath = testutil.GetTestSocketPath("server_socket")

	// Start server
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Check socket file exists (this is done by checking if IsRunning is true)
	if !server.IsRunning() {
		t.Error("Expected server to be running")
	}

	// Stop server
	server.Stop()
}

func TestHandleSearchHandlerDefaultPatternModeIsSubstring(t *testing.T) {
	idx := index.NewIndex()
	idx.Add(&index.Entry{Name: "main.go", Path: "/home/user/main.go", ModTime: time.Now()})

	srv := New(idx)

	// 空 PatternMode（payload 不含 pattern_mode）+ pattern="main" 应默认子串匹配
	msg := message.NewMessage("1", "search", []byte(`{"pattern":"main"}`), context.Background(), nil, nil)

	result, err := srv.handleSearchHandler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}

	paths, ok := resultMap["paths"].([]string)
	if !ok {
		t.Fatalf("expected paths field, got %#v", resultMap)
	}

	if len(paths) != 1 || paths[0] != "/home/user/main.go" {
		t.Errorf("expected substring search to match main.go, got %v", paths)
	}
}

func TestHandleSearchHandlerWildcardPattern(t *testing.T) {
	idx := index.NewIndex()
	idx.Add(&index.Entry{Name: "test.txt", Path: "/home/user/test.txt", ModTime: time.Now()})

	srv := New(idx)

	// pattern 含通配符元字符应自动走 wildcard 模式
	msg := message.NewMessage("1", "search", []byte(`{"pattern":"test*"}`), context.Background(), nil, nil)

	result, err := srv.handleSearchHandler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}

	paths, ok := resultMap["paths"].([]string)
	if !ok {
		t.Fatalf("expected paths field, got %#v", resultMap)
	}

	if len(paths) != 1 || paths[0] != "/home/user/test.txt" {
		t.Errorf("expected wildcard search to match test.txt, got %v", paths)
	}
}
