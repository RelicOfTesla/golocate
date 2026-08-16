// Package main provides the golocated daemon CLI.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	svc "github.com/RelicOfTesla/golocate/internal/svc"
	cliclient "github.com/RelicOfTesla/golocate/pkg/cli"
	"github.com/RelicOfTesla/golocate/pkg/config"
	errpkg "github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	// Service flags
	flagService       bool
	flagInstall       bool
	flagInstallServer bool // Alias for --install
	flagUninstall     bool
	flagStart         bool
	flagStop          bool
	flagServiceStatus bool
	flagUser          bool
	flagGetConfig     bool
	flagSetConfig     string
	
	// CLI query flags
	flagIgnoreCase bool
	flagBasename   bool
	flagCount      bool
	flagLimit      int
	flagRegexp     bool
	flagRegex      bool
	
	// Index flags
	flagBuild     bool
	flagSchedule  string
	flagStrategy  string
	flagSort      string
	
	// Config flags
	flagConfig      string
	flagVerbose     bool
	flagVerboseType string
	
	// Connection flags
	flagSocket string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "golocated",
		Short: "golocate daemon - background file indexing service",
		Long: `golocated is the daemon component of golocate.

By default (without --service), it acts as a CLI client that connects to the server.
With --service flag, it starts the background service that maintains file indexes.`,
		Run: runDaemon,
	}
	
	// Service management flags
	rootCmd.Flags().BoolVar(&flagService, "service", false, "run as daemon service (default: CLI mode)")
	rootCmd.Flags().BoolVar(&flagInstall, "install", false, "install as system service")
	rootCmd.Flags().BoolVar(&flagInstallServer, "install-server", false, "install as system service (alias for --install)")
	rootCmd.Flags().BoolVar(&flagUninstall, "uninstall", false, "uninstall system service")
	rootCmd.Flags().BoolVar(&flagStart, "start", false, "start the service")
	rootCmd.Flags().BoolVar(&flagStop, "stop", false, "stop the service")
	rootCmd.Flags().BoolVar(&flagServiceStatus, "service-status", false, "show service status")
	rootCmd.Flags().BoolVar(&flagUser, "user", false, "install as user service (with --install)")
	rootCmd.Flags().BoolVar(&flagGetConfig, "get-config", false, "show server configuration")
	rootCmd.Flags().StringVar(&flagSetConfig, "set-config", "", "set configuration (key=value, '-' for stdin, 'interactive' or 'edit' for interactive mode)")
	
	// CLI query flags (same as golocate)
	rootCmd.Flags().BoolVarP(&flagIgnoreCase, "ignore-case", "i", false, "case-insensitive search")
	rootCmd.Flags().BoolVarP(&flagBasename, "basename", "b", false, "search only in file names")
	rootCmd.Flags().BoolVar(&flagCount, "count", false, "print count of matches")
	rootCmd.Flags().IntVarP(&flagLimit, "limit", "l", 0, "limit number of results")
	rootCmd.Flags().BoolVarP(&flagRegexp, "regexp", "r", false, "use basic regex")
	rootCmd.Flags().BoolVar(&flagRegex, "regex", false, "use extended regex")
	rootCmd.Flags().StringVar(&flagSort, "sort", "", "sort by: name, size, time, path (append :desc for descending)")
	
	// Index management flags
	rootCmd.Flags().BoolVar(&flagBuild, "build", false, "request index rebuild")
	rootCmd.Flags().StringVar(&flagSchedule, "schedule", "", "schedule periodic index rebuild (e.g., '2h', '30m')")
	rootCmd.Flags().StringVar(&flagStrategy, "strategy", "", "index update strategy: 'replace', 'merge', or 'auto' (default: auto)")
	
	// Config flags
	rootCmd.Flags().StringVarP(&flagConfig, "config", "c", "", "config file path")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().StringVar(&flagVerboseType, "verbose-type", "text", "log format: text or json")
	
	// Connection flags
	rootCmd.Flags().StringVarP(&flagSocket, "socket", "s", "", "socket path or named pipe name (default: system default)")
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDaemon(cmd *cobra.Command, args []string) {
	initSlog(flagVerbose, flagVerboseType)

	// Load config
	cfg, err := config.Load(flagConfig)
	if err != nil {
		slog.Warn("failed to load config", "error", err)
	}
	
	// Handle service management
	if flagInstall || flagInstallServer {
		installService(cfg, flagUser)
		return
	}
	
	if flagUninstall {
		uninstallService(cfg)
		return
	}
	
	if flagStart {
		startService(cfg)
		return
	}
	
	if flagStop {
		stopService(cfg)
		return
	}
	
	if flagServiceStatus {
		showStatus(cfg)
		return
	}
	
	if flagGetConfig {
		getConfig(cfg)
		return
	}
	
	// Handle --set-config
	if cmd.Flags().Changed("set-config") {
		setConfig(cfg, flagSetConfig)
		return
	}
	
	// Handle --service mode (run as server)
	if flagService {
		runAsServer(cfg)
		return
	}
	
	// Handle build request (CLI mode)
	if flagBuild {
		requestBuild(cfg)
		return
	}
	
	// Default: CLI mode (run as client)
	// If there are arguments, treat them as search pattern
	if len(args) > 0 {
		runAsClient(args[0], cfg)
		return
	}
	
	// No arguments and no --service: show help
	cmd.Help()
}

