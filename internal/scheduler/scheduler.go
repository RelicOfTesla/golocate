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
	cfg        *config.Config
	db         *database.DB
	interval   time.Duration
	throttle   bool
	workerCnt  int
	lastBuild  time.Time
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// SchedulerConfig is the scheduler configuration.
type SchedulerConfig struct {
	// Interval is the time between rebuilds (default: 2 hours)
	Interval time.Duration
	// Throttle enables throttled indexing (lower CPU usage)
	Throttle bool
	// WorkerCount is the number of concurrent workers
	WorkerCount int
}

// NewScheduler creates a new index scheduler.
func NewScheduler(cfg *config.Config, db *database.DB, scfg *SchedulerConfig) *Scheduler {
	if scfg == nil {
		scfg = &SchedulerConfig{}
	}

	// Set defaults
	if scfg.Interval == 0 {
		scfg.Interval = 3 * time.Hour  // 默认 3 小时
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
		cfg:       cfg,
		db:        db,
		interval:  scfg.Interval,
		throttle:  scfg.Throttle,
		workerCnt: workerCount,
		ctx:       ctx,
		cancel:    cancel,
	}
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

	// Initial build
	s.rebuild()

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
	start := time.Now()
	slog.Info("starting index rebuild", "throttle", s.throttle)

	// Build new index
	builder := index.NewBuilder(index.BuilderOptions{
		IgnorePatterns: s.cfg.IgnorePatterns,
		WorkerCount:    s.workerCnt,
	})

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

	// Replace index in database
	newIdx := builder.Index()
	count := newIdx.Len()

	// Atomically replace all entries in the database
	if err := s.db.ReplaceAllEntries(newIdx.GetAllEntries()); err != nil {
		slog.Error("failed to replace index in database", "error", err)
		return
	}

	elapsed := time.Since(start)
	s.mu.Lock()
	s.lastBuild = time.Now()
	s.mu.Unlock()

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
