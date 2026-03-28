// Package security provides security utilities for golocate.
package security

import (
	"path/filepath"
	"strings"
)

// PathValidator validates file paths for security.
type PathValidator struct {
	allowedDirs []string
}

// NewPathValidator creates a new path validator.
func NewPathValidator(allowedDirs []string) *PathValidator {
	return &PathValidator{
		allowedDirs: allowedDirs,
	}
}

// IsPathAllowed checks if a path is within allowed directories.
func (v *PathValidator) IsPathAllowed(path string) bool {
	// Clean the path to prevent path traversal
	cleanPath := filepath.Clean(path)
	
	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return false
	}
	
	// If no allowed directories specified, allow all (for backward compatibility)
	if len(v.allowedDirs) == 0 {
		return true
	}
	
	// Check if path is within allowed directories
	for _, allowedDir := range v.allowedDirs {
		allowedDirClean := filepath.Clean(allowedDir)
		if strings.HasPrefix(cleanPath, allowedDirClean) {
			return true
		}
	}
	
	return false
}

// SanitizePath sanitizes a file path.
func SanitizePath(path string) string {
	// Clean the path
	cleanPath := filepath.Clean(path)
	
	// Remove any null bytes
	cleanPath = strings.ReplaceAll(cleanPath, "\x00", "")
	
	return cleanPath
}

// IsPathTraversal checks if a path contains path traversal attempts.
func IsPathTraversal(path string) bool {
	cleanPath := filepath.Clean(path)
	return strings.Contains(cleanPath, "..") || 
	       strings.Contains(path, "..") ||
	       strings.Contains(path, "\x00")
}