func installService(cfg *config.Config, user bool) {
	fmt.Println("Installing golocated service...")
	
	err := svc.Install()
	if err != nil {
		slog.Error("failed to install service", "error", err)
		os.Exit(1)
	}
}

func uninstallService(cfg *config.Config) {
	fmt.Println("Uninstalling golocated service...")
	
	err := svc.Uninstall()
	if err != nil {
		slog.Error("failed to uninstall service", "error", err)
		os.Exit(1)
	}
}

func startService(cfg *config.Config) {
	fmt.Println("Starting golocated service...")
	
	err := svc.Start()
	if err != nil {
		slog.Error("failed to start service", "error", err)
		os.Exit(1)
	}
}

func stopService(cfg *config.Config) {
	fmt.Println("Stopping golocated service...")
	
	err := svc.Stop()
	if err != nil {
		slog.Error("failed to stop service", "error", err)
		os.Exit(1)
	}
}

func showStatus(cfg *config.Config) {
	// Determine config file path (for display)
	configPath := config.ConfigPath()
	if flagConfig != "" {
		configPath = flagConfig
	}
	
	// Check if service is installed and running
	status, err := svc.Status()
	if err != nil {
		// Service not installed, but try to connect to running server
		fmt.Println("Service status: not installed")
		fmt.Println()
		
		// Try to connect to running server
		serverStatus, err := cliclient.Status(flagSocket)
		if err == nil {
			// Server is running even though service is not installed
			fmt.Println("Server Status:")
			if running, ok := serverStatus["running"].(bool); ok {
				fmt.Printf("  Running: %v\n", running)
			}
			
			// Display config file path (prefer from server, fallback to local)
			if serverConfigPath, ok := serverStatus["config_path"].(string); ok && serverConfigPath != "" {
				fmt.Printf("  Config file: %s\n", serverConfigPath)
			} else {
				fmt.Printf("  Config file: %s\n", configPath)
			}
			
			// Display index status
			fmt.Println()
			fmt.Println("Index Status:")
			if isBuilding, ok := serverStatus["is_building"].(bool); ok {
				if isBuilding {
					fmt.Printf("  Building: Yes")
					if duration, ok := serverStatus["build_duration"].(string); ok {
						fmt.Printf(" (duration: %s)\n", duration)
					} else {
						fmt.Println()
					}
				} else {
					fmt.Println("  Building: No")
				}
			}
			if indexedFileCount, ok := serverStatus["indexed_file_count"].(int); ok {
				fmt.Printf("  Indexed files: %d\n", indexedFileCount)
			}
			if indexSize, ok := serverStatus["index_size"].(int); ok {
				fmt.Printf("  Index size: %d files\n", indexSize)
			}
			if lastBuildTime, ok := serverStatus["last_build_time"].(string); ok {
				fmt.Printf("  Last index time: %s\n", lastBuildTime)
				if lastBuildAgo, ok := serverStatus["last_build_ago"].(string); ok {
					fmt.Printf("  Last indexed: %s ago\n", lastBuildAgo)
				}
			}
		} else {
			// Server is not running
			fmt.Println("Server is not running")
			fmt.Printf("Config file: %s\n", configPath)
			fmt.Println()
			fmt.Println("To install golocated as a service:")
			fmt.Println("  golocated --install --user   # Install as user service")
			fmt.Println("  golocated --install          # Install as system service")
			fmt.Println()
			fmt.Println("Or run directly:")
			fmt.Println("  golocated --service")
		}
		return
	}
	
	fmt.Printf("Service status: %s\n", status)
	fmt.Println()
	
	// If service is running, try to get server status
	if status == "running" {
		// Try to connect to the server
		serverStatus, err := cliclient.Status(flagSocket)
		if err != nil {
			if errpkg.IsServerNotRunningError(err) {
				fmt.Println("⚠️  Service is marked as 'running' but server is not responding")
				fmt.Printf("Config file: %s\n", configPath)
				fmt.Println("Try restarting the service:")
				fmt.Println("  golocated --stop")
				fmt.Println("  golocated --start")
			} else {
				fmt.Printf("Server error: %v\n", err)
			}
			return
		}
		
		// Display server status
		fmt.Println("Server Information:")
		if running, ok := serverStatus["running"].(bool); ok {
			fmt.Printf("  Running: %v\n", running)
		}
		
		// Display config file path (prefer from server, fallback to local)
		if serverConfigPath, ok := serverStatus["config_path"].(string); ok && serverConfigPath != "" {
			fmt.Printf("  Config file: %s\n", serverConfigPath)
		} else {
			fmt.Printf("  Config file: %s\n", configPath)
		}
		
		// Display index status
		fmt.Println()
		fmt.Println("Index Status:")
		if isBuilding, ok := serverStatus["is_building"].(bool); ok {
			if isBuilding {
				fmt.Printf("  Building: Yes")
				if duration, ok := serverStatus["build_duration"].(string); ok {
					fmt.Printf(" (duration: %s)\n", duration)
				} else {
					fmt.Println()
				}
			} else {
				fmt.Println("  Building: No")
			}
		}
		if indexedFileCount, ok := serverStatus["indexed_file_count"].(int); ok {
			fmt.Printf("  Indexed files: %d\n", indexedFileCount)
		}
		if indexSize, ok := serverStatus["index_size"].(int); ok {
			fmt.Printf("  Index size: %d files\n", indexSize)
		}
		if lastBuildTime, ok := serverStatus["last_build_time"].(string); ok {
			fmt.Printf("  Last index time: %s\n", lastBuildTime)
			if lastBuildAgo, ok := serverStatus["last_build_ago"].(string); ok {
				fmt.Printf("  Last indexed: %s ago\n", lastBuildAgo)
			}
		}
	} else {
		// Service is not running
		fmt.Printf("Config file: %s\n", configPath)
		fmt.Println()
		fmt.Println("To start golocated:")
		fmt.Println("  golocated --start   # Start the service")
		fmt.Println("  golocated --service # Run directly (foreground)")
	}
}

