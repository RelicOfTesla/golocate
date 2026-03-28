// Package index provides n-gram based inverted index for fast substring search.
package index

import (
	"strings"
	"sync"
)

// NGramIndex represents an n-gram based inverted index.
// This index enables fast substring search by breaking strings into n-grams
// and mapping each n-gram to the entries that contain it.
type NGramIndex struct {
	mu      sync.RWMutex
	n       int                       // n-gram size (default: 3)
	index   map[string][]*Entry       // n-gram -> entries
	entries map[string]bool           // all indexed entry paths
}

// NewNGramIndex creates a new n-gram index with the given n-gram size.
func NewNGramIndex(n int) *NGramIndex {
	if n <= 0 {
		n = 3 // default trigram
	}
	return &NGramIndex{
		n:       n,
		index:   make(map[string][]*Entry),
		entries: make(map[string]bool),
	}
}

// Add adds an entry to the n-gram index.
func (idx *NGramIndex) Add(entry *Entry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Skip if already indexed
	if idx.entries[entry.Path] {
		return
	}
	idx.entries[entry.Path] = true

	// Extract n-grams from the entry name
	ngrams := idx.extractNGrams(entry.Name)
	for _, ngram := range ngrams {
		idx.index[ngram] = append(idx.index[ngram], entry)
	}
}

// Remove removes an entry from the n-gram index.
func (idx *NGramIndex) Remove(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.entries[path] {
		return
	}
	delete(idx.entries, path)

	// Remove from all n-gram lists
	for ngram, entries := range idx.index {
		filtered := make([]*Entry, 0, len(entries))
		for _, e := range entries {
			if e.Path != path {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			idx.index[ngram] = filtered
		} else {
			delete(idx.index, ngram)
		}
	}
}

// Search searches for entries that contain the given query.
// Returns a list of candidate entries that likely contain the query.
func (idx *NGramIndex) Search(query string, ignoreCase bool) []*Entry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if ignoreCase {
		query = strings.ToLower(query)
	}

	// Extract n-grams from the query
	ngrams := idx.extractNGrams(query)
	if len(ngrams) == 0 {
		return nil
	}

	// Count how many n-grams each entry matches
	entryScores := make(map[string]int)
	for _, ngram := range ngrams {
		if entries, ok := idx.index[ngram]; ok {
			for _, entry := range entries {
				entryScores[entry.Path]++
			}
		}
	}

	// Return entries that match all n-grams (high precision)
	// or at least half of n-grams (high recall)
	minScore := len(ngrams)
	if minScore > 2 {
		minScore = (len(ngrams) + 1) / 2
	}

	var results []*Entry
	seen := make(map[string]bool)
	for path, score := range entryScores {
		if score >= minScore {
			// Find the entry in the n-gram index
			for _, entry := range idx.index[ngrams[0]] {
				if entry.Path == path && !seen[path] {
					results = append(results, entry)
					seen[path] = true
					break
				}
			}
		}
	}

	return results
}

// extractNGrams extracts n-grams from a string.
func (idx *NGramIndex) extractNGrams(s string) []string {
	if len(s) < idx.n {
		return []string{s}
	}

	// Convert to lowercase for case-insensitive indexing
	s = strings.ToLower(s)

	ngrams := make([]string, 0, len(s)-idx.n+1)
	for i := 0; i <= len(s)-idx.n; i++ {
		ngrams = append(ngrams, s[i:i+idx.n])
	}
	return ngrams
}

// Len returns the number of indexed entries.
func (idx *NGramIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// Clear clears the n-gram index.
func (idx *NGramIndex) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.index = make(map[string][]*Entry)
	idx.entries = make(map[string]bool)
}

// Stats returns statistics about the n-gram index.
func (idx *NGramIndex) Stats() map[string]interface{} {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	totalEntries := 0
	for _, entries := range idx.index {
		totalEntries += len(entries)
	}

	return map[string]interface{}{
		"ngram_size":      idx.n,
		"unique_ngrams":   len(idx.index),
		"total_entries":   totalEntries,
		"indexed_entries": len(idx.entries),
	}
}
