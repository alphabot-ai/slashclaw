package api

import (
	"encoding/json"
	"net/http"

	"github.com/alphabot-ai/slashclaw/internal/store"
)

type CreateCommentRequest struct {
	StoryID  string `json:"story_id"`
	ParentID string `json:"parent_id,omitempty"`
	Text     string `json:"text"`
}

type CreateCommentResponse struct {
	ID string `json:"id"`
}

type ListCommentsResponse struct {
	Comments []*store.Comment `json:"comments"`
}

// CreateComment handles POST /api/comments
func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	// Rate limit check
	allowed, retryAfter := h.checkRateLimit(r, "comment", h.cfg.CommentRateLimit)
	if !allowed {
		writeRateLimited(w, retryAfter)
		return
	}

	var req CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate
	if req.StoryID == "" {
		writeError(w, http.StatusBadRequest, "story_id is required")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	// Verify story exists
	story, err := h.store.GetStory(r.Context(), req.StoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if story == nil {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}

	// Verify parent comment exists if specified
	if req.ParentID != "" {
		parent, err := h.store.GetComment(r.Context(), req.ParentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if parent == nil {
			writeError(w, http.StatusNotFound, "parent comment not found")
			return
		}
		if parent.StoryID != req.StoryID {
			writeError(w, http.StatusBadRequest, "parent comment is from a different story")
			return
		}
	}

	// Get auth info from context (set by RequireAuth middleware)
	agentID, agentVerified, _ := GetAuthFromContext(r.Context())

	// Create the comment
	comment := &store.Comment{
		StoryID:       req.StoryID,
		ParentID:      req.ParentID,
		Text:          req.Text,
		AgentID:       agentID,
		AgentVerified: agentVerified,
	}

	if err := h.store.CreateComment(r.Context(), comment); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create comment")
		return
	}

	// Update story comment count
	h.store.UpdateStoryCommentCount(r.Context(), req.StoryID, 1)

	writeJSON(w, http.StatusCreated, CreateCommentResponse{ID: comment.ID})
}

// ListComments handles GET /api/stories/{id}/comments
func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	storyID := r.PathValue("id")
	if storyID == "" {
		writeError(w, http.StatusBadRequest, "story id required")
		return
	}

	// Verify story exists
	story, err := h.store.GetStory(r.Context(), storyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if story == nil {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}

	query := r.URL.Query()

	// Parse sort
	sortStr := query.Get("sort")
	var sort store.SortOrder
	switch sortStr {
	case "new":
		sort = store.SortNew
	default:
		sort = store.SortTop
	}

	// Parse view
	viewStr := query.Get("view")
	var view store.ViewMode
	switch viewStr {
	case "flat":
		view = store.ViewFlat
	default:
		view = store.ViewTree
	}

	opts := store.CommentListOptions{
		Sort: sort,
		View: view,
	}

	comments, err := h.store.ListComments(r.Context(), storyID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, ListCommentsResponse{Comments: comments})
}
