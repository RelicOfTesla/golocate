// Package content provides file content search functionality.
package content

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// SearchResult represents a content search result.
type SearchResult struct {
	Path    string
	LineNum int
	Line    string
	Match   string
}

// SearchOptions contains options for content search.
type SearchOptions struct {
	Pattern     string
	IgnoreCase  bool
	Regex       bool
	ExtendedRegex bool
	MaxFileSize int64 // Maximum file size to search (in bytes)
	MaxResults  int   // Maximum number of results
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
	
	// Open file
	file, err := os.Open(path)
	if err != nil {
		return results
	}
	defer file.Close()
	
	// Scan file line by line
	scanner := bufio.NewScanner(file)
	lineNum := 0
	
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		
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
			results = append(results, &SearchResult{
				Path:    path,
				LineNum: lineNum,
				Line:    line,
				Match:   match,
			})
			
			if len(results) >= s.opts.MaxResults {
				return results
			}
		}
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
	
	// Check for null bytes (common in binary files)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	
	return false
}
