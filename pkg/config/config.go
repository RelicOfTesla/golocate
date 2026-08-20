// Package config provides configuration management.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
	// ThrottleWindow is how long after service start automatic background
	// scans run throttled (low IO). A search request during this window lifts
	// the throttle immediately. "0" disables the window (no boot throttling).
	ThrottleWindow string `yaml:"throttle_window"`
	// IndexStrategy is the index update strategy: "replace", "merge", or "auto"
	IndexStrategy string `yaml:"index_strategy"`
	// PersistMode selects the persistence strategy component:
	//   "incremental" (default) - baseline snapshot + watcher-driven changes
	//                             applied in batched low-volume writes; the
	//                             stored index stays current without periodic
	//                             full rewrites
	//   "snapshot"              - full snapshot written after each index build
	//   "none"                  - no persistence at all; cold start rebuilds
	PersistMode string `yaml:"persist_mode"`
	// SnapshotMaxAge is how old a snapshot may be before it is considered
	// stale and a background rebuild is triggered on start (e.g. "24h", "0" = never stale)
	SnapshotMaxAge string `yaml:"snapshot_max_age"`
	// PersistFlushInterval is how often buffered watcher changes are flushed
	// to disk in incremental mode (e.g. "30s"). "0" = flush on threshold only.
	PersistFlushInterval string `yaml:"persist_flush_interval"`
	// ContentIndex enables the optional in-memory content token index (keyword
	// -> files). It makes single-word content searches use precise candidates
	// instead of scanning up to the candidate cap. NOT persisted: rebuilt on
	// every index build, and watcher changes are NOT reflected until the next
	// rebuild. Costs extra memory proportional to indexed tokens.
	ContentIndex bool `yaml:"content_index"`
	// ContentIndexMaxTokens caps tokens kept per file in the optional content
	// index (0 = default 256). Tune down to shrink its memory footprint.
	ContentIndexMaxTokens int `yaml:"content_index_max_tokens"`
}

// getWindowsDrives returns a list of available drive letters on Windows.
// On non-Windows systems, it returns nil.
func getWindowsDrives() []string {
	if runtime.GOOS != "windows" {
		return nil
	}

	// Use wmic to get list of drives
	// Note: This requires wmic to be available on the system
	cmd := exec.Command("wmic", "logicaldisk", "get", "caption")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: return common drives
		return []string{"C:\\"}
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var drives []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 2 && line[1] == ':' {
			drives = append(drives, line+"\\")
		}
	}

	if len(drives) == 0 {
		return []string{"C:\\"}
	}

	return drives
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = os.TempDir()
	}

	// 根据操作系统设置默认目录
	var defaultDirs []string
	if runtime.GOOS == "windows" {
		// Windows: 获取所有磁盘分区
		defaultDirs = getWindowsDrives()
		if len(defaultDirs) == 0 {
			// Fallback: 使用用户主目录
			defaultDirs = []string{homeDir}
		}
	} else {
		// Unix/Linux 使用根目录
		defaultDirs = []string{"/"}
	}

	// 根据操作系统设置忽略模式
	var ignorePatterns = DefaultIgnorePatterns()

	return &Config{
		Directories:           defaultDirs,
		IgnorePatterns:        ignorePatterns,
		DatabasePath:          filepath.Join(homeDir, ".local/share/golocate/index.db"),
		SocketPath:            GetDefaultSocketPath(), // 使用跨平台函数
		PIDFile:               filepath.Join(homeDir, ".local/run/golocate.pid"),
		LogFile:               filepath.Join(homeDir, ".local/log/golocate.log"),
		FollowSymlinks:        false,
		WorkerCount:           4,
		ContentSearch:         false,
		MaxContentFileSize:    10 * 1024 * 1024, // 10MB
		IndexInterval:         "",               // 定时全量重建默认关闭（incremental 模式下不需要）
		ThrottleIndex:         true,             // 默认降频
		ThrottleWindow:        "10m",            // 开机窗口内后台扫描节流，搜索即提速
		IndexStrategy:         "auto",           // 默认自动选择
		PersistMode:           "incremental",    // 默认增量持久化（写量 ≈ 变更量，无定期全量）
		SnapshotMaxAge:        "24h",            // snapshot 模式下：快照超过 24h 视为过期
		PersistFlushInterval:  "30s",            // incremental 批量落盘间隔
		ContentIndex:          false,            // 可选内容倒排索引（默认关：内存代价）
		ContentIndexMaxTokens: 0,                // 0 = content 包默认 256
	}
}

