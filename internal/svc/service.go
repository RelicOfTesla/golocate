// Package svc provides cross-platform service management.
package svc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/internal/persist"
	"github.com/RelicOfTesla/golocate/internal/scheduler"
	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/pkg/config"
	contentpkg "github.com/RelicOfTesla/golocate/pkg/content"
	"github.com/RelicOfTesla/golocate/pkg/ignore"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/watcher"
	"github.com/kardianos/service"
)

// DaemonService implements service.Interface for cross-platform service management.
type DaemonService struct {
	cfg        *config.Config
	configPath string // 配置文件路径
	db         *database.DB
	persister  persist.Strategy // pluggable persistence component
	watcher    watcher.Watcher
	updater    *index.Updater
	server     *server.Server
	scheduler  *scheduler.Scheduler
	ctx        context.Context
	cancel     context.CancelFunc

	// ignoreMatcher filters watcher events for ignored paths so the index
	// stays consistent with the builder's ignore patterns.
	ignoreMatcher *ignore.Matcher

	// logWriter holds the rotated daemon log (nil when logging to stderr).
	logWriter *rotatingFile

	// Boot-scan throttle lifting: within throttleWindow after service start,
	// an automatic background scan runs at low IO; the first search request
	// lifts the throttle to full speed immediately.
	serviceStart      time.Time
	throttleWindow    time.Duration
	backgroundBuilder *index.Builder // non-nil while a boot scan is running (guarded by mu)
	buildBoosted      atomic.Bool    // search already lifted the throttle

	// contentIndex is the optional in-memory content token index (guarded by
	// mu): rebuilt on every index build and kept fresh by watcher events.
	contentIndex *contentpkg.Index

	// mu guards watcher/updater/cfg swaps performed at runtime
	// (watcher restart on config change, updater swap after rebuilds).
	mu sync.Mutex
}

// NewDaemonService creates a new daemon service.
func NewDaemonService(cfg *config.Config, configPath string) *DaemonService {
	ctx, cancel := context.WithCancel(context.Background())
	return &DaemonService{
		cfg:           cfg,
		configPath:    configPath,
		ctx:           ctx,
		cancel:        cancel,
		ignoreMatcher: ignore.NewMatcher(buildWatchPatterns(cfg)),
	}
}

