package index

import (
	"testing"
	"time"
)

func TestNewIndex(t *testing.T) {
	idx := NewIndex()
	if idx == nil {
		t.Error("Expected non-nil index")
	}
}

func TestIndexAdd(t *testing.T) {
	idx := NewIndex()

	entry := &Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		IsDir:   false,
		Size:    1024,
		ModTime: time.Now(),
	}

	idx.Add(entry)

	if idx.Len() != 1 {
		t.Errorf("Expected count 1, got %d", idx.Len())
	}
}

func TestIndexSearch(t *testing.T) {
	idx := NewIndex()

	// Add test entries
	entries := []*Entry{
		{Name: "test.txt", Path: "/home/user/test.txt", Size: 1024, ModTime: time.Now()},
		{Name: "test2.txt", Path: "/home/user/test2.txt", Size: 2048, ModTime: time.Now()},
		{Name: "other.txt", Path: "/home/user/other.txt", Size: 512, ModTime: time.Now()},
	}

	for _, entry := range entries {
		idx.Add(entry)
	}

	// Test search
	results := idx.Search("test", SearchOptions{IgnoreCase: true})
	if len(results) < 2 {
		t.Errorf("Expected at least 2 results, got %d", len(results))
	}
}

func TestIndexGet(t *testing.T) {
	idx := NewIndex()

	entry := &Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		Size:    1024,
		ModTime: time.Now(),
	}

	idx.Add(entry)

	// Test Get
	retrieved, exists := idx.Get("/home/user/test.txt")
	if !exists {
		t.Error("Expected to retrieve entry")
	}

	if retrieved.Name != "test.txt" {
		t.Errorf("Expected name 'test.txt', got %q", retrieved.Name)
	}
}

func TestIndexGetNonExistent(t *testing.T) {
	idx := NewIndex()

	retrieved, exists := idx.Get("/nonexistent/path")
	if exists {
		t.Error("Expected not to find non-existent entry")
	}
	if retrieved != nil {
		t.Error("Expected nil for non-existent entry")
	}
}

func TestIndexRemove(t *testing.T) {
	idx := NewIndex()

	entry := &Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		Size:    1024,
		ModTime: time.Now(),
	}

	idx.Add(entry)

	if idx.Len() != 1 {
		t.Errorf("Expected count 1, got %d", idx.Len())
	}

	// Remove entry
	idx.Remove("/home/user/test.txt")

	if idx.Len() != 0 {
		t.Errorf("Expected count 0 after remove, got %d", idx.Len())
	}
}

func TestIndexRemoveNonExistent(t *testing.T) {
	idx := NewIndex()

	// Should not panic
	idx.Remove("/nonexistent/path")

	if idx.Len() != 0 {
		t.Errorf("Expected count 0, got %d", idx.Len())
	}
}

func TestIndexSearchIgnoreCase(t *testing.T) {
	idx := NewIndex()

	entries := []*Entry{
		{Name: "Test.txt", Path: "/home/user/Test.txt", ModTime: time.Now()},
		{Name: "TEST.txt", Path: "/home/user/TEST.txt", ModTime: time.Now()},
		{Name: "other.txt", Path: "/home/user/other.txt", ModTime: time.Now()},
	}

	for _, entry := range entries {
		idx.Add(entry)
	}

	// Case-insensitive search
	results := idx.Search("test", SearchOptions{IgnoreCase: true})
	if len(results) != 2 {
		t.Errorf("Expected 2 results (case-insensitive), got %d", len(results))
	}

	// Case-sensitive search
	results = idx.Search("test", SearchOptions{IgnoreCase: false})
	if len(results) != 0 {
		t.Errorf("Expected 0 results (case-sensitive), got %d", len(results))
	}
}

func TestIndexSearchBasename(t *testing.T) {
	idx := NewIndex()

	entries := []*Entry{
		{Name: "test.txt", Path: "/home/user/documents/test.txt", ModTime: time.Now()},
		{Name: "other.txt", Path: "/home/user/test/other.txt", ModTime: time.Now()},
	}

	for _, entry := range entries {
		idx.Add(entry)
	}

	// Basename search - should only match file names, not paths
	results := idx.Search("test", SearchOptions{Basename: true, IgnoreCase: true})
	if len(results) != 1 {
		t.Errorf("Expected 1 result (basename search), got %d", len(results))
	}
}

func TestIndexSearchWithLimit(t *testing.T) {
	idx := NewIndex()

	// Add multiple entries
	for i := 0; i < 100; i++ {
		entry := &Entry{
			Name:    "test.txt",
			Path:    "/home/user/test.txt",
			ModTime: time.Now(),
		}
		idx.Add(entry)
	}

	// Search with limit
	results := idx.Search("", SearchOptions{Limit: 10})
	if len(results) > 10 {
		t.Errorf("Expected at most 10 results, got %d", len(results))
	}
}