// DefaultIgnorePatterns returns common patterns to ignore.
func DefaultIgnorePatterns() []string {

	var ignorePatterns []string
	if runtime.GOOS == "windows" {
		// Windows 特定的忽略模式
		ignorePatterns = []string{
			"*.git",
			"*.svn",
			"*.hg",
			"*.bzr",
			"*node_modules",
			"*.cache",
			"*.tmp",
			"*.swp",
			"*.swo",
			"*.bak",
			"Thumbs.db",
			"desktop.ini",
			"$RECYCLE.BIN",
		}
	} else {
		// Unix/Linux 特定的忽略模式
		ignorePatterns = []string{
			"/proc",
			"/sys",
			"/dev",
			"/run",
			// 版本控制元数据目录（任意层级）
			"*.git",
			"*.svn",
			"*.hg",
			"*.bzr",
			// 依赖目录
			"*node_modules",
			// 缓存 / 临时文件
			"*.cache",
			"*.tmp",
			"*.swp",
			"*.swo",
			"*.bak",
			// 系统/工具产生的杂项文件
			".DS_Store",
			"Thumbs.db",
		}
	}
	return ignorePatterns
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

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate worker count
	if c.WorkerCount < 1 {
		return fmt.Errorf("worker_count must be at least 1")
	}
	if c.WorkerCount > 100 {
		return fmt.Errorf("worker_count must not exceed 100")
	}

	// Validate max content file size
	if c.MaxContentFileSize < 0 {
		return fmt.Errorf("max_content_file_size must be non-negative")
	}

	// Validate index interval (if not empty)
	if c.IndexInterval != "" {
		if _, err := parseDuration(c.IndexInterval); err != nil {
			return fmt.Errorf("invalid index_interval: %w", err)
		}
	}

	// Validate index strategy
	validStrategies := map[string]bool{
		"replace": true,
		"merge":   true,
		"auto":    true,
		"":        true,
	}
	if !validStrategies[c.IndexStrategy] {
		return fmt.Errorf("invalid index_strategy: must be 'replace', 'merge', or 'auto'")
	}

	// Validate persist mode
	validModes := map[string]bool{
		"snapshot":    true,
		"none":        true,
		"incremental": true,
		"":            true,
	}
	if !validModes[c.PersistMode] {
		return fmt.Errorf("invalid persist_mode: must be 'snapshot', 'none', or 'incremental'")
	}

	// Validate snapshot max age (if not empty)
	if c.SnapshotMaxAge != "" {
		if _, err := parseDuration(c.SnapshotMaxAge); err != nil {
			return fmt.Errorf("invalid snapshot_max_age: %w", err)
		}
	}

	// Validate throttle window (if not empty)
	if c.ThrottleWindow != "" {
		if _, err := parseDuration(c.ThrottleWindow); err != nil {
			return fmt.Errorf("invalid throttle_window: %w", err)
		}
	}

	// Validate persist flush interval (if not empty)
	if c.PersistFlushInterval != "" {
		if _, err := parseDuration(c.PersistFlushInterval); err != nil {
			return fmt.Errorf("invalid persist_flush_interval: %w", err)
		}
	}

	return nil
}

// ParseDuration parses a duration string (e.g., "2h", "30m", "1h30m") into a time.Duration.
func ParseDuration(s string) (time.Duration, error) {
	secs, err := parseDuration(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs) * time.Second, nil
}

// parseDuration parses a duration string (e.g., "2h", "30m", "1h30m").
func parseDuration(s string) (int64, error) {
	// Simple parser for duration strings like "2h", "30m", "1h30m"
	total := int64(0)
	current := int64(0)

	for _, ch := range s {
		switch {
		case ch >= '0' && ch <= '9':
			current = current*10 + int64(ch-'0')
		case ch == 'h':
			total += current * 3600
			current = 0
		case ch == 'm':
			total += current * 60
			current = 0
		case ch == 's':
			total += current
			current = 0
		default:
			return 0, fmt.Errorf("invalid duration format: %s", s)
		}
	}

	if current != 0 {
		return 0, fmt.Errorf("invalid duration format: %s (no unit specified)", s)
	}

	if total == 0 {
		return 0, fmt.Errorf("duration cannot be zero")
	}

	return total, nil
}

