package testutil

import (
	"fmt"
	"runtime"
)

// GetTestSocketPath returns a platform-appropriate test socket path.
// On Linux/macOS: returns /tmp/golocate_test_{suffix}.sock
// On Windows: returns \\.\pipe\golocate_test_{suffix}
func GetTestSocketPath(suffix string) string {
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf(`\\.\pipe\golocate_test_%s`, suffix)
	default:
		return fmt.Sprintf("/tmp/golocate_test_%s.sock", suffix)
	}
}

// GetDefaultTestSocketPath returns the default test socket path.
// Equivalent to GetTestSocketPath("default").
func GetDefaultTestSocketPath() string {
	return GetTestSocketPath("default")
}

// GetNonExistentSocketPath returns a platform-appropriate non-existent socket path.
// This is used for testing error handling when the socket doesn't exist.
// On Linux/macOS: returns /tmp/golocate_nonexistent_{random}.sock
// On Windows: returns \\.\pipe\golocate_nonexistent_{random}
func GetNonExistentSocketPath() string {
	switch runtime.GOOS {
	case "windows":
		return `\\.\pipe\golocate_nonexistent_for_test`
	default:
		return "/tmp/golocate_nonexistent_for_test.sock"
	}
}
