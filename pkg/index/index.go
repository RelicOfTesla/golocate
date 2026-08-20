// Package index provides file indexing capabilities.
package index

import (
	"container/heap"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/ignore"
	"github.com/RelicOfTesla/golocate/pkg/security"
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
	// PatternModeTerms performs multi-term matching: space-separated terms
	// are ANDed, a leading "-" makes a term exclusive (e.g. "foo -bar").
	PatternModeTerms PatternMode = "terms"
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
	// Dev/Ino identify the underlying file (device + inode) so hard links
	// can be deduplicated. Zero on platforms where they are unavailable.
	Dev uint64
	Ino uint64
	// nameLower is the precomputed lowercase name for case-insensitive search
	nameLower string
	// pathLower is the precomputed lowercase path for case-insensitive search
	pathLower string
}

// maxRecentEntries caps the in-memory "most recently modified" candidate set
// used by content search, avoiding a full index snapshot + sort per query.
const maxRecentEntries = 4096

// Index is the file index.
type Index struct {
	mu      sync.RWMutex
	entries map[string]*Entry   // path -> Entry
	byName  map[string][]*Entry // name -> []Entry (for basename search)
	recent  *recentMinHeap      // cap-limited min-heap by ModTime (top = oldest)
}

// NewIndex creates a new file index.
func NewIndex() *Index {
	return &Index{
		entries: make(map[string]*Entry),
		byName:  make(map[string][]*Entry),
	}
}

