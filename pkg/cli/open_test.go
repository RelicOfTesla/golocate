package cliclient

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestOpenCommand verifies the platform-specific open command selection.
func TestOpenCommand(t *testing.T) {
	name, args := OpenCommand("/some/path")

	switch runtime.GOOS {
	case "darwin":
		if name != "open" {
			t.Errorf("expected 'open' on darwin, got %q", name)
		}
	case "windows":
		if name != "rundll32" {
			t.Errorf("expected 'rundll32' on windows, got %q", name)
		}
		if len(args) != 2 || args[1] != "/some/path" {
			t.Errorf("unexpected rundll32 args: %v", args)
		}
	default:
		if name != "xdg-open" {
			t.Errorf("expected 'xdg-open' on %s, got %q", runtime.GOOS, name)
		}
	}

	if len(args) == 0 || args[len(args)-1] != "/some/path" {
		t.Errorf("last argument must be the path, got %v", args)
	}
}

// TestOpenCommand_Directory tests opening a directory (path resolution is a
// formality; the command selection must be identical for dirs and files).
func TestOpenCommand_Directory(t *testing.T) {
	name, args := OpenCommand(filepath.Join("a", "b"))
	if len(args) == 0 || args[len(args)-1] != filepath.Join("a", "b") {
		t.Errorf("expected directory path passed to %s, got %v", name, args)
	}
}
