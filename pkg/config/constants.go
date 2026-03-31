// Package constants provides constants for golocate.
package config

import (
	"time"
)



// Connection configuration.
const (
	DefaultTimeout    = 30 * time.Second
	DefaultRetryCount = 3
	DefaultRetryDelay = 100 * time.Millisecond
	DefaultMaxConns   = 100
)
