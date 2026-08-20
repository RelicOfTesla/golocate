package search

import (
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/index"
)

func TestParseSortField(t *testing.T) {
	tests := []struct {
		input    string
		expected SortField
	}{
		{"name", SortByName},
		{"n", SortByName},
		{"NAME", SortByName},
		{"name-case", SortByNameCaseSensitive},
		{"nc", SortByNameCaseSensitive},
		{"size", SortBySize},
		{"s", SortBySize},
		{"time", SortByModTime},
		{"t", SortByModTime},
		{"mtime", SortByModTime},
		{"date", SortByModTime},
		{"path", SortByPath},
		{"p", SortByPath},
		{"invalid", SortNone},
		{"", SortNone},
	}

	for _, test := range tests {
		result := ParseSortField(test.input)
		if result != test.expected {
			t.Errorf("ParseSortField(%q): expected %v, got %v", test.input, test.expected, result)
		}
	}
}

func TestSortFieldString(t *testing.T) {
	tests := []struct {
		field    SortField
		expected string
	}{
		{SortByName, "name"},
		{SortByNameCaseSensitive, "name-case"},
		{SortBySize, "size"},
		{SortByModTime, "time"},
		{SortByPath, "path"},
		{SortNone, "none"},
	}

	for _, test := range tests {
		result := test.field.String()
		if result != test.expected {
			t.Errorf("SortField.String(): expected %q, got %q", test.expected, result)
		}
	}
}

func TestSortOrderString(t *testing.T) {
	if OrderAsc.String() != "asc" {
		t.Errorf("Expected 'asc', got %q", OrderAsc.String())
	}
	if OrderDesc.String() != "desc" {
		t.Errorf("Expected 'desc', got %q", OrderDesc.String())
	}
}

func TestSort(t *testing.T) {
	now := time.Now()
	entries := []*index.Entry{
		{Name: "zzz.txt", Path: "/home/user/zzz.txt", Size: 4096, ModTime: now.Add(2 * time.Hour)},
		{Name: "aaa.txt", Path: "/home/user/aaa.txt", Size: 1024, ModTime: now},
		{Name: "BBB.txt", Path: "/home/user/BBB.txt", Size: 2048, ModTime: now.Add(1 * time.Hour)},
	}

	// Test sort by name ascending (case-insensitive)
	Sort(entries, SortOptions{Field: SortByName, Order: OrderAsc})
	if entries[0].Name != "aaa.txt" {
		t.Errorf("Expected first result 'aaa.txt', got %q", entries[0].Name)
	}
	if entries[2].Name != "zzz.txt" {
		t.Errorf("Expected last result 'zzz.txt', got %q", entries[2].Name)
	}

	// Test sort by name descending
	Sort(entries, SortOptions{Field: SortByName, Order: OrderDesc})
	if entries[0].Name != "zzz.txt" {
		t.Errorf("Expected first result 'zzz.txt', got %q", entries[0].Name)
	}

	// Test sort by size ascending
	Sort(entries, SortOptions{Field: SortBySize, Order: OrderAsc})
	if entries[0].Size != 1024 {
		t.Errorf("Expected first result size 1024, got %d", entries[0].Size)
	}
	if entries[2].Size != 4096 {
		t.Errorf("Expected last result size 4096, got %d", entries[2].Size)
	}

	// Test sort by size descending
	Sort(entries, SortOptions{Field: SortBySize, Order: OrderDesc})
	if entries[0].Size != 4096 {
		t.Errorf("Expected first result size 4096, got %d", entries[0].Size)
	}

	// Test sort by mod time ascending
	Sort(entries, SortOptions{Field: SortByModTime, Order: OrderAsc})
	if entries[0].Name != "aaa.txt" {
		t.Errorf("Expected first result 'aaa.txt' (oldest), got %q", entries[0].Name)
	}

	// Test sort by mod time descending
	Sort(entries, SortOptions{Field: SortByModTime, Order: OrderDesc})
	if entries[0].Name != "zzz.txt" {
		t.Errorf("Expected first result 'zzz.txt' (newest), got %q", entries[0].Name)
	}

	// Test sort by path
	Sort(entries, SortOptions{Field: SortByPath, Order: OrderAsc})
	if entries[0].Path != "/home/user/BBB.txt" && entries[0].Path != "/home/user/aaa.txt" {
		t.Errorf("Expected sorted path, got %q", entries[0].Path)
	}
}

