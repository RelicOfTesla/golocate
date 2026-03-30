package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/RelicOfTesla/golocate/pkg/ignore"
)

// fsnotifyWatcher implements Watcher using fsnotify on non-Linux platforms.
type fsnotifyWatcher struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	watcher       *fsnotify.Watcher
	events        chan Event
	errors        chan error
	done          chan struct{}
	config        *Config
	ignoreMatcher *ignore.Matcher
}

// newFsnotifyWatcher creates a new fsnotify-based watcher.
func newFsnotifyWatcher(ctx context.Context, cfg *Config) (Watcher, error) {
	childCtx, cancel := context.WithCancel(ctx)
	
	w, err := fsnotify.NewWatcher()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("fsnotify init failed: %w", err)
	}
	
	fw := &fsnotifyWatcher{
		ctx:      childCtx,
		cancel:   cancel,
		watcher:  w,
		events:   make(chan Event, 1000),
		errors:   make(chan error, 100),
		done:     make(chan struct{}),
		config:   cfg,
	}
	
	if len(cfg.IgnorePatterns) > 0 {
		fw.ignoreMatcher = ignore.NewMatcher(cfg.IgnorePatterns)
	}
	
	// Start watching specified directories
	if len(cfg.Directories) > 0 {
		for _, dir := range cfg.Directories {
			if cfg.Recursive {
				if err := fw.AddRecursive(dir); err != nil {
					fw.Close()
					return nil, fmt.Errorf("failed to watch %s: %w", dir, err)
				}
			} else {
				if err := fw.Add(dir); err != nil {
					fw.Close()
					return nil, fmt.Errorf("failed to watch %s: %w", dir, err)
				}
			}
		}
		log.Printf("INFO: fsnotify directory traversal completed, watching %d directories", len(cfg.Directories))
		// 延迟 60 秒后打印详细的 fanotify WARNING（避免 log 并发问题）
		go func() {
			time.Sleep(60 * time.Second)
			PrintFanotifyWarning()
		}()
	}
	
	// Start event loop
	go fw.eventLoop()
	
	return fw, nil
}

// Add starts watching the named directory.
func (w *fsnotifyWatcher) Add(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if err := w.watcher.Add(name); err != nil {
		return fmt.Errorf("fsnotify add failed for %s: %w", name, err)
	}
	
	log.Printf("fsnotify: watching %s", name)
	return nil
}

// AddRecursive starts watching the named directory and all subdirectories.
func (w *fsnotifyWatcher) AddRecursive(name string) error {
	return filepath.Walk(name, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		
		if !info.IsDir() {
			return nil
		}
		
		// Check if path should be ignored
		if w.ignoreMatcher != nil && w.ignoreMatcher.Match(path) {
			return filepath.SkipDir
		}
		
		return w.Add(path)
	})
}

// Remove stops watching the named directory.
func (w *fsnotifyWatcher) Remove(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	return w.watcher.Remove(name)
}

// Close closes the watcher.
func (w *fsnotifyWatcher) Close() error {
	w.cancel()
	<-w.done
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	close(w.events)
	close(w.errors)
	
	return w.watcher.Close()
}

// Events returns the channel of file system events.
func (w *fsnotifyWatcher) Events() <-chan Event {
	return w.events
}

// Errors returns the channel of errors.
func (w *fsnotifyWatcher) Errors() <-chan error {
	return w.errors
}

// eventLoop reads events from fsnotify.
func (w *fsnotifyWatcher) eventLoop() {
	defer close(w.done)
	
	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			
			// Check if path should be ignored
			if w.ignoreMatcher != nil && w.ignoreMatcher.Match(event.Name) {
				continue
			}
			
			// Convert fsnotify event to our Event
			var op Op
			if event.Op&fsnotify.Create != 0 {
				op |= Create
			}
			if event.Op&fsnotify.Write != 0 {
				op |= Write
			}
			if event.Op&fsnotify.Remove != 0 {
				op |= Remove
			}
			if event.Op&fsnotify.Rename != 0 {
				op |= Rename
			}
			if event.Op&fsnotify.Chmod != 0 {
				op |= Chmod
			}
			
			// For Create events, add subdirectories to watch
			if op&Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if w.config.Recursive {
						go w.AddRecursive(event.Name)
					} else {
						go w.Add(event.Name)
					}
				}
			}
			
			w.events <- Event{
				Name: filepath.Base(event.Name),
				Path: event.Name,
				Op:   op,
			}
			
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			case <-w.ctx.Done():
				return
			}
		}
	}
}
