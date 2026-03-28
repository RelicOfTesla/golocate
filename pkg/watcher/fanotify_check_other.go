// +build !linux

package watcher

// PrintFanotifyWarning prints detailed warning about using fsnotify instead of fanotify.
// On non-Linux systems, this is a no-op.
func PrintFanotifyWarning() {
	// No-op on non-Linux systems
}