func TestSortByNameCaseSensitive(t *testing.T) {
	entries := []*index.Entry{
		{Name: "b.txt", Path: "/b.txt"},
		{Name: "A.txt", Path: "/A.txt"},
		{Name: "a.txt", Path: "/a.txt"},
	}

	Sort(entries, SortOptions{Field: SortByNameCaseSensitive, Order: OrderAsc})
	// Case-sensitive: 'A' < 'a' < 'b'
	if entries[0].Name != "A.txt" {
		t.Errorf("Expected 'A.txt' first (case-sensitive), got %q", entries[0].Name)
	}
}

func TestSortEmpty(t *testing.T) {
	entries := []*index.Entry{}

	// Should not panic
	Sort(entries, SortOptions{Field: SortByName, Order: OrderAsc})
}

func TestSortNone(t *testing.T) {
	entries := []*index.Entry{
		{Name: "zzz.txt", Path: "/zzz.txt"},
		{Name: "aaa.txt", Path: "/aaa.txt"},
	}

	original := entries[0].Name
	Sort(entries, SortOptions{Field: SortNone, Order: OrderAsc})
	// Should not change order
	if entries[0].Name != original {
		t.Errorf("Sort with SortNone should not change order")
	}
}

func TestSortByMultiple(t *testing.T) {
	now := time.Now()
	entries := []*index.Entry{
		{Name: "file.txt", Path: "/a/file.txt", Size: 100, ModTime: now},
		{Name: "file.txt", Path: "/b/file.txt", Size: 200, ModTime: now.Add(1 * time.Hour)},
		{Name: "file.txt", Path: "/c/file.txt", Size: 50, ModTime: now.Add(2 * time.Hour)},
	}

	// Sort by name first, then by size
	SortByMultiple(entries, []SortOptions{
		{Field: SortByName, Order: OrderAsc},
		{Field: SortBySize, Order: OrderAsc},
	})

	// All have same name, so should be sorted by size
	if entries[0].Size != 50 {
		t.Errorf("Expected size 50 first, got %d", entries[0].Size)
	}
}

func TestParseSort(t *testing.T) {
	tests := []struct {
		input     string
		wantField SortField
		wantOrder SortOrder
	}{
		{"name:asc", SortByName, OrderAsc},
		{"name:desc", SortByName, OrderDesc},
		{"size:desc", SortBySize, OrderDesc},
		{"time:asc", SortByModTime, OrderAsc},
		{"path", SortByPath, OrderAsc}, // default order
		{"", SortByName, OrderAsc},     // default
	}

	for _, test := range tests {
		opts := ParseSort(test.input)
		if opts.Field != test.wantField {
			t.Errorf("ParseSort(%q): expected field %v, got %v", test.input, test.wantField, opts.Field)
		}
		if opts.Order != test.wantOrder {
			t.Errorf("ParseSort(%q): expected order %v, got %v", test.input, test.wantOrder, opts.Order)
		}
	}
}

func TestDefaultSortOptions(t *testing.T) {
	opts := DefaultSortOptions()
	if opts.Field != SortByName {
		t.Errorf("Expected default field SortByName, got %v", opts.Field)
	}
	if opts.Order != OrderAsc {
		t.Errorf("Expected default order OrderAsc, got %v", opts.Order)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, test := range tests {
		result := FormatSize(test.size)
		if result != test.expected {
			t.Errorf("FormatSize(%d): expected %q, got %q", test.size, test.expected, result)
		}
	}
}

func TestFormatModTime(t *testing.T) {
	// Test with a known time
	tm := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := FormatModTime(tm)
	expected := "2024-01-15 10:30:00"
	if result != expected {
		t.Errorf("FormatModTime: expected %q, got %q", expected, result)
	}
}

func TestCompareByField(t *testing.T) {
	now := time.Now()
	a := &index.Entry{Name: "apple.txt", Path: "/a/apple.txt", Size: 100, ModTime: now}
	b := &index.Entry{Name: "banana.txt", Path: "/b/banana.txt", Size: 200, ModTime: now.Add(1 * time.Hour)}

	// Compare by name
	if cmp := compareByField(a, b, SortByName); cmp >= 0 {
		t.Errorf("Expected a < b by name")
	}

	// Compare by size
	if cmp := compareByField(a, b, SortBySize); cmp >= 0 {
		t.Errorf("Expected a < b by size")
	}

	// Compare by mod time
	if cmp := compareByField(a, b, SortByModTime); cmp >= 0 {
		t.Errorf("Expected a < b by mod time")
	}

	// Compare by path
	if cmp := compareByField(a, b, SortByPath); cmp >= 0 {
		t.Errorf("Expected a < b by path")
	}

	// Compare by none
	if cmp := compareByField(a, b, SortNone); cmp != 0 {
		t.Errorf("Expected 0 for SortNone")
	}
}
