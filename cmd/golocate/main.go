// Package main provides the CLI for golocate client.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	cliclient "github.com/RelicOfTesla/golocate/pkg/cli"
	errpkg "github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
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
	flagStop         bool
	flagVerbose      bool
	flagReloadConfig bool
	flagStatus       bool
	flagGetConfig    bool
	flagSetConfig    string
	flagSort         string
	flagSocket       string
	flagVerboseType  string
	flagNull         bool
	flagExisting     bool
	flagScope        string
	flagExclude      []string
	flagTerms        bool
	flagJSON         bool
	flagTypes        []string
	flagMinSize      string
	flagMaxSize      string
	flagMtimeAfter   string
	flagMtimeBefore  string
	flagNoHidden     bool
	flagDedupe       bool
	flagLong         bool
	flagOpen         bool
	flagOpenDir      bool
	flagCopy         bool
)

var (
	// version is overridable at build time:
	//   go build -ldflags "-X main.version=1.2.3" ./cmd/golocate/
	version = "dev"
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
  golocate --content keyword # Search file contents (grep-style output)
  golocate --content keyword pkg/ # Content search restricted to pkg/ paths
  golocate --terms "server -test" # Multi-term: paths containing "server" but not "test"
  golocate --scope pkg/ main.go   # Only search under pkg/
  golocate -0 main.go | xargs -0 ls -l  # NUL-separated output for xargs
  golocate --build         # Request index rebuild from server
  golocate --reload-config # Reload configuration file
  golocate --stop          # Stop the daemon server
`,
	Args: cobra.ArbitraryArgs,
	Run:  runSearch,
}

func init() {
	rootCmd.Version = version

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
	rootCmd.Flags().BoolVar(&flagContent, "content", false, "search file contents (slower, grep-style output)")
	rootCmd.Flags().BoolVar(&flagBuild, "build", false, "request index rebuild from server")
	rootCmd.Flags().BoolVar(&flagStop, "stop", false, "stop the daemon server")
	rootCmd.Flags().BoolVar(&flagReloadConfig, "reload-config", false, "reload configuration file")
	rootCmd.Flags().BoolVar(&flagStatus, "status", false, "show server status")
	rootCmd.Flags().BoolVar(&flagGetConfig, "get-config", false, "show server configuration")
	rootCmd.Flags().StringVar(&flagSetConfig, "set-config", "", "set configuration (key=value or YAML content)")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().StringVar(&flagVerboseType, "verbose-type", "text", "log format: text or json")
	rootCmd.Flags().StringVar(&flagSort, "sort", "", "sort by: name, size, time, path (append :desc for descending)")
	rootCmd.Flags().StringVarP(&flagSocket, "socket", "s", "", "socket path or named pipe name (default: system default)")

	// locate-compatible extras
	rootCmd.Flags().BoolVarP(&flagNull, "null", "0", false, "separate output with NUL bytes (xargs -0 friendly)")
	rootCmd.Flags().BoolVarP(&flagExisting, "existing", "e", false, "only show files that still exist on disk")

	// Query scoping
	rootCmd.Flags().StringVar(&flagScope, "scope", "", "only search under this directory")
	rootCmd.Flags().StringSliceVar(&flagExclude, "exclude", nil, "exclude paths matching glob (repeatable)")
	rootCmd.Flags().BoolVar(&flagTerms, "terms", false, "multi-term mode: space-separated terms are ANDed, '-term' excludes")
	rootCmd.Flags().BoolVar(&flagJSON, "json", false, "output structured JSON (path results or content matches)")

	// Metadata filters
	rootCmd.Flags().StringSliceVarP(&flagTypes, "type", "t", nil, "only files with these extensions, e.g. 'go,md' (repeatable)")
	rootCmd.Flags().StringVar(&flagMinSize, "min-size", "", "minimum file size, e.g. 1024, 1K, 2M")
	rootCmd.Flags().StringVar(&flagMaxSize, "max-size", "", "maximum file size, e.g. 1024, 1K, 2M")
	rootCmd.Flags().StringVar(&flagMtimeAfter, "mtime-after", "", "only files modified after this time (YYYY-MM-DD[ HH:MM])")
	rootCmd.Flags().StringVar(&flagMtimeBefore, "mtime-before", "", "only files modified before this time (YYYY-MM-DD[ HH:MM])")
	rootCmd.Flags().BoolVar(&flagNoHidden, "no-hidden", false, "exclude hidden files (dot-prefixed paths)")
	rootCmd.Flags().BoolVar(&flagDedupe, "dedupe", false, "collapse hard links to the same file into one result")
	rootCmd.Flags().BoolVar(&flagLong, "long", false, "long format: size<TAB>mtime<TAB>path (no shorthand: -l is --limit)")
	rootCmd.Flags().BoolVar(&flagOpen, "open", false, "open the first match with the default application")
	rootCmd.Flags().BoolVar(&flagOpenDir, "open-dir", false, "open the directory containing the first match")
	rootCmd.Flags().BoolVar(&flagCopy, "copy", false, "copy the first match path to the clipboard (xclip/xsel, pbcopy, or Set-Clipboard)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func runSearch(cmd *cobra.Command, args []string) {
	initSlog(flagVerbose, flagVerboseType)

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
			slog.Error("failed to reload config")
			os.Exit(1)
		}
		fmt.Println("Configuration reloaded successfully")
		return
	}

	// Handle index building (send request to server)
	if flagBuild {
		buildIndex()
		return
	}

	// Handle server stop
	if flagStop {
		stopServer()
		return
	}

	// Handle search. Without a pattern, still search when filter flags are
	// present (e.g. `golocate --type go` lists all .go files); otherwise help.
	pattern := ""
	if len(args) > 0 {
		pattern = args[0]
	}
	if pattern == "" && !hasSearchFilters() {
		cmd.Help()
		return
	}

	searchIndex(pattern, args)
}

// hasSearchFilters reports whether any search filter/output flag was set,
// allowing a pattern-less search (e.g. `golocate --type go` or `golocate --long`).
func hasSearchFilters() bool {
	return flagScope != "" || len(flagExclude) > 0 || len(flagTypes) > 0 ||
		flagMinSize != "" || flagMaxSize != "" ||
		flagMtimeAfter != "" || flagMtimeBefore != "" || flagNoHidden || flagLong
}

// buildIndex sends a build request to the server.
func buildIndex() {
	slog.Debug("requesting index rebuild")

	// Use client to send build request to server
	if err := cliclient.Build(flagSocket); err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("failed to request index build")
		os.Exit(1)
	}

	slog.Debug("index build request sent successfully")
}

// stopServer sends a stop request to the server.
func stopServer() {
	slog.Debug("requesting server stop")

	if err := cliclient.Stop(flagSocket); err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("failed to stop server")
		os.Exit(1)
	}

	fmt.Println("Stop request sent to server")
}

// searchIndex searches the file index via server.
func searchIndex(pattern string, args []string) {
	slog.Debug("searching",
		"pattern", pattern,
		"basename", flagBasename, "ignore_case", flagIgnoreCase,
		"limit", flagLimit, "regex", flagRegex || flagRegexp)

	// Use client to connect to server
	opts := cliclient.SearchOptions{
		Pattern:    pattern,
		Content:    "",
		IgnoreCase: flagIgnoreCase,
		// -w/--wholename (default true): search full paths; -b forces basename-only
		Basename:   flagBasename || !flagWholename,
		Limit:      flagLimit,
		Regex:      flagRegex,
		Regexp:     flagRegexp,
		Terms:      flagTerms,
		Count:      flagCount,
		Sort:       flagSort,
		Scope:      flagScope,
		Exclude:    flagExclude,
		Types:      flagTypes,
		NoHidden:   flagNoHidden,
		Dedupe:     flagDedupe,
		Existing:   flagExisting,
		Null:       flagNull,
		Long:       flagLong,
		SocketPath: flagSocket,
	}
	if flagContent {
		// --content: search file contents. The positional pattern is the
		// keyword; an optional second argument restricts candidate paths.
		opts.Content = pattern
		opts.Pattern = ""
		if len(args) > 1 {
			opts.Pattern = args[1]
		}
	}

	// Size filters (e.g. --min-size 1K)
	if flagMinSize != "" {
		n, err := cliclient.ParseSize(flagMinSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		opts.MinSize = n
	}
	if flagMaxSize != "" {
		n, err := cliclient.ParseSize(flagMaxSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		opts.MaxSize = n
	}

	// Time filters (e.g. --mtime-after 2024-01-01)
	if flagMtimeAfter != "" {
		n, err := cliclient.ParseMtime(flagMtimeAfter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		opts.MtimeAfter = n
	}
	if flagMtimeBefore != "" {
		n, err := cliclient.ParseMtime(flagMtimeBefore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		opts.MtimeBefore = n
	}

	// --json: structured machine-readable output.
	if flagJSON {
		if opts.Content != "" {
			jsonContent(opts)
		} else {
			jsonStream(opts)
		}
		return
	}

	// --count: fetch only the total (Limit=1) instead of pulling the whole
	// result set just to count it (docs/PERFORMANCE.md C1).
	if flagCount && opts.Content == "" && !flagOpen && !flagOpenDir && !flagCopy {
		o := opts
		o.Limit = 1
		res, err := cliclient.Search(o)
		if err != nil {
			errpkg.PrintFriendlyError(err)
			slog.Error("count failed")
			os.Exit(1)
		}
		total := res.Total
		if total <= 0 {
			total = res.Count
		}
		fmt.Println(total)
		if total == 0 {
			os.Exit(1)
		}
		return
	}

	// Large path searches (no --limit) stream page-by-page: print each page
	// as it arrives instead of buffering every result, so huge result sets
	// (e.g. `golocate a`) start producing output immediately with bounded
	// memory. Count/content/open modes keep the single-shot behaviour.
	if flagLimit == 0 && !flagCount && opts.Content == "" && !flagOpen && !flagOpenDir && !flagCopy {
		if streamSearch(opts) {
			return
		}
		os.Exit(1)
	}

	results, err := cliclient.Search(opts)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("search failed")
		os.Exit(1)
	}

	// --open / --open-dir / --copy: act on the first match instead of
	// printing results.
	if flagOpen || flagOpenDir || flagCopy {
		target := firstResultPath(results)
		if target == "" {
			fmt.Fprintln(os.Stderr, "Error: no match to open")
			os.Exit(1)
		}
		switch {
		case flagCopy:
			if err := cliclient.CopyPathToClipboard(target); err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to copy %s: %v\n", target, err)
				os.Exit(1)
			}
			fmt.Printf("Copied: %s\n", target)
		default:
			if flagOpenDir {
				target = filepath.Dir(target)
			}
			if err := cliclient.OpenInDefaultApp(target); err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to open %s: %v\n", target, err)
				os.Exit(1)
			}
			fmt.Printf("Opened: %s\n", target)
		}
		return
	}

	// Output results
	cliclient.PrintResults(results, flagCount, flagNull, flagLong)

	// locate-compatible exit codes: 0 = matches found, 1 = no matches
	if results.Count == 0 {
		os.Exit(1)
	}
}

// jsonStream prints path search results as a JSON array, streaming page-by-page
// (bounded memory, server-side sort preserved).
func jsonStream(opts cliclient.SearchOptions) {
	const batch = 5000
	if opts.Sort == "" {
		opts.Sort = "path:asc" // stable order so paging is seamless
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	fmt.Fprintln(w, "[")
	wrote := false
	total, hasTotal := 0, false

	for offset := 0; ; offset += batch {
		page := opts
		page.Limit = batch
		page.Offset = int64(offset)
		res, err := cliclient.Search(page)
		if err != nil {
			errpkg.PrintFriendlyError(err)
			slog.Error("json search failed", "offset", offset)
			fmt.Fprintln(w, "]")
			os.Exit(1)
		}
		if !hasTotal {
			total, hasTotal = res.Total, true
		}
		for _, e := range res.Entries {
			if wrote {
				fmt.Fprint(w, ",\n")
			}
			b, _ := json.Marshal(map[string]any{
				"name":     e.Name,
				"path":     e.Path,
				"size":     e.Size,
				"mod_time": fmtModTime(e.ModTime),
			})
			fmt.Fprintf(w, "  %s", b)
			wrote = true
		}
		if hasTotal && total > 0 && offset+batch >= total {
			break
		}
		if res.Count < batch && !hasTotal {
			break
		}
	}
	if wrote {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "]")
	if !wrote {
		os.Exit(1)
	}
}

// jsonContent prints content-search matches as a JSON array.
func jsonContent(opts cliclient.SearchOptions) {
	res, err := cliclient.Search(opts)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("content search failed")
		os.Exit(1)
	}
	arr := make([]map[string]any, 0, len(res.Matches))
	for _, m := range res.Matches {
		ctx := make([]string, 0, len(m.Before)+len(m.After))
		ctx = append(ctx, m.Before...)
		ctx = append(ctx, m.After...)
		arr = append(arr, map[string]any{
			"path":     m.Path,
			"line_num": m.LineNum,
			"line":     m.Line,
			"match":    m.Match,
			"context":  ctx,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(arr)
	if len(arr) == 0 {
		os.Exit(1)
	}
}

// fmtModTime formats a modtime for JSON output (RFC3339, empty if zero).
func fmtModTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// streamSearch fetches a path search page-by-page (streamBatch per page) and
// prints each page as soon as it arrives. Returns true when at least one row
// was printed (used for the locate-compatible exit code).
func streamSearch(opts cliclient.SearchOptions) bool {
	const streamBatch = 5000
	var found, total int
	hasTotal := false

	// Streaming paging needs a stable server-side order so consecutive
	// pages don't overlap or skip rows (the unsorted order is map-random).
	// Default to a deterministic path sort when the user didn't pick one.
	if pageSort := opts.Sort; pageSort == "" {
		opts.Sort = "path:asc"
		_ = pageSort
	}

	for offset := 0; ; offset += streamBatch {
		page := opts
		page.Limit = streamBatch
		page.Offset = int64(offset)

		res, err := cliclient.Search(page)
		if err != nil {
			errpkg.PrintFriendlyError(err)
			slog.Error("streamed search failed", "offset", offset)
			return false
		}
		if !hasTotal {
			total = res.Total
			hasTotal = true
		}
		found += res.Count
		cliclient.PrintResults(res, false, flagNull, flagLong)

		if hasTotal && total > 0 && offset+streamBatch >= total {
			break // consumed the whole result set
		}
		if !hasTotal || total <= 0 {
			if res.Count < streamBatch {
				break // server returned no more (total unavailable)
			}
		}
	}
	return found > 0
}

// firstResultPath returns the path of the first match (path search) or the
// first content match (content search). Empty when there are no results.
func firstResultPath(results *cliclient.SearchResult) string {
	if len(results.Entries) > 0 {
		return results.Entries[0].Path
	}
	if len(results.Matches) > 0 {
		return results.Matches[0].Path
	}
	return ""
}

// getStatus gets and displays the server status.
func getStatus() {
	slog.Debug("getting server status")

	status, err := cliclient.Status(flagSocket)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("failed to get server status")
		os.Exit(1)
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

	// Display protocol version (for client/server compatibility checks)
	if proto, ok := status["protocol_version"]; ok {
		v := statusInt(proto)
		fmt.Printf("  Protocol version: %d\n", v)
		if v != protocol.ProtocolVersion {
			fmt.Printf("  ⚠️  Version mismatch: client speaks protocol %d, server speaks %d — upgrade the older side.\n", protocol.ProtocolVersion, v)
		}
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
	if indexedFileCount, ok := status["indexed_file_count"]; ok {
		fmt.Printf("  Indexed files: %d\n", statusInt(indexedFileCount))
	}
	if buildFiles, ok := status["last_build_files"]; ok {
		fmt.Printf("  Last build scanned: %d files, %d dirs\n", statusInt(buildFiles), statusInt(status["last_build_dirs"]))
	}
	if perDir, ok := status["last_build_per_dir"].(map[string]any); ok && len(perDir) > 0 {
		fmt.Println("  Per directory:")
		for dir, c := range perDir {
			if counts, ok := c.(map[string]any); ok {
				fmt.Printf("    %s: %d files, %d dirs\n", dir, statusInt(counts["files"]), statusInt(counts["dirs"]))
			}
		}
	}
	if stats, ok := status["stats"].(map[string]any); ok {
		fmt.Printf("  Stats: %d searches, %d content searches, %d opens, %d builds\n",
			statusInt(stats["searches"]), statusInt(stats["content_searches"]),
			statusInt(stats["opens"]), statusInt(stats["builds"]))
	}
	if history, ok := status["build_history"].([]any); ok && len(history) > 0 {
		fmt.Println("  Build history (recent):")
		shown := 0
		for _, recAny := range history {
			rec, ok := recAny.(map[string]any)
			if !ok || shown >= 3 {
				continue
			}
			fmt.Printf("    %s: %d files, %d dirs, %s\n",
				rec["time"], statusInt(rec["files"]), statusInt(rec["dirs"]), rec["elapsed"])
			shown++
		}
	}
	if indexSize, ok := status["index_size"]; ok {
		fmt.Printf("  Index size: %d files\n", statusInt(indexSize))
	}
	if lastBuildTime, ok := status["last_build_time"].(string); ok {
		fmt.Printf("  Last index time: %s\n", lastBuildTime)
		if lastBuildAgo, ok := status["last_build_ago"].(string); ok {
			fmt.Printf("  Last indexed: %s ago\n", lastBuildAgo)
		}
	}
}

// statusInt converts a JSON-decoded number (float64/int/string) to int.
func statusInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		var out int
		fmt.Sscanf(n, "%d", &out)
		return out
	}
	return 0
}

// getConfig gets and displays the server configuration.
func getConfig() {
	slog.Debug("getting server configuration")

	config, err := cliclient.GetConfig(flagSocket)
	if err != nil {
		errpkg.PrintFriendlyError(err)
		slog.Error("failed to get server configuration")
		os.Exit(1)
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

	// Content index (optional in-memory token index)
	if ci, ok := config["content_index"].(bool); ok {
		fmt.Printf("  Content index: %v\n", ci)
	}
}

// setConfig sets the server configuration.
func setConfig(arg string) {
	if arg == "" {
		fmt.Fprintln(os.Stderr, "Error: --set-config requires an argument")
		os.Exit(1)
	}

	slog.Debug("setting server configuration")

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
		slog.Error("failed to set server configuration")
		os.Exit(1)
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

// initSlog configures the global slog logger.
// Default format is text; --verbose-type=json selects JSON output.
// -v lowers the level to Debug so client request/response logs appear.
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
