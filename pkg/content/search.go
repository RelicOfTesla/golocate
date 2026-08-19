// Package content provides file content search functionality.
package content

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// SearchResult represents a content search result.
type SearchResult struct {
	Path    string
	LineNum int
	Line    string
	Match   string
	Before  []string // context lines before the match (grep -C style, may be empty)
	After   []string // context lines after the match (may be empty)
}

// SearchOptions contains options for content search.
type SearchOptions struct {
	Pattern       string
	IgnoreCase    bool
	Regex         bool
	ExtendedRegex bool
	MaxFileSize   int64 // Maximum file size to search (in bytes)
	MaxResults    int   // Maximum number of results
}

// Searcher performs content search.
type Searcher struct {
	opts    SearchOptions
	pattern *regexp.Regexp
}

// NewSearcher creates a new content searcher.
func NewSearcher(opts SearchOptions) (*Searcher, error) {
	s := &Searcher{opts: opts}

	// Compile pattern if regex
	if opts.Regex || opts.ExtendedRegex {
		var re *regexp.Regexp
		var err error

		if opts.ExtendedRegex {
			if opts.IgnoreCase {
				re, err = regexp.Compile("(?i)" + opts.Pattern)
			} else {
				re, err = regexp.Compile(opts.Pattern)
			}
		} else {
			if opts.IgnoreCase {
				re, err = regexp.CompilePOSIX("(?i)" + opts.Pattern)
			} else {
				re, err = regexp.CompilePOSIX(opts.Pattern)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}

		s.pattern = re
	}

	// Set defaults
	if s.opts.MaxFileSize == 0 {
		s.opts.MaxFileSize = 10 * 1024 * 1024 // 10MB default
	} else if s.opts.MaxFileSize < 0 {
		return nil, fmt.Errorf("invalid parameter: max_file_size cannot be negative")
	}
	if s.opts.MaxResults == 0 {
		s.opts.MaxResults = 1000 // 1000 results default
	} else if s.opts.MaxResults < 0 {
		return nil, fmt.Errorf("invalid parameter: max_results cannot be negative")
	}

	return s, nil
}

// Search searches for the pattern in the given files.
func (s *Searcher) Search(ctx context.Context, files []string) ([]*SearchResult, error) {
	var (
		results []*SearchResult
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	// Create a semaphore to limit concurrency
	sem := make(chan struct{}, 10) // Max 10 concurrent files

	for _, file := range files {
		// Check context
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// Check if we have enough results
		if len(results) >= s.opts.MaxResults {
			break
		}

		wg.Add(1)
		go func(path string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Search in file
			fileResults := s.searchFile(path)

			// Append results
			mu.Lock()
			if len(results)+len(fileResults) <= s.opts.MaxResults {
				results = append(results, fileResults...)
			} else {
				remaining := s.opts.MaxResults - len(results)
				results = append(results, fileResults[:remaining]...)
			}
			mu.Unlock()
		}(file)
	}

	wg.Wait()
	return results, nil
}

// searchFile searches for the pattern in a single file.
func (s *Searcher) searchFile(path string) []*SearchResult {
	var results []*SearchResult

	// Check file size
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return results
	}

	if info.Size() > s.opts.MaxFileSize {
		return results
	}

	// Skip binary files (simple heuristic) so binary garbage never
	// pollutes results or the protocol response. UTF-16 text (BOM or
	// NUL-spaced ASCII) is NOT treated as binary.
	if isBinaryFile(path) {
		return results
	}

	// Read the file and decode to UTF-8 text (UTF-8 as-is, UTF-16 by BOM,
	// otherwise GBK fallback).
	text, err := readTextFile(path, s.opts.MaxFileSize)
	if err != nil {
		return results
	}

	// Scan line by line, keeping one line of context on each side
	// of every match (grep -C1 style).
	lines := strings.Split(text, "\n")
	lineNum := 0
	prevLine := ""               // previous line, becomes Before of the next match
	var lastResult *SearchResult // most recent match awaiting its After line

	fillAfter := func(line string) {
		if lastResult != nil && len(lastResult.After) == 0 {
			lastResult.After = append(lastResult.After, line)
		}
	}

	for _, raw := range lines {
		line := strings.TrimSuffix(raw, "\r") // normalize CRLF
		lineNum++
		fillAfter(line)

		// Search in line
		var matches []string
		if s.pattern != nil {
			// Regex search
			matches = s.pattern.FindAllString(line, -1)
		} else {
			// Substring search
			if s.opts.IgnoreCase {
				if strings.Contains(strings.ToLower(line), strings.ToLower(s.opts.Pattern)) {
					matches = []string{s.opts.Pattern}
				}
			} else {
				if strings.Contains(line, s.opts.Pattern) {
					matches = []string{s.opts.Pattern}
				}
			}
		}

		// Add results
		for _, match := range matches {
			res := &SearchResult{
				Path:    path,
				LineNum: lineNum,
				Line:    line,
				Match:   match,
			}
			// Blank lines are legitimate context (grep shows them too), so
			// always carry the previous line when one exists.
			if lineNum > 1 {
				res.Before = append(res.Before, prevLine)
			}
			results = append(results, res)
			lastResult = res

			if len(results) >= s.opts.MaxResults {
				return results
			}
		}

		prevLine = line
	}

	return results
}

// SearchInDirectory searches for the pattern in all files under the directory.
func (s *Searcher) SearchInDirectory(ctx context.Context, dir string) ([]*SearchResult, error) {
	var files []string

	// Walk directory
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip large files
		if info.Size() > s.opts.MaxFileSize {
			return nil
		}

		// Skip binary files (simple heuristic)
		if isBinaryFile(path) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.Search(ctx, files)
}

// isBinaryFile checks if a file is binary (simple heuristic).
// Files with a UTF-16 byte order mark are treated as text, because UTF-16
// text contains plenty of NUL bytes by design.
func isBinaryFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()

	// Read first 512 bytes
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return true
	}
	buf = buf[:n]

	// UTF-16 BOM (FF FE or FE FF): textual, despite the NUL bytes inside.
	if len(buf) >= 2 {
		if (buf[0] == 0xFF && buf[1] == 0xFE) || (buf[0] == 0xFE && buf[1] == 0xFF) {
			return false
		}
	}

	// Check for null bytes (common in binary files)
	for _, b := range buf {
		if b == 0 {
			// BOM-less UTF-16 text (e.g. ASCII "h\0e\0l\0l\0o\0") also
			// contains NULs; accept it when the alternating-NUL pattern
			// is clear, otherwise treat as binary.
			return !looksLikeUTF16(buf)
		}
	}

	return false
}

