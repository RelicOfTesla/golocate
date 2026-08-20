// Package scheduler provides periodic index rebuilding.
package scheduler

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

// Scheduler manages periodic index rebuilding.
type Scheduler struct {
	cfg          *config.Config
	db           *database.DB
	interval     time.Duration
	throttle     bool
	workerCnt    int
	skipInitial  bool
	onIndexBuilt func(*index.Index) // called after a rebuild completes (outside locks)
	onBuildStart func()             // called before each rebuild (outside locks)
	onBuildEnd   func()             // called after each rebuild finishes (outside locks)
	onProgress   func(int64)        // called with the scanned count during a rebuild
	lastBuild    time.Time
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// SchedulerConfig is the scheduler configuration.
type SchedulerConfig struct {
	// Interval is the time between rebuilds (default: 2 hours)
	Interval time.Duration
	// Throttle enables throttled indexing (lower CPU usage)
	Throttle bool
	// WorkerCount is the number of concurrent workers
	WorkerCount int
	// SkipInitialBuild skips the rebuild that normally runs when Start() is called.
	// Set this when the daemon already builds the initial index itself.
	SkipInitialBuild bool
}

// NewScheduler creates a new index scheduler.
func NewScheduler(cfg *config.Config, db *database.DB, scfg *SchedulerConfig) *Scheduler {
	if scfg == nil {
		scfg = &SchedulerConfig{}
	}

	// Set defaults
	if scfg.Interval == 0 {
		scfg.Interval = 3 * time.Hour // 默认 3 小时
	}

	// Calculate worker count based on throttle mode
	workerCount := scfg.WorkerCount
	if workerCount <= 0 {
		if scfg.Throttle {
			// Throttled: use fewer workers
			workerCount = runtime.NumCPU() / 2
			if workerCount < 1 {
				workerCount = 1
			}
		} else {
			// Full load: use more workers
			workerCount = runtime.NumCPU() * 2
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		cfg:         cfg,
		db:          db,
		interval:    scfg.Interval,
		throttle:    scfg.Throttle,
		workerCnt:   workerCount,
		skipInitial: scfg.SkipInitialBuild,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetOnIndexBuilt registers a callback invoked after each completed rebuild
// with the freshly built index. The callback runs outside any scheduler lock.
func (s *Scheduler) SetOnIndexBuilt(fn func(*index.Index)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onIndexBuilt = fn
}

// SetOnBuildStart registers a callback invoked just before each rebuild starts
// (e.g. to mark the daemon as building).
func (s *Scheduler) SetOnBuildStart(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBuildStart = fn
}

// SetOnBuildEnd registers a callback invoked after each rebuild finishes
// (successfully or not), e.g. to clear the daemon's building flag.
func (s *Scheduler) SetOnBuildEnd(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBuildEnd = fn
}

// SetOnProgress registers a callback invoked periodically during a rebuild
// with the number of entries scanned so far. Runs on the rebuild goroutine.
func (s *Scheduler) SetOnProgress(fn func(int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProgress = fn
}

// SetConfig swaps the configuration used by subsequent rebuilds.
func (s *Scheduler) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
}

// Start starts the periodic index rebuilding.
func (s *Scheduler) Start() {
	slog.Info("starting index scheduler", "interval", s.interval, "throttle", s.throttle, "workers", s.workerCnt)

	s.wg.Add(1)
	go s.loop()
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	slog.Info("index scheduler stopped")
}

// loop runs the periodic rebuild loop.
func (s *Scheduler) loop() {
	defer s.wg.Done()

	// Initial build (unless the daemon already built one)
	if !s.skipInitial {
		s.rebuild()
	}

	// Periodic rebuild
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.rebuild()
		}
	}
}

// rebuild performs a full index rebuild.
func (s *Scheduler) rebuild() {
	s.mu.RLock()
	onBuildStart := s.onBuildStart
	onBuildEnd := s.onBuildEnd
	onProgress := s.onProgress
	s.mu.RUnlock()

	if onBuildStart != nil {
		onBuildStart()
	}
	defer func() {
		if onBuildEnd != nil {
			onBuildEnd()
		}
	}()

	start := time.Now()
	slog.Info("starting index rebuild", "throttle", s.throttle)

	// Build new index
	builder := index.NewBuilder(index.BuilderOptions{
		IgnorePatterns: s.cfg.IgnorePatterns,
		WorkerCount:    s.workerCnt,
	})
	if onProgress != nil {
		builder.SetProgressCallback(onProgress)
	}

	// Add throttling delay if enabled
	if s.throttle {
		// Use a separate context with timeout for throttled builds
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
		defer cancel()

		if err := builder.BuildThrottled(ctx, s.cfg.Directories, s.throttleDelay()); err != nil {
			slog.Error("index rebuild failed", "error", err)
			return
		}
	} else {
		if err := builder.Build(s.ctx, s.cfg.Directories); err != nil {
			slog.Error("index rebuild failed", "error", err)
			return
		}
	}

	// Replace index in database (nil db = persistence disabled, e.g. persist_mode: none)
	newIdx := builder.Index()
	count := newIdx.Len()
	if s.db != nil {
		// Atomically replace all entries in the database
		if err := s.db.ReplaceAllEntries(newIdx.GetAllEntries()); err != nil {
			slog.Error("failed to replace index in database", "error", err)
		}
	}

	elapsed := time.Since(start)
	s.mu.Lock()
	s.lastBuild = time.Now()
	hook := s.onIndexBuilt
	s.mu.Unlock()

	// Notify the daemon so the live index/updater are swapped too
	if hook != nil {
		hook(newIdx)
	}

	slog.Info("index rebuild completed", "entries", count, "elapsed", elapsed)
}

// throttleDelay returns the delay between operations in throttle mode.
func (s *Scheduler) throttleDelay() time.Duration {
	if s.throttle {
		return 10 * time.Millisecond
	}
	return 0
}

// GetLastBuild returns the last build time.
func (s *Scheduler) GetLastBuild() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastBuild
}

// TriggerBuild triggers an immediate rebuild.
func (s *Scheduler) TriggerBuild(throttle bool) {
	slog.Info("manual index rebuild triggered", "throttle", throttle)

	// Temporarily adjust throttle setting
	originalThrottle := s.throttle
	originalWorkers := s.workerCnt

	if throttle != s.throttle {
		s.mu.Lock()
		s.throttle = throttle
		if throttle {
			s.workerCnt = runtime.NumCPU() / 2
			if s.workerCnt < 1 {
				s.workerCnt = 1
			}
		} else {
			s.workerCnt = runtime.NumCPU() * 2
		}
		s.mu.Unlock()
	}

	// Perform rebuild
	s.rebuild()

	// Restore original settings
	s.mu.Lock()
	s.throttle = originalThrottle
	s.workerCnt = originalWorkers
	s.mu.Unlock()
}

// Stats returns scheduler statistics.
func (s *Scheduler) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]any{
		"interval":     s.interval.String(),
		"throttle":     s.throttle,
		"worker_count": s.workerCnt,
		"last_build":   s.lastBuild.Format(time.RFC3339),
	}
}
