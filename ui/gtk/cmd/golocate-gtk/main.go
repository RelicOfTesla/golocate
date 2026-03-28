// Package main provides a GTK4-based UI for golocate.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/RelicOfTesla/golocate/internal/client"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	app := gtk.NewApplication("com.github.golocate", 0)
	
	app.Connect("activate", func() {
		// Create main window
		win := gtk.NewApplicationWindow(app)
		win.SetTitle("golocate - Fast File Search")
		win.SetDefaultSize(800, 600)
		
		// Create main container
		box := gtk.NewBox(gtk.OrientationVertical, 10)
		box.SetMarginTop(10)
		box.SetMarginBottom(10)
		box.SetMarginStart(10)
		box.SetMarginEnd(10)
		
		// Create search box
		searchBox := gtk.NewBox(gtk.OrientationHorizontal, 10)
		
		// Create search entry
		entry := gtk.NewEntry()
		entry.SetPlaceholderText("Search files...")
		searchBox.Append(entry)
		
		// Create search button
		searchBtn := gtk.NewButtonWithLabel("Search")
		searchBox.Append(searchBtn)
		
		box.Append(searchBox)
		
		// Create scrolled window for results
		scrolled := gtk.NewScrolledWindow()
		scrolled.SetVExpand(true)
		
		// Create results view
		resultsView := gtk.NewTextView()
		resultsView.SetEditable(false)
		resultsView.SetMonospace(true)
		buffer := resultsView.Buffer()
		buffer.SetText("Search results will appear here...")
		scrolled.SetChild(resultsView)
		
		box.Append(scrolled)
		
		// Create client
		c := client.New()
		
		// Search function
		doSearch := func() {
			query := entry.Text()
			if query == "" {
				buffer.SetText("Please enter a search query")
				return
			}
			
			// Search
			results, err := c.Search(query, index.SearchOptions{
				Limit: 100,
			})
			if err != nil {
				buffer.SetText(fmt.Sprintf("Error: %v", err))
				return
			}
			
			// Display results
			if len(results) == 0 {
				buffer.SetText("No results found")
				return
			}
			
			text := ""
			for _, r := range results {
				text += r.Path + "\n"
			}
			buffer.SetText(fmt.Sprintf("Found %d results:\n\n%s", len(results), text))
		}
		
		// Connect search button
		searchBtn.Connect("clicked", doSearch)
		
		// Connect entry activate (Enter key)
		entry.Connect("activate", doSearch)
		
		// Set window child
		win.SetChild(box)
		
		// Show window
		win.Present()
	})
	
	// Run application
	status := app.Run(os.Args)
	if status > 0 {
		log.Fatal("Application exited with status:", status)
	}
}
