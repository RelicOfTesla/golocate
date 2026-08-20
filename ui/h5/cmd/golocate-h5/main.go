package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/autostart"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/ui/h5/internal/api"
	"github.com/RelicOfTesla/golocate/ui/h5/internal/handler"
)

var (
	flagAddr    string
	flagVerbose bool
	flagSocket  string
	flagVersion     bool
	flagAutoStart   string
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	flag.StringVar(&flagAddr, "addr", "127.0.0.1:8080", "server address (default binds to loopback for security)")
	flag.BoolVar(&flagVerbose, "verbose", false, "verbose output")
	flag.StringVar(&flagSocket, "socket", "", "socket path or named pipe name (default: system default)")
	flag.BoolVar(&flagVersion, "version", false, "print version and exit")
	flag.StringVar(&flagAutoStart, "auto-start-server", "child", "auto-start golocated when unreachable: none, child (default), background")
	flag.Parse()

	if flagVersion {
		fmt.Println("golocate-h5", version)
		return
	}

	// Create API client
	client := api.NewClient()
	if flagSocket != "" {
		client.SetSocketPath(flagSocket)
	}

	// Auto-start golocated if needed (single shared daemon; not killed here).
	if mode, err := autostart.ParseMode(flagAutoStart); err != nil {
		slog.Warn("invalid --auto-start-server", "error", err)
	} else if mode != autostart.None {
		sock := flagSocket
		if sock == "" {
			sock = config.GetDefaultSocketPath()
		}
		if _, err := (&autostart.Launcher{SocketPath: sock, Mode: mode}).Ensure(); err != nil {
			slog.Warn("auto-start golocated failed", "error", err)
		}
	}

	// Check if golocated is running
	if !isGolocatedRunning() {
		slog.Warn("golocated is not running. Start it with: golocated --service")
	}

	// Create handlers
	h := handler.New(client)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.Index)
	mux.HandleFunc("/api/search", h.Search)
	mux.HandleFunc("/api/status", h.Status)
	mux.HandleFunc("/healthz", h.Healthz)
	mux.HandleFunc("/metrics", h.Metrics)
	mux.HandleFunc("/api/build", h.Build)
	mux.HandleFunc("/api/open", h.Open)
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		// Route based on HTTP method
		switch r.Method {
		case http.MethodGet:
			h.GetConfig(w, r)
		case http.MethodPost:
			h.SetConfig(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Always revalidate embedded assets so UI fixes reach the browser.
	w.Header().Set("Cache-Control", "no-cache")
	http.FileServer(http.FS(handler.Static)).ServeHTTP(w, r)
})))

	// Start server
	slog.Info("starting golocate-h5", "addr", flagAddr)
	slog.Info("open browser", "url", "http://"+flagAddr)

	if err := http.ListenAndServe(flagAddr, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func isGolocatedRunning() bool {
	// Determine socket path
	socketPath := flagSocket
	if socketPath == "" {
		socketPath = config.GetDefaultSocketPath()
	}

	// 使用平台抽象的 socket 检测（Windows 上 named pipe 不是文件路径）
	return socket.IsRunning(socketPath)
}
