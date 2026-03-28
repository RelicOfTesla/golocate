// Package constants provides constants for golocate.
package constants

import (
	"time"
)

// Socket configuration.
const (
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
