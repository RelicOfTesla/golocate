package cliclient

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCopyPathToClipboard_NoTool verifies the error path when no clipboard
// tool exists (the sandbox normally has neither xclip nor xsel).
func TestCopyPathToClipboard_NoTool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific: xclip/xsel availability")
	}
	// Ensure neither xclip nor xsel is found by PATH.
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", oldPath+string(os.PathListSeparator)+filepath.Join(t.TempDir(), "empty"))

	name, _ := clipboardCommand()
	if name == "" {
		err := CopyPathToClipboard("/tmp/x")
		if err == nil {
			t.Fatal("expected an error without a clipboard tool")
		}
		if !IsNoClipboardTool(err) {
			t.Errorf("expected no-clipboard-tool error, got %v", err)
		}
	} else {
		t.Logf("system has %s; skipping the no-tool assertion", name)
	}
}

// TestCopyPathToClipboard_WithFakeTool verifies the full copy path using a
// fake xclip that dumps stdin into a file.
func TestCopyPathToClipboard_WithFakeTool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific fake xclip")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "xclip")
	script := "#!/bin/sh\ncat > " + filepath.Join(dir, "clip.txt") + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend the fake dir so LookPath finds our xclip.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	name, args := clipboardCommand()
	if name != "xclip" {
		t.Fatalf("expected fake xclip to be selected, got %q", name)
	}
	if err := CopyPathToClipboard("/tmp/result.txt"); err != nil {
		t.Fatalf("CopyPathToClipboard: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "clip.txt"))
	if err != nil {
		t.Fatalf("fake xclip did not capture stdin: %v", err)
	}
	if string(data) != "/tmp/result.txt" {
		t.Errorf("clipboard content = %q, want /tmp/result.txt", data)
	}
	_ = args
}
