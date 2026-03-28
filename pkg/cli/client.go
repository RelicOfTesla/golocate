// Package cliclient provides CLI client functions for golocate/golocated.
package cliclient

import (
	"fmt"
	"log"
	"os"

	"github.com/RelicOfTesla/golocate/internal/client"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/search"
)

// SearchOptions contains search options for CLI.
type SearchOptions struct {
	Pattern     string
	IgnoreCase  bool
	Basename    bool
	Limit       int
	Regex       bool
	Regexp      bool
	Count       bool
	Sort        string
	SocketPath  string
}

// SearchResult contains search results.
type SearchResult struct {
	Entries []*index.Entry
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
	
	// Check if server is running
	if !c.IsServerRunning() {
		return nil, fmt.Errorf("golocated server is not running. Please start it with: golocated --service")
	}
	
	// Build search options
	searchOpts := index.SearchOptions{
		IgnoreCase: opts.IgnoreCase,
		Basename:   opts.Basename,
		Limit:      opts.Limit,
	}
	
	// Handle sorting
	if opts.Sort != "" {
		sortOpts := search.ParseSort(opts.Sort)
		searchOpts.SortField = sortOpts.Field.String()
		searchOpts.SortOrder = sortOpts.Order.String()
	}
	
	// Handle regex
	if opts.Regex || opts.Regexp {
		searchOpts.Regex = true
		if opts.Regexp {
			searchOpts.ExtendedRegex = false // basic regex
		} else {
			searchOpts.ExtendedRegex = true // extended regex
		}
	}
	
	// Perform search
	entries, err := c.Search(opts.Pattern, searchOpts)
	if err != nil {
		return nil, err
	}
	
	// Sort if needed (server should handle this, but fallback)
	if opts.Sort != "" {
		sortOpts := search.ParseSort(opts.Sort)
		search.Sort(entries, sortOpts)
	}
	
	return &SearchResult{
		Entries: entries,
		Count:   len(entries),
	}, nil
}

// SearchStream performs a search with streaming results.
func SearchStream(opts SearchOptions, callback func(*index.Entry) bool) error {
	// Create client
	c := client.New()
	if opts.SocketPath != "" {
		c.SetSocketPath(opts.SocketPath)
	}
	
	// Check if server is running
	if !c.IsServerRunning() {
		return fmt.Errorf("golocated server is not running. Please start it with: golocated --service")
	}
	
	// Build search options
	searchOpts := index.SearchOptions{
		IgnoreCase: opts.IgnoreCase,
		Basename:   opts.Basename,
		Limit:      opts.Limit,
		Regex:      opts.Regex || opts.Regexp,
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
func ReloadConfig() error {
	c := client.New()
	return c.ReloadConfig()
}

// Build sends a build request to the server.
func Build(socketPath string) error {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}
	
	// Check if server is running
	if !c.IsServerRunning() {
		return fmt.Errorf("golocated server is not running. Please start it with: golocated --service")
	}
	
	// Send build request
	return c.Build()
}

// Status gets the server status.
func Status(socketPath string) (map[string]interface{}, error) {
	// Create client
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}
	
	// Check if server is running
	if !c.IsServerRunning() {
		return nil, fmt.Errorf("golocated server is not running")
	}
	
	// Get status
	return c.Status()
}

// PrintResults prints search results to stdout.
func PrintResults(results *SearchResult, countOnly bool) {
	if countOnly {
		fmt.Println(results.Count)
		return
	}
	
	for _, entry := range results.Entries {
		fmt.Println(entry.Path)
	}
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
func Fatal(format string, args ...interface{}) {
	log.Printf(format, args...)
	os.Exit(1)
}
