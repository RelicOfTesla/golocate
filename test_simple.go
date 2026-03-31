// +build ignore

package main

import (
	"fmt"
	"time"

	"github.com/RelicOfTesla/golocate/internal/client"
	"github.com/RelicOfTesla/golocate/internal/server"
	"github.com/RelicOfTesla/golocate/internal/testutil"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

func main() {
	// Create and start server
	idx := index.NewIndex()
	srv := server.New(idx)
	socketPath := testutil.GetTestSocketPath("simple_test")
	srv.SetSocketPath(socketPath)

	if err := srv.Start(); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		return
	}
	defer srv.Stop()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create client with longer timeout
	c := client.New()
	c.SetSocketPath(socketPath)
	c.SetTimeout(60 * time.Second) // Set longer timeout

	// Perform search
	fmt.Println("Starting search...")
	start := time.Now()
	results, err := c.Search("test", index.SearchOptions{IgnoreCase: true})
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("Search failed after %v: %v\n", elapsed, err)
	} else {
		fmt.Printf("Search succeeded after %v, found %d results\n", elapsed, len(results))
	}
}
