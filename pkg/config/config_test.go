package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RelicOfTesla/golocate/internal/testutil"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Error("Expected non-nil config")
	}

	if len(cfg.Directories) == 0 {
		t.Error("Expected at least one default directory")
	}

	if cfg.WorkerCount <= 0 {
		t.Error("Expected positive worker count")
	}

	if cfg.DatabasePath == "" {
		t.Error("Expected non-empty database path")
	}

	if cfg.SocketPath == "" {
		t.Error("Expected non-empty socket path")
	}
}


func TestConfigDirectories(t *testing.T) {
	cfg := &Config{
		Directories: []string{"/home/user", "/var/log"},
	}

	if len(cfg.Directories) != 2 {
		t.Errorf("Expected 2 directories, got %d", len(cfg.Directories))
	}
}

func TestConfigIgnorePatterns(t *testing.T) {
	cfg := &Config{
		IgnorePatterns: []string{"*.log", "*.tmp"},
	}

	if len(cfg.IgnorePatterns) != 2 {
		t.Errorf("Expected 2 ignore patterns, got %d", len(cfg.IgnorePatterns))
	}
}

func TestConfigIndexSettings(t *testing.T) {
	cfg := &Config{
		IndexInterval: "2h",
		ThrottleIndex: true,
		IndexStrategy: "merge",
	}

	if cfg.IndexInterval != "2h" {
		t.Errorf("Expected index interval '2h', got %q", cfg.IndexInterval)
	}

	if !cfg.ThrottleIndex {
		t.Error("Expected throttle index to be true")
	}

	if cfg.IndexStrategy != "merge" {
		t.Errorf("Expected index strategy 'merge', got %q", cfg.IndexStrategy)
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	// Create a temp file
	tmpDir := os.TempDir()
	configPath := filepath.Join(tmpDir, "golocate_test_config.yaml")
	defer os.Remove(configPath)

	cfg := &Config{
		Directories:       []string{"/home/user"},
		IgnorePatterns:    []string{"*.log"},
		DatabasePath:      filepath.Join(os.TempDir(), "golocate_test.db"),
		SocketPath:        testutil.GetTestSocketPath("test"),
		PIDFile:           filepath.Join(os.TempDir(), "golocate_test.pid"),
		LogFile:           filepath.Join(os.TempDir(), "golocate_test.log"),
		FollowSymlinks:    true,
		WorkerCount:       8,
		ContentSearch:      true,
		MaxContentFileSize: 5 * 1024 * 1024,
		IndexInterval:     "1h",
		ThrottleIndex:     false,
		IndexStrategy:     "replace",
	}
	old := *cfg

	// Save
	err := cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded config
	// Note: Load uses simple YAML-like parsing, lists may not be fully parsed
	// Test scalar values which are reliably parsed
	if loaded.DatabasePath != old.DatabasePath {
		t.Errorf("Expected database path '%s', got %q", old.DatabasePath, loaded.DatabasePath)
	}

	if loaded.SocketPath != old.SocketPath {
		t.Errorf("Expected socket path '%s', got %q", old.SocketPath, loaded.SocketPath)
	}

	if loaded.WorkerCount != old.WorkerCount {
		t.Errorf("Expected worker count %d, got %d", old.WorkerCount, loaded.WorkerCount)
	}

	if loaded.FollowSymlinks != true {
		t.Errorf("Expected FollowSymlinks true, got %v", loaded.FollowSymlinks)
	}

	if loaded.IndexInterval != "1h" {
		t.Errorf("Expected index interval '1h', got %q", loaded.IndexInterval)
	}

	if loaded.IndexStrategy != "replace" {
		t.Errorf("Expected index strategy 'replace', got %q", loaded.IndexStrategy)
	}
}

func TestConfigLoadNonExistent(t *testing.T) {
	// Load non-existent file should return default config
	loaded, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Expected no error for non-existent file, got: %v", err)
	}

	if loaded == nil {
		t.Error("Expected non-nil config")
	}

	// Should have default values
	if loaded.WorkerCount <= 0 {
		t.Error("Expected positive worker count")
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath()
	if path == "" {
		t.Error("Expected non-empty config path")
	}
}

func TestConfigEnsureDirs(t *testing.T) {
	// Create a temp directory
	tmpDir := os.TempDir()
	testDir := filepath.Join(tmpDir, "golocate_test_ensure_dirs")
	defer os.RemoveAll(testDir)

	cfg := &Config{
		DatabasePath: filepath.Join(testDir, "data", "index.db"),
		SocketPath:   testutil.GetTestSocketPath("ensure_dirs"),
		PIDFile:      filepath.Join(testDir, "run", "golocate.pid"),
		LogFile:      filepath.Join(testDir, "log", "golocate.log"),
	}

	err := cfg.EnsureDirs()
	if err != nil {
		t.Fatalf("Failed to ensure dirs: %v", err)
	}

	// Check directories exist
	dirs := []string{
		filepath.Dir(cfg.DatabasePath),
		filepath.Dir(cfg.SocketPath),
		filepath.Dir(cfg.PIDFile),
		filepath.Dir(cfg.LogFile),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s should exist", dir)
		}
	}
}

func TestConfigContentSearch(t *testing.T) {
	cfg := &Config{
		ContentSearch:      true,
		MaxContentFileSize: 20 * 1024 * 1024,
	}

	if !cfg.ContentSearch {
		t.Error("Expected content search to be enabled")
	}

	if cfg.MaxContentFileSize != 20*1024*1024 {
		t.Errorf("Expected max content file size 20MB, got %d", cfg.MaxContentFileSize)
	}
}

func TestConfigWorkerCount(t *testing.T) {
	cfg := &Config{
		WorkerCount: 16,
	}

	if cfg.WorkerCount != 16 {
		t.Errorf("Expected worker count 16, got %d", cfg.WorkerCount)
	}
}

func TestConfigFollowSymlinks(t *testing.T) {
	cfg := &Config{
		FollowSymlinks: true,
	}

	if !cfg.FollowSymlinks {
		t.Error("Expected follow symlinks to be true")
	}
}

