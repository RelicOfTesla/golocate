//go:build !windows

package config

// Socket configuration.
const (
	// SocketPermission allows any local user to connect to the socket.
	// The daemon usually runs as root (fanotify needs root to watch the
	// whole filesystem) while the CLI runs as the regular user; 0600 would
	// lock that user out. The socket lives under /tmp and stays local-only,
	// so 0666 is the accepted trade-off (same as syslog/docker conventions).
	SocketPermission = 0666
)

// GetDefaultSocketPath returns the default Unix socket path for Linux.
func GetDefaultSocketPath() string {
	return "/tmp/golocate.sock"
}
