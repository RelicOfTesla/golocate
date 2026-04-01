// Package protocol provides protocol abstraction for golocate.
package protocol

// Result represents a search result.
type Result struct {
	Path string
	Name string
	Size int64
}

// SearchResults represents search results with metadata.
type SearchResults struct {
	Results []*Result
	Count   int
	Total   int
}
