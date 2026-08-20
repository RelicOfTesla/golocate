package svc

import (
	"os"
	"path/filepath"
	"sync"
)

// DefaultLogMaxSize is the size at which the daemon log is rotated
// (a fresh file starts and the old content is kept as <path>.1).
const DefaultLogMaxSize = 10 * 1024 * 1024 // 10MB

// rotatingFile is an io.Writer that appends to a log file and rotates it
// (renames to <path>.1, then starts a fresh file) once the size exceeds max.
type rotatingFile struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

// openLogFile opens (creating if needed) the rotating log writer.
func openLogFile(path string, maxSize int64) (*rotatingFile, error) {
	if maxSize <= 0 {
		maxSize = DefaultLogMaxSize
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &rotatingFile{path: path, max: maxSize, f: f, size: st.Size()}, nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > r.max {
		if err := r.rotate(); err != nil {
			// Keep writing to the current file even if rotation failed.
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate renames the current log to <path>.1 (dropping any previous backup)
// and starts a fresh file.
func (r *rotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	backup := r.path + ".1"
	_ = os.Remove(backup) // keep a single backup generation
	if err := os.Rename(r.path, backup); err != nil {
		// Re-open the original file so logging keeps working.
		f, ferr := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr != nil {
			return err
		}
		r.f = f
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	r.f = f
	r.size = 0
	return nil
}

// Close closes the underlying file.
func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		return r.f.Close()
	}
	return nil
}
