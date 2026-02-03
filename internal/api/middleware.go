package api

import (
	"context"
	"log"
	"net/http"
)

type contextKey string

const (
	ContextKeyAgentID   contextKey = "agent_id"
	ContextKeyVerified  contextKey = "verified"
	ContextKeyAccountID contextKey = "account_id"
)

// RequireAuth returns middleware that requires a valid auth token
func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := h.validateToken(r)
		if err != nil || token == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		// Add auth info to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, ContextKeyAgentID, token.AgentID)
		ctx = context.WithValue(ctx, ContextKeyVerified, true)
		if token.AccountID != "" {
			ctx = context.WithValue(ctx, ContextKeyAccountID, token.AccountID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// OptionalAuth adds auth info to context if present, but doesn't require it
func (h *Handler) OptionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		token, _ := h.validateToken(r)
		if token != nil {
			ctx = context.WithValue(ctx, ContextKeyAgentID, token.AgentID)
			ctx = context.WithValue(ctx, ContextKeyVerified, true)
			if token.AccountID != "" {
				ctx = context.WithValue(ctx, ContextKeyAccountID, token.AccountID)
			}
		} else {
			// Check for unverified agent ID header
			agentID := r.Header.Get("X-Agent-Id")
			if agentID != "" {
				ctx = context.WithValue(ctx, ContextKeyAgentID, agentID)
				ctx = context.WithValue(ctx, ContextKeyVerified, false)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetAuthFromContext extracts auth info from request context
func GetAuthFromContext(ctx context.Context) (agentID string, verified bool, accountID string) {
	if v := ctx.Value(ContextKeyAgentID); v != nil {
		agentID = v.(string)
	}
	if v := ctx.Value(ContextKeyVerified); v != nil {
		verified = v.(bool)
	}
	if v := ctx.Value(ContextKeyAccountID); v != nil {
		accountID = v.(string)
	}
	return
}

// LogRequests returns middleware that logs all incoming requests
func LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

