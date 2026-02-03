package api

import (
	"encoding/json"
	"net/http"

	"github.com/alphabot-ai/slashclaw/internal/auth"
)

type ChallengeRequest struct {
	AgentID   string `json:"agent_id"`
	Algorithm string `json:"alg"`
}

type ChallengeResponse struct {
	Challenge string `json:"challenge"`
	ExpiresAt string `json:"expires_at"`
}

type VerifyRequest struct {
	AgentID   string `json:"agent_id"`
	Algorithm string `json:"alg"`
	PublicKey string `json:"public_key"`
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

type VerifyResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
	KeyID       string `json:"key_id"`
	AccountID   string `json:"account_id,omitempty"`
}

// CreateChallenge handles POST /api/auth/challenge
func (h *Handler) CreateChallenge(w http.ResponseWriter, r *http.Request) {
	var req ChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	if req.Algorithm == "" {
		writeError(w, http.StatusBadRequest, "alg is required")
		return
	}

	challenge, err := h.auth.CreateChallenge(r.Context(), req.AgentID, req.Algorithm)
	if err != nil {
		if err == auth.ErrInvalidAlgorithm {
			writeError(w, http.StatusBadRequest, "invalid algorithm; supported: ed25519, secp256k1, rsa-pss, rsa-sha256")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create challenge")
		return
	}

	writeJSON(w, http.StatusOK, ChallengeResponse{
		Challenge: challenge.Challenge,
		ExpiresAt: challenge.ExpiresAt.Format("2006-01-02T15:04:05Z"),
	})
}

// VerifyChallenge handles POST /api/auth/verify
func (h *Handler) VerifyChallenge(w http.ResponseWriter, r *http.Request) {
	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.AgentID == "" || req.Algorithm == "" || req.PublicKey == "" || req.Challenge == "" || req.Signature == "" {
		writeError(w, http.StatusBadRequest, "all fields are required: agent_id, alg, public_key, challenge, signature")
		return
	}

	token, err := h.auth.VerifyAndCreateToken(r.Context(), req.AgentID, req.Algorithm, req.PublicKey, req.Challenge, req.Signature)
	if err != nil {
		switch err {
		case auth.ErrInvalidAlgorithm:
			writeError(w, http.StatusBadRequest, "invalid algorithm")
		case auth.ErrInvalidPublicKey:
			writeError(w, http.StatusBadRequest, "invalid public key format")
		case auth.ErrInvalidSignature:
			writeError(w, http.StatusUnauthorized, "invalid signature")
		case auth.ErrChallengeNotFound, auth.ErrChallengeExpired:
			writeError(w, http.StatusBadRequest, "challenge expired or not found")
		default:
			writeError(w, http.StatusInternalServerError, "verification failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, VerifyResponse{
		AccessToken: token.Token,
		ExpiresAt:   token.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		KeyID:       token.KeyID,
		AccountID:   token.AccountID,
	})
}
