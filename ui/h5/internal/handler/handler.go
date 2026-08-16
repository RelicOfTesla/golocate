// Package handler provides HTTP handlers for golocate-h5.
package handler

import (
	"embed"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

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

	data := map[string]any{
		"Title": "golocate - Fast File Search",
	}

	if err := h.tmpl.Execute(w, data); err != nil {
		slog.Error("template error", "error", err)
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
	
	// Parse pagination parameters
	var offset int64 = 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.ParseInt(offsetStr, 10, 64); err == nil {
			offset = o
		}
	}
	
	// Parse limit from query parameter (override default)
	limit := params.Limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Parse ignore_case and regex from URL query parameters (frontend sends these)
	ignoreCase := params.IgnoreCase
	if ic := r.URL.Query().Get("ignore_case"); ic != "" {
		ignoreCase = ic == "true"
	}
	regexMode := params.Regex
	if rg := r.URL.Query().Get("regex"); rg != "" {
		regexMode = rg == "true"
	}

	// Call API client with parsed parameters
	resp, err := h.client.Search(params.Content, ignoreCase, regexMode, limit, offset)
	if err != nil {
		slog.Error("search error", "error", err)
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
		slog.Error("status error", "error", err)
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

	// Trigger index build
	if err := h.client.Build(); err != nil {
		slog.Error("build error", "error", err)
		json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "build started",
	})
}

// GetConfig handles GET /api/config requests.
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.client.GetConfig()
	if err != nil {
		slog.Error("get config error", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// SetConfig handles POST /api/config requests.
func (h *Handler) SetConfig(w http.ResponseWriter, r *http.Request) {
	// Read YAML content from request body
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Error("set config decode error", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set config via client
	if err := h.client.SetConfig(body.YAML); err != nil {
		slog.Error("set config error", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "saved",
	})
}
