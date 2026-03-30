// Package index provides file indexing capabilities.
package index

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/ignore"
	"github.com/RelicOfTesla/golocate/pkg/watcher"
)

// Entry represents a file or directory entry in the index.
type Entry struct {
	// Name is the file/directory name
	Name string
	// Path is the full path
	Path string
	// IsDir indicates if this is a directory
	IsDir bool
	// Size is the file size in bytes
	Size int64
	// ModTime is the modification time
	ModTime time.Time
}

// Index is the file index.
type Index struct {
	mu      sync.RWMutex
	entries map[string]*Entry // path -> Entry
	byName  map[string][]*Entry // name -> []Entry (for basename search)
	pathTrie *Trie    // Patricia Trie for fast path prefix search
	nameFilter map[string]bool // Bloom filter for fast name filtering (simplified)
}

// NewIndex creates a new file index.
func NewIndex() *Index {
	return &Index{
		entries:    make(map[string]*Entry),
		byName:     make(map[string][]*Entry),
		pathTrie:   NewTrie(),
		nameFilter: make(map[string]bool),
	}
}

// Add adds an entry to the index.
func (idx *Index) Add(entry *Entry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	// Remove old entry if exists
	if old, exists := idx.entries[entry.Path]; exists {
		idx.removeFromNameIndex(old)
		// Remove from Patricia Trie (note: Patricia Trie doesn't support delete, will rebuild if needed)
	}
	
	// Add to path index
	idx.entries[entry.Path] = entry
	
	// Add to name index
	idx.byName[entry.Name] = append(idx.byName[entry.Name], entry)
	
	// Add to Patricia Trie
	idx.pathTrie.Insert(entry.Path, &Entry{
		Path: entry.Path,
		Name: entry.Name,
		Size: entry.Size,
	})
}

// Remove removes an entry from the index.
func (idx *Index) Remove(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	if entry, exists := idx.entries[path]; exists {
		idx.removeFromNameIndex(entry)
		delete(idx.entries, path)
	}
}

// removeFromNameIndex removes an entry from the name index.
func (idx *Index) removeFromNameIndex(entry *Entry) {
	entries := idx.byName[entry.Name]
	for i, e := range entries {
		if e.Path == entry.Path {
			// Remove without preserving order
			entries[i] = entries[len(entries)-1]
			entries = entries[:len(entries)-1]
			break
		}
	}
	if len(entries) == 0 {
		delete(idx.byName, entry.Name)
	} else {
		idx.byName[entry.Name] = entries
	}
}

// Get retrieves an entry by path.
func (idx *Index) Get(path string) (*Entry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	entry, exists := idx.entries[path]
	return entry, exists
}

// Search searches for entries matching the query.
func (idx *Index) Search(opts SearchOptions) []*Entry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	// Handle regex search
	if opts.Regex || opts.ExtendedRegex {
		return idx.searchRegex(opts.Pattern, opts)
	}
	
	// Normal substring search
	query := opts.Pattern
	var results []*Entry
	
	switch {
	case opts.Basename:
		// Search only in file names
		for name, entries := range idx.byName {
			if opts.IgnoreCase {
				if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
					results = append(results, entries...)
				}
			} else {
				if strings.Contains(name, query) {
					results = append(results, entries...)
				}
			}
		}
		
	default:
		// Search in full paths
		for path, entry := range idx.entries {
			if opts.IgnoreCase {
				if strings.Contains(strings.ToLower(path), strings.ToLower(query)) {
					results = append(results, entry)
				}
			} else {
				if strings.Contains(path, query) {
					results = append(results, entry)
				}
			}
		}
	}
	
	// Filter by path if specified
	if opts.Path != "" {
		filtered := make([]*Entry, 0, len(results))
		for _, entry := range results {
			// Support wildcard "*" to match all paths
			if opts.Path == "*" {
				filtered = append(filtered, entry)
			} else if strings.HasPrefix(entry.Path, opts.Path) {
				filtered = append(filtered, entry)
			}
		}
		results = filtered
	}
	
	// Apply offset (skip first N results)
	offset := int(opts.Offset) // Convert int64 to int for slice operations
	if offset > 0 && offset < len(results) {
		results = results[offset:]
	} else if offset > 0 && offset >= len(results) {
		results = []*Entry{} // Offset exceeds result count, return empty
	}
	
	// Apply limit
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	
	return results
}

// searchRegex performs a regex search.
func (idx *Index) searchRegex(query string, opts SearchOptions) []*Entry {
	var results []*Entry
	
	// Compile regex
	var re *regexp.Regexp
	var err error
	
	if opts.ExtendedRegex {
		// Extended regex (POSIX ERE)
		if opts.IgnoreCase {
			re, err = regexp.Compile("(?i)" + query)
		} else {
			re, err = regexp.Compile(query)
		}
	} else {
		// Basic regex (POSIX BRE)
		if opts.IgnoreCase {
			re, err = regexp.CompilePOSIX("(?i)" + query)
		} else {
			re, err = regexp.CompilePOSIX(query)
		}
	}
	
	if err != nil {
		// Invalid regex, return empty results
		return results
	}
	
	switch {
	case opts.Basename:
		// Search only in file names
		for name, entries := range idx.byName {
			if re.MatchString(name) {
				for _, entry := range entries {
					// Filter by path if specified
					if opts.Path != "" && !strings.HasPrefix(entry.Path, opts.Path) {
						continue
					}
					results = append(results, entry)
					if opts.Limit > 0 && len(results) >= opts.Limit {
						return results
					}
				}
			}
		}
		
	default:
		// Search in full paths
		for path, entry := range idx.entries {
			// Filter by path if specified
			if opts.Path != "" && !strings.HasPrefix(entry.Path, opts.Path) {
				continue
			}
			if re.MatchString(path) {
				results = append(results, entry)
				if opts.Limit > 0 && len(results) >= opts.Limit {
					return results
				}
			}
		}
	}
	
	return results
}