func TestIndexSearchRegex(t *testing.T) {
	idx := NewIndex()

	entries := []*Entry{
		{Name: "test1.txt", Path: "/home/user/test1.txt", ModTime: time.Now()},
		{Name: "test2.txt", Path: "/home/user/test2.txt", ModTime: time.Now()},
		{Name: "other.txt", Path: "/home/user/other.txt", ModTime: time.Now()},
	}

	for _, entry := range entries {
		idx.Add(entry)
	}

	// Regex search for test[0-9]+.txt
	results := idx.Search("test[0-9]+", SearchOptions{Regex: true})
	if len(results) != 2 {
		t.Errorf("Expected 2 results (regex), got %d", len(results))
	}
}

func TestIndexSearchExtendedRegex(t *testing.T) {
	idx := NewIndex()

	entries := []*Entry{
		{Name: "test1.txt", Path: "/home/user/test1.txt", ModTime: time.Now()},
		{Name: "test2.txt", Path: "/home/user/test2.txt", ModTime: time.Now()},
		{Name: "other.txt", Path: "/home/user/other.txt", ModTime: time.Now()},
	}

	for _, entry := range entries {
		idx.Add(entry)
	}

	// Extended regex search
	results := idx.Search("test[0-9]+", SearchOptions{ExtendedRegex: true})
	if len(results) != 2 {
		t.Errorf("Expected 2 results (extended regex), got %d", len(results))
	}
}

func TestIndexCount(t *testing.T) {
	idx := NewIndex()

	entries := []*Entry{
		{Name: "test.txt", Path: "/home/user/test.txt", ModTime: time.Now()},
		{Name: "test2.txt", Path: "/home/user/test2.txt", ModTime: time.Now()},
		{Name: "other.txt", Path: "/home/user/other.txt", ModTime: time.Now()},
	}

	for _, entry := range entries {
		idx.Add(entry)
	}

	count := idx.Count("test", SearchOptions{IgnoreCase: true})
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func TestIndexLen(t *testing.T) {
	idx := NewIndex()

	if idx.Len() != 0 {
		t.Errorf("Expected initial length 0, got %d", idx.Len())
	}

	// Add entries with different paths
	for i := 0; i < 10; i++ {
		entry := &Entry{
			Name:    "test.txt",
			Path:    "/home/user/test.txt",
			ModTime: time.Now(),
		}
		entry.Path = entry.Path[:len("/home/user/")] + string(rune('0'+i)) + ".txt"
		entry.Name = string(rune('0'+i)) + ".txt"
		idx.Add(entry)
	}

	if idx.Len() != 10 {
		t.Errorf("Expected length 10, got %d", idx.Len())
	}
}

func TestIndexUpdateExisting(t *testing.T) {
	idx := NewIndex()

	entry := &Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		Size:    1024,
		ModTime: time.Now(),
	}
	idx.Add(entry)

	if idx.Len() != 1 {
		t.Errorf("Expected length 1, got %d", idx.Len())
	}

	// Update with same path
	updated := &Entry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		Size:    2048, // Changed size
		ModTime: time.Now(),
	}
	idx.Add(updated)

	// Should still have 1 entry
	if idx.Len() != 1 {
		t.Errorf("Expected length 1 after update, got %d", idx.Len())
	}

	// Check updated size
	retrieved, _ := idx.Get("/home/user/test.txt")
	if retrieved.Size != 2048 {
		t.Errorf("Expected updated size 2048, got %d", retrieved.Size)
	}
}

func TestNewBuilder(t *testing.T) {
	opts := BuilderOptions{
		IgnorePatterns: []string{"*.log"},
		WorkerCount:    4,
	}

	builder := NewBuilder(opts)
	if builder == nil {
		t.Error("Expected non-nil builder")
	}

	if builder.idx == nil {
		t.Error("Expected builder to have an index")
	}
}

func TestNewBuilderDefaults(t *testing.T) {
	builder := NewBuilder(BuilderOptions{})
	if builder == nil {
		t.Error("Expected non-nil builder")
	}

	if builder.workerCount <= 0 {
		t.Errorf("Expected positive worker count, got %d", builder.workerCount)
	}
}

func TestBuilderIndex(t *testing.T) {
	builder := NewIndex()
	if builder == nil {
		t.Error("Expected non-nil builder")
	}
}

func TestNewUpdater(t *testing.T) {
	idx := NewIndex()
	updater := NewUpdater(idx)
	if updater == nil {
		t.Error("Expected non-nil updater")
	}
}

func TestSearchOptionsDefaults(t *testing.T) {
	opts := SearchOptions{}
	if opts.IgnoreCase != false {
		t.Error("Expected IgnoreCase to default to false")
	}
	if opts.Basename != false {
		t.Error("Expected Basename to default to false")
	}
	if opts.Limit != 0 {
		t.Error("Expected Limit to default to 0")
	}
}
