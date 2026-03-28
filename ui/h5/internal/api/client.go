// Package api provides API client for golocate-h5.
package api

import (
	"github.com/RelicOfTesla/golocate/internal/client"
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

// SearchResponse represents a search response.
type SearchResponse struct {
	Results []*index.Entry `json:"results"`
	Count   int            `json:"count"`
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
func (c *Client) Search(pattern, path string, ignoreCase bool, limit int) (*SearchResponse, error) {
	opts := index.SearchOptions{
		IgnoreCase: ignoreCase,
		Basename:   false,
		Limit:      limit,
		Path:       path,
	}

	// Use SearchFast for better performance
	entries, err := c.socketClient.SearchFast(pattern, opts)
	if err != nil {
		return nil, err
	}

	return &SearchResponse{
		Results: entries,
		Count:   len(entries),
	}, nil
}

// Status gets the server status.
func (c *Client) Status() (*StatusResponse, error) {
	status, err := c.socketClient.Status()
	if err != nil {
		return nil, err
	}

	running, _ := status["running"].(bool)
	indexSize, _ := status["index_size"].(int)

	return &StatusResponse{
		Running:   running,
		IndexSize: indexSize,
	}, nil
}
