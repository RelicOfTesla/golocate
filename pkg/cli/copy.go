// Package cliclient provides CLI client helpers for golocate.
package cliclient

import (
	"os/exec"
	"runtime"
	"strings"
)

// clipboardCommand returns the platform clipboard command (name + args) or
// ("", nil) when no tool is available.
func clipboardCommand() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil
	case "windows":
		return "powershell", []string{"-NoProfile", "-Command", "Set-Clipboard"}
	default:
		// Linux: prefer xclip, fall back to xsel.
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard"}
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--input"}
		}
		return "", nil
	}
}

// CopyPathToClipboard copies text to the system clipboard (pbcopy on macOS,
// Set-Clipboard on Windows, xclip/xsel on Linux). It returns a descriptive
// error when no clipboard tool is installed.
func CopyPathToClipboard(text string) error {
	name, args := clipboardCommand()
	if name == "" {
		return errNoClipboardTool
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// errNoClipboardTool is returned when no clipboard utility is available.
var errNoClipboardTool = &clipboardError{}

type clipboardError struct{}

func (e *clipboardError) Error() string {
	return "no clipboard tool available (install xclip or xsel on Linux)"
}

// IsNoClipboardTool reports whether err is the missing-tool error.
func IsNoClipboardTool(err error) bool {
	_, ok := err.(*clipboardError)
	return ok
}
