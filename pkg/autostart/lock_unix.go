//go:build unix

package autostart

import (
	"os"
	"syscall"
)

// lockFor takes an exclusive advisory lock on "<socket>.lck" so concurrent
// clients cannot spawn the daemon twice. Blocks until the lock is held.
func lockFor(socketPath string) (func(), error) {
	f, err := os.OpenFile(socketPath+".lck", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// detachAttr lets a Background-spawned process survive the parent.
func detachAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
