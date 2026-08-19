// Package cliclient provides CLI client functions for golocate/golocated.
package cliclient

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/RelicOfTesla/golocate/internal/client"
	errpkg "github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/search"
)

// SearchOptions contains search options for CLI.
type SearchOptions struct {
	Pattern     string
	Content     string // file content keyword (empty = path search only)
	IgnoreCase  bool
	Basename    bool
	Limit       int
	Regex       bool
	Regexp      bool
	Terms       bool // multi-term mode: "foo bar -baz" (AND, exclude with -)
	Count       bool
	Sort        string
	Scope       string   // restrict results to this directory
	Exclude     []string // drop paths matching any glob
	Types       []string // file extension filters (e.g. "go")
	MinSize     int64    // min file size in bytes
	MaxSize     int64    // max file size in bytes
	MtimeAfter  int64    // modified after (Unix seconds)
	MtimeBefore int64    // modified before (Unix seconds)
	NoHidden    bool     // exclude dotfiles
	Dedupe      bool     // collapse hard links to one result
	Existing    bool     // only show files that still exist on disk
	Null        bool     // NUL-separated output (xargs-friendly)
	Long        bool     // long format: size + mtime + path
	SocketPath  string
}

// SearchResult contains search results.
type SearchResult struct {
	Entries []*index.Entry
	Matches []*client.ContentMatch // set when opts.Content != ""
	Count   int
	Error   error
}

// Search performs a search using the client.
func Search(opts SearchOptions) (*SearchResult, error) {
	// Create client
	c := client.New()
	if opts.SocketPath != "" {
		c.SetSocketPath(opts.SocketPath)
	}

	// Build search options
	searchOpts := index.SearchOptions{
		IgnoreCase:    opts.IgnoreCase,
		Basename:      opts.Basename,
		Limit:         opts.Limit,
		Scope:         opts.Scope,
		Exclude:       opts.Exclude,
		Types:         opts.Types,
		MinSize:       opts.MinSize,
		MaxSize:       opts.MaxSize,
		MtimeAfter:    opts.MtimeAfter,
		MtimeBefore:   opts.MtimeBefore,
		ExcludeHidden: opts.NoHidden,
		Dedupe:        opts.Dedupe,
	}

	// Handle sorting
	if opts.Sort != "" {
		sortOpts := search.ParseSort(opts.Sort)
		searchOpts.SortField = sortOpts.Field.String()
		searchOpts.SortOrder = sortOpts.Order.String()
	}

	// Handle regex
	if opts.Regex || opts.Regexp {
		if opts.Regexp {
			searchOpts.PatternMode = index.PatternModeRegex // basic regex
		} else {
			searchOpts.PatternMode = index.PatternModeExtendedRegex // extended regex
		}
	}

	// Handle multi-term mode ("foo bar -baz")
	if opts.Terms {
		searchOpts.PatternMode = index.PatternModeTerms
	}

	// Perform search
	var result *SearchResult
	if opts.Content != "" {
		// Content search: server returns matches with line numbers
		contentResult, err := c.SearchContent(opts.Pattern, opts.Content, searchOpts)
		if err != nil {
			// If it's a server not running error, wrap it with friendly message
			if errpkg.IsServerNotRunningError(err) {
				return nil, err
			}
			return nil, err
		}
		result = &SearchResult{
			Matches: contentResult.Matches,
			Count:   contentResult.Count,
		}
	} else {
		entries, err := c.Search(opts.Pattern, searchOpts)
		if err != nil {
			// If it's a server not running error, wrap it with friendly message
			if errpkg.IsServerNotRunningError(err) {
				return nil, err
			}
			return nil, err
		}
		result = &SearchResult{
			Entries: entries,
			Count:   len(entries),
		}
	}

	// -e/--existing: drop entries whose file no longer exists on disk
	if opts.Existing {
		if len(result.Entries) > 0 {
			filtered := result.Entries[:0]
			for _, entry := range result.Entries {
				if _, err := os.Stat(entry.Path); err == nil {
					filtered = append(filtered, entry)
				}
			}
			result.Entries = filtered
			result.Count = len(filtered)
		}
		if len(result.Matches) > 0 {
			// Content-search matches carry the path too; filter them as well
			// so --content -e reports only files that still exist.
			filtered := result.Matches[:0]
			for _, m := range result.Matches {
				if _, err := os.Stat(m.Path); err == nil {
					filtered = append(filtered, m)
				}
			}
			result.Matches = filtered
			result.Count = len(filtered)
		}
	}

	// Sort if needed (server should handle this, but fallback)
	if opts.Sort != "" && len(result.Entries) > 0 {
		sortOpts := search.ParseSort(opts.Sort)
		search.Sort(result.Entries, sortOpts)
	}

	return result, nil
}

