// Package errors provides friendly error messages for golocate.
package errors

import (
	"fmt"
	"os"
	"strings"
)

// ServerNotRunningError represents an error when the server is not running.
type ServerNotRunningError struct {
	SocketPath string
	Err        error
}

// Error implements the error interface.
func (e *ServerNotRunningError) Error() string {
	return fmt.Sprintf("golocated server is not running at %s", e.SocketPath)
}

// Unwrap returns the underlying error.
func (e *ServerNotRunningError) Unwrap() error {
	return e.Err
}

// FriendlyMessage returns a user-friendly error message.
func (e *ServerNotRunningError) FriendlyMessage() string {
	var sb strings.Builder

	sb.WriteString("❌ Cannot connect to golocated server\n\n")
	sb.WriteString("The golocated server is not running. To start it:\n\n")
	sb.WriteString("  golocated --service\n\n")
	sb.WriteString("Or install it as a system service:\n\n")
	sb.WriteString("  golocated --install --user   # Install as user service\n")
	sb.WriteString("  golocated --install          # Install as system service\n")
	sb.WriteString("  golocated --start            # Start the service\n")

	return sb.String()
}

// IsServerNotRunningError checks if an error is a server not running error.
func IsServerNotRunningError(err error) bool {
	_, ok := err.(*ServerNotRunningError)
	return ok
}

// WrapConnectError wraps a connection error with a friendly message.
func WrapConnectError(err error, socketPath string) error {
	return &ServerNotRunningError{
		SocketPath: socketPath,
		Err:        err,
	}
}

// PrintFriendlyError prints a friendly error message to stderr.
func PrintFriendlyError(err error) {
	if serverErr, ok := err.(*ServerNotRunningError); ok {
		fmt.Fprintln(os.Stderr, serverErr.FriendlyMessage())
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// GetFriendlyErrorMessage returns a friendly error message for any error.
func GetFriendlyErrorMessage(err error) string {
	if serverErr, ok := err.(*ServerNotRunningError); ok {
		return serverErr.FriendlyMessage()
	}
	return fmt.Sprintf("Error: %v", err)
}
