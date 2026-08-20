//go:build windows

package autostart

import (
	"os"
	"syscall"
)

func lockFor(socketPath string) (func(), error) {
	f, err := os.OpenFile(socketPath+".lck", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	// Best-effort: no cross-process lock on Windows here; the socket bind
	// itself prevents a true duplicate daemon.
	return func() { _ = f.Close() }, nil
}

func detachAttr() *syscall.SysProcAttr { return nil }
