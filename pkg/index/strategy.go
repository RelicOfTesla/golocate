// Package index provides file indexing capabilities with multiple update strategies.
package index

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/RelicOfTesla/golocate/pkg/ignore"
)

// UpdateStrategy defines how the index is updated.
type UpdateStrategy int

const (
	// StrategyReplace builds a completely new index and replaces the old one.
	// Suitable for large changes or periodic full rebuilds.
	// Memory usage: 2x during build
	StrategyReplace UpdateStrategy = iota

	// StrategyMerge updates the existing index incrementally.
	// Suitable for small changes or continuous updates.
	// Memory usage: 1x, but may have stale entries
	StrategyMerge

	// StrategyAuto automatically chooses between Replace and Merge based on change size.
	// Small changes (< 10% of total files) -> Merge
	// Large changes (>= 10% of total files) -> Replace
	StrategyAuto
)

func (s UpdateStrategy) String() string {
	switch s {
	case StrategyReplace:
		return "replace"
	case StrategyMerge:
		return "merge"
	case StrategyAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ParseUpdateStrategy parses a string into UpdateStrategy.
func ParseUpdateStrategy(s string) UpdateStrategy {
	switch strings.ToLower(s) {
	case "replace":
		return StrategyReplace
	case "merge":
		return StrategyMerge
	case "auto":
		return StrategyAuto
	default:
		return StrategyAuto
	}
}

// MergeResult contains the result of a merge operation.
type MergeResult struct {
	Added    int
	Deleted  int
	Updated  int
	Unchanged int
}

// Merge merges the current index with the file system state.
// This is an incremental update strategy that:
// 1. Scans directories for new files
// 2. Removes deleted files from index
// 3. Updates modified files
func (idx *Index) Merge(ctx context.Context, directories []string, ignoreMatcher *ignore.Matcher) (*MergeResult, error) {
	result := &MergeResult{}
	
	// Track which files we've seen during this scan
	seen := make(map[string]bool)
	
	// Scan all directories
	for _, dir := range directories {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			
			if err != nil {
				return nil // Skip errors
			}
			
			// Check if path should be ignored
			if ignoreMatcher != nil && ignoreMatcher.Match(path) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			
			// Mark as seen
			seen[path] = true
			
			// Check if already in index
			if existing, exists := idx.Get(path); exists {
				// Check if modified
				if existing.ModTime.Before(info.ModTime()) {
					// Update existing entry
					existing.Name = info.Name()
					existing.IsDir = info.IsDir()
					existing.Size = info.Size()
					existing.ModTime = info.ModTime()
					result.Updated++
				} else {
					result.Unchanged++
				}
			} else {
				// Add new entry
				entry := &Entry{
					Name:    info.Name(),
					Path:    path,
					IsDir:   info.IsDir(),
					Size:    info.Size(),
					ModTime: info.ModTime(),
				}
				idx.Add(entry)
				result.Added++
			}
			
			return nil
		})
		
		if err != nil && err != context.Canceled {
			log.Printf("warning: error walking %s: %v", dir, err)
		}
	}
	
	// Remove entries that no longer exist
	// This is O(n) but necessary for correctness
	idx.mu.Lock()
	for path := range idx.entries {
		if !seen[path] {
			delete(idx.entries, path)
			// Also remove from name index
			name := filepath.Base(path)
			if entries, ok := idx.byName[name]; ok {
				for i, e := range entries {
					if e.Path == path {
						entries[i] = entries[len(entries)-1]
						entries = entries[:len(entries)-1]
						break
					}
				}
				if len(entries) == 0 {
					delete(idx.byName, name)
				} else {
					idx.byName[name] = entries
				}
			}
			result.Deleted++
		}
	}
	idx.mu.Unlock()
	
	return result, nil
}

// AutoUpdateStrategy chooses between Replace and Merge based on change estimation.
func (idx *Index) AutoUpdateStrategy(directories []string, ignoreMatcher *ignore.Matcher) UpdateStrategy {
	// Estimate: if we have many files already, use merge
	// If we have few files or empty index, use replace
	total := idx.Len()
	
	if total == 0 {
		return StrategyReplace
	}
	
	// Simple heuristic: if index has more than 10,000 files, prefer merge
	// This reduces memory pressure
	if total > 10000 {
		return StrategyMerge
	}
	
	return StrategyReplace
}

// UpdateWithStrategy updates the index using the specified strategy.
func (b *Builder) UpdateWithStrategy(ctx context.Context, directories []string, strategy UpdateStrategy) (*MergeResult, error) {
	switch strategy {
	case StrategyReplace:
		// Build new index and replace
		if err := b.Build(ctx, directories); err != nil {
			return nil, err
		}
		return &MergeResult{Added: b.idx.Len()}, nil
		
	case StrategyMerge:
		// Merge with existing index
		return b.idx.Merge(ctx, directories, b.ignoreMatcher)
		
	case StrategyAuto:
		// Auto-select strategy
		autoStrategy := b.idx.AutoUpdateStrategy(directories, b.ignoreMatcher)
		return b.UpdateWithStrategy(ctx, directories, autoStrategy)
		
	default:
		return nil, nil
	}
}