func getConfig(cfg *config.Config) {
	// Try to connect to the server
	configResult, err := cliclient.GetConfig(flagSocket)
	if err != nil {
		if errpkg.IsServerNotRunningError(err) {
			fmt.Println("Server is not running")
			fmt.Println()
			fmt.Println("To view configuration, start the server first:")
			fmt.Println("  golocated --service")
		} else {
			errpkg.PrintFriendlyError(err)
		}
		return
	}
	
	// Display configuration
	fmt.Println("Server Configuration:")
	fmt.Println()
	
	// Socket path
	if socketPath, ok := configResult["socket_path"].(string); ok {
		fmt.Printf("  Socket path: %s\n", socketPath)
	}
	
	// Directories
	if dirs, ok := configResult["directories"].([]any); ok && len(dirs) > 0 {
		fmt.Println("  Watch directories:")
		for _, dir := range dirs {
			if dirStr, ok := dir.(string); ok {
				fmt.Printf("    - %s\n", dirStr)
			}
		}
	}
	
	// Database path
	if dbPath, ok := configResult["database_path"].(string); ok {
		fmt.Printf("  Database path: %s\n", dbPath)
	}
	
	// PID file
	if pidFile, ok := configResult["pid_file"].(string); ok {
		fmt.Printf("  PID file: %s\n", pidFile)
	}
	
	// Log file
	if logFile, ok := configResult["log_file"].(string); ok {
		fmt.Printf("  Log file: %s\n", logFile)
	}
	
	// Ignore patterns
	if patterns, ok := configResult["ignore_patterns"].([]any); ok && len(patterns) > 0 {
		fmt.Println("  Ignore patterns:")
		for _, pattern := range patterns {
			if patternStr, ok := pattern.(string); ok {
				fmt.Printf("    - %s\n", patternStr)
			}
		}
	}
	
	// Worker count
	if workerCount, ok := configResult["worker_count"].(int); ok {
		fmt.Printf("  Worker count: %d\n", workerCount)
	}
	
	// Follow symlinks
	if followSymlinks, ok := configResult["follow_symlinks"].(bool); ok {
		fmt.Printf("  Follow symlinks: %v\n", followSymlinks)
	}
	
	// Content search
	if contentSearch, ok := configResult["content_search"].(bool); ok {
		fmt.Printf("  Content search: %v\n", contentSearch)
	}
	
	// Max content file size
	if maxContentFileSize, ok := configResult["max_content_file_size"].(int64); ok {
		fmt.Printf("  Max content file size: %d bytes (%.2f MB)\n", maxContentFileSize, float64(maxContentFileSize)/(1024*1024))
	}
	
	// Index interval
	if indexInterval, ok := configResult["index_interval"].(string); ok {
		fmt.Printf("  Index interval: %s\n", indexInterval)
	}
	
	// Throttle index
	if throttleIndex, ok := configResult["throttle_index"].(bool); ok {
		fmt.Printf("  Throttle index: %v\n", throttleIndex)
	}
	
	// Index strategy
	if indexStrategy, ok := configResult["index_strategy"].(string); ok {
		fmt.Printf("  Index strategy: %s\n", indexStrategy)
	}
}

