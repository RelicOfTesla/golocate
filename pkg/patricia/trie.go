// Package patricia implements a Patricia trie for efficient file path indexing.
package patricia

import (
	"strings"
	"sync"
)

// Node represents a node in the Patricia trie.
type Node struct {
	prefix   string
	children map[string]*Node
	entries  []*Entry
	isLeaf   bool
}

// Entry represents a file entry in the trie.
type Entry struct {
	Path string
	Name string
	Size int64
}

// Trie represents a Patricia trie.
type Trie struct {
	root *Node
	mu   sync.RWMutex
}

// NewTrie creates a new Patricia trie.
func NewTrie() *Trie {
	return &Trie{
		root: &Node{
			children: make(map[string]*Node),
		},
	}
}

// Insert inserts a file path into the trie.
func (t *Trie) Insert(path string, entry *Entry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Normalize path
	path = normalizePath(path)

	node := t.root
	remaining := path

	for len(remaining) > 0 {
		// Find the longest matching prefix among children
		var matchKey string
		var matchNode *Node
		var matchLen int

		for key, child := range node.children {
			prefixLen := commonPrefix(remaining, child.prefix)
			if prefixLen > matchLen {
				matchLen = prefixLen
				matchKey = key
				matchNode = child
			}
		}

		if matchLen == 0 {
			// No matching child, create new node
			newNode := &Node{
				prefix:   remaining,
				children: make(map[string]*Node),
				entries:  []*Entry{entry},
				isLeaf:   true,
			}
			node.children[remaining] = newNode
			return
		}

		if matchLen < len(matchNode.prefix) {
			// Partial match, split the node
			splitNode := &Node{
				prefix:   matchNode.prefix[matchLen:],
				children: matchNode.children,
				entries:  matchNode.entries,
				isLeaf:   matchNode.isLeaf,
			}

			newParent := &Node{
				prefix:   remaining[:matchLen],
				children: make(map[string]*Node),
			}
			newParent.children[splitNode.prefix] = splitNode

			// Update parent
			delete(node.children, matchKey)
			node.children[remaining[:matchLen]] = newParent

			// Continue from new parent
			node = newParent
			remaining = remaining[matchLen:]

			// Add remaining as new child
			if len(remaining) > 0 {
				newLeaf := &Node{
					prefix:   remaining,
					children: make(map[string]*Node),
					entries:  []*Entry{entry},
					isLeaf:   true,
				}
				node.children[remaining] = newLeaf
				return
			}
			return
		}

		// Full match, continue to child
		node = matchNode
		remaining = remaining[matchLen:]
	}

	// Path already exists, add entry
	node.entries = append(node.entries, entry)
	node.isLeaf = true
}

// Search searches for all paths matching the given prefix.
func (t *Trie) Search(prefix string) []*Entry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	prefix = normalizePath(prefix)

	node := t.root
	remaining := prefix

	// Navigate to the node matching the prefix
	for len(remaining) > 0 {
		found := false
		for _, child := range node.children {
			if strings.HasPrefix(remaining, child.prefix) {
				node = child
				remaining = remaining[len(child.prefix):]
				found = true
				break
			}
			if strings.HasPrefix(child.prefix, remaining) {
				// Prefix is shorter than child.prefix
				// Return all entries under this child
				return t.collectAllEntries(child)
			}
		}
		if !found {
			return nil
		}
	}

	// Collect all entries under this node
	return t.collectAllEntries(node)
}

// collectAllEntries collects all entries under a node.
func (t *Trie) collectAllEntries(node *Node) []*Entry {
	var entries []*Entry

	// Add entries from this node
	entries = append(entries, node.entries...)

	// Recursively collect from children
	for _, child := range node.children {
		entries = append(entries, t.collectAllEntries(child)...)
	}

	return entries
}

// Delete removes a path from the trie.
func (t *Trie) Delete(path string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	path = normalizePath(path)

	// Simple implementation: mark as deleted
	// A full implementation would compress the trie
	node := t.root
	remaining := path

	for len(remaining) > 0 {
		found := false
		for key, child := range node.children {
			if strings.HasPrefix(remaining, child.prefix) {
				if len(remaining) == len(child.prefix) {
					// Exact match, delete this node
					delete(node.children, key)
					return true
				}
				node = child
				remaining = remaining[len(child.prefix):]
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return false
}

// commonPrefix returns the length of the common prefix between two strings.
func commonPrefix(a, b string) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}

	return minLen
}

// normalizePath normalizes a file path for the trie.
func normalizePath(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Remove trailing /
	path = strings.TrimSuffix(path, "/")
	return path
}

// Size returns the number of nodes in the trie.
func (t *Trie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.countNodes(t.root)
}

// countNodes recursively counts the number of nodes.
func (t *Trie) countNodes(node *Node) int {
	count := 1
	for _, child := range node.children {
		count += t.countNodes(child)
	}
	return count
}

// EntryCount returns the total number of entries in the trie.
func (t *Trie) EntryCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.countEntries(t.root)
}

// countEntries recursively counts the number of entries.
func (t *Trie) countEntries(node *Node) int {
	count := len(node.entries)
	for _, child := range node.children {
		count += t.countEntries(child)
	}
	return count
}