// SearchStream performs a search with streaming results.
func SearchStream(opts SearchOptions, callback func(*index.Entry) bool) error {
	// Create client
	c := client.New()
	if opts.SocketPath != "" {
		c.SetSocketPath(opts.SocketPath)
	}

	// Build search options
	searchOpts := index.SearchOptions{
		IgnoreCase: opts.IgnoreCase,
		Basename:   opts.Basename,
		Limit:      opts.Limit,
	}

	// Handle regex pattern mode
	if opts.Regex || opts.Regexp {
		if opts.Regexp {
			searchOpts.PatternMode = index.PatternModeRegex // basic regex
		} else {
			searchOpts.PatternMode = index.PatternModeExtendedRegex // extended regex
		}
	}

	// Handle sorting
	if opts.Sort != "" {
		sortOpts := search.ParseSort(opts.Sort)
		searchOpts.SortField = sortOpts.Field.String()
		searchOpts.SortOrder = sortOpts.Order.String()
	}

	// Perform streaming search
	return c.SearchStream(opts.Pattern, searchOpts, callback)
}

// ReloadConfig sends a reload-config request to the server.
func ReloadConfig(socketPath string) error {
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}
	return c.ReloadConfig()
}

// Build sends a build request to the server.
func Build(socketPath string) error {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}

	// Send build request
	return c.Build()
}

// Stop sends a stop request to the server.
func Stop(socketPath string) error {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}

	// Send stop request
	return c.Stop()
}

// Status gets the server status.
func Status(socketPath string) (map[string]any, error) {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}

	// Get status
	return c.Status()
}

// GetConfig gets the server configuration (including default values).
func GetConfig(socketPath string) (map[string]any, error) {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}

	// Get config
	return c.GetConfig()
}

// SetConfig sets the server configuration from YAML content.
func SetConfig(socketPath, yamlContent string) error {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}

	// Set config
	return c.SetConfig(yamlContent)
}

// PrintResults prints search results to stdout.
// nullMode selects NUL-separated output (-0, xargs-friendly); longMode prints
// "size<TAB>mtime<TAB>path" (locate -l style).
func PrintResults(results *SearchResult, countOnly, nullMode, longMode bool) {
	if countOnly {
		fmt.Println(results.Count)
		return
	}

	// Content search results: grep-style "path:line:text" with optional
	// context lines (path-N for lines around the match).
	if len(results.Matches) > 0 {
		for _, m := range results.Matches {
			if nullMode {
				// NUL mode: match line only, so output stays xargs-safe.
				fmt.Printf("%s:%d:%s\x00", m.Path, m.LineNum, m.Line)
				continue
			}
			for i, b := range m.Before {
				fmt.Printf("%s-%d:%s\n", m.Path, m.LineNum-len(m.Before)+i, b)
			}
			fmt.Printf("%s:%d:%s\n", m.Path, m.LineNum, m.Line)
			for i, a := range m.After {
				fmt.Printf("%s-%d:%s\n", m.Path, m.LineNum+i+1, a)
			}
		}
		return
	}

	for _, entry := range results.Entries {
		if nullMode {
			fmt.Print(entry.Path)
			fmt.Print("\x00")
		} else if longMode {
			fmt.Printf("%d\t%s\t%s\n", entry.Size, entry.ModTime.Format("2006-01-02 15:04"), entry.Path)
		} else {
			fmt.Println(entry.Path)
		}
	}
}

// ParseSize parses a size string with optional K/M/G suffix (e.g. "1024", "1M").
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'K', 'k':
		mult = 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size: %q", s)
	}
	return n * mult, nil
}

// ParseMtime parses a date/time into Unix seconds. Accepts "2006-01-02" and
// "2006-01-02 15:04" (local time).
func ParseMtime(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty time")
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("invalid time: %q (use YYYY-MM-DD or YYYY-MM-DD HH:MM)", s)
}

// PrintResultsStream prints streaming search results to stdout.
func PrintResultsStream(opts SearchOptions) error {
	count := 0
	err := SearchStream(opts, func(entry *index.Entry) bool {
		fmt.Println(entry.Path)
		count++
		return true // continue streaming
	})

	if opts.Count && err == nil {
		fmt.Println(count)
	}

	return err
}

// IsServerRunning checks if the server is running.
func IsServerRunning(socketPath string) bool {
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}
	return c.IsServerRunning()
}

// Fatal logs a fatal error and exits.
func Fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