func requestBuild(cfg *config.Config) {
	slog.Debug("requesting index rebuild")

	// Use client to send build request to server
	if err := cliclient.Build(flagSocket); err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("failed to request index build")
		os.Exit(1)
	}

	slog.Debug("index build request sent successfully")
}

// runAsClient runs as CLI client (same as golocate)
func runAsClient(pattern string, cfg *config.Config) {
	slog.Debug("searching", "pattern", pattern)
	
	// Use client to connect to server (reuse golocate's client logic)
	opts := cliclient.SearchOptions{
		Pattern:    pattern,
		IgnoreCase: flagIgnoreCase,
		Basename:   flagBasename,
		Limit:      flagLimit,
		Regex:      flagRegex,
		Regexp:     flagRegexp,
		Count:      flagCount,
		Sort:       flagSort,
		SocketPath: flagSocket,
	}
	
	results, err := cliclient.Search(opts)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("search failed")
		os.Exit(1)
	}
	
	// Output results
	cliclient.PrintResults(results, flagCount)
}

// runAsServer runs as daemon server
func runAsServer(cfg *config.Config) {
	fmt.Println("Starting golocated daemon...")
	
	// Determine config file path
	configPath := config.ConfigPath()
	if flagConfig != "" {
		configPath = flagConfig
	}
	
	// Run the service
	err := svc.Run(cfg, configPath)
	if err != nil {
		slog.Error("failed to run daemon", "error", err)
		os.Exit(1)
	}
}

