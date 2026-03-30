package handler

import (
	"regexp"
	"strconv"
	"strings"
)

// SearchParams represents parsed search parameters
type SearchParams struct {
	Content    string
	Path       string
	IgnoreCase bool
	Limit      int
	Regex      bool
}

// ParseSearchQuery parses a search query with optional command-line style parameters
// Examples:
//   "test123" -> {Content: "test123"}
//   "test123 --content:xxxx" -> {Content: "xxxx"}
//   "test123 --path:/home/user" -> {Content: "test123", Path: "/home/user"}
//   "test123 --ignore-case --limit:50" -> {Content: "test123", IgnoreCase: true, Limit: 50}
func ParseSearchQuery(input string) *SearchParams {
	params := &SearchParams{
		Limit: 100, // default limit
	}

	// Regular expression to match parameters like --key:value or --flag
	// Supports formats:
	//   --key:value (long form, double dash)
	//   --key:"value with spaces"
	//   --flag (boolean flag, double dash)
	re := regexp.MustCompile(`(--\w+:\"[^\"]*\"|--\w+:\S+|--\w+)`)
	
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
			case "path":
				params.Path = value
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
			}
		}
	}

	// If content not specified via parameter, use the base query
	if params.Content == "" {
		params.Content = content
	}

	return params
}

// parseInt parses a string to int, returns error if invalid
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
