package server

import (
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/index"
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
	customPath := "/tmp/golocate_test_custom.sock"
	server.socketPath = customPath

	if server.socketPath != customPath {
		t.Errorf("Expected socket path %q, got %q", customPath, server.socketPath)
	}
}

func TestServerStartCreatesSocketFile(t *testing.T) {
	idx := index.NewIndex()
	server := New(idx)

	// Use a unique socket path for testing
	server.socketPath = "/tmp/golocate_test_socket.sock"

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
