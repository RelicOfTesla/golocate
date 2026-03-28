package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/RelicOfTesla/golocate/ui/h5/internal/api"
	"github.com/RelicOfTesla/golocate/ui/h5/internal/handler"
)

var (
	flagAddr    string
	flagVerbose bool
)

func main() {
	flag.StringVar(&flagAddr, "addr", ":8080", "server address")
	flag.BoolVar(&flagVerbose, "verbose", false, "verbose output")
	flag.Parse()

	// Check if golocated is running
	if !isGolocatedRunning() {
		log.Println("Warning: golocated is not running. Start it with: golocated --service")
	}

	// Create API client
	client := api.NewClient()

	// Create handlers
	h := handler.New(client)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.Index)
	mux.HandleFunc("/api/search", h.Search)
	mux.HandleFunc("/api/status", h.Status)
	mux.HandleFunc("/api/build", h.Build)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Start server
	log.Printf("Starting golocate-h5 on %s", flagAddr)
	log.Printf("Open http://localhost%s in your browser", flagAddr)
	
	if err := http.ListenAndServe(flagAddr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func isGolocatedRunning() bool {
	// Check if Unix socket exists
	socketPath := "/tmp/golocate.sock"
	if _, err := os.Stat(socketPath); err == nil {
		return true
	}
	return false
}
