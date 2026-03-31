// Package main provides the CLI for golocate client.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	cliclient "github.com/RelicOfTesla/golocate/pkg/cli"
	errpkg "github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	// CLI flags
	flagBasename     bool
	flagCount        bool
	flagIgnoreCase   bool
	flagLimit        int
	flagLiteral      bool
	flagRegexp       bool
	flagRegex        bool
	flagWholename    bool
	flagContent      bool
	flagBuild        bool
	flagVerbose      bool
	flagReloadConfig bool
	flagStatus       bool
	flagGetConfig    bool
	flagSetConfig    string
	flagSort         string
	flagSocket       string
)

var rootCmd = &cobra.Command{
	Use:   "golocate [pattern]",
	Short: "A fast file search utility (client)",
	Long: `golocate is a fast file search utility (client component).

This is the client component that connects to the golocated server.
Use 'golocated' for server and service management.

Examples:
  golocate myfile          # Search for files containing "myfile"
  golocate -b myfile       # Search only in file names (basename)
  golocate -i MyFile       # Case-insensitive search
  golocate -c myfile       # Print count of matches
  golocate -l 10 myfile    # Limit to 10 results
  golocate --regex "\.go$" # Use regex (files ending with .go)
  golocate --build         # Request index rebuild from server
  golocate --reload-config # Reload configuration file
`,
	Args: cobra.ArbitraryArgs,
	Run:  runSearch,
}

func init() {
	// locate-compatible flags
	rootCmd.Flags().BoolVarP(&flagBasename, "basename", "b", false, "search only the file name portion of path names")
	rootCmd.Flags().BoolVarP(&flagCount, "count", "c", false, "print number of matches instead of the matches")
	rootCmd.Flags().BoolVarP(&flagIgnoreCase, "ignore-case", "i", false, "search case-insensitively")
	rootCmd.Flags().IntVarP(&flagLimit, "limit", "l", 0, "stop after LIMIT matches")
	rootCmd.Flags().BoolVarP(&flagLiteral, "literal", "N", false, "do not quote filenames, even if printing to a tty")
	rootCmd.Flags().BoolVarP(&flagRegexp, "regexp", "r", false, "interpret patterns as basic regexps (slow)")
	rootCmd.Flags().BoolVar(&flagRegex, "regex", false, "interpret patterns as extended regexps (slow)")
	rootCmd.Flags().BoolVarP(&flagWholename, "wholename", "w", true, "search the entire path name (default)")
	
	// Additional flags
	rootCmd.Flags().BoolVar(&flagContent, "content", false, "search file contents (slower)")
	rootCmd.Flags().BoolVar(&flagBuild, "build", false, "request index rebuild from server")
	rootCmd.Flags().BoolVar(&flagReloadConfig, "reload-config", false, "reload configuration file")
	rootCmd.Flags().BoolVar(&flagStatus, "status", false, "show server status")
	rootCmd.Flags().BoolVar(&flagGetConfig, "get-config", false, "show server configuration")
	rootCmd.Flags().StringVar(&flagSetConfig, "set-config", "", "set configuration (key=value or YAML content)")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().StringVar(&flagSort, "sort", "", "sort by: name, size, time, path (append :desc for descending)")
	rootCmd.Flags().StringVarP(&flagSocket, "socket", "s", "", "socket path or named pipe name (default: system default)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runSearch(cmd *cobra.Command, args []string) {
	// Handle status
	if flagStatus {
		getStatus()
		return
	}
	
	// Handle get-config
	if flagGetConfig {
		getConfig()
		return
	}
	
	// Handle set-config
	if cmd.Flags().Changed("set-config") {
		setConfig(flagSetConfig)
		return
	}
	
	// Handle reload-config
	if flagReloadConfig {
		if err := cliclient.ReloadConfig(flagSocket); err != nil {
			errpkg.PrintFriendlyError(err)
			log.Fatalf("Failed to reload config")
		}
		fmt.Println("Configuration reloaded successfully")
		return
	}
	
	// Handle index building (send request to server)
	if flagBuild {
		buildIndex()
		return
	}
	
	// Handle search
	pattern := ""
	if len(args) > 0 {
		pattern = args[0]
	}
	
	searchIndex(pattern)
}

// buildIndex sends a build request to the server.
func buildIndex() {
	if flagVerbose {
		log.Printf("requesting index rebuild...")
	}
	
	// Use client to send build request to server
	if err := cliclient.Build(flagSocket); err != nil {
		errpkg.PrintFriendlyError(err)
		log.Fatalf("failed to request index build")
	}
	
	if flagVerbose {
		log.Printf("index build request sent successfully")
	}
}

// searchIndex searches the file index via server.
func searchIndex(pattern string) {
	if flagVerbose {
		log.Printf("searching for: %s", pattern)
		log.Printf("options: basename=%v, ignore-case=%v, limit=%d, regex=%v", 
			flagBasename, flagIgnoreCase, flagLimit, flagRegex||flagRegexp)
	}
	
	// Use client to connect to server
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
		log.Fatalf("search failed")
	}
	
	// Output results
	cliclient.PrintResults(results, flagCount)
}

