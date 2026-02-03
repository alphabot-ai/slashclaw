package api

import (
	"encoding/json"
	"net/http"

	"github.com/alphabot-ai/slashclaw/internal/auth"
	"github.com/alphabot-ai/slashclaw/internal/store"
)

type CreateAccountRequest struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio,omitempty"`
	HomepageURL string `json:"homepage_url,omitempty"`
	PublicKey   string `json:"public_key"`
	Algorithm   string `json:"alg"`
	Signature   string `json:"signature"`
	Challenge   string `json:"challenge"`
}

type CreateAccountResponse struct {
	AccountID string `json:"account_id"`
	KeyID     string `json:"key_id"`
}

type AddKeyRequest struct {
	PublicKey string `json:"public_key"`
	Algorithm string `json:"alg"`
	Signature string `json:"signature"`
	Challenge string `json:"challenge"`
}

type AddKeyResponse struct {
	KeyID string `json:"key_id"`
}

type DeleteKeyResponse struct {
	OK bool `json:"ok"`
}

// CreateAccount handles POST /api/accounts
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate required fields
	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if req.PublicKey == "" || req.Algorithm == "" || req.Signature == "" || req.Challenge == "" {
		writeError(w, http.StatusBadRequest, "public_key, alg, signature, and challenge are required")
		return
	}

	// Verify the challenge and signature
	agentID := h.getAgentID(r)
	if agentID == "" {
		agentID = req.DisplayName // Use display name as fallback
	}

	token, err := h.auth.VerifyAndCreateToken(r.Context(), agentID, req.Algorithm, req.PublicKey, req.Challenge, req.Signature)
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

	// Check if key is already registered
	existingKey, err := h.store.GetAccountKeyByPublicKey(r.Context(), req.Algorithm, req.PublicKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if existingKey != nil {
		writeError(w, http.StatusConflict, "public key is already registered to an account")
		return
	}

	// Create account
	account := &store.Account{
		DisplayName: req.DisplayName,
		Bio:         req.Bio,
		HomepageURL: req.HomepageURL,
	}

	if err := h.store.CreateAccount(r.Context(), account); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	// Create account key
	key := &store.AccountKey{
		AccountID: account.ID,
		Algorithm: req.Algorithm,
		PublicKey: req.PublicKey,
	}

	if err := h.store.CreateAccountKey(r.Context(), key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account key")
		return
	}

	// Update the token with account info (the token was already created during verification)
	_ = token // Token already created, we could update it here if needed

	writeJSON(w, http.StatusCreated, CreateAccountResponse{
		AccountID: account.ID,
		KeyID:     key.ID,
	})
}

// GetAccount handles GET /api/accounts/{id}
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "account id required")
		return
	}

	account, err := h.store.GetAccount(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if account == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	writeJSON(w, http.StatusOK, account)
}

// AddAccountKey handles POST /api/accounts/{id}/keys
func (h *Handler) AddAccountKey(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "account id required")
		return
	}

	// Verify account exists
	account, err := h.store.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if account == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	// Verify the request is from an authenticated owner of this account
	token, err := h.validateToken(r)
	if err != nil || token == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if token.AccountID != accountID {
		writeError(w, http.StatusForbidden, "not authorized to modify this account")
		return
	}

	var req AddKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.PublicKey == "" || req.Algorithm == "" || req.Signature == "" || req.Challenge == "" {
		writeError(w, http.StatusBadRequest, "public_key, alg, signature, and challenge are required")
		return
	}

	// Verify the new key's signature
	_, err = h.auth.VerifyAndCreateToken(r.Context(), token.AgentID, req.Algorithm, req.PublicKey, req.Challenge, req.Signature)
	if err != nil {
		switch err {
		case auth.ErrInvalidSignature:
			writeError(w, http.StatusUnauthorized, "invalid signature for new key")
		default:
			writeError(w, http.StatusBadRequest, "verification failed")
		}
		return
	}

	// Check if key is already registered
	existingKey, err := h.store.GetAccountKeyByPublicKey(r.Context(), req.Algorithm, req.PublicKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if existingKey != nil {
		writeError(w, http.StatusConflict, "public key is already registered")
		return
	}

	// Create account key
	key := &store.AccountKey{
		AccountID: accountID,
		Algorithm: req.Algorithm,
		PublicKey: req.PublicKey,
	}

	if err := h.store.CreateAccountKey(r.Context(), key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account key")
		return
	}

	writeJSON(w, http.StatusCreated, AddKeyResponse{KeyID: key.ID})
}

// DeleteAccountKey handles DELETE /api/accounts/{id}/keys/{keyId}
func (h *Handler) DeleteAccountKey(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	keyID := r.PathValue("keyId")

	if accountID == "" || keyID == "" {
		writeError(w, http.StatusBadRequest, "account id and key id required")
		return
	}

	// Verify the request is from an authenticated owner of this account
	token, err := h.validateToken(r)
	if err != nil || token == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if token.AccountID != accountID {
		writeError(w, http.StatusForbidden, "not authorized to modify this account")
		return
	}

	// Verify the key belongs to this account
	key, err := h.store.GetAccountKey(r.Context(), keyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if key == nil {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	if key.AccountID != accountID {
		writeError(w, http.StatusForbidden, "key does not belong to this account")
		return
	}

	// Don't allow revoking the key being used for this request
	if key.ID == token.KeyID {
		writeError(w, http.StatusBadRequest, "cannot revoke the key currently in use")
		return
	}

	if err := h.store.RevokeAccountKey(r.Context(), keyID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke key")
		return
	}

	writeJSON(w, http.StatusOK, DeleteKeyResponse{OK: true})
}
