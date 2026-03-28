//go:build !linux
// +build !linux

package watcher

import (
	"context"
)

// canUseFanotify returns false on non-Linux platforms.
func canUseFanotify() bool {
	return false
}

// newFanotifyWatcher returns an error on non-Linux platforms.
func newFanotifyWatcher(ctx context.Context, cfg *Config) (Watcher, error) {
	return nil, nil // Will fall back to fsnotify
}

// GetWatcherType returns the type of watcher being used.
// On non-Linux platforms, always returns "fsnotify".
func GetWatcherType() string {
	return "fsnotify"
}
