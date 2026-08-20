//go:build !unix

package index

import "os"

// deviceIdentity is unavailable on this platform (e.g. Windows); dedupe
// falls back to (size, modtime) heuristics.
func deviceIdentity(info os.FileInfo) (dev, ino uint64) {
	return 0, 0
}
