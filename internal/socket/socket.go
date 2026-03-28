// Package socket provides cross-platform socket utilities.
package socket

import (
	"net"
)

// CreateListener creates a platform-appropriate listener.
// On Linux: Unix socket
// On Windows: TCP socket on localhost
func CreateListener(socketPath string) (net.Listener, error) {
	return createListener(socketPath)
}

// GetDefaultAddress returns the default address for the platform.
// On Linux: Unix socket path
// On Windows: TCP address (localhost:port)
func GetDefaultAddress() string {
	return getDefaultAddress()
}

// Connect connects to the server using platform-appropriate method.
func Connect(socketPath string) (net.Conn, error) {
	return connect(socketPath)
}

// IsRunning checks if the server is running.
func IsRunning(socketPath string) bool {
	return isRunning(socketPath)
}
