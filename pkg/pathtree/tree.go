// Package pathtree provides an optimized tree structure for file paths.
package pathtree

import (
	"strings"
	"sync"
)

// Node represents a node in the path tree.
type Node struct {
	name     string
	children map[string]*Node
	entries  []*FileEntry
}

// FileEntry represents a file entry.
type FileEntry struct {
	Name string
	Size int64
}

// Tree represents a file path tree.
type Tree struct {
	root *Node
	mu   sync.RWMutex
}

// NewTree creates a new path tree.
func NewTree() *Tree {
	return &Tree{
		root: &Node{
			name:     "/",
			children: make(map[string]*Node),
		},
	}
}

// Insert inserts a file path into the tree.
func (t *Tree) Insert(path string, entry *FileEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Normalize path
	path = normalizePath(path)
	segments := splitPath(path)

	// Special case: root path
	if len(segments) == 0 {
		t.root.entries = append(t.root.entries, entry)
		return
	}

	// Navigate to the parent directory node
	node := t.root
	for i := 0; i < len(segments)-1; i++ {
		segment := segments[i]
		if segment == "" {
			continue
		}

		if _, ok := node.children[segment]; !ok {
			node.children[segment] = &Node{
				name:     segment,
				children: make(map[string]*Node),
			}
		}
		node = node.children[segment]
	}

	// Create the file node (last segment)
	lastSegment := segments[len(segments)-1]
	if lastSegment != "" {
		if _, ok := node.children[lastSegment]; !ok {
			node.children[lastSegment] = &Node{
				name:     lastSegment,
				children: make(map[string]*Node),
			}
		}
	}

	// Add entry to the parent directory node
	node.entries = append(node.entries, entry)
}

// Search searches for all files under the given path prefix.
func (t *Tree) Search(prefix string) []*FileEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	prefix = normalizePath(prefix)
	segments := splitPath(prefix)

	node := t.root
	for _, segment := range segments {
		if segment == "" {
			continue
		}

		child, ok := node.children[segment]
		if !ok {
			return nil
		}
		node = child
	}

	// Collect all entries under this node
	return t.collectEntries(node)
}

// collectEntries collects all entries under a node.
func (t *Tree) collectEntries(node *Node) []*FileEntry {
	var entries []*FileEntry

	// Add entries from this node
	entries = append(entries, node.entries...)

	// Recursively collect from children
	for _, child := range node.children {
		entries = append(entries, t.collectEntries(child)...)
	}

	return entries
}

// Delete removes a path from the tree.
func (t *Tree) Delete(path string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	path = normalizePath(path)
	segments := splitPath(path)

	node := t.root
	parents := []*Node{t.root}

	for i, segment := range segments {
		if segment == "" {
			continue
		}

		child, ok := node.children[segment]
		if !ok {
			return false
		}

		node = child
		if i < len(segments)-1 {
			parents = append(parents, node)
		}
	}

	// Delete the node
	if len(parents) > 0 {
		parent := parents[len(parents)-1]
		delete(parent.children, node.name)
		return true
	}

	return false
}

// List lists all files in a directory (non-recursive).
func (t *Tree) List(dir string) []*FileEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	dir = normalizePath(dir)
	segments := splitPath(dir)

	node := t.root
	for _, segment := range segments {
		if segment == "" {
			continue
		}

		child, ok := node.children[segment]
		if !ok {
			return nil
		}
		node = child
	}

	// Return entries only from this node (non-recursive)
	return node.entries
}

// Count returns the total number of files in the tree.
func (t *Tree) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.countNodes(t.root)
}

// countNodes recursively counts the number of entries.
func (t *Tree) countNodes(node *Node) int {
	count := len(node.entries)
	for _, child := range node.children {
		count += t.countNodes(child)
	}
	return count
}

// normalizePath normalizes a file path.
func normalizePath(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Remove trailing /
	path = strings.TrimSuffix(path, "/")
	return path
}

// splitPath splits a path into segments.
func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/") // Remove trailing slash
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// Size returns the number of nodes in the tree.
func (t *Tree) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.countAllNodes(t.root)
}

// countAllNodes recursively counts all nodes.
func (t *Tree) countAllNodes(node *Node) int {
	count := 1
	for _, child := range node.children {
		count += t.countAllNodes(child)
	}
	return count
}
