package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"

	"github.com/alphabot-ai/slashclaw/internal/config"
	"github.com/alphabot-ai/slashclaw/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

// Handler holds dependencies for web handlers
type Handler struct {
	store     store.Store
	cfg       *config.Config
	templates *template.Template
}

// NewHandler creates a new web handler
func NewHandler(s store.Store, cfg *config.Config) (*Handler, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		store:     s,
		cfg:       cfg,
		templates: tmpl,
	}, nil
}

// HomeData is the data for the home page template
type HomeData struct {
	Stories []*store.Story
	Sort    string
	BaseURL string
}

// StoryData is the data for the story page template
type StoryData struct {
	Story    *store.Story
	Comments []*store.Comment
	BaseURL  string
}

// SubmitData is the data for the submit page template
type SubmitData struct {
	BaseURL string
	Error   string
}

// Home handles GET /
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	query := r.URL.Query()
	sortStr := query.Get("sort")
	if sortStr == "" {
		sortStr = "top"
	}

	var sort store.SortOrder
	switch sortStr {
	case "new":
		sort = store.SortNew
	case "discussed":
		sort = store.SortDiscussed
	default:
		sort = store.SortTop
		sortStr = "top"
	}

	opts := store.ListOptions{
		Sort:  sort,
		Limit: 30,
	}

	stories, _, err := h.store.ListStories(r.Context(), opts)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Content negotiation
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"stories": stories,
			"sort":    sortStr,
		})
		return
	}

	data := HomeData{
		Stories: stories,
		Sort:    sortStr,
		BaseURL: h.cfg.BaseURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "home.html", data)
}

// Story handles GET /story/{id}
func (h *Handler) Story(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	story, err := h.store.GetStory(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if story == nil {
		http.NotFound(w, r)
		return
	}

	comments, err := h.store.ListComments(r.Context(), id, store.CommentListOptions{
		Sort: store.SortTop,
		View: store.ViewTree,
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Content negotiation
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"story":    story,
			"comments": comments,
		})
		return
	}

	data := StoryData{
		Story:    story,
		Comments: comments,
		BaseURL:  h.cfg.BaseURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "story.html", data)
}

// Submit handles GET /submit
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	// Content negotiation - return form schema for JSON
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"fields": map[string]any{
				"title": map[string]any{
					"type":      "string",
					"required":  true,
					"minLength": 8,
					"maxLength": 180,
				},
				"url": map[string]any{
					"type":     "string",
					"required": false,
					"format":   "uri",
				},
				"text": map[string]any{
					"type":     "string",
					"required": false,
					"format":   "markdown",
				},
				"tags": map[string]any{
					"type":     "array",
					"required": false,
					"maxItems": 5,
				},
			},
			"constraints": []string{
				"Exactly one of 'url' or 'text' must be provided",
			},
		})
		return
	}

	data := SubmitData{
		BaseURL: h.cfg.BaseURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "submit.html", data)
}

// Helper functions

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "application/json" || r.URL.Query().Get("format") == "json"
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// FormatScore formats a score for display
func FormatScore(score int) string {
	if score == 1 || score == -1 {
		return strconv.Itoa(score) + " point"
	}
	return strconv.Itoa(score) + " points"
}