// Start is called when the service starts.
func (d *DaemonService) Start(s service.Service) error {
	slog.Info("starting golocate daemon...")

	// Redirect daemon logs to the configured log file (with rotation) when
	// running as a service. In foreground/CLI mode logs stay on stderr.
	if d.cfg.LogFile != "" {
		w, err := openLogFile(d.cfg.LogFile, DefaultLogMaxSize)
		if err != nil {
			slog.Warn("failed to open log_file, keeping stderr logging", "path", d.cfg.LogFile, "error", err)
		} else {
			d.logWriter = w
			slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
			slog.Info("daemon log redirect", "file", d.cfg.LogFile, "max_size", DefaultLogMaxSize)
		}
	}

	// Ensure directories exist
	if err := d.cfg.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Open database unless persistence is disabled entirely.
	if d.cfg.PersistMode != persist.ModeNone {
		db, err := database.Open(d.cfg.DatabasePath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		d.db = db
	}

	// Create the persistence strategy component (pluggable).
	maxAgeSecs := int64(0)
	if d.cfg.SnapshotMaxAge != "" {
		if dur, err := config.ParseDuration(d.cfg.SnapshotMaxAge); err == nil {
			maxAgeSecs = int64(dur.Seconds())
		} else {
			slog.Warn("invalid snapshot_max_age, snapshots never expire", "error", err)
		}
	}
	flushInterval := time.Duration(0)
	if d.cfg.PersistFlushInterval != "" {
		if dur, err := config.ParseDuration(d.cfg.PersistFlushInterval); err == nil {
			flushInterval = dur
		} else {
			slog.Warn("invalid persist_flush_interval, using default", "error", err)
		}
	}
	d.persister = persist.New(d.cfg.PersistMode, d.db, persist.Options{
		SnapshotMaxAgeSecs: maxAgeSecs,
		FlushInterval:      flushInterval,
	})

	// Try to restore a previously persisted index so the service is usable
	// immediately; a failed restore falls back to a background rebuild.
	initialIdx, restored, err := d.persister.Restore(d.cfg.Directories)
	if err != nil {
		slog.Error("failed to restore index", "error", err)
		restored = false
		initialIdx = index.NewIndex()
	}

	// Create server with the initial index
	d.server = server.New(initialIdx)

	// Set status tracking fields
	d.server.SetSocketPath(d.cfg.SocketPath)
	d.server.SetDatabasePath(d.cfg.DatabasePath)
	d.server.SetConfigPath(d.configPath)
	d.server.SetConfig(d.cfg)

	// Register runtime hooks:
	//  - index built  -> swap the watcher updater + persist to DB
	//  - config changed -> restart watcher + trigger a rebuild
	//  - search       -> lift the boot-scan throttle if one is running
	d.server.SetIndexBuiltHook(d.swapIndex)
	d.server.SetConfigChangedHook(d.applyConfig)
	d.server.SetSearchHook(d.onSearch)

	// Boot-scan throttle window: automatic background scans run at low IO
	// shortly after startup, and speed up the moment a search arrives.
	d.serviceStart = time.Now()
	if d.cfg.ThrottleWindow != "" {
		if dur, err := config.ParseDuration(d.cfg.ThrottleWindow); err == nil {
			d.throttleWindow = dur
		}
	}

	// Mark as building index
	buildStartTime := time.Now()
	d.server.SetBuildingStatus(true, buildStartTime)

	// Start Unix socket server: the service is usable from here on.
	slog.Debug("starting Unix socket server...")
	if err := d.server.Start(); err != nil {
		slog.Error("failed to start server", "error", err)
		if d.db != nil {
			d.db.Close()
		}
		return fmt.Errorf("failed to start server: %w", err)
	}

	slog.Info("Server started", "path", d.cfg.SocketPath)

	if restored {
		// Snapshot restored: ready immediately, watcher keeps it fresh.
		d.server.SetBuildingStatus(false, time.Time{})
		d.server.SetLastBuildTime(time.Now())
		d.server.SetIndexedFileCount(initialIdx.Len())
		d.updater = d.newUpdater(initialIdx)
		slog.Info("index restored from database", "count", initialIdx.Len())
	} else {
		// No usable snapshot (first run, config changed, dirty, or too old):
		// serve with an empty index now and rebuild in the background with
		// throttled IO so startup does not freeze the machine.
		slog.Info("no usable snapshot, building index in background")
		go d.buildInBackground()
	}

	// Create watcher after the index is ready, so events update a live index
	w, err := watcher.NewWatcher(d.ctx, &watcher.Config{
		Directories:    d.cfg.Directories,
		IgnorePatterns: buildWatchPatterns(d.cfg),
		Recursive:      true,
		FollowSymlinks: d.cfg.FollowSymlinks,
	})
	if err != nil {
		d.server.Stop()
		if d.db != nil {
			d.db.Close()
		}
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	d.watcher = w
	slog.Info("watcher type", "type", watcher.GetWatcherType())

	// Start periodic index rebuilds (index_interval)
	d.startScheduler()

	// Start file watching in background
	go d.watchLoop()

	return nil
}

// daemonOwnPatterns returns the daemon's own output files (db/socket/log/pid)
// as ignore patterns (exact path + temporary siblings), so the watcher never
// re-indexes — and thus never re-persists / re-builds on — its own writes.
// Without this, placing the db/log/socket inside a monitored directory feeds a
// self-triggering event→persist→event loop that keeps the daemon busy (CPU).
func daemonOwnPatterns(cfg *config.Config) []string {
	var pats []string
	add := func(p string) {
		if p == "" {
			return
		}
		pats = append(pats, p, p+"*") // exact + "<path>.tmp"-style siblings
	}
	add(cfg.DatabasePath)
	add(cfg.SocketPath)
	add(cfg.LogFile)
	add(cfg.PIDFile)
	return pats
}

// buildWatchPatterns combines user ignore patterns with the daemon's own
// outputs, for both the watcher and the ignore matcher.
func buildWatchPatterns(cfg *config.Config) []string {
	owns := daemonOwnPatterns(cfg)
	if len(owns) == 0 {
		return cfg.IgnorePatterns
	}
	return append(append([]string{}, cfg.IgnorePatterns...), owns...)
}

// buildInBackground builds the index with throttled IO and hot-swaps it once
// complete. Runs when no usable snapshot exists at startup. Within the boot
// throttle window the scan is low-IO; the first search request lifts it.
func (d *DaemonService) buildInBackground() {
	defer func() {
		d.server.SetBuildingStatus(false, time.Time{})
		d.mu.Lock()
		d.backgroundBuilder = nil
		d.mu.Unlock()
		d.buildBoosted.Store(false)
	}()

	builder := index.NewBuilder(index.BuilderOptions{
		IgnorePatterns: buildWatchPatterns(d.cfg),
		WorkerCount:    d.cfg.WorkerCount,
	})
	builder.SetProgressCallback(func(scanned int64) {
		d.server.SetBuildProgress(scanned)
	})

	delay := time.Duration(0)
	if d.cfg.ThrottleIndex && d.throttleWindow > 0 && time.Since(d.serviceStart) < d.throttleWindow {
		delay = 10 * time.Millisecond // low-IO boot scan
	}
	d.mu.Lock()
	d.backgroundBuilder = builder
	d.mu.Unlock()
	d.buildBoosted.Store(false)

	slog.Info("starting background index build",
		"directories", d.cfg.Directories, "throttle", delay > 0, "throttle_window", d.throttleWindow)
	buildStart := time.Now()
	if err := builder.BuildThrottled(d.ctx, d.cfg.Directories, delay); err != nil {
		if d.ctx.Err() != nil {
			slog.Info("background index build cancelled")
		} else {
			slog.Error("background index build failed", "error", err)
		}
		return
	}

	newIdx := builder.Index()
	d.server.SetIndex(newIdx)
	d.server.SetLastBuildTime(time.Now())
	d.server.SetIndexedFileCount(newIdx.Len())
	if f, dd := builder.Stats(); f > 0 || dd > 0 {
		d.server.SetLastBuildStats(f, dd)
		d.server.RecordBuild(f, dd, time.Since(buildStart))
	}
	if perDir := builder.PerDirStats(); len(perDir) > 0 {
		converted := make(map[string]server.PerDirCount, len(perDir))
		for dir, c := range perDir {
			converted[dir] = server.PerDirCount{Files: c.Files, Dirs: c.Dirs}
		}
		d.server.SetLastBuildPerDir(converted)
	}

	// Swap updater + persist (same hook the socket-triggered builds use).
	d.swapIndex(newIdx)
	slog.Info("background index build completed", "count", newIdx.Len())
}

// onSearch is the server search hook: if a throttled boot scan is running,
// lift it to full speed so the waiting user gets results as fast as possible.
func (d *DaemonService) onSearch() {
	if !d.buildBoosted.CompareAndSwap(false, true) {
		return // already boosted
	}
	d.mu.Lock()
	builder := d.backgroundBuilder
	d.mu.Unlock()
	if builder != nil {
		slog.Info("search request during boot scan, lifting throttle to full speed")
		builder.SetThrottleDelay(0)
	}
}

// swapIndex is the index-built hook: point the watcher updater at the new index
// and persist it via the configured strategy.
func (d *DaemonService) swapIndex(idx *index.Index) {
	// Snapshot config outside the lock, then build the content index OUTSIDE
	// the lock too (tokenizing reads every file and can take a while).
	d.mu.Lock()
	dirs := append([]string(nil), d.cfg.Directories...)
	contentIndexEnabled := d.cfg.ContentIndex
	d.mu.Unlock()

	var newContentIndex *contentpkg.Index
	if contentIndexEnabled {
		newContentIndex = d.buildContentIndex(idx)
	}

	d.mu.Lock()
	d.contentIndex = newContentIndex
	d.updater = d.newUpdater(idx)
	d.mu.Unlock()
	d.server.SetContentIndex(newContentIndex)

	if d.persister != nil {
		if err := d.persister.Persist(idx, dirs); err != nil {
			slog.Error("failed to persist index", "error", err)
		}
	}
}

// buildContentIndex tokenizes every indexed file into the in-memory content
// index (keyword -> paths). Cost: reads every file once per build.
func (d *DaemonService) buildContentIndex(idx *index.Index) *contentpkg.Index {
	ix := contentpkg.NewIndexParam(d.cfg.MaxContentFileSize, d.cfg.ContentIndexMaxTokens)
	start := time.Now()
	for _, e := range idx.GetAllEntries() {
		if e.IsDir {
			continue
		}
		ix.AddFile(e.Path)
	}
	slog.Info("content index built", "files", ix.FileCount(), "elapsed", time.Since(start))
	return ix
}

// newUpdater builds an updater wired to the persistence strategy and the
// content index, so every watcher mutation is also fed to the incremental
// change buffer and the content token index.
func (d *DaemonService) newUpdater(idx *index.Index) *index.Updater {
	return index.NewUpdaterWithCallbacks(idx, d.ignoreMatcher,
		func(e *index.Entry) {
			if d.persister != nil {
				_ = d.persister.ApplyChange(persist.Change{Upsert: e})
			}
			// Keep the content index fresh: new/written files are re-tokenized.
			d.mu.Lock()
			ci := d.contentIndex
			d.mu.Unlock()
			if ci != nil && !e.IsDir {
				ci.AddFile(e.Path)
			}
		},
		func(path string) {
			if d.persister != nil {
				_ = d.persister.ApplyChange(persist.Change{Delete: path})
			}
			d.mu.Lock()
			ci := d.contentIndex
			d.mu.Unlock()
			if ci != nil {
				ci.RemoveFile(path)
			}
		},
	)
}

// applyConfig is the config-changed hook (set-config / reload-config):
// restart the watcher with the new directories and trigger an index rebuild.
func (d *DaemonService) applyConfig(cfg *config.Config) {
	slog.Info("applying new configuration at runtime", "directories", cfg.Directories)

	d.mu.Lock()
	d.cfg = cfg
	d.ignoreMatcher = ignore.NewMatcher(buildWatchPatterns(cfg))
	if d.server != nil {
		d.server.SetConfig(cfg)
	}
	if d.scheduler != nil {
		d.scheduler.SetConfig(cfg)
	}
	old := d.watcher
	d.watcher = nil
	d.mu.Unlock()

	// Restart the watcher with the new directories
	if old != nil {
		old.Close()
	}
	w, err := watcher.NewWatcher(d.ctx, &watcher.Config{
		Directories:    cfg.Directories,
		IgnorePatterns: buildWatchPatterns(cfg),
		Recursive:      true,
		FollowSymlinks: cfg.FollowSymlinks,
	})
	if err != nil {
		slog.Error("failed to recreate watcher after config change", "error", err)
	} else {
		d.mu.Lock()
		d.watcher = w
		d.mu.Unlock()
	}

	// Rebuild the index with the new configuration
	if d.server != nil {
		if err := d.server.StartBuild(); err != nil {
			slog.Warn("failed to start rebuild after config change", "error", err)
		}
	}
}

// startScheduler starts the periodic index rebuild scheduler (index_interval).
// An empty index_interval disables periodic rebuilds entirely: with
// incremental persistence the stored index stays current, so a periodic full
// rescan is only needed as an explicit safety net.
func (d *DaemonService) startScheduler() {
	if d.cfg.IndexInterval == "" {
		slog.Info("periodic index rebuild disabled (index_interval empty)")
		return
	}

	interval := 24 * time.Hour
	if dur, err := config.ParseDuration(d.cfg.IndexInterval); err == nil {
		interval = dur
	} else {
		slog.Warn("invalid index_interval, using default", "interval", d.cfg.IndexInterval, "error", err)
	}

	d.scheduler = scheduler.NewScheduler(d.cfg, d.db, &scheduler.SchedulerConfig{
		Interval:         interval,
		Throttle:         d.cfg.ThrottleIndex,
		WorkerCount:      d.cfg.WorkerCount,
		SkipInitialBuild: true,
	})
	d.scheduler.SetOnIndexBuilt(d.swapIndex)
	// Report scheduled rebuilds through the same status fields used by
	// socket-triggered builds (is_building + build_scanned), so CLI/H5
	// progress UIs work for periodic rebuilds too.
	d.scheduler.SetOnBuildStart(func() {
		d.server.SetBuildingStatus(true, time.Now())
	})
	d.scheduler.SetOnBuildEnd(func() {
		d.server.SetBuildingStatus(false, time.Time{})
	})
	d.scheduler.SetOnProgress(func(scanned int64) {
		d.server.SetBuildProgress(scanned)
	})
	d.scheduler.Start()
}

// Stop is called when the service stops.
func (d *DaemonService) Stop(s service.Service) error {
	slog.Info("stopping golocate daemon...")

	d.cancel()

	// Stop periodic rebuilds (cancels any in-flight build)
	if d.scheduler != nil {
		d.scheduler.Stop()
	}

	if d.server != nil {
		d.server.Stop()
	}

	if d.watcher != nil {
		d.watcher.Close()
	}

	if d.persister != nil {
		d.persister.Close()
	}

	if d.logWriter != nil {
		d.logWriter.Close()
	}

	if d.db != nil {
		d.db.Close()
	}

	slog.Info("daemon stopped")
	return nil
}

// watchLoop watches for file system changes.
// Channels are re-fetched on every iteration so a watcher restart
// (triggered by a config change) takes effect on the next event.
func (d *DaemonService) watchLoop() {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		d.mu.Lock()
		w := d.watcher
		d.mu.Unlock()
		if w == nil {
			// Watcher is being (re)created; wait briefly.
			select {
			case <-d.ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}

		evCh, errCh := w.Events(), w.Errors()

		select {
		case <-d.ctx.Done():
			return
		case event, ok := <-evCh:
			if !ok {
				continue // watcher closed/replaced; re-fetch
			}
			// Skip events for ignored paths (keeps the index consistent
			// with the builder's ignore patterns).
			if d.ignoreMatcher != nil && d.ignoreMatcher.MatchPath(event.Path) {
				continue
			}
			d.mu.Lock()
			updater := d.updater
			d.mu.Unlock()
			if updater != nil {
				updater.HandleEvent(event)
			}
		case err, ok := <-errCh:
			if !ok {
				continue
			}
			slog.Error("watcher error", "error", err)
			// Events may have been lost: mark the snapshot dirty so the next
			// restart refuses it, and rebuild now to heal the in-memory index.
			if d.persister != nil {
				if merr := d.persister.MarkDirty(); merr != nil {
					slog.Warn("failed to mark index dirty", "error", merr)
				}
			}
			if d.server != nil {
				if berr := d.server.StartBuild(); berr != nil {
					slog.Warn("rebuild after watcher error failed", "error", berr)
				}
			}
		}
	}
}

// ServiceConfig returns the service configuration.
func ServiceConfig() *service.Config {
	return &service.Config{
		Name:        "golocate",
		DisplayName: "golocate file indexing daemon",
		Description: "Fast file search utility with real-time indexing",
	}
}

// Install installs the service.
func Install() error {
	svcConfig := ServiceConfig()
	svc, err := service.New(nil, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	if err := svc.Install(); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	fmt.Println("Service installed successfully")
	fmt.Println("\nTo start the service:")
	fmt.Println("  golocate --start")
	fmt.Println("\nOr using system commands:")
	fmt.Println("  systemctl --user start golocate  # Linux")
	fmt.Println("  sc start golocate                # Windows")
	fmt.Println("  launchctl start golocate         # macOS")

	return nil
}

// Uninstall uninstalls the service.
func Uninstall() error {
	svcConfig := ServiceConfig()
	svc, err := service.New(nil, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("failed to uninstall service: %w", err)
	}

	fmt.Println("Service uninstalled successfully")
	return nil
}

// Start starts the service.
func Start() error {
	svcConfig := ServiceConfig()
	svc, err := service.New(nil, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	if err := svc.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Println("Service started successfully")
	return nil
}

// Stop stops the service.
func Stop() error {
	svcConfig := ServiceConfig()
	svc, err := service.New(nil, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	if err := svc.Stop(); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	fmt.Println("Service stopped successfully")
	return nil
}

// Run runs the service (called by service manager).
func Run(cfg *config.Config, configPath string) error {
	d := NewDaemonService(cfg, configPath)
	svcConfig := ServiceConfig()
	svc, err := service.New(d, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return svc.Run()
}

// Status returns the service status.
func Status() (string, error) {
	svcConfig := ServiceConfig()
	svc, err := service.New(nil, svcConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create service: %w", err)
	}

	status, err := svc.Status()
	if err != nil {
		return "", err
	}

	switch status {
	case service.StatusRunning:
		return "running", nil
	case service.StatusStopped:
		return "stopped", nil
	default:
		return "unknown", nil
	}
}
