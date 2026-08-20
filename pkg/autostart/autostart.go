// Package autostart lets CLI/GTK/H5 auto-start the golocated daemon when the
// socket is unreachable, with a mode to control process lifetime and a
// cross-process lock to avoid double-starting under concurrency.
package autostart

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/config"
)

// Mode controls how golocated is launched when it is not already running.
type Mode int

const (
	// None disables auto-start entirely.
	None Mode = iota
	// Child starts golocated as a child of this process; its lifetime follows
	// the client (the caller should kill it when the client exits).
	Child
	// Background starts golocated detached, so it keeps running after the
	// client exits.
	Background
)

// DefaultMode is the default policy (child).
func DefaultMode() Mode { return Child }

// ParseMode parses "none"/"child"/"background".
func ParseMode(s string) (Mode, error) {
	switch s {
	case "", "child":
		return Child, nil
	case "none":
		return None, nil
	case "background":
		return Background, nil
	}
	return None, fmt.Errorf("invalid auto-start-server mode %q (want none|child|background)", s)
}

// Launcher holds the parameters for Ensure.
type Launcher struct {
	SocketPath string
	Mode       Mode
}

// ChildCmd is the spawned child (non-nil only in Child mode) that the caller
// is expected to kill when its own process exits.
type ChildCmd = *exec.Cmd

// Ensure checks the socket; if unreachable, finds golocated and starts it
// (unless Mode is None). A cross-process file lock guards against concurrent
// starts — whoever takes the lock first runs the spawn, others see the socket
// already up afterwards. Returns the child command for Child mode (may be nil),
// and any error.
func (l *Launcher) Ensure() (ChildCmd, error) {
	if l.SocketPath == "" {
		l.SocketPath = config.GetDefaultSocketPath()
	}
	if socket.IsRunning(l.SocketPath) {
		return nil, nil // already up
	}
	if l.Mode == None {
		return nil, nil
	}

	gd, err := findGolocated()
	if err != nil {
		return nil, err
	}

	unlock, err := lockFor(l.SocketPath)
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Re-check under the lock: a concurrent start may have won already.
	if socket.IsRunning(l.SocketPath) {
		return nil, nil
	}

	// Keep the daemon's socket identical to what the client uses.
	args := []string{"--service"}
	if l.SocketPath != "" {
		args = append(args, "--socket", l.SocketPath)
	}
	var cmd *exec.Cmd
	switch l.Mode {
	case Child:
		cmd = exec.Command(gd, args...)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		go cmd.Wait() // reap zombie while caller keeps the reference
	case Background:
		cmd = exec.Command(gd, args...)
		cmd.SysProcAttr = detachAttr()
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		_ = cmd.Process.Release()
		return nil, nil
	}

	slog.Info("auto-started golocated", "mode", "child", "binary", gd, "socket", l.SocketPath)
	if !waitReady(l.SocketPath, 5*time.Second) {
		return cmd, fmt.Errorf("golocated started but did not become ready within 5s")
	}
	return cmd, nil
}

// findGolocated locates the golocated binary: first alongside this executable
// (so a bundled deployment just works), then on PATH.
func findGolocated() (string, error) {
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "golocated")
		if fi, err := os.Stat(sibling); err == nil && !fi.IsDir() {
			return sibling, nil
		}
		if goruntime := filepath.Join(filepath.Dir(exe), "golocated.exe"); fileExists(goruntime) {
			return goruntime, nil
		}
	}
	if p, err := exec.LookPath("golocated"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("golocated not found on PATH or alongside the client binary")
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// waitReady polls the socket until the daemon answers or the timeout elapses.
func waitReady(socketPath string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if socket.IsRunning(socketPath) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
