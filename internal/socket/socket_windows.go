//go:build windows
// +build windows

// Package socket provides cross-platform socket utilities.
package socket

import (
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/RelicOfTesla/golocate/pkg/config"
)

// createListener creates a Named Pipe listener on Windows.
func createListener(socketPath string) (net.Listener, error) {
	// Use default pipe name if not specified
	pipeName := config.GetDefaultSocketPath()
	if socketPath != "" {
		pipeName = socketPath
	}

	// Create Named Pipe listener
	// Security attributes: only current user can access
	pipeConfig := &winio.PipeConfig{
		SecurityDescriptor: "", // Default: only owner can access
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	listener, err := winio.ListenPipe(pipeName, pipeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Named Pipe: %w", err)
	}

	return listener, nil
}

// getDefaultAddress returns the default Named Pipe address on Windows.
func getDefaultAddress() string {
	return config.GetDefaultSocketPath()
}

// connect connects to the Named Pipe on Windows.
func connect(socketPath string) (net.Conn, error) {
	// Use default pipe name if not specified
	pipeName := config.GetDefaultSocketPath()
	if socketPath != "" {
		pipeName = socketPath
	}

	// Connect to Named Pipe with timeout
	conn, err := winio.DialPipe(pipeName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Named Pipe: %w", err)
	}

	return conn, nil
}

// isRunning checks if the Named Pipe server is running.
func isRunning(socketPath string) bool {
	// Use default pipe name if not specified
	pipeName := config.GetDefaultSocketPath()
	if socketPath != "" {
		pipeName = socketPath
	}

	// Try to connect with a short timeout
	timeout := 100 * time.Millisecond
	conn, err := winio.DialPipe(pipeName, &timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