// setConfig handles the --set-config flag.
func setConfig(cfg *config.Config, arg string) {
	// Determine config file path
	configPath := config.ConfigPath()
	if flagConfig != "" {
		configPath = flagConfig
	}
	
	// Parse the argument
	if arg == "" {
		// Should not happen due to Cobra requiring argument, but handle it
		fmt.Fprintln(os.Stderr, "Error: --set-config requires an argument")
		os.Exit(1)
	}
	
	if arg == "interactive" || arg == "edit" {
		// Interactive mode
		setConfigInteractive(cfg, configPath)
		return
	}
	
	if arg == "-" {
		// Read from stdin
		setConfigFromStdin(cfg, configPath)
		return
	}
	
	// Check if it's key=value format
	if idx := findEqualSign(arg); idx >= 0 {
		key := arg[:idx]
		value := arg[idx+1:]
		setConfigKey(cfg, configPath, key, value)
		return
	}
	
	// Invalid format
	fmt.Fprintf(os.Stderr, "Error: invalid --set-config format: %s\n", arg)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  --set-config key=value        Set a configuration key")
	fmt.Fprintln(os.Stderr, "  --set-config -                Read YAML config from stdin")
	fmt.Fprintln(os.Stderr, "  --set-config interactive      Interactive mode")
	fmt.Fprintln(os.Stderr, "  --set-config edit             Interactive mode (alias)")
	os.Exit(1)
}

// findEqualSign finds the first '=' in a string, ignoring those inside quotes.
func findEqualSign(s string) int {
	inQuotes := false
	escape := false
	
	for i, ch := range s {
		switch {
		case escape:
			escape = false
		case ch == '\\':
			escape = true
		case ch == '"':
			inQuotes = !inQuotes
		case ch == '=' && !inQuotes:
			return i
		}
	}
	
	return -1
}

// setConfigKey sets a single configuration key.
func setConfigKey(cfg *config.Config, configPath, key, value string) {
	if flagVerbose {
		fmt.Printf("Setting %s = %s\n", key, value)
	}
	
	// Set the field
	if err := cfg.SetField(key, value); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid configuration: %v\n", err)
		os.Exit(1)
	}
	
	// Save the configuration
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save configuration: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Configuration saved to: %s\n", configPath)
	
	// Check if server is running and suggest reload
	suggestConfigReload()
}

// setConfigFromStdin reads YAML configuration from stdin.
func setConfigFromStdin(cfg *config.Config, configPath string) {
	if flagVerbose {
		fmt.Println("Reading configuration from stdin...")
	}

	// Read from stdin (cross-platform)
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read from stdin: %v\n", err)
		os.Exit(1)
	}
	
	// Parse YAML
	newCfg, err := config.LoadFromYAML(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse YAML: %v\n", err)
		os.Exit(1)
	}
	
	// Validate the new configuration
	if err := newCfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid configuration: %v\n", err)
		os.Exit(1)
	}
	
	// Save the configuration
	if err := newCfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save configuration: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Configuration saved to: %s\n", configPath)
	
	// Check if server is running and suggest reload
	suggestConfigReload()
}

