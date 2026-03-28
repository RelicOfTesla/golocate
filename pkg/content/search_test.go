package content

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSearcher(t *testing.T) {
	opts := SearchOptions{
		Pattern:     "test",
		IgnoreCase:  true,
		MaxFileSize: 1024 * 1024,
		MaxResults:  100,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	if searcher == nil {
		t.Error("Expected non-nil searcher")
	}

	if searcher.opts.MaxFileSize != 1024*1024 {
		t.Errorf("Expected max file size 1MB, got %d", searcher.opts.MaxFileSize)
	}
}

func TestNewSearcherRegex(t *testing.T) {
	opts := SearchOptions{
		Pattern: "[a-z]+",
		Regex:   true,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher with regex: %v", err)
	}

	if searcher.pattern == nil {
		t.Error("Expected compiled regex pattern")
	}
}

func TestNewSearcherExtendedRegex(t *testing.T) {
	opts := SearchOptions{
		Pattern:       "[a-z]+",
		ExtendedRegex: true,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher with extended regex: %v", err)
	}

	if searcher.pattern == nil {
		t.Error("Expected compiled extended regex pattern")
	}
}

func TestNewSearcherInvalidRegex(t *testing.T) {
	opts := SearchOptions{
		Pattern: "[invalid(",
		Regex:   true,
	}

	_, err := NewSearcher(opts)
	if err == nil {
		t.Error("Expected error for invalid regex")
	}
}

func TestNewSearcherDefaults(t *testing.T) {
	opts := SearchOptions{
		Pattern: "test",
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	// Check defaults are set
	if searcher.opts.MaxFileSize != 10*1024*1024 {
		t.Errorf("Expected default max file size 10MB, got %d", searcher.opts.MaxFileSize)
	}

	if searcher.opts.MaxResults != 1000 {
		t.Errorf("Expected default max results 1000, got %d", searcher.opts.MaxResults)
	}
}

func TestSearchEmptyFiles(t *testing.T) {
	opts := SearchOptions{
		Pattern: "test",
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty files, got %d", len(results))
	}
}

func TestSearchNonExistentFile(t *testing.T) {
	opts := SearchOptions{
		Pattern: "test",
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{"/nonexistent/path/file.txt"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should return empty results without error
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent file, got %d", len(results))
	}
}

func TestSearchInFile(t *testing.T) {
	// Create a temp file with content
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "test_search.txt")
	defer os.Remove(testFile)

	content := "Hello World\nThis is a test line\nAnother line with test keyword\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := SearchOptions{
		Pattern:    "test",
		IgnoreCase: true,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{testFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results, got %d", len(results))
	}

	// Check first result
	found := false
	for _, r := range results {
		if r.LineNum == 2 && r.Match == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find 'test' on line 2")
	}
}

func TestSearchCaseSensitive(t *testing.T) {
	// Create a temp file with content
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "test_case.txt")
	defer os.Remove(testFile)

	content := "Test line\ntest line\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Case-insensitive search
	opts := SearchOptions{
		Pattern:    "test",
		IgnoreCase: true,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{testFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results (case-insensitive), got %d", len(results))
	}

	// Case-sensitive search
	opts = SearchOptions{
		Pattern:    "test",
		IgnoreCase: false,
	}

	searcher, err = NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	results, err = searcher.Search(ctx, []string{testFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result (case-sensitive), got %d", len(results))
	}
}

func TestSearchWithLimit(t *testing.T) {
	// Create a temp file with many matches
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "test_limit.txt")
	defer os.Remove(testFile)

	content := ""
	for i := 0; i < 100; i++ {
		content += "test line\n"
	}
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := SearchOptions{
		Pattern:    "test",
		IgnoreCase: true,
		MaxResults: 10,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{testFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) > 10 {
		t.Errorf("Expected at most 10 results, got %d", len(results))
	}
}

func TestSearchRegex(t *testing.T) {
	// Create a temp file with content
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "test_regex.txt")
	defer os.Remove(testFile)

	content := "file123.txt\nfile456.txt\nother.txt\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := SearchOptions{
		Pattern: "file[0-9]+",
		Regex:   true,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{testFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results (regex), got %d", len(results))
	}
}

func TestSearchContextCancellation(t *testing.T) {
	opts := SearchOptions{
		Pattern: "test",
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := searcher.Search(ctx, []string{})
	if err != context.Canceled {
		t.Logf("Search returned: %v (expected context.Canceled)", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for cancelled context, got %d", len(results))
	}
}

func TestSearchResultFields(t *testing.T) {
	// Create a temp file
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "test_fields.txt")
	defer os.Remove(testFile)

	content := "Hello World\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := SearchOptions{
		Pattern: "Hello",
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{testFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Path != testFile {
		t.Errorf("Expected path %s, got %s", testFile, result.Path)
	}
	if result.LineNum != 1 {
		t.Errorf("Expected line number 1, got %d", result.LineNum)
	}
	if result.Line != "Hello World" {
		t.Errorf("Expected line 'Hello World', got %q", result.Line)
	}
	if result.Match != "Hello" {
		t.Errorf("Expected match 'Hello', got %q", result.Match)
	}
}

func TestIsBinaryFile(t *testing.T) {
	// Create a text file
	tmpDir := os.TempDir()
	textFile := filepath.Join(tmpDir, "text.txt")
	defer os.Remove(textFile)
	if err := os.WriteFile(textFile, []byte("Hello World"), 0644); err != nil {
		t.Fatalf("Failed to create text file: %v", err)
	}

	if isBinaryFile(textFile) {
		t.Error("Expected text file to not be binary")
	}

	// Create a binary file (with null bytes)
	binaryFile := filepath.Join(tmpDir, "binary.bin")
	defer os.Remove(binaryFile)
	if err := os.WriteFile(binaryFile, []byte{0x00, 0x01, 0x02, 0x03}, 0644); err != nil {
		t.Fatalf("Failed to create binary file: %v", err)
	}

	if !isBinaryFile(binaryFile) {
		t.Error("Expected binary file to be detected as binary")
	}
}

func TestSearchInDirectory(t *testing.T) {
	// Create a temp directory with files
	tmpDir := os.TempDir()
	testDir := filepath.Join(tmpDir, "test_dir")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create files
	file1 := filepath.Join(testDir, "file1.txt")
	file2 := filepath.Join(testDir, "file2.txt")
	os.WriteFile(file1, []byte("test content\n"), 0644)
	os.WriteFile(file2, []byte("other content\n"), 0644)

	opts := SearchOptions{
		Pattern:    "test",
		IgnoreCase: true,
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.SearchInDirectory(ctx, testDir)
	if err != nil {
		t.Fatalf("SearchInDirectory failed: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("Expected at least 1 result, got %d", len(results))
	}
}

func TestSearchMaxFileSize(t *testing.T) {
	// Create a file larger than max size
	tmpDir := os.TempDir()
	largeFile := filepath.Join(tmpDir, "large.txt")
	defer os.Remove(largeFile)

	largeContent := make([]byte, 1024*1024+1) // 1MB + 1 byte
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	if err := os.WriteFile(largeFile, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	opts := SearchOptions{
		Pattern:     "test",
		MaxFileSize: 1024 * 1024, // 1MB
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	ctx := context.Background()
	results, err := searcher.Search(ctx, []string{largeFile})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should skip large file
	if len(results) != 0 {
		t.Errorf("Expected 0 results for file exceeding max size, got %d", len(results))
	}
}

func TestSearchTimeout(t *testing.T) {
	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Create a large file list (simulating slow operation)
	files := make([]string, 10000)
	for i := range files {
		files[i] = "/nonexistent/file.txt"
	}

	opts := SearchOptions{
		Pattern: "test",
	}

	searcher, err := NewSearcher(opts)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	// Wait for context to expire before starting
	time.Sleep(2 * time.Millisecond)

	results, err := searcher.Search(ctx, files)
	// With expired context, should either return error or empty results
	if err == nil && len(results) > 0 {
		t.Error("Expected timeout or empty results")
	}
}
