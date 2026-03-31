//go:build !windows

package config

// Socket configuration.
const (
	SocketPermission  = 0600 // Socket file permission (more secure)
)


// GetDefaultSocketPath returns the default Unix socket path for Linux.
func GetDefaultSocketPath() string {
	return "/tmp/golocate.sock"
}