// NewIndexWithCapacity creates a new file index with pre-allocated capacity.
// This is more efficient for large indexes as it avoids map rehashing.
func NewIndexWithCapacity(capacity int) *Index {
	return &Index{
		entries: make(map[string]*Entry, capacity),
		byName:  make(map[string][]*Entry, capacity/10), // Assume avg 10 files share same name
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

	// Keep the "most recently modified" candidate set (bounded), skipping
	// entries without a real modtime (e.g. constructed in tests).
	idx.addRecentLocked(entry)
}

// addRecentLocked pushes entry onto the bounded min-heap of newest entries.
func (idx *Index) addRecentLocked(entry *Entry) {
	if entry.ModTime.IsZero() {
		return
	}
	if idx.recent == nil {
		idx.recent = &recentMinHeap{}
	}
	heap.Push(idx.recent, entry)
	if idx.recent.Len() > maxRecentEntries {
		heap.Pop(idx.recent)
	}
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

	// Early termination is only valid when nothing after collection can
	// remove or reorder entries (scope/exclude/metadata filters, dedupe,
	// sort, or a page offset).
	opts.earlyStop = opts.Limit > 0 && opts.Offset == 0 && opts.SortField == "" &&
		opts.Scope == "" && len(opts.Exclude) == 0 && !opts.Dedupe && !hasMetadataFilters(opts)

	var results []*Entry

	switch {
	case opts.PatternMode == PatternModeRegex || opts.PatternMode == PatternModeExtendedRegex:
		results = idx.searchRegex(opts.Pattern, opts)
	case opts.PatternMode == PatternModeWildcard:
		results = idx.searchWildcard(opts.Pattern, opts)
	case opts.PatternMode == PatternModeTerms:
		results = idx.searchTerms(opts.Pattern, opts)
	default:
		results = idx.searchNormal(opts.Pattern, opts)
	}

	// 统一后处理：先 scope/exclude 过滤，再元数据过滤，再去重，再排序，再 offset，最后 limit。
	if opts.Scope != "" || len(opts.Exclude) > 0 {
		results = FilterScopeExclude(results, opts.Scope, opts.Exclude)
	}
	results = FilterMetadata(results, opts)
	if opts.Dedupe {
		results = DedupeEntries(results)
	}

	if opts.SortField != "" {
		sortEntries(results, opts.SortField, opts.SortOrder)
	}

	if opts.Offset > 0 {
		offset := int(opts.Offset)
		if offset >= len(results) {
			results = []*Entry{}
		} else {
			results = results[offset:]
		}
	}

	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}

// searchNormal performs a normal substring search.
func (idx *Index) searchNormal(query string, opts SearchOptions) []*Entry {
	var results []*Entry
	queryLower := strings.ToLower(query)

	if opts.Basename {
		for name, entries := range idx.byName {
			if opts.IgnoreCase {
				if len(entries) > 0 && strings.Contains(entries[0].nameLower, queryLower) {
					results = append(results, entries...)
				}
			} else if strings.Contains(name, query) {
				results = append(results, entries...)
			}
			if opts.earlyStop && len(results) >= opts.Limit {
				break
			}
		}
	} else {
		for path, entry := range idx.entries {
			if opts.IgnoreCase {
				if strings.Contains(entry.pathLower, queryLower) {
					results = append(results, entry)
				}
			} else if strings.Contains(path, query) {
				results = append(results, entry)
			}
			if opts.earlyStop && len(results) >= opts.Limit {
				break
			}
		}
	}

	return results
}

// sortEntries sorts entries in place by field and order.
func sortEntries(entries []*Entry, field, order string) {
	if len(entries) == 0 || field == "" {
		return
	}

	desc := strings.EqualFold(order, "desc")
	switch strings.ToLower(field) {
	case "name":
		sort.Slice(entries, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name))
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	case "name-case":
		sort.Slice(entries, func(i, j int) bool {
			cmp := strings.Compare(entries[i].Name, entries[j].Name)
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	case "size":
		sort.Slice(entries, func(i, j int) bool {
			if desc {
				return entries[i].Size > entries[j].Size
			}
			return entries[i].Size < entries[j].Size
		})
	case "time":
		sort.Slice(entries, func(i, j int) bool {
			if desc {
				return entries[i].ModTime.After(entries[j].ModTime)
			}
			return entries[i].ModTime.Before(entries[j].ModTime)
		})
	case "path":
		sort.Slice(entries, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(entries[i].Path), strings.ToLower(entries[j].Path))
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	}
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
		for name, entries := range idx.byName {
			if re.MatchString(name) {
				results = append(results, entries...)
			}
		}
	} else {
		for path, entry := range idx.entries {
			if re.MatchString(path) {
				results = append(results, entry)
			}
			if opts.earlyStop && len(results) >= opts.Limit {
				break
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
		if opts.Basename {
			for _, entries := range idx.byName {
				results = append(results, entries...)
			}
		} else {
			for _, entry := range idx.entries {
				results = append(results, entry)
			}
		}
		return results
	}

	// Decide search target based on Basename
	if opts.Basename {
		for name, entries := range idx.byName {
			comparePattern := pattern
			compareName := name
			if opts.IgnoreCase {
				comparePattern = strings.ToLower(pattern)
				compareName = strings.ToLower(name)
			}

			matched, err := filepath.Match(comparePattern, compareName)
			if err == nil && matched {
				results = append(results, entries...)
			}
			if opts.earlyStop && len(results) >= opts.Limit {
				break
			}
		}
	} else {
		for path, entry := range idx.entries {
			comparePattern := pattern
			comparePath := path
			if opts.IgnoreCase {
				comparePattern = strings.ToLower(pattern)
				comparePath = strings.ToLower(path)
			}

			matched, err := filepath.Match(comparePattern, comparePath)
			if err == nil && matched {
				results = append(results, entry)
			}
			if opts.earlyStop && len(results) >= opts.Limit {
				break
			}
		}
	}

	return results
}

// searchTerms performs multi-term matching:
//   - terms are space-separated; every positive term must match (AND)
//   - a term with a leading "-" must NOT match (e.g. "foo -bar")
//   - with IgnoreCase, matching is case-insensitive
func (idx *Index) searchTerms(query string, opts SearchOptions) []*Entry {
	var results []*Entry

	var (
		positive []string
		negative []string
	)
	for _, term := range strings.Fields(query) {
		if strings.HasPrefix(term, "-") && len(term) > 1 {
			negative = append(negative, strings.TrimPrefix(term, "-"))
		} else {
			positive = append(positive, term)
		}
	}

	// Empty query matches everything (same semantics as normal mode).
	if len(positive) == 0 && len(negative) == 0 {
		if opts.Basename {
			for _, entries := range idx.byName {
				results = append(results, entries...)
			}
		} else {
			for _, entry := range idx.entries {
				results = append(results, entry)
			}
		}
		return results
	}

	lower := func(s string) string {
		if opts.IgnoreCase {
			return strings.ToLower(s)
		}
		return s
	}
	for i := range positive {
		positive[i] = lower(positive[i])
	}
	for i := range negative {
		negative[i] = lower(negative[i])
	}

	matchesAll := func(s string) bool {
		s = lower(s)
		for _, t := range positive {
			if !strings.Contains(s, t) {
				return false
			}
		}
		for _, t := range negative {
			if strings.Contains(s, t) {
				return false
			}
		}
		return true
	}

	if opts.Basename {
		for name, entries := range idx.byName {
			if matchesAll(name) {
				results = append(results, entries...)
			}
		}
	} else {
		for path, entry := range idx.entries {
			if matchesAll(path) {
				results = append(results, entry)
			}
			if opts.earlyStop && len(results) >= opts.Limit {
				break
			}
		}
	}
	return results
}

// FilterScopeExclude applies a scope restriction and exclude globs to entries.
// - scope: keep only entries whose path is inside this directory ("" = keep all)
// - exclude: drop entries whose path OR basename matches any glob
func FilterScopeExclude(entries []*Entry, scope string, exclude []string) []*Entry {
	if scope == "" && len(exclude) == 0 {
		return entries
	}

	var validator *security.PathValidator
	if scope != "" {
		validator = security.NewPathValidator([]string{scope})
	}

	out := make([]*Entry, 0, len(entries))
	for _, e := range entries {
		if validator != nil && !validator.IsPathAllowed(e.Path) {
			continue
		}
		if isExcludedPath(e.Path, exclude) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// isExcludedPath reports whether the path (or its basename) matches any exclude glob.
func isExcludedPath(path string, exclude []string) bool {
	base := filepath.Base(path)
	for _, pat := range exclude {
		if ok, err := filepath.Match(pat, path); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(pat, base); err == nil && ok {
			return true
		}
	}
	return false
}

// FilterMetadata applies type / size / mtime / hidden filters to entries.
func FilterMetadata(entries []*Entry, opts SearchOptions) []*Entry {
	if !hasMetadataFilters(opts) {
		return entries
	}
	out := make([]*Entry, 0, len(entries))
	for _, e := range entries {
		if metadataOK(e, opts) {
			out = append(out, e)
		}
	}
	return out
}

// hasMetadataFilters reports whether any metadata filter is active.
func hasMetadataFilters(opts SearchOptions) bool {
	return len(opts.Types) > 0 || opts.MinSize > 0 || opts.MaxSize > 0 ||
		opts.MtimeAfter > 0 || opts.MtimeBefore > 0 || opts.ExcludeHidden
}

// metadataOK reports whether an entry passes the metadata filters.
func metadataOK(e *Entry, opts SearchOptions) bool {
	if opts.ExcludeHidden && isHiddenPath(e.Path) {
		return false
	}
	if len(opts.Types) > 0 {
		if e.IsDir {
			return false
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(e.Name), "."))
		matched := false
		for _, t := range opts.Types {
			if strings.ToLower(t) == ext {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if opts.MinSize > 0 && e.Size < opts.MinSize {
		return false
	}
	if opts.MaxSize > 0 && e.Size > opts.MaxSize {
		return false
	}
	if opts.MtimeAfter > 0 && e.ModTime.Unix() < opts.MtimeAfter {
		return false
	}
	if opts.MtimeBefore > 0 && e.ModTime.Unix() > opts.MtimeBefore {
		return false
	}
	return true
}

// isHiddenPath reports whether any path segment is dot-prefixed (hidden).
func isHiddenPath(path string) bool {
	for _, seg := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

// DedupeEntries collapses entries that refer to the same underlying file
// (hard links), keeping only the first occurrence of each identity.
// Identities come from (Dev, Ino) when available; entries without device
// info (e.g. on Windows, or entries built by tests) fall back to
// (Size, ModTime), which is a best-effort heuristic.
func DedupeEntries(entries []*Entry) []*Entry {
	if len(entries) < 2 {
		return entries
	}
	seen := make(map[fileIdentity]struct{}, len(entries))
	out := make([]*Entry, 0, len(entries))
	for _, e := range entries {
		id := identityOf(e)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, e)
	}
	return out
}

type fileIdentity struct {
	dev, ino    uint64
	size        int64
	modTimeNano int64
}

func identityOf(e *Entry) fileIdentity {
	if e.Dev != 0 || e.Ino != 0 {
		return fileIdentity{dev: e.Dev, ino: e.Ino}
	}
	return fileIdentity{size: e.Size, modTimeNano: e.ModTime.UnixNano()}
}

// fillDeviceInfo records the device/inode identity of the file (a no-op on
// platforms that cannot provide one, e.g. Windows) so hard links can be
// deduplicated later.
func fillDeviceInfo(entry *Entry, info os.FileInfo) {
	entry.Dev, entry.Ino = deviceIdentity(info)
}

// Count returns the number of entries matching the query.
// It honors PatternMode the same way Search does, so total counts stay
// consistent with the actual search results.
func (idx *Index) Count(query string, opts SearchOptions) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Precompile regex for regex modes.
	var re *regexp.Regexp
	if opts.PatternMode == PatternModeRegex || opts.PatternMode == PatternModeExtendedRegex {
		var err error
		if opts.IgnoreCase {
			re, err = regexp.Compile("(?i)" + query)
		} else if opts.PatternMode == PatternModeExtendedRegex {
			re, err = regexp.Compile(query)
		} else {
			re, err = regexp.CompilePOSIX(query)
		}
		if err != nil {
			return 0
		}
	}

	match := func(s string) bool {
		switch opts.PatternMode {
		case PatternModeRegex, PatternModeExtendedRegex:
			return re.MatchString(s)
		case PatternModeWildcard:
			if query == "*" {
				return true
			}
			pattern, target := query, s
			if opts.IgnoreCase {
				pattern = strings.ToLower(query)
				target = strings.ToLower(s)
			}
			matched, err := filepath.Match(pattern, target)
			return err == nil && matched
		default:
			if opts.IgnoreCase {
				return strings.Contains(strings.ToLower(s), strings.ToLower(query))
			}
			return strings.Contains(s, query)
		}
	}

	var validator *security.PathValidator
	if opts.Scope != "" {
		validator = security.NewPathValidator([]string{opts.Scope})
	}
	allFiltersOK := func(e *Entry) bool {
		if validator != nil && !validator.IsPathAllowed(e.Path) {
			return false
		}
		if isExcludedPath(e.Path, opts.Exclude) {
			return false
		}
		return metadataOK(e, opts)
	}

	count := 0
	scoped := opts.Scope != "" || len(opts.Exclude) > 0 || hasMetadataFilters(opts)
	if opts.Basename {
		for name, entries := range idx.byName {
			if !match(name) {
				continue
			}
			if scoped {
				for _, e := range entries {
					if allFiltersOK(e) {
						count++
					}
				}
			} else {
				count += len(entries)
			}
		}
	} else {
		for path, entry := range idx.entries {
			if match(path) && (!scoped || allFiltersOK(entry)) {
				count++
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

// RecentEntries returns up to max entries ordered by modification time,
// newest first. Entries that were removed, or duplicate paths whose newer
// version superseded an older one, are filtered out. This is the bounded
// candidate set for content search (see docs/PERFORMANCE.md S3) — it avoids
// snapshotting and sorting the whole index on every query.
func (idx *Index) RecentEntries(max int) []*Entry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.recent == nil || idx.recent.Len() == 0 || max <= 0 {
		return nil
	}
	arr := append([]*Entry(nil), (*idx.recent)...)
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].ModTime.After(arr[j].ModTime)
	})
	out := make([]*Entry, 0, min(max, len(arr)))
	seen := make(map[string]struct{}, len(arr))
	for _, e := range arr {
		if _, exists := idx.entries[e.Path]; !exists {
			continue // removed since it entered the heap
		}
		if _, dup := seen[e.Path]; dup {
			continue // keep the newest occurrence of a path
		}
		seen[e.Path] = struct{}{}
		out = append(out, e)
		if len(out) >= max {
			break
		}
	}
	return out
}

// recentMinHeap is a min-heap on ModTime; the top is the oldest entry.
type recentMinHeap []*Entry

func (h recentMinHeap) Len() int           { return len(h) }
func (h recentMinHeap) Less(i, j int) bool { return h[i].ModTime.Before(h[j].ModTime) }
func (h recentMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *recentMinHeap) Push(x any)        { *h = append(*h, x.(*Entry)) }
func (h *recentMinHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
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
	// Scope restricts results to paths under this directory ("" = no restriction)
	Scope string
	// Exclude drops paths matching any of these glob patterns ("" = no exclusion)
	Exclude []string
	// Types filters by file extension (no dot, case-insensitive, e.g. "go").
	// Directories are excluded when any type filter is set. Empty = no filter.
	Types []string
	// MinSize / MaxSize filter by file size in bytes (0 = unlimited).
	MinSize int64
	MaxSize int64
	// MtimeAfter / MtimeBefore filter by modification time (Unix seconds, 0 = unlimited).
	MtimeAfter  int64
	MtimeBefore int64
	// ExcludeHidden drops paths with a hidden (dot-prefixed) path segment.
	ExcludeHidden bool
	// Dedupe collapses multiple paths that refer to the same underlying file
	// (hard links), keeping only the first path for each identity. On
	// platforms without device/inode ids it falls back to (size, modtime).
	Dedupe bool

	// earlyStop is an internal optimisation flag: when it is safe (no sorting,
	// paging or post-filters that could change the result size), collection
	// stops once Limit entries have been gathered instead of scanning the
	// whole index. (docs/PERFORMANCE.md S2)
	earlyStop bool
}

// Builder builds the file index.
type Builder struct {
	idx           *Index
	ignoreMatcher *ignore.Matcher
	workerCount   int
	// throttleDelay holds the current per-entry throttle in nanoseconds.
	// Stored atomically so it can be changed while a build is running.
	throttleDelay atomic.Int64
	// progressFn reports build progress (number of entries scanned) when set.
	// Called from the build goroutine; must not block for long.
	progressFn func(scanned int64)
	// lastFiles/lastDirs record the most recent build totals (for status).
	lastFiles int64
	lastDirs  int64
	// lastPerDir records per-root-directory totals of the most recent build.
	lastPerDir map[string]struct{ Files, Dirs int64 }
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

// SetThrottleDelay updates the per-entry throttle delay of an in-flight or
// future build. 0 disables throttling. Safe to call from other goroutines.
func (b *Builder) SetThrottleDelay(d time.Duration) {
	b.throttleDelay.Store(int64(d))
}

// SetProgressCallback registers a callback invoked periodically during a
// build with the number of entries scanned so far, plus once with the final
// count when the build finishes. Set before calling Build/BuildThrottled.
func (b *Builder) SetProgressCallback(fn func(scanned int64)) {
	b.progressFn = fn
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
// The delay is dynamic: SetThrottleDelay can raise or lower it mid-build
// (e.g. speed up when a search request arrives during a throttled boot scan).
func (b *Builder) BuildThrottled(ctx context.Context, directories []string, throttleDelay time.Duration) error {
	b.SetThrottleDelay(throttleDelay)
	start := time.Now()

	var fileCount, dirCount int64
	perDir := make(map[string]struct{ Files, Dirs int64 })

	// Progress reporting: report every 500 entries or every 200ms, whichever
	// comes first, so UIs get a live "scanned N" count even during slow
	// throttled scans. The final total is always reported at the end.
	var scanned int64
	lastProgress := time.Now()
	reportProgress := func() {
		if b.progressFn != nil {
			b.progressFn(scanned)
		}
	}

	for _, dir := range directories {
		root := dir
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Apply throttling delay (read dynamically so a search request
			// can lift the throttle while the scan is running).
			if d := time.Duration(b.throttleDelay.Load()); d > 0 {
				time.Sleep(d)
			}

			if err != nil {
				return nil // Skip errors
			}

			// Check if path should be ignored (path, basename, or any
			// ancestor directory matches an ignore pattern)
			if b.ignoreMatcher != nil && b.ignoreMatcher.MatchPath(path) {
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
			fillDeviceInfo(entry, info)

			b.idx.Add(entry)

			if info.IsDir() {
				dirCount++
				pd := perDir[root]
				pd.Dirs++
				perDir[root] = pd
			} else {
				fileCount++
				pd := perDir[root]
				pd.Files++
				perDir[root] = pd
			}
			scanned++

			if b.progressFn != nil && (scanned%500 == 0 || time.Since(lastProgress) >= 200*time.Millisecond) {
				reportProgress()
				lastProgress = time.Now()
			}

			return nil
		})

		if err != nil && err != context.Canceled {
			slog.Warn("error walking directory", "path", dir, "error", err)
		}
	}

	reportProgress()

	elapsed := time.Since(start)
	b.lastFiles = fileCount
	b.lastDirs = dirCount
	b.lastPerDir = perDir
	slog.Info("indexed files and directories", "files", fileCount, "directories", dirCount, "elapsed", elapsed)

	return nil
}

// Stats returns the totals of the most recent build (files, directories).
func (b *Builder) Stats() (files, dirs int64) {
	return b.lastFiles, b.lastDirs
}

// PerDirStats returns per-root-directory totals of the most recent build,
// keyed by the configured root directory.
func (b *Builder) PerDirStats() map[string]struct{ Files, Dirs int64 } {
	return b.lastPerDir
}

// Index returns the built index.
func (b *Builder) Index() *Index {
	return b.idx
}

// Updater updates the index based on file system events.
type Updater struct {
	idx *Index
	// ignoreMatcher filters paths added during directory backfill so events
	// for ignored trees stay consistent with the builder's ignore patterns.
	ignoreMatcher *ignore.Matcher
	// Change callbacks let the daemon feed the same mutations to the
	// persistence strategy (incremental mode) without duplicating logic.
	onUpsert func(*Entry)
	onDelete func(string)
}

// NewUpdater creates a new index updater.
func NewUpdater(idx *Index) *Updater {
	return &Updater{idx: idx}
}

// NewUpdaterWithIgnore creates an updater that also respects ignore patterns
// when backfilling files inside newly created/moved-in directories.
func NewUpdaterWithIgnore(idx *Index, matcher *ignore.Matcher) *Updater {
	return &Updater{idx: idx, ignoreMatcher: matcher}
}

// NewUpdaterWithCallbacks creates an updater with ignore filtering and change
// callbacks (called for every mutation applied to the index).
func NewUpdaterWithCallbacks(idx *Index, matcher *ignore.Matcher, onUpsert func(*Entry), onDelete func(string)) *Updater {
	return &Updater{idx: idx, ignoreMatcher: matcher, onUpsert: onUpsert, onDelete: onDelete}
}

// HandleEvent handles a file system event.
func (u *Updater) HandleEvent(event watcher.Event) {
	switch {
	case event.Op&watcher.Create != 0:
		u.handleCreate(event.Path)
	case event.Op&watcher.Write != 0:
		// File content changed: refresh metadata (size/mtime).
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
	fillDeviceInfo(entry, info)

	u.idx.Add(entry)
	if u.onUpsert != nil {
		u.onUpsert(entry)
	}
	slog.Info("indexed", "path", path)

	// A new/moved-in directory: inotify only delivers a single event for the
	// directory itself, not for the files inside it. Backfill the existing
	// contents so a `mv dir` into the watched tree is fully indexed.
	if info.IsDir() {
		u.backfillDirectory(path)
	}
}

// backfillDirectory indexes the existing files under a directory (used when a
// directory appears via create/move, whose children produce no events).
func (u *Updater) backfillDirectory(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if u.ignoreMatcher != nil && u.ignoreMatcher.MatchPath(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// The directory itself was already added by handleCreate.
		if path != dir {
			entry := &Entry{
				Name:    info.Name(),
				Path:    path,
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}
			fillDeviceInfo(entry, info)
			u.idx.Add(entry)
			if u.onUpsert != nil {
				u.onUpsert(entry)
			}
		}
		return nil
	})
}

// handleRemove handles a file removal event.
func (u *Updater) handleRemove(path string) {
	u.idx.Remove(path)
	if u.onDelete != nil {
		u.onDelete(path)
	}
	slog.Info("removed", "path", path)
}