// Count returns the number of entries matching the query.
func (idx *Index) Count(query string, opts SearchOptions) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	count := 0
	
	switch {
	case opts.Basename:
		for name, entries := range idx.byName {
			if opts.IgnoreCase {
				if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
					count += len(entries)
				}
			} else {
				if strings.Contains(name, query) {
					count += len(entries)
				}
			}
		}
		
	default:
		for path := range idx.entries {
			if opts.IgnoreCase {
				if strings.Contains(strings.ToLower(path), strings.ToLower(query)) {
					count++
				}
			} else {
				if strings.Contains(path, query) {
					count++
				}
			}
		}
	}
	
	return count
}

// Len returns the total number of entries in the index.
func (idx *Index) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// SearchOptions contains options for searching.
type SearchOptions struct {
	// Pattern is the search pattern (filename or content)
	Pattern string
	// IgnoreCase makes the search case-insensitive
	IgnoreCase bool
	// Basename searches only in file names
	Basename bool
	// Limit limits the number of results
	Limit int
	// Offset skips the first N results (for pagination)
	Offset int64
	// Regex enables regex search
	Regex bool
	// ExtendedRegex enables extended regex search
	ExtendedRegex bool
	// SortField specifies the field to sort by (name, size, time, path)
	SortField string
	// SortOrder specifies the sort order (asc, desc)
	SortOrder string
	// Path filters results to only include entries under this path
	Path string
}

// Builder builds the file index.
type Builder struct {
	idx           *Index
	ignoreMatcher *ignore.Matcher
	workerCount   int
}

// NewBuilder creates a new index builder.
func NewBuilder(opts BuilderOptions) *Builder {
	b := &Builder{
		idx:         NewIndex(),
		workerCount: opts.WorkerCount,
	}
	
	if len(opts.IgnorePatterns) > 0 {
		b.ignoreMatcher = ignore.NewMatcher(opts.IgnorePatterns)
	}
	
	if b.workerCount <= 0 {
		b.workerCount = 4
	}
	
	return b
}

// BuilderOptions contains options for building the index.
type BuilderOptions struct {
	// IgnorePatterns are glob patterns to ignore
	IgnorePatterns []string
	// WorkerCount is the number of concurrent workers
	WorkerCount int
}

// Build builds the index by scanning the specified directories.
func (b *Builder) Build(ctx context.Context, directories []string) error {
	return b.BuildThrottled(ctx, directories, 0)
}

// BuildThrottled builds the index with optional throttling.
// throttleDelay > 0 enables throttled mode with delays between operations.
func (b *Builder) BuildThrottled(ctx context.Context, directories []string, throttleDelay time.Duration) error {
	start := time.Now()
	
	var fileCount, dirCount int64
	
	for _, dir := range directories {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			
			// Apply throttling delay
			if throttleDelay > 0 {
				time.Sleep(throttleDelay)
			}
			
			if err != nil {
				return nil // Skip errors
			}
			
			// Check if path should be ignored
			if b.ignoreMatcher != nil && b.ignoreMatcher.Match(path) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			
			entry := &Entry{
				Name:    info.Name(),
				Path:    path,
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}
			
			b.idx.Add(entry)
			
			if info.IsDir() {
				dirCount++
			} else {
				fileCount++
			}
			
			return nil
		})
		
		if err != nil && err != context.Canceled {
			log.Printf("warning: error walking %s: %v", dir, err)
		}
	}
	
	elapsed := time.Since(start)
	log.Printf("indexed %d files and %d directories in %v", fileCount, dirCount, elapsed)
	
	return nil
}

// Index returns the built index.
func (b *Builder) Index() *Index {
	return b.idx
}

// Updater updates the index based on file system events.
type Updater struct {
	idx *Index
}

// NewUpdater creates a new index updater.
func NewUpdater(idx *Index) *Updater {
	return &Updater{idx: idx}
}

// HandleEvent handles a file system event.
func (u *Updater) HandleEvent(event watcher.Event) {
	switch {
	case event.Op&watcher.Create != 0:
		u.handleCreate(event.Path)
	case event.Op&watcher.Remove != 0:
		u.handleRemove(event.Path)
	case event.Op&watcher.MoveFrom != 0:
		u.handleRemove(event.Path)
	case event.Op&watcher.MoveTo != 0:
		u.handleCreate(event.Path)
	}
}

// handleCreate handles a file creation event.
func (u *Updater) handleCreate(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	
	entry := &Entry{
		Name:    info.Name(),
		Path:    path,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	
	u.idx.Add(entry)
	log.Printf("indexed: %s", path)
}

// handleRemove handles a file removal event.
func (u *Updater) handleRemove(path string) {
	u.idx.Remove(path)
	log.Printf("removed: %s", path)
}
