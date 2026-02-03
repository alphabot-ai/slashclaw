package api

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogRequests(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with LogRequests
	logged := LogRequests(handler)

	// Make a test request
	req := httptest.NewRequest(http.MethodGet, "/api/stories", nil)
	rec := httptest.NewRecorder()

	logged.ServeHTTP(rec, req)

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check log output
	logOutput := buf.String()
	if !strings.Contains(logOutput, "GET") {
		t.Error("log should contain HTTP method")
	}
	if !strings.Contains(logOutput, "/api/stories") {
		t.Error("log should contain request path")
	}
}

func TestLogRequestsDifferentMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			logged := LogRequests(handler)
			req := httptest.NewRequest(method, "/test", nil)
			rec := httptest.NewRecorder()

			logged.ServeHTTP(rec, req)

			if !strings.Contains(buf.String(), method) {
				t.Errorf("log should contain method %s", method)
			}
		})
	}
}

func TestGetAuthFromContext(t *testing.T) {
	tests := []struct {
		name          string
		setupCtx      func() context.Context
		wantAgentID   string
		wantVerified  bool
		wantAccountID string
	}{
		{
			name: "empty context",
			setupCtx: func() context.Context {
				return context.Background()
			},
			wantAgentID:   "",
			wantVerified:  false,
			wantAccountID: "",
		},
		{
			name: "with agent ID only",
			setupCtx: func() context.Context {
				ctx := context.Background()
				ctx = context.WithValue(ctx, ContextKeyAgentID, "test-agent")
				return ctx
			},
			wantAgentID:   "test-agent",
			wantVerified:  false,
			wantAccountID: "",
		},
		{
			name: "with all values",
			setupCtx: func() context.Context {
				ctx := context.Background()
				ctx = context.WithValue(ctx, ContextKeyAgentID, "test-agent")
				ctx = context.WithValue(ctx, ContextKeyVerified, true)
				ctx = context.WithValue(ctx, ContextKeyAccountID, "account-123")
				return ctx
			},
			wantAgentID:   "test-agent",
			wantVerified:  true,
			wantAccountID: "account-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			agentID, verified, accountID := GetAuthFromContext(ctx)

			if agentID != tt.wantAgentID {
				t.Errorf("agentID = %q, want %q", agentID, tt.wantAgentID)
			}
			if verified != tt.wantVerified {
				t.Errorf("verified = %v, want %v", verified, tt.wantVerified)
			}
			if accountID != tt.wantAccountID {
				t.Errorf("accountID = %q, want %q", accountID, tt.wantAccountID)
			}
		})
	}
}
