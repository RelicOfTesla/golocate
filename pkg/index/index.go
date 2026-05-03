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

// PatternMode defines the pattern matching mode.
type PatternMode string

const (
	// PatternModeNormal performs substring matching
	PatternModeNormal PatternMode = "normal"
	// PatternModeRegex performs regex matching
	PatternModeRegex PatternMode = "regex"
	// PatternModeExtendedRegex performs extended regex matching
	PatternModeExtendedRegex PatternMode = "extended_regex"
	// PatternModeWildcard performs wildcard matching
	PatternModeWildcard PatternMode = "wildcard"
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
	// nameLower is the precomputed lowercase name for case-insensitive search
	nameLower string
	// pathLower is the precomputed lowercase path for case-insensitive search
	pathLower string
}

// Index is the file index.
type Index struct {
	mu      sync.RWMutex
	entries map[string]*Entry // path -> Entry
	byName  map[string][]*Entry // name -> []Entry (for basename search)
}

// NewIndex creates a new file index.
func NewIndex() *Index {
	return &Index{
		entries:    make(map[string]*Entry),
		byName:     make(map[string][]*Entry),
	}
}

// NewIndexWithCapacity creates a new file index with pre-allocated capacity.
// This is more efficient for large indexes as it avoids map rehashing.
func NewIndexWithCapacity(capacity int) *Index {
	return &Index{
		entries:    make(map[string]*Entry, capacity),
		byName:     make(map[string][]*Entry, capacity/10), // Assume avg 10 files share same name
	}
}

// Add adds an entry to the index.
func (idx *Index) Add(entry *Entry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	idx.addLocked(entry)
}

// addLocked adds an entry to the index without locking (caller must hold lock).
func (idx *Index) addLocked(entry *Entry) {
	// Precompute lowercase strings for case-insensitive search
	entry.nameLower = strings.ToLower(entry.Name)
	entry.pathLower = strings.ToLower(entry.Path)
	
	// Remove old entry if exists
	if old, exists := idx.entries[entry.Path]; exists {
		idx.removeFromNameIndexLocked(old)
	}
	
	// Add to path index
	idx.entries[entry.Path] = entry
	
	// Add to name index
	idx.byName[entry.Name] = append(idx.byName[entry.Name], entry)
}

// AddBatch adds multiple entries to the index efficiently.
// This is more efficient than calling Add multiple times as it only acquires the lock once.
func (idx *Index) AddBatch(entries []*Entry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	for _, entry := range entries {
		idx.addLocked(entry)
	}
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
	idx.removeFromNameIndexLocked(entry)
}

// removeFromNameIndexLocked removes an entry from the name index without locking.
func (idx *Index) removeFromNameIndexLocked(entry *Entry) {
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
	if opts.PatternMode == PatternModeRegex || opts.PatternMode == PatternModeExtendedRegex {
		return idx.searchRegex(opts.Pattern, opts)
	}
	
	// Handle wildcard search
	if opts.PatternMode == PatternModeWildcard {
		return idx.searchWildcard(opts.Pattern, opts)
	}
	
	// Normal substring search
	query := opts.Pattern
	var results []*Entry
	
	// Precompute lowercase query for case-insensitive search
	queryLower := strings.ToLower(query)
	
	// Decide search target based on Basename
	if opts.Basename {
		// Search only in file names
		for name, entries := range idx.byName {
			if opts.IgnoreCase {
				// Use first entry's nameLower (all entries with same name have same nameLower)
				if len(entries) > 0 && strings.Contains(entries[0].nameLower, queryLower) {
					results = append(results, entries...)
				}
			} else {
				if strings.Contains(name, query) {
					results = append(results, entries...)
				}
			}
		}
	} else {
		// Search in full paths
		for path, entry := range idx.entries {
			if opts.IgnoreCase {
				// Use precomputed lowercase path
				if strings.Contains(entry.pathLower, queryLower) {
					results = append(results, entry)
				}
			} else {
				if strings.Contains(path, query) {
					results = append(results, entry)
				}
			}
		}
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
	
	if opts.PatternMode == PatternModeExtendedRegex {
		// Extended regex (POSIX ERE)
		if opts.IgnoreCase {
			re, err = regexp.Compile("(?i)" + query)
		} else {
			re, err = regexp.Compile(query)
		}
	} else {
		// Basic regex (POSIX BRE)
		// 注意：regexp.CompilePOSIX 不支持 (?i) 语法
		// 如果需要 IgnoreCase，使用 regexp.Compile 代替
		if opts.IgnoreCase {
			re, err = regexp.Compile("(?i)" + query)
		} else {
			re, err = regexp.CompilePOSIX(query)
		}
	}
	
	if err != nil {
		// Invalid regex, return empty results
		return results
	}
	
	// Decide search target based on Basename
	if opts.Basename {
		// Search only in file names
		for name, entries := range idx.byName {
			if re.MatchString(name) {
				results = append(results, entries...)
				if opts.Limit > 0 && len(results) >= opts.Limit {
					return results
				}
			}
		}
	} else {
		// Search in full paths
		for path, entry := range idx.entries {
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

// searchWildcard performs a wildcard pattern search.
func (idx *Index) searchWildcard(pattern string, opts SearchOptions) []*Entry {
	var results []*Entry
	
	// Special case: "*" matches everything
	if pattern == "*" {
		for _, entry := range idx.entries {
			results = append(results, entry)
			if opts.Limit > 0 && len(results) >= opts.Limit {
				return results
			}
		}
		return results
	}
	
	// Decide search target based on Basename
	if opts.Basename {
		// Search only in file names
		for name, entries := range idx.byName {
			// Handle IgnoreCase
			comparePattern := pattern
			compareName := name
			if opts.IgnoreCase {
				comparePattern = strings.ToLower(pattern)
				compareName = strings.ToLower(name)
			}
			
			matched, err := filepath.Match(comparePattern, compareName)
			if err != nil {
				// Invalid pattern, skip
				continue
			}
			if matched {
				results = append(results, entries...)
				if opts.Limit > 0 && len(results) >= opts.Limit {
					return results
				}
			}
		}
	} else {
		// Search in full paths
		for path, entry := range idx.entries {
			// Handle IgnoreCase
			comparePattern := pattern
			comparePath := path
			if opts.IgnoreCase {
				comparePattern = strings.ToLower(pattern)
				comparePath = strings.ToLower(path)
			}
			
			// For full path matching, use filepath.Match on the full path
			matched, err := filepath.Match(comparePattern, comparePath)
			if err != nil {
				// Invalid pattern, skip
				continue
			}
			if matched {
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
	
	// Decide search target based on Basename
	if opts.Basename {
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
		
	} else {
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

// GetAllEntries returns all entries in the index.
// The returned slice is a shallow copy of the internal entries map.
func (idx *Index) GetAllEntries() []*Entry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := make([]*Entry, 0, len(idx.entries))
	for _, entry := range idx.entries {
		entries = append(entries, entry)
	}
	return entries
}

// SearchOptions contains options for searching.
type SearchOptions struct {
	// Pattern is the search pattern (regex path, normal path, or wildcard path)
	Pattern string
	// PatternMode defines how the pattern is interpreted
	PatternMode PatternMode
	// Basename searches only in file names (can be combined with any PatternMode)
	Basename bool
	// IgnoreCase makes the search case-insensitive
	IgnoreCase bool
	// Limit limits the number of results
	Limit int
	// Offset skips the first N results (for pagination)
	Offset int64
	// SortField specifies the field to sort by (name, size, time, path)
	SortField string
	// SortOrder specifies the sort order (asc, desc)
	SortOrder string
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
