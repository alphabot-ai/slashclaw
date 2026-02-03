package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/alphabot-ai/slashclaw/internal/store"
)

type CreateStoryRequest struct {
	Title string   `json:"title"`
	URL   string   `json:"url,omitempty"`
	Text  string   `json:"text,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

type CreateStoryResponse struct {
	ID       string `json:"id"`
	Existing bool   `json:"existing,omitempty"`
}

type ListStoriesResponse struct {
	Stories    []*store.Story `json:"stories"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// CreateStory handles POST /api/stories
func (h *Handler) CreateStory(w http.ResponseWriter, r *http.Request) {
	// Rate limit check
	allowed, retryAfter := h.checkRateLimit(r, "story", h.cfg.StoryRateLimit)
	if !allowed {
		writeRateLimited(w, retryAfter)
		return
	}

	var req CreateStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate title
	titleLen := utf8.RuneCountInString(req.Title)
	if titleLen < 8 || titleLen > 180 {
		writeError(w, http.StatusBadRequest, "title must be 8-180 characters")
		return
	}

	// Validate URL or text
	hasURL := req.URL != ""
	hasText := req.Text != ""
	if hasURL == hasText {
		writeError(w, http.StatusBadRequest, "exactly one of url or text must be provided")
		return
	}

	// Validate URL format
	if hasURL {
		if _, err := url.ParseRequestURI(req.URL); err != nil {
			writeError(w, http.StatusBadRequest, "invalid URL format")
			return
		}

		// Check for duplicate URL
		since := time.Now().Add(-h.cfg.DuplicateWindow)
		existing, err := h.store.FindStoryByURL(r.Context(), req.URL, since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if existing != nil {
			writeJSON(w, http.StatusOK, CreateStoryResponse{
				ID:       existing.ID,
				Existing: true,
			})
			return
		}
	}

	// Validate tags
	if len(req.Tags) > 5 {
		writeError(w, http.StatusBadRequest, "maximum 5 tags allowed")
		return
	}

	// Get auth info
	token, _ := h.validateToken(r)
	agentID := h.getAgentID(r)
	agentVerified := token != nil

	if token != nil && agentID == "" {
		agentID = token.AgentID
	}

	// Create the story
	story := &store.Story{
		Title:         req.Title,
		URL:           req.URL,
		Text:          req.Text,
		Tags:          req.Tags,
		AgentID:       agentID,
		AgentVerified: agentVerified,
	}

	if err := h.store.CreateStory(r.Context(), story); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create story")
		return
	}

	writeJSON(w, http.StatusCreated, CreateStoryResponse{ID: story.ID})
}

// GetStory handles GET /api/stories/{id}
func (h *Handler) GetStory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "story id required")
		return
	}

	story, err := h.store.GetStory(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if story == nil {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}

	writeJSON(w, http.StatusOK, story)
}

// ListStories handles GET /api/stories
func (h *Handler) ListStories(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse sort
	sortStr := query.Get("sort")
	var sort store.SortOrder
	switch sortStr {
	case "new":
		sort = store.SortNew
	case "discussed":
		sort = store.SortDiscussed
	default:
		sort = store.SortTop
	}

	// Parse limit
	limit := 30
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	cursor := query.Get("cursor")

	opts := store.ListOptions{
		Sort:   sort,
		Limit:  limit,
		Cursor: cursor,
	}

	stories, nextCursor, err := h.store.ListStories(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, ListStoriesResponse{
		Stories:    stories,
		NextCursor: nextCursor,
	})
}
