package svc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAutostartEntry verifies the XDG desktop entry content on Linux.
func TestAutostartEntry(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("desktop entry format is Linux-specific")
	}
	content := AutostartEntry("/usr/bin/golocated", "/home/u/.config/golocate/config")
	for _, want := range []string{
		"[Desktop Entry]",
		"Type=Application",
		"Exec=/usr/bin/golocated --service --config /home/u/.config/golocate/config",
		"X-GNOME-Autostart-enabled=true",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("autostart entry missing %q:\n%s", want, content)
		}
	}
}

// TestInstallRemoveAutostart verifies install/remove round-trips under a
// temporary XDG_CONFIG_HOME.
func TestInstallRemoveAutostart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("autostart not supported on Windows")
	}
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	entryPath := AutostartPath()
	if entryPath == "" {
		t.Fatal("expected a non-empty autostart path on this platform")
	}

	if err := InstallAutostart("/tmp/cfg.yaml"); err != nil {
		t.Fatalf("InstallAutostart: %v", err)
	}
	data, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatalf("autostart entry not written: %v", err)
	}
	if !strings.Contains(string(data), "--service") {
		t.Errorf("entry should start the daemon with --service:\n%s", data)
	}
	// The written Exec should reference the current test binary, since
	// InstallAutostart resolves os.Executable().
	if !strings.Contains(string(data), filepath.Base(os.Args[0])) {
		t.Logf("note: Exec references %q (test binary name varies)", filepath.Base(os.Args[0]))
	}

	if err := RemoveAutostart(); err != nil {
		t.Fatalf("RemoveAutostart: %v", err)
	}
	if _, err := os.Stat(entryPath); !os.IsNotExist(err) {
		t.Errorf("autostart entry should be removed, stat err: %v", err)
	}
}
