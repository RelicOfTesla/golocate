// Package watcher provides file system watching capabilities.
// It uses fanotify on Linux and fsnotify on other platforms.
package watcher

import (
	"context"
	"fmt"
	"log"
	"runtime"
)

// Event represents a file system event.
type Event struct {
	// Name is the file/directory name
	Name string
	// Path is the full path to the file/directory
	Path string
	// Op is the operation that occurred
	Op Op
}

// Op represents a file system operation.
type Op uint32

const (
	Create Op = 1 << iota
	Write
	Remove
	Rename
	Chmod
	MoveFrom
	MoveTo
)

func (op Op) String() string {
	var s string
	if op&Create == Create {
		s += "|CREATE"
	}
	if op&Write == Write {
		s += "|WRITE"
	}
	if op&Remove == Remove {
		s += "|REMOVE"
	}
	if op&Rename == Rename {
		s += "|RENAME"
	}
	if op&Chmod == Chmod {
		s += "|CHMOD"
	}
	if op&MoveFrom == MoveFrom {
		s += "|MOVE_FROM"
	}
	if op&MoveTo == MoveTo {
		s += "|MOVE_TO"
	}
	if s == "" {
		return ""
	}
	return s[1:]
}

// Watcher is the interface for file system watchers.
// It is inspired by fsnotify's API design.
type Watcher interface {
	// Add starts watching the named directory (non-recursive).
	Add(name string) error
	
	// AddRecursive starts watching the named directory and all subdirectories.
	AddRecursive(name string) error
	
	// Remove stops watching the named directory.
	Remove(name string) error
	
	// Close closes the watcher.
	Close() error
	
	// Events returns the channel of file system events.
	Events() <-chan Event
	
	// Errors returns the channel of errors.
	Errors() <-chan error
}

// Config is the configuration for the watcher.
type Config struct {
	// Directories to watch
	Directories []string
	
	// IgnorePatterns are glob patterns to ignore
	IgnorePatterns []string
	
	// Recursive indicates whether to watch subdirectories
	Recursive bool
	
	// FollowSymlinks indicates whether to follow symbolic links
	FollowSymlinks bool
}

// NewWatcher creates a new Watcher based on the platform.
// On Linux 5.1+, it tries fanotify first, then falls back to fsnotify.
// On other platforms, it uses fsnotify.
func NewWatcher(ctx context.Context, cfg *Config) (Watcher, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	
	// On Linux, try fanotify first
	if runtime.GOOS == "linux" {
		// Check if fanotify is supported
		if canUseFanotify() {
			w, err := newFanotifyWatcher(ctx, cfg)
			if err == nil {
				return w, nil
			}
			// fanotify failed, fall through to fsnotify
			log.Printf("fanotify not available: %v, falling back to fsnotify", err)
		} else {
			// fanotify not supported (kernel < 5.1 or no permissions)
			log.Printf("WARNING: Using fsnotify as fallback (fanotify not available)")
		}
		// Fall back to fsnotify
		return newFsnotifyWatcher(ctx, cfg)
	}
	
	// Use fsnotify on other platforms
	return newFsnotifyWatcher(ctx, cfg)
}

// NewWatcherForPath creates a new Watcher for a specific path.
// This is a convenience function for watching a single directory.
func NewWatcherForPath(ctx context.Context, path string) (Watcher, error) {
	return NewWatcher(ctx, &Config{
		Directories: []string{path},
		Recursive:   true,
	})
}

// ErrWatcherClosed is returned when the watcher is closed.
var ErrWatcherClosed = fmt.Errorf("watcher closed")

// ErrNotSupported is returned when a feature is not supported.
var ErrNotSupported = fmt.Errorf("not supported")
