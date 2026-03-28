// Package handler provides HTTP handlers for golocate-h5.
package handler

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"

	"github.com/RelicOfTesla/golocate/ui/h5/internal/api"
)

//go:embed static/index.html
var staticFS embed.FS

// Handler represents the HTTP handler.
type Handler struct {
	client *api.Client
	tmpl   *template.Template
}

// New creates a new handler.
func New(client *api.Client) *Handler {
	// Parse template from embedded file
	tmpl := template.Must(template.ParseFS(staticFS, "static/index.html"))
	
	return &Handler{
		client: client,
		tmpl:   tmpl,
	}
}

// Index handles the index page.
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := map[string]interface{}{
		"Title": "golocate - Fast File Search",
	}

	if err := h.tmpl.Execute(w, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Search handles search API requests.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		json.NewEncoder(w).Encode(&api.SearchResponse{
			Error: "missing query parameter 'q'",
		})
		return
	}

	// Parse command-line style parameters from query
	params := ParseSearchQuery(query)

	// Call API client with parsed parameters
	resp, err := h.client.Search(params.Content, params.Path, params.IgnoreCase, params.Limit)
	if err != nil {
		log.Printf("Search error: %v", err)
		json.NewEncoder(w).Encode(&api.SearchResponse{
			Error: err.Error(),
		})
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Status handles status API requests.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.Status()
	if err != nil {
		log.Printf("Status error: %v", err)
		json.NewEncoder(w).Encode(&api.StatusResponse{
			Error: err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Build handles build API requests.
func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement build trigger
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "build triggered",
	})
}
