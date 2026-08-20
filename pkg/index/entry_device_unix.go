//go:build unix

package index

import (
	"os"
	"syscall"
)

// deviceIdentity extracts the device + inode of a file on Unix platforms so
// hard links can be deduplicated.
func deviceIdentity(info os.FileInfo) (dev, ino uint64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev), uint64(st.Ino)
	}
	return 0, 0
}
