package cliclient

import (
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/internal/testutil"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

func TestSearchOptionsDefaults(t *testing.T) {
	opts := SearchOptions{}

	if opts.Pattern != "" {
		t.Errorf("Expected empty pattern, got %q", opts.Pattern)
	}

	if opts.Limit != 0 {
		t.Errorf("Expected limit 0, got %d", opts.Limit)
	}
}

func TestSearchResultFields(t *testing.T) {
	result := &SearchResult{
		Entries: []*index.Entry{
			{Name: "test.txt", Path: "/home/user/test.txt"},
		},
		Count: 1,
	}

	if len(result.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(result.Entries))
	}

	if result.Count != 1 {
		t.Errorf("Expected count 1, got %d", result.Count)
	}
}

func TestIsServerRunning(t *testing.T) {
	// Test without server
	running := IsServerRunning(testutil.GetNonExistentSocketPath())
	if running {
		t.Error("Expected IsServerRunning to return false for non-existent socket")
	}
}

func TestIsServerRunningWithServer(t *testing.T) {
	// Create and start server
	idx := index.NewIndex()
	srv := server.New(idx)
	srv.SetSocketPath(testutil.GetTestSocketPath("cli"))

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Check if server is running
	running := IsServerRunning(testutil.GetTestSocketPath("cli"))
	if !running {
		t.Error("Expected IsServerRunning to return true for running server")
	}
}

func TestSearchWithoutServer(t *testing.T) {
	opts := SearchOptions{
		Pattern:    "test",
		IgnoreCase: true,
		SocketPath: testutil.GetNonExistentSocketPath(),
	}

	_, err := Search(opts)
	if err == nil {
		t.Error("Expected error when searching without server")
	}
}

func TestSearchWithServer(t *testing.T) {
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
	srv.SetSocketPath(testutil.GetTestSocketPath("cli_search"))

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Perform search
	opts := SearchOptions{
		Pattern:    "test",
		IgnoreCase: true,
		SocketPath: testutil.GetTestSocketPath("cli_search"),
	}

	result, err := Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Entries) < 1 {
		t.Errorf("Expected at least 1 result, got %d", len(result.Entries))
	}
}

func TestStatusWithoutServer(t *testing.T) {
	_, err := Status(testutil.GetNonExistentSocketPath())
	if err == nil {
		t.Error("Expected error when getting status without server")
	}
}

func TestStatusWithServer(t *testing.T) {
	// Create and start server
	idx := index.NewIndex()
	srv := server.New(idx)
	srv.SetSocketPath(testutil.GetTestSocketPath("cli_status"))

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Get status
	status, err := Status(testutil.GetTestSocketPath("cli_status"))
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status == nil {
		t.Error("Expected non-nil status")
	}
}

func TestBuildWithoutServer(t *testing.T) {
	err := Build(testutil.GetNonExistentSocketPath())
	if err == nil {
		t.Error("Expected error when building without server")
	}
}

func TestPrintResults(t *testing.T) {
	result := &SearchResult{
		Entries: []*index.Entry{
			{Name: "test.txt", Path: "/home/user/test.txt"},
		},
		Count: 1,
	}

	// Test with count only
	PrintResults(result, true)

	// Test with full output
	PrintResults(result, false)
}

func TestSearchStreamWithoutServer(t *testing.T) {
	opts := SearchOptions{
		Pattern:    "test",
		SocketPath: testutil.GetNonExistentSocketPath(),
	}

	err := SearchStream(opts, func(e *index.Entry) bool {
		return true
	})
	if err == nil {
		t.Error("Expected error when streaming search without server")
	}
}

func TestSearchStreamWithServer(t *testing.T) {
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
	srv.SetSocketPath(testutil.GetTestSocketPath("cli_stream"))

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	// Stream search
	opts := SearchOptions{
		Pattern:    "",
		SocketPath: testutil.GetTestSocketPath("cli_stream"),
		Limit:      10,
	}

	count := 0
	err := SearchStream(opts, func(e *index.Entry) bool {
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

func TestFatal(t *testing.T) {
	// Fatal logs and exits - we can't easily test the exit behavior
	// Just ensure it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Log("Fatal caused panic (expected behavior)")
		}
	}()

	// Note: This will call os.Exit(1), which we can't test without subprocess
	// Skipping actual Fatal test
}

func TestSearchOptionsWithSort(t *testing.T) {
	opts := SearchOptions{
		Pattern: "test",
		Sort:    "name:asc",
	}

	if opts.Sort != "name:asc" {
		t.Errorf("Expected sort 'name:asc', got %q", opts.Sort)
	}
}

func TestSearchOptionsWithRegex(t *testing.T) {
	opts := SearchOptions{
		Pattern: "[a-z]+",
		Regex:   true,
	}

	if !opts.Regex {
		t.Error("Expected Regex to be true")
	}
}

func TestSearchOptionsWithRegexp(t *testing.T) {
	opts := SearchOptions{
		Pattern: "[a-z]+",
		Regexp:  true,
	}

	if !opts.Regexp {
		t.Error("Expected Regexp to be true")
	}
}

func TestSearchOptionsWithCount(t *testing.T) {
	opts := SearchOptions{
		Pattern: "test",
		Count:   true,
	}

	if !opts.Count {
		t.Error("Expected Count to be true")
	}
}
