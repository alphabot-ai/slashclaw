package api

import (
	"encoding/json"
	"net/http"

	"github.com/alphabot-ai/slashclaw/internal/auth"
	"github.com/alphabot-ai/slashclaw/internal/store"
)

type CreateVoteRequest struct {
	TargetType string `json:"target_type"` // "story" or "comment"
	TargetID   string `json:"target_id"`
	Value      int    `json:"value"` // 1 or -1
}

type CreateVoteResponse struct {
	OK bool `json:"ok"`
}

// CreateVote handles POST /api/votes
func (h *Handler) CreateVote(w http.ResponseWriter, r *http.Request) {
	// Rate limit check
	allowed, retryAfter := h.checkRateLimit(r, "vote", h.cfg.VoteRateLimit)
	if !allowed {
		writeRateLimited(w, retryAfter)
		return
	}

	var req CreateVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate target type
	if req.TargetType != "story" && req.TargetType != "comment" {
		writeError(w, http.StatusBadRequest, "target_type must be 'story' or 'comment'")
		return
	}

	// Validate value
	if req.Value != 1 && req.Value != -1 {
		writeError(w, http.StatusBadRequest, "value must be 1 or -1")
		return
	}

	// Get auth info from context (set by RequireAuth middleware)
	agentID, agentVerified, _ := GetAuthFromContext(r.Context())

	// Validate target exists and check for self-voting
	if req.TargetType == "story" {
		story, err := h.store.GetStory(r.Context(), req.TargetID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if story == nil {
			writeError(w, http.StatusNotFound, "story not found")
			return
		}
		// Prevent self-voting
		if story.AgentID != "" && story.AgentID == agentID {
			writeError(w, http.StatusForbidden, "cannot vote on your own content")
			return
		}
	} else {
		comment, err := h.store.GetComment(r.Context(), req.TargetID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if comment == nil {
			writeError(w, http.StatusNotFound, "comment not found")
			return
		}
		// Prevent self-voting
		if comment.AgentID != "" && comment.AgentID == agentID {
			writeError(w, http.StatusForbidden, "cannot vote on your own content")
			return
		}
	}

	// Hash IP for vote tracking
	ipHash := auth.HashIP(h.getClientIP(r))

	// Check for existing vote
	existingVote, err := h.store.GetVote(r.Context(), req.TargetType, req.TargetID, ipHash, agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if existingVote != nil {
		// Update existing vote if value changed
		if existingVote.Value != req.Value {
			if err := h.store.UpdateVote(r.Context(), existingVote.ID, req.Value); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to update vote")
				return
			}

			// Update score: delta is the difference between new and old value
			delta := req.Value - existingVote.Value
			if req.TargetType == "story" {
				h.store.UpdateStoryScore(r.Context(), req.TargetID, delta)
			} else {
				h.store.UpdateCommentScore(r.Context(), req.TargetID, delta)
			}
		}
	} else {
		// Create new vote
		vote := &store.Vote{
			TargetType:    req.TargetType,
			TargetID:      req.TargetID,
			Value:         req.Value,
			IPHash:        ipHash,
			AgentID:       agentID,
			AgentVerified: agentVerified,
		}

		if err := h.store.CreateVote(r.Context(), vote); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create vote")
			return
		}

		// Update score
		if req.TargetType == "story" {
			h.store.UpdateStoryScore(r.Context(), req.TargetID, req.Value)
		} else {
			h.store.UpdateCommentScore(r.Context(), req.TargetID, req.Value)
		}
	}

	writeJSON(w, http.StatusOK, CreateVoteResponse{OK: true})
}