// SetField sets a configuration field by key name.
// Supports both simple keys (e.g., "worker_count") and array access (e.g., "directories[0]").
// Value should be a string that will be parsed to the appropriate type.
func (c *Config) SetField(key, value string) error {
	switch key {
	case "directories":
		// Parse as comma-separated list
		dirs := parseStringList(value)
		c.Directories = dirs

	case "ignore_patterns":
		// Parse as comma-separated list
		patterns := parseStringList(value)
		c.IgnorePatterns = patterns

	case "database_path":
		c.DatabasePath = value

	case "socket_path":
		c.SocketPath = value

	case "pid_file":
		c.PIDFile = value

	case "log_file":
		c.LogFile = value

	case "follow_symlinks":
		val, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.FollowSymlinks = val

	case "worker_count":
		val, err := parseInt(value, 1, 100)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.WorkerCount = val

	case "content_search":
		val, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.ContentSearch = val

	case "max_content_file_size":
		val, err := parseInt64(value, 0)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.MaxContentFileSize = val

	case "index_interval":
		// Validate the interval format
		if value != "" {
			if _, err := parseDuration(value); err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
		}
		c.IndexInterval = value

	case "throttle_index":
		val, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.ThrottleIndex = val

	case "throttle_window":
		if value != "" {
			if _, err := parseDuration(value); err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
		}
		c.ThrottleWindow = value

	case "index_strategy":
		validStrategies := map[string]bool{
			"replace": true,
			"merge":   true,
			"auto":    true,
		}
		if !validStrategies[value] {
			return fmt.Errorf("invalid value for %s: must be 'replace', 'merge', or 'auto'", key)
		}
		c.IndexStrategy = value

	case "persist_mode":
		validModes := map[string]bool{
			"snapshot":    true,
			"none":        true,
			"incremental": true,
		}
		if !validModes[value] {
			return fmt.Errorf("invalid value for %s: must be 'snapshot', 'none', or 'incremental'", key)
		}
		c.PersistMode = value

	case "snapshot_max_age":
		if value != "" {
			if _, err := parseDuration(value); err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
		}
		c.SnapshotMaxAge = value

	case "persist_flush_interval":
		if value != "" {
			if _, err := parseDuration(value); err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
		}
		c.PersistFlushInterval = value

	case "content_index":
		val, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.ContentIndex = val

	case "content_index_max_tokens":
		val, err := parseInt(value, 0, 100000)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		c.ContentIndexMaxTokens = val

	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	return nil
}

// parseStringList parses a comma-separated string into a slice.
// Handles quoted values and escapes.
func parseStringList(s string) []string {
	if s == "" {
		return nil
	}

	var result []string
	current := ""
	inQuotes := false
	escape := false

	for _, ch := range s {
		switch {
		case escape:
			current += string(ch)
			escape = false

		case ch == '\\':
			escape = true

		case ch == '"':
			inQuotes = !inQuotes

		case ch == ',' && !inQuotes:
			if current != "" {
				result = append(result, current)
				current = ""
			}

		default:
			current += string(ch)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// parseBool parses a string into a boolean.
func parseBool(s string) (bool, error) {
	switch s {
	case "true", "yes", "1", "on":
		return true, nil
	case "false", "no", "0", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}

// parseInt parses a string into an integer within a range.
func parseInt(s string, min, max int) (int, error) {
	var val int
	if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
		return 0, fmt.Errorf("invalid integer value: %s", s)
	}
	if val < min || val > max {
		return 0, fmt.Errorf("value must be between %d and %d", min, max)
	}
	return val, nil
}

// parseInt64 parses a string into an int64 with a minimum value.
func parseInt64(s string, min int64) (int64, error) {
	var val int64
	if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
		return 0, fmt.Errorf("invalid integer value: %s", s)
	}
	if val < min {
		return 0, fmt.Errorf("value must be at least %d", min)
	}
	return val, nil
}

// LoadFromYAML loads configuration from YAML content.
func LoadFromYAML(content []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	return cfg, nil
}