// readTextFile reads a file (up to maxSize bytes) and decodes it to UTF-8:
//   - UTF-8 (valid) and plain ASCII are used as-is
//   - UTF-16 (LE/BE) is detected by BOM and decoded
//   - anything else falls back to GBK, which covers the common Chinese
//     documents that are not UTF-8 encoded
func readTextFile(path string, maxSize int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxSize))
	if err != nil {
		return "", err
	}
	return decodeText(data), nil
}

// decodeText converts raw file bytes to a UTF-8 string.
func decodeText(data []byte) string {
	// UTF-16 by BOM.
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			return decodeUTF16(data[2:], false)
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			return decodeUTF16(data[2:], true)
		}
	}
	// UTF-8 (includes plain ASCII): use as-is, unless the bytes carry the
	// alternating-NUL pattern of BOM-less UTF-16 (NULs are valid UTF-8, so
	// utf8.Valid alone is not enough to tell them apart).
	if utf8.Valid(data) {
		if !looksLikeUTF16(data) {
			return string(data)
		}
		return decodeUTF16(data, false)
	}
	// Fallback: GBK (common for Chinese text saved without BOM).
	if r, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), string(data)); err == nil && utf8.ValidString(r) {
		return r
	}
	// Last resort: raw bytes (garbage in, garbage out).
	return string(data)
}

// looksLikeUTF16 reports whether data has the alternating-NUL byte pattern of
// BOM-less UTF-16 text (e.g. ASCII "h\0e\0l\0l\0o\0"): at least half of the
// bytes on one parity are NULs. Short buffers are never classified as UTF-16
// (e.g. a 4-byte {0x00,0x01,0x02,0x03} seed is too ambiguous).
func looksLikeUTF16(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	var evenZeros, oddZeros int
	n := len(data) / 2
	for i := 0; i+1 < len(data); i += 2 {
		if data[i] == 0 {
			evenZeros++
		}
		if data[i+1] == 0 {
			oddZeros++
		}
	}
	return evenZeros >= n/2 || oddZeros >= n/2
}

// decodeUTF16 decodes UTF-16 code units into a UTF-8 string.
func decodeUTF16(b []byte, bigEndian bool) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1] // drop a dangling odd byte
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
		}
	}
	return string(utf16.Decode(units))
}
