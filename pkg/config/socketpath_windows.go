//go:build windows

package config

// GetDefaultSocketPath returns the default Named Pipe path for Windows.
func GetDefaultSocketPath() string {
	return `\\.\pipe\golocate`
}
