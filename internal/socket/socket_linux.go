//go:build linux
// +build linux

// Package socket provides cross-platform socket utilities.
package socket

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/config"
)

// createListener creates a Unix socket listener on Linux.
func createListener(socketPath string) (net.Listener, error) {
	// Remove existing socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Unix socket: %w", err)
	}

	// Set socket permissions (secure: only owner can read/write)
	if err := os.Chmod(socketPath, config.SocketPermission); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}

	return listener, nil
}

// getDefaultAddress returns the default Unix socket path on Linux.
func getDefaultAddress() string {
	return config.GetDefaultSocketPath()
}

// connect connects to the Unix socket on Linux.
func connect(socketPath string) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Unix socket: %w", err)
	}
	return conn, nil
}

// isRunning checks if the Unix socket exists and is connectable.
func isRunning(socketPath string) bool {
	// Check if socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return false
	}

	// Try to connect with timeout
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
