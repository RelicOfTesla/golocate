// Package api provides API client for golocate-h5.
package api

import (
	"fmt"
	"time"

	"github.com/RelicOfTesla/golocate/internal/client"
	errpkg "github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

// Client represents the API client.
type Client struct {
	socketClient *client.Client
}

// NewClient creates a new API client.
func NewClient() *Client {
	return &Client{
		socketClient: client.New(),
	}
}

// SetSocketPath sets the socket path for the client.
func (c *Client) SetSocketPath(path string) {
	c.socketClient.SetSocketPath(path)
}

// SearchResponse represents a search response.
type SearchResponse struct {
	Results []*index.Entry  `json:"results"`
	Matches []*ContentMatch `json:"matches,omitempty"` // content search matches (when content keyword given)
	Count   int             `json:"count"`
	Total   int             `json:"total"` // Total results count (for pagination)
	Error   string          `json:"error,omitempty"`
}

// ContentMatch represents a content search match (mirrors internal/client.ContentMatch).
type ContentMatch struct {
	Path    string    `json:"Path"`
	LineNum int       `json:"LineNum"`
	Line    string    `json:"Line"`
	Match   string    `json:"Match"`
	Before  []string  `json:"Before"`
	After   []string  `json:"After"`
	Name    string    `json:"Name"`
	Size    int64     `json:"Size"`
	ModTime time.Time `json:"ModTime"`
}

// StatusResponse represents a status response.
type StatusResponse struct {
	Running          bool           `json:"running"`
	IndexSize        int            `json:"index_size"`
	IndexedFileCount int            `json:"indexed_file_count"`
	IsBuilding       bool           `json:"is_building"`
	Uptime           string         `json:"uptime"`
	LastBuildTime    string         `json:"last_build_time"`
	ConfigPath       string         `json:"config_path"`
	Pid              int            `json:"pid,omitempty"`
	OpenSupported    bool           `json:"open_supported"`
	Stats            map[string]int `json:"stats,omitempty"`
	Error            string         `json:"error,omitempty"`
}

// asInt converts a JSON-decoded number (float64 or int) to int.
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		var out int
		fmt.Sscanf(n, "%d", &out)
		return out
	}
	return 0
}

// asString converts a value to string when possible.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// asBool converts a value to bool when possible.
func asBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// SearchParams carries the parsed search options from the HTTP layer.
type SearchParams struct {
	Pattern     string // path pattern
	Content     string // file content keyword ("" = path search only)
	IgnoreCase  bool
	Regex       bool
	Basename    bool
	Dedupe      bool   // collapse hard links to one result
	PatternMode string // "", "regex", "wildcard"
	Limit       int
	Offset      int64
	SortField   string   // "", "name", "size", "time", "path"
	SortOrder   string   // "", "asc", "desc"
	Scope       string   // restrict results to this directory
	Exclude     []string // drop paths matching any glob
	Types       []string // file extension filters (e.g. "go")
	MinSize     int64    // min file size in bytes
	MaxSize     int64    // max file size in bytes
}

// searchOptions converts HTTP-level search params into index.SearchOptions.
// Pure function, unit-testable without a socket.
func searchOptions(p SearchParams) index.SearchOptions {
	opts := index.SearchOptions{
		IgnoreCase: p.IgnoreCase,
		Basename:   p.Basename,
		Dedupe:     p.Dedupe,
		Limit:      p.Limit,
		Offset:     p.Offset,
		SortField:  p.SortField,
		SortOrder:  p.SortOrder,
		Scope:      p.Scope,
		Exclude:    p.Exclude,
		Types:      p.Types,
		MinSize:    p.MinSize,
		MaxSize:    p.MaxSize,
	}
	if p.Regex {
		opts.PatternMode = index.PatternModeExtendedRegex
	}
	switch p.PatternMode {
	case "regex":
		opts.PatternMode = index.PatternModeExtendedRegex
	case "wildcard":
		opts.PatternMode = index.PatternModeWildcard
	case "terms":
		opts.PatternMode = index.PatternModeTerms
	}
	return opts
}

// Search performs a search query using the fast protocol.
func (c *Client) Search(p SearchParams) (*SearchResponse, error) {
	opts := searchOptions(p)

	// Content search path
	if p.Content != "" {
		result, err := c.socketClient.SearchContent(p.Pattern, p.Content, opts)
		if err != nil {
			// Check if it's a server not running error
			if errpkg.IsServerNotRunningError(err) {
				return &SearchResponse{
					Error: errpkg.GetFriendlyErrorMessage(err),
				}, nil
			}
			return nil, err
		}
		matches := make([]*ContentMatch, 0, len(result.Matches))
		for _, m := range result.Matches {
			matches = append(matches, &ContentMatch{
				Path:    m.Path,
				LineNum: m.LineNum,
				Line:    m.Line,
				Match:   m.Match,
				Before:  m.Before,
				After:   m.After,
				Name:    m.Name,
				Size:    m.Size,
				ModTime: m.ModTime,
			})
		}
		return &SearchResponse{
			Matches: matches,
			Count:   result.Count,
			Total:   result.Total,
		}, nil
	}

	// Use SearchFast for better performance
	result, err := c.socketClient.SearchFast(p.Pattern, opts)
	if err != nil {
		// Check if it's a server not running error
		if errpkg.IsServerNotRunningError(err) {
			return &SearchResponse{
				Error: errpkg.GetFriendlyErrorMessage(err),
			}, nil
		}
		return nil, err
	}

	return &SearchResponse{
		Results: result.Entries,
		Count:   result.Count,
		Total:   result.Total,
	}, nil
}

// Open asks the daemon to open a file or directory with the platform's
// default application (validated against the daemon's allowed directories).
func (c *Client) Open(path string) error {
	return c.socketClient.Open(path)
}

// Status gets the server status.
func (c *Client) Status() (*StatusResponse, error) {
	status, err := c.socketClient.Status()
	if err != nil {
		// Check if it's a server not running error
		if errpkg.IsServerNotRunningError(err) {
			return &StatusResponse{
				Running: false,
				Error:   errpkg.GetFriendlyErrorMessage(err),
			}, nil
		}
		return nil, err
	}

	return &StatusResponse{
		Running:          asBool(status["running"]),
		IndexSize:        asInt(status["index_size"]),
		IndexedFileCount: asInt(status["indexed_file_count"]),
		IsBuilding:       asBool(status["is_building"]),
		Uptime:           asString(status["uptime"]),
		LastBuildTime:    asString(status["last_build_time"]),
		ConfigPath:       asString(status["config_path"]),
		Pid:              asInt(status["pid"]),
		OpenSupported:    asBool(status["open_supported"]),
		Stats:            asStats(status["stats"]),
	}, nil
}

// asStats converts a decoded "stats" object ({key: number}) into ints.
func asStats(v any) map[string]int {
	out := map[string]int{}
	if m, ok := v.(map[string]any); ok {
		for k, val := range m {
			out[k] = asInt(val)
		}
	}
	return out
}

// GetConfig gets the server configuration.
func (c *Client) GetConfig() (map[string]any, error) {
	config, err := c.socketClient.GetConfig()
	if err != nil {
		return nil, err
	}
	return config, nil
}

// SetConfig sets the server configuration from YAML content.
func (c *Client) SetConfig(yamlContent string) error {
	err := c.socketClient.SetConfig(yamlContent)
	if err != nil {
		return err
	}
	return nil
}

// Build triggers an index rebuild on the server.
func (c *Client) Build() error {
	err := c.socketClient.Build()
	if err != nil {
		return err
	}
	return nil
}
