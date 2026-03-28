// +build linux

package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fanotify constants
const (
	fanReportDirFid   uint = 0x00000400
	fanReportFid      uint = 0x00000200
	fanReportName     uint = 0x00000800
	fanMarkAdd        uint = 0x00000001
	fanMarkFilesystem uint = 0x00000100
	fanOnDir          uint64 = 0x40000000
	fanEventOnChild   uint64 = 0x08000000
	fanCreate         uint64 = 0x00000100
	fanDelete         uint64 = 0x00000200
	fanMoveFrom       uint64 = 0x00000040
	fanMoveTo         uint64 = 0x00000080
	fanDeleteSelf     uint64 = 0x00000400
	fanMoveSelf       uint64 = 0x00000800
	atFDCWD           int  = -100
)

// fanotifyEventMetadataLen is the size of FanotifyEventMetadata
const fanotifyEventMetadataLen = 24

// fanotifyWatcher implements Watcher using fanotify on Linux.
type fanotifyWatcher struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	fd            int
	events        chan Event
	errors        chan error
	done          chan struct{}
	config        *Config
	watched       map[string]bool
	ignoreMatcher *ignoreMatcher
}

// newFanotifyWatcher creates a new fanotify-based watcher.
func newFanotifyWatcher(ctx context.Context, cfg *Config) (Watcher, error) {
	childCtx, cancel := context.WithCancel(ctx)
	
	// Initialize fanotify
	// We use FAN_REPORT_DIR_FID | FAN_REPORT_FID | FAN_REPORT_NAME for comprehensive events
	flags := fanReportDirFid | fanReportFid | fanReportName
	
	fd, err := unix.FanotifyInit(flags, 0)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("fanotify init failed: %w", err)
	}
	
	w := &fanotifyWatcher{
		ctx:      childCtx,
		cancel:   cancel,
		fd:       fd,
		events:   make(chan Event, 1000),
		errors:   make(chan error, 100),
		done:     make(chan struct{}),
		config:   cfg,
		watched:  make(map[string]bool),
	}
	
	if len(cfg.IgnorePatterns) > 0 {
		w.ignoreMatcher = newIgnoreMatcher(cfg.IgnorePatterns)
	}
	
	// Start watching specified directories
	if len(cfg.Directories) > 0 {
		for _, dir := range cfg.Directories {
			if err := w.AddRecursive(dir); err != nil {
				w.Close()
				return nil, fmt.Errorf("failed to watch %s: %w", dir, err)
			}
		}
	}
	
	// Start event loop
	go w.eventLoop()
	
	return w, nil
}

// Add starts watching the named directory (non-recursive).
func (w *fanotifyWatcher) Add(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.watched[name] {
		return nil
	}
	
	// Mark the directory for watching
	mask := fanCreate | fanDelete | fanMoveFrom | fanMoveTo | fanOnDir
	err := unix.FanotifyMark(w.fd, fanMarkAdd, mask, atFDCWD, name)
	if err != nil {
		return fmt.Errorf("fanotify mark failed for %s: %w", name, err)
	}
	
	w.watched[name] = true
	log.Printf("fanotify: watching %s", name)
	return nil
}

// AddRecursive starts watching the named directory and all subdirectories.
func (w *fanotifyWatcher) AddRecursive(name string) error {
	// For fanotify, we can mark the filesystem to watch all files
	// This is the key advantage over inotify
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// Mark the entire filesystem
	mask := fanCreate | fanDelete | fanMoveFrom | fanMoveTo | fanOnDir | fanEventOnChild
	
	// Use FAN_MARK_FILESYSTEM to watch the entire filesystem
	err := unix.FanotifyMark(w.fd, fanMarkAdd|fanMarkFilesystem, mask, atFDCWD, "/")
	if err != nil {
		return fmt.Errorf("fanotify mark filesystem failed: %w", err)
	}
	
	w.watched[name] = true
	log.Printf("fanotify: watching entire filesystem from %s", name)
	return nil
}

// Remove stops watching the named directory.
func (w *fanotifyWatcher) Remove(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	delete(w.watched, name)
	return nil
}

// Close closes the watcher.
func (w *fanotifyWatcher) Close() error {
	w.cancel()
	<-w.done
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.fd >= 0 {
		unix.Close(w.fd)
		w.fd = -1
	}
	
	close(w.events)
	close(w.errors)
	
	return nil
}

// Events returns the channel of file system events.
func (w *fanotifyWatcher) Events() <-chan Event {
	return w.events
}

// Errors returns the channel of errors.
func (w *fanotifyWatcher) Errors() <-chan error {
	return w.errors
}

// eventLoop reads events from fanotify.
func (w *fanotifyWatcher) eventLoop() {
	defer close(w.done)
	
	f := os.NewFile(uintptr(w.fd), "fanotify")
	buf := make([]byte, 4096)
	
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}
		
		n, err := f.Read(buf)
		if err != nil {
			if w.ctx.Err() != nil {
				return
			}
			select {
			case w.errors <- err:
			case <-w.ctx.Done():
				return
			}
			continue
		}
		
		if n == 0 {
			continue
		}
		
		// Parse fanotify event
		w.parseEvent(buf[:n])
	}
}

// parseEvent parses a fanotify event.
func (w *fanotifyWatcher) parseEvent(buf []byte) {
	if len(buf) < fanotifyEventMetadataLen {
		return
	}
	
	// Parse metadata
	meta := (*unix.FanotifyEventMetadata)(unsafe.Pointer(&buf[0]))
	
	// Determine operation type
	var op Op
	if meta.Mask&fanCreate != 0 {
		op |= Create
	}
	if meta.Mask&fanDelete != 0 || meta.Mask&fanDeleteSelf != 0 {
		op |= Remove
	}
	if meta.Mask&fanMoveFrom != 0 {
		op |= MoveFrom
	}
	if meta.Mask&fanMoveTo != 0 {
		op |= MoveTo
	}
	
	// Get the file path from fd
	if meta.Fd >= 0 {
		sym := fmt.Sprintf("/proc/self/fd/%d", meta.Fd)
		path, err := os.Readlink(sym)
		if err == nil && len(path) > 0 {
			// Check if path should be ignored
			if w.ignoreMatcher != nil && w.ignoreMatcher.Match(path) {
				syscall.Close(int(meta.Fd))
				return
			}
			
			event := Event{
				Name: filepath.Base(path),
				Path: path,
				Op:   op,
			}
			
			select {
			case w.events <- event:
			case <-w.ctx.Done():
			}
		}
		syscall.Close(int(meta.Fd))
	}
}
