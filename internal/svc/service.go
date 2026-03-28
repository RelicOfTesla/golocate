// Package svc provides cross-platform service management.
package svc

import (
	"context"
	"fmt"
	"log"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/watcher"
	"github.com/kardianos/service"
)

// DaemonService implements service.Interface for cross-platform service management.
type DaemonService struct {
	cfg     *config.Config
	db      *database.DB
	watcher watcher.Watcher
	updater *index.Updater
	server  *server.Server
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewDaemonService creates a new daemon service.
func NewDaemonService(cfg *config.Config) *DaemonService {
	ctx, cancel := context.WithCancel(context.Background())
	return &DaemonService{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start is called when the service starts.
func (d *DaemonService) Start(s service.Service) error {
	log.Println("starting golocate daemon...")

	// Ensure directories exist
	if err := d.cfg.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Open database
	db, err := database.Open(d.cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	d.db = db

	// Create watcher
	w, err := watcher.NewWatcher(d.ctx, &watcher.Config{
		Directories:    d.cfg.Directories,
		IgnorePatterns: d.cfg.IgnorePatterns,
		Recursive:      true,
	})
	if err != nil {
		d.db.Close()
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	d.watcher = w

	// Build initial index
	builder := index.NewBuilder(index.BuilderOptions{
		IgnorePatterns: d.cfg.IgnorePatterns,
		WorkerCount:    d.cfg.WorkerCount,
	})

	if err := builder.Build(d.ctx, d.cfg.Directories); err != nil {
		d.watcher.Close()
		d.db.Close()
		return fmt.Errorf("failed to build index: %w", err)
	}

	d.updater = index.NewUpdater(builder.Index())

	log.Printf("indexed %d entries", builder.Index().Len())
	log.Printf("watcher type: %s", watcher.GetWatcherType())

	// Start Unix socket server
	log.Printf("[DEBUG] Creating Unix socket server...")
	d.server = server.New(builder.Index())
	log.Printf("[DEBUG] Starting Unix socket server...")
	if err := d.server.Start(); err != nil {
		log.Printf("[ERROR] Failed to start server: %v", err)
		d.watcher.Close()
		d.db.Close()
		return fmt.Errorf("failed to start server: %w", err)
	}

	log.Printf("Unix socket server started on /tmp/golocate.sock")

	// Start file watching in background
	go d.watchLoop()

	return nil
}

// Stop is called when the service stops.
func (d *DaemonService) Stop(s service.Service) error {
	log.Println("stopping golocate daemon...")

	d.cancel()

	if d.server != nil {
		d.server.Stop()
	}

	if d.watcher != nil {
		d.watcher.Close()
	}

	if d.db != nil {
		d.db.Close()
	}

	log.Println("daemon stopped")
	return nil
}

// watchLoop watches for file system changes.
func (d *DaemonService) watchLoop() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case event := <-d.watcher.Events():
			d.updater.HandleEvent(event)
		case err := <-d.watcher.Errors():
			log.Printf("watcher error: %v", err)
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
func Run(cfg *config.Config) error {
	d := NewDaemonService(cfg)
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
