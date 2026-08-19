package handler

import (
	"regexp"
	"strconv"
	"strings"
)

// SearchParams represents parsed search parameters
type SearchParams struct {
	Pattern    string // path pattern (base query after stripping flags)
	Content    string // file content keyword (--content:xxx)
	IgnoreCase bool
	Limit      int
	Regex      bool
	Basename   bool // search only the file name portion of paths
}

// ParseSearchQuery parses a search query with optional command-line style parameters
// Examples:
//
//	"test123" -> {Pattern: "test123"}
//	"test123 --content:xxxx" -> {Pattern: "test123", Content: "xxxx"}
//	"test123 --path:/home/user" -> {Pattern: "test123"}
//	"test123 --ignore-case --limit:50" -> {Pattern: "test123", IgnoreCase: true, Limit: 50}
func ParseSearchQuery(input string) *SearchParams {
	params := &SearchParams{
		Limit: 100, // default limit
	}

	// Regular expression to match parameters like --key:value or --flag
	// Supports formats:
	//   --key:value (long form, double dash)
	//   --key:"value with spaces"
	//   --flag (boolean flag, double dash)
	// Note: flag names may contain hyphens (e.g. --ignore-case).
	re := regexp.MustCompile(`(--[a-zA-Z0-9_-]+:"[^"]*"|--[a-zA-Z0-9_-]+:\S+|--[a-zA-Z0-9_-]+)`)

	// Find all parameter matches
	matches := re.FindAllString(input, -1)

	// Remove parameters from input to get the base query
	content := input
	for _, match := range matches {
		content = strings.Replace(content, match, "", 1)
	}
	content = strings.TrimSpace(content)

	// Parse each parameter
	for _, match := range matches {
		// Remove the leading dash(es)
		param := strings.TrimLeft(match, "-")

		if strings.Contains(param, ":") {
			// Parameter with value
			parts := strings.SplitN(param, ":", 2)
			key := parts[0]
			value := parts[1]

			// Remove quotes if present
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = strings.Trim(value, "\"")
			}

			switch key {
			case "content":
				params.Content = value
			case "limit":
				if n, err := parseInt(value); err == nil {
					params.Limit = n
				}
			}
		} else {
			// Boolean flag
			switch param {
			case "ignore-case":
				params.IgnoreCase = true
			case "regex":
				params.Regex = true
			case "basename":
				params.Basename = true
			}
		}
	}

	// The base query (after stripping flags) is the path pattern
	params.Pattern = content

	return params
}

// parseInt parses a string to int, returns error if invalid
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
