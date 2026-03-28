package patricia

import (
	"testing"
)

func TestTrieInsert(t *testing.T) {
	trie := NewTrie()

	entry := &Entry{
		Path: "/home/user/test.txt",
		Name: "test.txt",
		Size: 1024,
	}

	trie.Insert(entry.Path, entry)

	if trie.EntryCount() != 1 {
		t.Errorf("Expected 1 entry, got %d", trie.EntryCount())
	}
}

func TestTrieSearch(t *testing.T) {
	trie := NewTrie()

	entries := []*Entry{
		{Path: "/home/user/test.txt", Name: "test.txt", Size: 1024},
		{Path: "/home/user/test2.txt", Name: "test2.txt", Size: 2048},
		{Path: "/home/user/dir/test3.txt", Name: "test3.txt", Size: 512},
		{Path: "/var/log/test.log", Name: "test.log", Size: 4096},
	}

	for _, entry := range entries {
		trie.Insert(entry.Path, entry)
	}

	// Test prefix search
	results := trie.Search("/home/user")
	if len(results) != 3 {
		t.Errorf("Expected 3 results for '/home/user', got %d", len(results))
	}

	// Test specific path search
	results = trie.Search("/home/user/test.txt")
	if len(results) != 1 {
		t.Errorf("Expected 1 result for '/home/user/test.txt', got %d", len(results))
	}

	// Test non-existent prefix
	results = trie.Search("/nonexistent")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for '/nonexistent', got %d", len(results))
	}
}

func TestTrieDelete(t *testing.T) {
	trie := NewTrie()

	entry := &Entry{
		Path: "/home/user/test.txt",
		Name: "test.txt",
		Size: 1024,
	}

	trie.Insert(entry.Path, entry)

	if trie.EntryCount() != 1 {
		t.Errorf("Expected 1 entry, got %d", trie.EntryCount())
	}

	deleted := trie.Delete(entry.Path)
	if !deleted {
		t.Errorf("Expected deletion to succeed")
	}

	if trie.EntryCount() != 0 {
		t.Errorf("Expected 0 entries after deletion, got %d", trie.EntryCount())
	}
}

func TestTrieSize(t *testing.T) {
	trie := NewTrie()

	entries := []*Entry{
		{Path: "/home/user/test.txt", Name: "test.txt", Size: 1024},
		{Path: "/home/user/test2.txt", Name: "test2.txt", Size: 2048},
		{Path: "/home/user/dir/test3.txt", Name: "test3.txt", Size: 512},
	}

	for _, entry := range entries {
		trie.Insert(entry.Path, entry)
	}

	size := trie.Size()
	if size < 1 {
		t.Errorf("Expected at least 1 node, got %d", size)
	}
}

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"hello", "help", 3},
		{"test", "testing", 4},
		{"/home/user", "/home/admin", 6}, // "/home/"
		{"", "test", 0},
		{"test", "", 0},
	}

	for _, test := range tests {
		result := commonPrefix(test.a, test.b)
		if result != test.expected {
			t.Errorf("commonPrefix(%q, %q) = %d, expected %d",
				test.a, test.b, result, test.expected)
		}
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"home/user", "/home/user"},
		{"/home/user/", "/home/user"},
		{"/home/user", "/home/user"},
	}

	for _, test := range tests {
		result := normalizePath(test.input)
		if result != test.expected {
			t.Errorf("normalizePath(%q) = %q, expected %q",
				test.input, result, test.expected)
		}
	}
}
