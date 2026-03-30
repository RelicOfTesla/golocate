// Package constants provides constants for golocate.
package config

import (
	"time"
)

// Socket configuration.
const (
	// DefaultSocketPath is the default Unix socket path for Linux/macOS.
	// Note: Windows uses Named Pipe (see internal/socket/socket_windows.go).
	// This constant is ignored on Windows platforms.
	DefaultSocketPath = "/tmp/golocate.sock"
	SocketPermission  = 0600 // Socket file permission (more secure)
)

// Connection configuration.
const (
	DefaultTimeout    = 30 * time.Second
	DefaultRetryCount = 3
	DefaultRetryDelay = 100 * time.Millisecond
	DefaultMaxConns   = 100
)
