// Package api provides API client for golocate-h5.
package api

import (
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
	Results []*index.Entry `json:"results"`
	Count   int            `json:"count"`
	Total   int            `json:"total"` // Total results count (for pagination)
	Error   string         `json:"error,omitempty"`
}

// StatusResponse represents a status response.
type StatusResponse struct {
	Running   bool   `json:"running"`
	IndexSize int    `json:"index_size"`
	Uptime    string `json:"uptime"`
	Error     string `json:"error,omitempty"`
}

// Search performs a search query using the fast protocol.
func (c *Client) Search(pattern string, ignoreCase bool, limit int, offset int64) (*SearchResponse, error) {
	opts := index.SearchOptions{
		IgnoreCase: ignoreCase,
		Basename:   false,
		Limit:      limit,
		Offset:     offset,
	}

	// Use SearchFast for better performance
	result, err := c.socketClient.SearchFast(pattern, opts)
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

	running, _ := status["running"].(bool)
	indexSize, _ := status["index_size"].(int)

	return &StatusResponse{
		Running:   running,
		IndexSize: indexSize,
	}, nil
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