// setConfigInteractive runs an interactive configuration editor.
func setConfigInteractive(cfg *config.Config, configPath string) {
	fmt.Println("Interactive Configuration Editor")
	fmt.Println("================================")
	fmt.Println()
	
	// Show current configuration
	fmt.Println("Current configuration:")
	showCurrentConfig(cfg)
	fmt.Println()
	
	// Prompt for key
	fmt.Print("Enter configuration key (or 'help' for available keys, 'quit' to exit): ")
	var key string
	if _, err := fmt.Scanln(&key); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read input: %v\n", err)
		os.Exit(1)
	}
	
	if key == "quit" || key == "exit" || key == "q" {
		fmt.Println("Cancelled.")
		return
	}
	
	if key == "help" {
		showConfigHelp()
		return
	}
	
	// Prompt for value
	fmt.Printf("Enter value for %s: ", key)
	var value string
	if _, err := fmt.Scanln(&value); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read input: %v\n", err)
		os.Exit(1)
	}
	
	// Set the field
	if err := cfg.SetField(key, value); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid configuration: %v\n", err)
		os.Exit(1)
	}
	
	// Save the configuration
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save configuration: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("\nConfiguration saved to: %s\n", configPath)
	
	// Check if server is running and suggest reload
	suggestConfigReload()
}

// showCurrentConfig displays the current configuration.
func showCurrentConfig(cfg *config.Config) {
	fmt.Printf("  directories: %v\n", cfg.Directories)
	fmt.Printf("  ignore_patterns: %v\n", cfg.IgnorePatterns)
	fmt.Printf("  database_path: %s\n", cfg.DatabasePath)
	fmt.Printf("  socket_path: %s\n", cfg.SocketPath)
	fmt.Printf("  pid_file: %s\n", cfg.PIDFile)
	fmt.Printf("  log_file: %s\n", cfg.LogFile)
	fmt.Printf("  follow_symlinks: %v\n", cfg.FollowSymlinks)
	fmt.Printf("  worker_count: %d\n", cfg.WorkerCount)
	fmt.Printf("  content_search: %v\n", cfg.ContentSearch)
	fmt.Printf("  max_content_file_size: %d\n", cfg.MaxContentFileSize)
	fmt.Printf("  index_interval: %s\n", cfg.IndexInterval)
	fmt.Printf("  throttle_index: %v\n", cfg.ThrottleIndex)
	fmt.Printf("  index_strategy: %s\n", cfg.IndexStrategy)
}

// showConfigHelp displays available configuration keys.
func showConfigHelp() {
	fmt.Println("\nAvailable configuration keys:")
	fmt.Println()
	fmt.Println("  directories          - List of directories to index (comma-separated)")
	fmt.Println("  ignore_patterns      - Glob patterns to ignore (comma-separated)")
	fmt.Println("  database_path        - Path to index database")
	fmt.Println("  socket_path          - Unix socket path for IPC")
	fmt.Println("  pid_file             - PID file path")
	fmt.Println("  log_file             - Log file path")
	fmt.Println("  follow_symlinks      - Follow symbolic links (true/false)")
	fmt.Println("  worker_count         - Number of concurrent workers (1-100)")
	fmt.Println("  content_search       - Enable content indexing (true/false)")
	fmt.Println("  max_content_file_size - Max file size for content indexing (bytes)")
	fmt.Println("  index_interval       - Periodic rebuild interval (e.g., '2h', '30m')")
	fmt.Println("  throttle_index       - Throttle periodic rebuilds (true/false)")
	fmt.Println("  index_strategy       - Index update strategy: 'replace', 'merge', or 'auto'")
}

// suggestConfigReload checks if the server is running and suggests reloading.
func suggestConfigReload() {
	// Try to connect to the server to check if it's running
	_, err := cliclient.Status(flagSocket)
	if err == nil {
		fmt.Println()
		fmt.Println("⚠️  The golocated server is currently running.")
		fmt.Println("The configuration has been saved, but the running server is still using the old configuration.")
		fmt.Println()
		fmt.Println("To apply the new configuration, restart the server:")
		fmt.Println("  golocated --stop")
		fmt.Println("  golocated --start")
		fmt.Println()
		fmt.Println("Or if running directly:")
		fmt.Println("  kill the running process and start again with: golocated --service")
	}
}

// initSlog configures the global slog logger.
// Default format is text; --verbose-type=json selects JSON output.
// -v lowers the level to Debug.
func initSlog(verbose bool, verboseType string) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if verboseType == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}
