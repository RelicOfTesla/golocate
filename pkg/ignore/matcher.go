// Package ignore provides pattern matching for file path ignoring.
package ignore

import (
	"path/filepath"
)

// Matcher handles ignore patterns for file paths.
type Matcher struct {
	patterns []string
}

// NewMatcher creates a new ignore matcher with the given patterns.
func NewMatcher(patterns []string) *Matcher {
	return &Matcher{patterns: patterns}
}

// Match checks if a path matches any of the ignore patterns.
// It uses filepath.Match to match patterns against the full path.
func (m *Matcher) Match(path string) bool {
	for _, pattern := range m.patterns {
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// MatchBase checks if the base name of a path matches any of the ignore patterns.
// This is useful for patterns like "*.log" that should match file names, not full paths.
func (m *Matcher) MatchBase(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range m.patterns {
		matched, err := filepath.Match(pattern, base)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// Patterns returns the list of patterns in the matcher.
func (m *Matcher) Patterns() []string {
	if m == nil {
		return nil
	}
	return m.patterns
}
