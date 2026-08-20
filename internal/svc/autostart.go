// User-level autostart entry management (XDG autostart on Linux, LaunchAgents
// on macOS). Used by `golocated --autostart` / `--no-autostart`.
package svc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AutostartFileName is the user-level autostart entry name.
const AutostartFileName = "golocated.desktop"

// AutostartPath returns the path of the user autostart entry for the current
// platform, or "" when the platform has no supported user-autostart location.
func AutostartPath() string {
	switch runtime.GOOS {
	case "linux":
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		return filepath.Join(cfgDir, "autostart", AutostartFileName)
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, "Library", "LaunchAgents", "com.golocate.daemon.plist")
	case "windows":
		appData, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		// The per-user Startup folder runs .bat/.cmd entries at login.
		return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "golocated.bat")
	default:
		return ""
	}
}

// AutostartEntry renders the autostart entry content for the current binary
// and config path (used by tests and on startup verification).
func AutostartEntry(exePath, configPath string) string {
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.golocate.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--service</string>
        <string>--config</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key><true/>
</dict>
</plist>
`, exePath, configPath)
	}
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("@echo off\r\nstart \"\" /min \"%s\" --service --config \"%s\"\r\n", exePath, configPath)
	}
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=golocate
Comment=golocate file indexing daemon
Exec=%s --service --config %s
Terminal=false
X-GNOME-Autostart-enabled=true
`, exePath, configPath)
}

// InstallAutostart writes a user autostart entry that starts the daemon with
// --service on login. exePath is the daemon binary ("" defaults to the
// running executable).
func InstallAutostart(configPath string) error {
	path := AutostartPath()
	if path == "" {
		return fmt.Errorf("user autostart is not supported on %s", runtime.GOOS)
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve daemon executable: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create autostart directory: %w", err)
	}
	content := AutostartEntry(exePath, configPath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write autostart entry: %w", err)
	}
	return nil
}

// RemoveAutostart deletes the user autostart entry, if present.
func RemoveAutostart() error {
	path := AutostartPath()
	if path == "" {
		return fmt.Errorf("user autostart is not supported on %s", runtime.GOOS)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
