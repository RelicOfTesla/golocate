// Optional in-memory content index: keyword tokens -> file paths.
// When enabled (config content_index: true) it makes content search
// candidates precise for single-word queries, avoiding the full-index scan
// (and its 5000-file cap). It is NOT persisted and is rebuilt on every index
// build; watcher-driven incremental updates are intentionally not performed,
// so the index is a snapshot of the last build.
package content

import (
	"os"
	"strings"
	"sync"
	"unicode"
)

// MaxTokensPerFile bounds how many tokens are extracted from one file, so the
// in-memory footprint stays predictable on large trees.
const MaxTokensPerFile = 256

// MinTokenLen / MaxTokenLen bound token sizes kept in the index.
const (
	MinTokenLen = 3
	MaxTokenLen = 32
)

// Index maps lowercased tokens to the set of file paths containing them.
type Index struct {
	mu          sync.RWMutex
	tokens      map[string]map[string]struct{}
	fileCount   int
	maxFileSize int64
}

// NewIndex creates an empty content index. Files larger than maxFileSize are
// skipped (0 = use the default 10MB).
func NewIndex(maxFileSize int64) *Index {
	if maxFileSize <= 0 {
		maxFileSize = 10 * 1024 * 1024
	}
	return &Index{
		tokens:      make(map[string]map[string]struct{}),
		maxFileSize: maxFileSize,
	}
}

// AddFile reads and tokenizes the file, recording each token -> path. Errors
// (unreadable, too large, binary) are ignored so indexing never fails builds.
func (ix *Index) AddFile(path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > ix.maxFileSize {
		return
	}
	if isBinaryFile(path) {
		return
	}
	text, err := readTextFile(path, ix.maxFileSize)
	if err != nil {
		return
	}

	seen := make(map[string]struct{}, MaxTokensPerFile)
	count := 0
	for _, tok := range tokenize(text) {
		if _, dup := seen[tok]; dup {
			continue
		}
		seen[tok] = struct{}{}
		ix.addToken(tok, path)
		count++
		if count >= MaxTokensPerFile {
			break
		}
	}
	if count > 0 {
		ix.mu.Lock()
		ix.fileCount++
		ix.mu.Unlock()
	}
}

// RemoveFile drops all tokens recorded for a path (for future incremental
// maintenance; currently unused by the daemon).
func (ix *Index) RemoveFile(path string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	for tok, paths := range ix.tokens {
		if _, ok := paths[path]; ok {
			delete(paths, path)
			if len(paths) == 0 {
				delete(ix.tokens, tok)
			}
		}
	}
	ix.fileCount--
	if ix.fileCount < 0 {
		ix.fileCount = 0
	}
}

func (ix *Index) addToken(tok, path string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	paths := ix.tokens[tok]
	if paths == nil {
		paths = make(map[string]struct{})
		ix.tokens[tok] = paths
	}
	paths[path] = struct{}{}
}

// Lookup returns the paths whose tokens include the keyword (case-insensitive).
// A nil/empty result means the keyword appears as no whole token — callers
// must fall back to the normal scan, because substring matches are still
// possible (e.g. keyword "hello" inside token "helloworld").
func (ix *Index) Lookup(keyword string) []string {
	key := strings.ToLower(keyword)
	if key == "" {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	paths, ok := ix.tokens[key]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(paths))
	for p := range paths {
		out = append(out, p)
	}
	return out
}

// FileCount returns the number of successfully indexed files.
func (ix *Index) FileCount() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.fileCount
}

// tokenize splits text into lowercased word tokens of alphanumeric runs.
// Length is measured in runes (a 2-character CJK word is too short).
func tokenize(text string) []string {
	var toks []string
	var cur strings.Builder
	runeCount := 0
	flush := func() {
		if runeCount >= MinTokenLen && runeCount <= MaxTokenLen {
			toks = append(toks, cur.String())
		}
		cur.Reset()
		runeCount = 0
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			cur.WriteRune(unicode.ToLower(r))
			runeCount++
		} else {
			flush()
		}
	}
	flush()
	return toks
}

// Paths returns all indexed paths (for diagnostics).
func (ix *Index) Paths() []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	seen := make(map[string]struct{})
	for _, paths := range ix.tokens {
		for p := range paths {
			seen[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}
