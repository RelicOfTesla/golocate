// Package cliclient provides CLI client helpers for golocate.
package cliclient

import (
	"os/exec"
	"runtime"
)

// OpenCommand returns the platform's default "open this path" command and its
// arguments (xdg-open on Linux, open on macOS, explorer on Windows). The path
// is passed as-is; callers should not shell-quote it, exec.Command does that.
func OpenCommand(path string) (name string, args []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{path}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", path}
	default:
		return "xdg-open", []string{path}
	}
}

// OpenInDefaultApp opens a file or directory with the platform's default
// application (xdg-open / open / explorer). It returns as soon as the command
// has been started; errors opening the resource surface in the app itself.
func OpenInDefaultApp(path string) error {
	name, args := OpenCommand(path)
	return exec.Command(name, args...).Start()
}
