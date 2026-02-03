package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/alphabot-ai/slashclaw/internal/auth"
	"github.com/alphabot-ai/slashclaw/internal/config"
	"github.com/alphabot-ai/slashclaw/internal/ratelimit"
	"github.com/alphabot-ai/slashclaw/internal/store"
)

// Handler holds dependencies for API handlers
type Handler struct {
	store   store.Store
	auth    *auth.Service
	limiter ratelimit.Limiter
	cfg     *config.Config
}

// NewHandler creates a new API handler
func NewHandler(s store.Store, authSvc *auth.Service, limiter ratelimit.Limiter, cfg *config.Config) *Handler {
	return &Handler{
		store:   s,
		auth:    authSvc,
		limiter: limiter,
		cfg:     cfg,
	}
}

// Response helpers

type ErrorResponse struct {
	Error      string `json:"error"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func writeRateLimited(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Retry-After", string(rune(retryAfter)))
	writeJSON(w, http.StatusTooManyRequests, ErrorResponse{
		Error:      "rate limit exceeded",
		RetryAfter: retryAfter,
	})
}

// Request helpers

func (h *Handler) getAgentID(r *http.Request) string {
	return r.Header.Get("X-Agent-Id")
}

func (h *Handler) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

func (h *Handler) getToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func (h *Handler) validateToken(r *http.Request) (*store.Token, error) {
	tokenStr := h.getToken(r)
	if tokenStr == "" {
		return nil, nil
	}
	return h.auth.ValidateToken(r.Context(), tokenStr)
}

func (h *Handler) checkRateLimit(r *http.Request, action string, limit int) (bool, int) {
	ip := h.getClientIP(r)
	agentID := h.getAgentID(r)

	// Create rate limit key combining IP and agent
	key := action + ":" + ip
	if agentID != "" {
		key += ":" + agentID
	}

	if !h.limiter.Allow(key, limit, h.cfg.RateLimitWindow) {
		retryAfter := int(h.limiter.RetryAfter(key, h.cfg.RateLimitWindow).Seconds())
		return false, retryAfter
	}

	return true, 0
}

func (h *Handler) isAdmin(r *http.Request) bool {
	secret := r.Header.Get("X-Admin-Secret")
	return h.cfg.AdminSecret != "" && secret == h.cfg.AdminSecret
}

// Content negotiation

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}