// getStatus gets and displays the server status.
func getStatus() {
	if flagVerbose {
		log.Printf("getting server status...")
	}
	
	status, err := cliclient.Status(flagSocket)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		log.Fatalf("failed to get server status")
	}
	
	// Display server status
	fmt.Println("Server Status:")
	if running, ok := status["running"].(bool); ok {
		fmt.Printf("  Running: %v\n", running)
	}
	
	// Display config file path
	if configPath, ok := status["config_path"].(string); ok {
		fmt.Printf("  Config file: %s\n", configPath)
	}
	
	// Display index status
	fmt.Println()
	fmt.Println("Index Status:")
	if isBuilding, ok := status["is_building"].(bool); ok {
		if isBuilding {
			fmt.Printf("  Building: Yes")
			if duration, ok := status["build_duration"].(string); ok {
				fmt.Printf(" (duration: %s)\n", duration)
			} else {
				fmt.Println()
			}
		} else {
			fmt.Println("  Building: No")
		}
	}
	if indexedFileCount, ok := status["indexed_file_count"].(int); ok {
		fmt.Printf("  Indexed files: %d\n", indexedFileCount)
	}
	if indexSize, ok := status["index_size"].(int); ok {
		fmt.Printf("  Index size: %d files\n", indexSize)
	}
	if lastBuildTime, ok := status["last_build_time"].(string); ok {
		fmt.Printf("  Last index time: %s\n", lastBuildTime)
		if lastBuildAgo, ok := status["last_build_ago"].(string); ok {
			fmt.Printf("  Last indexed: %s ago\n", lastBuildAgo)
		}
	}
}

// getConfig gets and displays the server configuration.
func getConfig() {
	if flagVerbose {
		log.Printf("getting server configuration...")
	}
	
	config, err := cliclient.GetConfig(flagSocket)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		log.Fatalf("failed to get server configuration")
	}
	
	// Display configuration
	fmt.Println("Server Configuration:")
	fmt.Println()
	
	// Socket path
	if socketPath, ok := config["socket_path"].(string); ok {
		fmt.Printf("  Socket path: %s\n", socketPath)
	}
	
	// Directories
	if dirs, ok := config["directories"].([]any); ok && len(dirs) > 0 {
		fmt.Println("  Watch directories:")
		for _, dir := range dirs {
			if dirStr, ok := dir.(string); ok {
				fmt.Printf("    - %s\n", dirStr)
			}
		}
	}
	
	// Database path
	if dbPath, ok := config["database_path"].(string); ok {
		fmt.Printf("  Database path: %s\n", dbPath)
	}
	
	// PID file
	if pidFile, ok := config["pid_file"].(string); ok {
		fmt.Printf("  PID file: %s\n", pidFile)
	}
	
	// Log file
	if logFile, ok := config["log_file"].(string); ok {
		fmt.Printf("  Log file: %s\n", logFile)
	}
	
	// Ignore patterns
	if patterns, ok := config["ignore_patterns"].([]any); ok && len(patterns) > 0 {
		fmt.Println("  Ignore patterns:")
		for _, pattern := range patterns {
			if patternStr, ok := pattern.(string); ok {
				fmt.Printf("    - %s\n", patternStr)
			}
		}
	}
	
	// Worker count
	if workerCount, ok := config["worker_count"].(int); ok {
		fmt.Printf("  Worker count: %d\n", workerCount)
	}
	
	// Follow symlinks
	if followSymlinks, ok := config["follow_symlinks"].(bool); ok {
		fmt.Printf("  Follow symlinks: %v\n", followSymlinks)
	}
	
	// Content search
	if contentSearch, ok := config["content_search"].(bool); ok {
		fmt.Printf("  Content search: %v\n", contentSearch)
	}
	
	// Max content file size
	if maxContentFileSize, ok := config["max_content_file_size"].(int64); ok {
		fmt.Printf("  Max content file size: %d bytes (%.2f MB)\n", maxContentFileSize, float64(maxContentFileSize)/(1024*1024))
	}
	
	// Index interval
	if indexInterval, ok := config["index_interval"].(string); ok {
		fmt.Printf("  Index interval: %s\n", indexInterval)
	}
	
	// Throttle index
	if throttleIndex, ok := config["throttle_index"].(bool); ok {
		fmt.Printf("  Throttle index: %v\n", throttleIndex)
	}
	
	// Index strategy
	if indexStrategy, ok := config["index_strategy"].(string); ok {
		fmt.Printf("  Index strategy: %s\n", indexStrategy)
	}
}

// setConfig sets the server configuration.
func setConfig(arg string) {
	if arg == "" {
		fmt.Fprintln(os.Stderr, "Error: --set-config requires an argument")
		os.Exit(1)
	}
	
	if flagVerbose {
		log.Printf("setting server configuration...")
	}
	
	// Check if it's YAML content or key=value format
	var yamlContent string
	if arg == "-" {
		// Read from stdin (cross-platform)
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to read from stdin: %v\n", err)
			os.Exit(1)
		}
		yamlContent = string(data)
	} else if idx := findEqualSign(arg); idx >= 0 {
		// key=value format, convert to simple YAML
		key := arg[:idx]
		value := arg[idx+1:]
		yamlContent = fmt.Sprintf("%s: %s\n", key, value)
	} else {
		// Assume it's already YAML content
		yamlContent = arg
	}
	
	// Send to server
	if err := cliclient.SetConfig(flagSocket, yamlContent); err != nil {
		errpkg.PrintFriendlyError(err)
		log.Fatalf("failed to set server configuration")
	}
	
	fmt.Println("Configuration updated successfully")
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
