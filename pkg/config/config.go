// Package config provides configuration management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config is the main configuration.
type Config struct {
	// Directories to index/watch
	Directories []string `yaml:"directories"`
	// IgnorePatterns are glob patterns to ignore
	IgnorePatterns []string `yaml:"ignore_patterns"`
	// DatabasePath is the path to the index database
	DatabasePath string `yaml:"database_path"`
	// SocketPath is the path to the Unix socket for IPC
	SocketPath string `yaml:"socket_path"`
	// PIDFile is the path to the PID file
	PIDFile string `yaml:"pid_file"`
	// LogFile is the path to the log file
	LogFile string `yaml:"log_file"`
	// FollowSymlinks indicates whether to follow symbolic links
	FollowSymlinks bool `yaml:"follow_symlinks"`
	// WorkerCount is the number of concurrent workers
	WorkerCount int `yaml:"worker_count"`
	// ContentSearch enables content indexing
	ContentSearch bool `yaml:"content_search"`
	// MaxContentFileSize is the maximum file size for content indexing (bytes)
	MaxContentFileSize int64 `yaml:"max_content_file_size"`
	// IndexInterval is the interval for periodic index rebuilding (e.g., "2h", "30m")
	IndexInterval string `yaml:"index_interval"`
	// ThrottleIndex enables throttled indexing for periodic rebuilds
	ThrottleIndex bool `yaml:"throttle_index"`
	// IndexStrategy is the index update strategy: "replace", "merge", or "auto"
	IndexStrategy string `yaml:"index_strategy"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "/tmp"
	}
	
	// 根据操作系统设置默认目录
	var defaultDirs []string
	if runtime.GOOS == "windows" {
		// Windows 没有根目录，使用用户主目录
		defaultDirs = []string{homeDir}
	} else {
		// Unix/Linux 使用根目录
		defaultDirs = []string{"/"}
	}
	
	return &Config{
		Directories: defaultDirs,
		IgnorePatterns: []string{
			"/proc",
			"/sys",
			"/dev",
			"/run",
			"/tmp",
			"*.git",
			"*.svn",
			"*.hg",
			"*node_modules",
			"*.cache",
			"*.Cache",
		},
		DatabasePath:       filepath.Join(homeDir, ".local/share/golocate/index.db"),
		SocketPath:         filepath.Join(homeDir, ".local/run/golocate.sock"),
		PIDFile:            filepath.Join(homeDir, ".local/run/golocate.pid"),
		LogFile:            filepath.Join(homeDir, ".local/log/golocate.log"),
		FollowSymlinks:     false,
		WorkerCount:        4,
		ContentSearch:      false,
		MaxContentFileSize: 10 * 1024 * 1024, // 10MB
		IndexInterval:      "3h",              // 默认 3 小时
		ThrottleIndex:      true,              // 默认降频
		IndexStrategy:      "auto",            // 默认自动选择
	}
}

// DefaultIgnorePatterns returns common patterns to ignore.
func DefaultIgnorePatterns() []string {
	return []string{
		"/proc",
		"/sys",
		"/dev",
		"/run",
		"/tmp",
		"/var/cache",
		"/var/tmp",
		"/var/log",
		"*.git",
		"*.svn",
		"*.hg",
		"*node_modules",
		"*.cache",
		"*.Cache",
		"*__pycache__",
		"*.pyc",
		"*.pyo",
		"*.o",
		"*.a",
		"*.so",
		"*.dylib",
		"*.dll",
		"*.exe",
		"*.class",
		"*.jar",
		"*.war",
		"*.ear",
	}
}

// Load loads the configuration from a file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	// Parse YAML using yaml.v3 library
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	return cfg, nil
}

// Save saves the configuration to a file.
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Marshal to YAML using yaml.v3 library
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Add header comment
	header := "# golocate configuration file\n\n"
	content := header + string(data)
	
	return os.WriteFile(path, []byte(content), 0644)
}

// ConfigPath returns the default config file path.
func ConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return "/etc/golocate/config"
	}
	return filepath.Join(homeDir, ".config/golocate/config")
}

// EnsureDirs ensures all necessary directories exist.
func (c *Config) EnsureDirs() error {
	dirs := []string{
		filepath.Dir(c.DatabasePath),
		filepath.Dir(c.SocketPath),
		filepath.Dir(c.PIDFile),
		filepath.Dir(c.LogFile),
	}
	
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	
	return nil
}
