// Package handler provides HTTP handlers for golocate-h5.
package handler

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RelicOfTesla/golocate/ui/h5/internal/api"
)

//go:embed static
var staticFS embed.FS

// Static serves embedded static assets (style.css, i18n.js, core.js,
// favorites.js, settings.js) under /static/.
var Static = func() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}()

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
	// Optional, AND-combined name/path + content search: at least one must be
	// present, but a pure content search (no name/path term) is allowed.
	if query == "" && r.URL.Query().Get("content") == "" {
		json.NewEncoder(w).Encode(&api.SearchResponse{
			Error: "missing query parameter 'q' or 'content'",
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

	// Basename toggle: search only the file name portion of paths.
	basename := params.Basename
	if b := r.URL.Query().Get("basename"); b != "" {
		basename = b == "true"
	}

	// Pattern mode: "", "regex", or "wildcard" (frontend select).
	patternMode := r.URL.Query().Get("pattern_mode")

	// Server-side sorting (cross-page): sort_field + sort_order.
	sortField := r.URL.Query().Get("sort_field")
	sortOrder := r.URL.Query().Get("sort_order")

	// Content keyword: explicit ?content= param (frontend), or --content:xxx in the query
	content := r.URL.Query().Get("content")
	if content == "" {
		content = params.Content
	}

	// Dedupe toggle: collapse hard links to one result.
	dedupe := r.URL.Query().Get("dedupe") == "true"

	// Advanced filters: scope, exclude globs, file types, size range.
	scope := r.URL.Query().Get("scope")
	exclude := splitCSV(r.URL.Query().Get("exclude"))
	types := splitCSV(r.URL.Query().Get("type"))
	var minSize, maxSize int64
	if v := r.URL.Query().Get("min_size"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			minSize = n
		}
	}
	if v := r.URL.Query().Get("max_size"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxSize = n
		}
	}

	// mtime ranges (YYYY-MM-DD[ HH:MM]) -> unix seconds
	var mtimeAfter, mtimeBefore int64
	if v := r.URL.Query().Get("mtime_after"); v != "" {
		mtimeAfter = parseMtimeQuery(v)
	}
	if v := r.URL.Query().Get("mtime_before"); v != "" {
		mtimeBefore = parseMtimeQuery(v)
	}

	// Call API client with parsed parameters
	resp, err := h.client.Search(api.SearchParams{
		Pattern:     params.Pattern,
		Content:     content,
		IgnoreCase:  ignoreCase,
		Regex:       regexMode,
		Basename:    basename,
		Dedupe:      dedupe,
		PatternMode: patternMode,
		Limit:       limit,
		Offset:      offset,
		SortField:   sortField,
		SortOrder:   sortOrder,
		Scope:       scope,
		Exclude:     exclude,
		Types:       types,
		MinSize:     minSize,
		MaxSize:     maxSize,
		MtimeAfter:  mtimeAfter,
		MtimeBefore: mtimeBefore,
	})
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

// Healthz handles GET /healthz: a liveness probe for process managers and
// container health checks. Returns 200 when the daemon is reachable, 503
// otherwise (the bridge itself is always up).
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.Status()
	if err != nil || !resp.Running {
		http.Error(w, "golocated daemon not reachable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"pid":    resp.Pid,
	})
}

// Metrics handles GET /metrics: lightweight Prometheus text-format metrics
// (request counters + index size) scraped by monitoring systems.
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.Status()
	if err != nil || !resp.Running {
		http.Error(w, "golocated daemon not reachable", http.StatusServiceUnavailable)
		return
	}
	s := resp.Stats
	if s == nil {
		s = map[string]int{}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP golocate_searches_total Total search requests served.\n")
	fmt.Fprintf(w, "# TYPE golocate_searches_total counter\n")
	fmt.Fprintf(w, "golocate_searches_total %d\n", s["searches"])
	fmt.Fprintf(w, "# TYPE golocate_content_searches_total counter\n")
	fmt.Fprintf(w, "golocate_content_searches_total %d\n", s["content_searches"])
	fmt.Fprintf(w, "# TYPE golocate_opens_total counter\n")
	fmt.Fprintf(w, "golocate_opens_total %d\n", s["opens"])
	fmt.Fprintf(w, "# TYPE golocate_builds_total counter\n")
	fmt.Fprintf(w, "golocate_builds_total %d\n", s["builds"])
	fmt.Fprintf(w, "# TYPE golocate_indexed_files gauge\n")
	fmt.Fprintf(w, "golocate_indexed_files %d\n", resp.IndexSize)
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
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "build started",
	})
}

// Open handles POST /api/open: ask the daemon to open a path with the
// platform default application.
func (h *Handler) Open(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "missing 'path' in request body", http.StatusBadRequest)
		return
	}
	if err := h.client.Open(body.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "opened", "path": body.Path})
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

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
// parseMtimeQuery parses "YYYY-MM-DD[ HH:MM]" into unix seconds (0 on error/empty).
func parseMtimeQuery(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
