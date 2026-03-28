// Package scheduler provides periodic index rebuilding.
package scheduler

import (
	"context"
	"log"
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
	log.Printf("starting index scheduler: interval=%v, throttle=%v, workers=%d",
		s.interval, s.throttle, s.workerCnt)

	s.wg.Add(1)
	go s.loop()
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	log.Println("index scheduler stopped")
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
	log.Printf("starting index rebuild (throttle=%v)", s.throttle)

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
			log.Printf("index rebuild failed: %v", err)
			return
		}
	} else {
		if err := builder.Build(s.ctx, s.cfg.Directories); err != nil {
			log.Printf("index rebuild failed: %v", err)
			return
		}
	}

	// Replace index in database
	newIdx := builder.Index()
	count := newIdx.Len()

	// TODO: Implement atomic index replacement in database
	// For now, we just log the result

	elapsed := time.Since(start)
	s.mu.Lock()
	s.lastBuild = time.Now()
	s.mu.Unlock()

	log.Printf("index rebuild completed: %d entries in %v", count, elapsed)
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
	log.Printf("manual index rebuild triggered (throttle=%v)", throttle)

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
func (s *Scheduler) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"interval":     s.interval.String(),
		"throttle":     s.throttle,
		"worker_count": s.workerCnt,
		"last_build":   s.lastBuild.Format(time.RFC3339),
	}
}
