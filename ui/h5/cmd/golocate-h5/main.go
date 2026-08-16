package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/RelicOfTesla/golocate/internal/socket"
	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/ui/h5/internal/api"
	"github.com/RelicOfTesla/golocate/ui/h5/internal/handler"
)

var (
	flagAddr    string
	flagVerbose bool
	flagSocket  string
)

func main() {
	flag.StringVar(&flagAddr, "addr", ":8080", "server address")
	flag.BoolVar(&flagVerbose, "verbose", false, "verbose output")
	flag.StringVar(&flagSocket, "socket", "", "socket path or named pipe name (default: system default)")
	flag.Parse()

	// Create API client
	client := api.NewClient()
	if flagSocket != "" {
		client.SetSocketPath(flagSocket)
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
	mux.HandleFunc("/api/build", h.Build)
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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Start server
	slog.Info("starting golocate-h5", "addr", flagAddr)
	slog.Info("open browser", "url", "http://localhost"+flagAddr)
	
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
