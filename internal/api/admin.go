package api

import (
	"encoding/json"
	"net/http"
)

type HideRequest struct {
	TargetType string `json:"target_type"` // "story" or "comment"
	TargetID   string `json:"target_id"`
}

type HideResponse struct {
	OK bool `json:"ok"`
}

// Hide handles POST /api/admin/hide
func (h *Handler) Hide(w http.ResponseWriter, r *http.Request) {
	// Check admin auth
	if !h.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "admin authentication required")
		return
	}

	var req HideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.TargetType != "story" && req.TargetType != "comment" {
		writeError(w, http.StatusBadRequest, "target_type must be 'story' or 'comment'")
		return
	}

	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}

	var err error
	if req.TargetType == "story" {
		// Verify story exists
		story, getErr := h.store.GetStory(r.Context(), req.TargetID)
		if getErr != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if story == nil {
			writeError(w, http.StatusNotFound, "story not found")
			return
		}
		err = h.store.HideStory(r.Context(), req.TargetID)
	} else {
		// Verify comment exists
		comment, getErr := h.store.GetComment(r.Context(), req.TargetID)
		if getErr != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if comment == nil {
			writeError(w, http.StatusNotFound, "comment not found")
			return
		}
		err = h.store.HideComment(r.Context(), req.TargetID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hide content")
		return
	}

	writeJSON(w, http.StatusOK, HideResponse{OK: true})
}
