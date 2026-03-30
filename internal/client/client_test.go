package client

import (
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

func TestNew(t *testing.T) {
	client := New()

	if client == nil {
		t.Error("Expected non-nil client")
	}

	if client.socketPath == "" {
		t.Error("Expected non-empty socket path")
	}

	if client.timeout == 0 {
		t.Error("Expected non-zero timeout")
	}
}

func TestClientSetSocketPath(t *testing.T) {
	client := New()

	customPath := "/tmp/custom.sock"
	client.SetSocketPath(customPath)

	if client.socketPath != customPath {
		t.Errorf("Expected socket path %q, got %q", customPath, client.socketPath)
	}
}

func TestClientSetTimeout(t *testing.T) {
	client := New()

	customTimeout := 10 * time.Second
	client.SetTimeout(customTimeout)

	if client.timeout != customTimeout {
		t.Errorf("Expected timeout %v, got %v", customTimeout, client.timeout)
	}
}

func TestClientIsServerRunning(t *testing.T) {
	client := New()
	client.SetSocketPath("/tmp/nonexistent_socket_for_test.sock")

	// Should return false for non-existent socket
	if client.IsServerRunning() {
		t.Error("Expected IsServerRunning to return false for non-existent socket")
	}
}

func TestClientIsServerRunningWithServer(t *testing.T) {
	// Create and start server
	idx := index.NewIndex()
	srv := server.New(idx)
	srv.SetSocketPath("/tmp/golocate_client_test.sock")

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Create client with matching socket path
	client := New()
	client.SetSocketPath("/tmp/golocate_client_test.sock")

	// Should return true for running server
	if !client.IsServerRunning() {
		t.Error("Expected IsServerRunning to return true for running server")
	}
}

func TestClientSearchWithoutServer(t *testing.T) {
	client := New()
	client.SetSocketPath("/tmp/nonexistent_socket_for_test.sock")

	_, err := client.Search("test", index.SearchOptions{Path: "*"})
	if err == nil {
		t.Error("Expected error when searching without server")
	}
}

func TestClientSearchWithServer(t *testing.T) {
	// Create index with test data
	idx := index.NewIndex()
	entry := &index.Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		Size:    1024,
		ModTime: time.Now(),
	}
	idx.Add(entry)

	// Create and start server
	srv := server.New(idx)
	srv.SetSocketPath("/tmp/golocate_client_search_test.sock")

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Create client
	client := New()
	client.SetSocketPath("/tmp/golocate_client_search_test.sock")

	// Perform search (PATH is required, CONTENT is optional)
	results, err := client.Search("test", index.SearchOptions{Path: "*", IgnoreCase: true})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("Expected at least 1 result, got %d", len(results))
	}
}

func TestClientStatusWithoutServer(t *testing.T) {
	client := New()
	client.SetSocketPath("/tmp/nonexistent_socket_for_test.sock")

	_, err := client.Status()
	if err == nil {
		t.Error("Expected error when getting status without server")
	}
}

func TestClientStatusWithServer(t *testing.T) {
	// Create and start server
	idx := index.NewIndex()
	srv := server.New(idx)
	srv.SetSocketPath("/tmp/golocate_client_status_test.sock")

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Create client
	client := New()
	client.SetSocketPath("/tmp/golocate_client_status_test.sock")

	// Get status
	status, err := client.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status == nil {
		t.Error("Expected non-nil status")
	}
}

func TestClientBuildWithoutServer(t *testing.T) {
	client := New()
	client.SetSocketPath("/tmp/nonexistent_socket_for_test.sock")

	err := client.Build()
	if err == nil {
		t.Error("Expected error when building without server")
	}
}

func TestClientUpdateDB(t *testing.T) {
	client := New()

	// UpdateDB is an alias for Build
	err := client.UpdateDB()
	// Will fail because no server is running, but the method should exist
	if err == nil {
		t.Log("UpdateDB succeeded (server running)")
	} else {
		t.Log("UpdateDB failed (no server), which is expected")
	}
}

func TestClientSearchStreamWithoutServer(t *testing.T) {
	client := New()
	client.SetSocketPath("/tmp/nonexistent_socket_for_test.sock")

	err := client.SearchStream("test", index.SearchOptions{Path: "*"}, func(e *index.Entry) bool {
		return true
	})
	if err == nil {
		t.Error("Expected error when streaming search without server")
	}
}

func TestClientSearchStreamWithServer(t *testing.T) {
	// Create index with test data
	idx := index.NewIndex()
	for i := 0; i < 5; i++ {
		entry := &index.Entry{
			Name:    "test.txt",
			Path:    "/home/user/test.txt",
			ModTime: time.Now(),
		}
		entry.Path = entry.Path[:len("/home/user/")] + string(rune('0'+i)) + ".txt"
		entry.Name = string(rune('0'+i)) + ".txt"
		idx.Add(entry)
	}

	// Create and start server
	srv := server.New(idx)
	srv.SetSocketPath("/tmp/golocate_client_stream_test.sock")

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Create client
	client := New()
	client.SetSocketPath("/tmp/golocate_client_stream_test.sock")

	// Stream search
	count := 0
	err := client.SearchStream("", index.SearchOptions{Path: "*", Limit: 10}, func(e *index.Entry) bool {
		count++
		return true
	})
	if err != nil {
		t.Fatalf("SearchStream failed: %v", err)
	}

	if count < 1 {
		t.Errorf("Expected at least 1 result, got %d", count)
	}
}

func TestClientSearchStreamStop(t *testing.T) {
	// Create index with test data
	idx := index.NewIndex()
	for i := 0; i < 10; i++ {
		entry := &index.Entry{
			Name:    "test.txt",
			Path:    "/home/user/test.txt",
			ModTime: time.Now(),
		}
		entry.Path = entry.Path[:len("/home/user/")] + string(rune('0'+i)) + ".txt"
		entry.Name = string(rune('0'+i)) + ".txt"
		idx.Add(entry)
	}

	// Create and start server
	srv := server.New(idx)
	srv.SetSocketPath("/tmp/golocate_client_stream_stop_test.sock")

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Create client
	client := New()
	client.SetSocketPath("/tmp/golocate_client_stream_stop_test.sock")

	// Stream search and stop after first result
	count := 0
	err := client.SearchStream("", index.SearchOptions{Path: "*"}, func(e *index.Entry) bool {
		count++
		return false // Stop streaming
	})
	if err != nil {
		t.Fatalf("SearchStream failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected exactly 1 result (stopped), got %d", count)
	}
}

func TestRequestDefaults(t *testing.T) {
	req := Request{
		Method:  "search",
		Content: "test",
	}

	if req.Method != "search" {
		t.Errorf("Expected action 'search', got %q", req.Method)
	}

	if req.Content != "test" {
		t.Errorf("Expected content 'test', got %q", req.Content)
	}
}

func TestResponseDefaults(t *testing.T) {
	resp := Response{}

	if resp.Type != "" {
		t.Errorf("Expected empty type, got %q", resp.Type)
	}

	if resp.Error != "" {
		t.Errorf("Expected empty error, got %q", resp.Error)
	}
}
