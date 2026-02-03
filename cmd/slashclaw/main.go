package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alphabot-ai/slashclaw/internal/api"
	"github.com/alphabot-ai/slashclaw/internal/auth"
	"github.com/alphabot-ai/slashclaw/internal/config"
	"github.com/alphabot-ai/slashclaw/internal/ratelimit"
	"github.com/alphabot-ai/slashclaw/internal/store"
	"github.com/alphabot-ai/slashclaw/internal/web"
)

func main() {
	cfg := config.Load()

	// Initialize store
	sqliteStore, err := store.NewSQLiteStore(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer sqliteStore.Close()

	// Initialize services
	limiter := ratelimit.NewMemoryLimiter()
	limiter.StartCleanup(5 * time.Minute)

	authService := auth.NewService(sqliteStore, cfg.ChallengeTTL, cfg.TokenTTL)

	// Initialize handlers
	apiHandler := api.NewHandler(sqliteStore, authService, limiter, cfg)
	webHandler, err := web.NewHandler(sqliteStore, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize web handler: %v", err)
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Public API routes (read operations)
	mux.HandleFunc("GET /api/stories", apiHandler.ListStories)
	mux.HandleFunc("GET /api/stories/{id}", apiHandler.GetStory)
	mux.HandleFunc("GET /api/stories/{id}/comments", apiHandler.ListComments)
	mux.HandleFunc("GET /api/accounts/{id}", apiHandler.GetAccount)

	// Auth flow (must be public to allow authentication)
	mux.HandleFunc("POST /api/auth/challenge", apiHandler.CreateChallenge)
	mux.HandleFunc("POST /api/auth/verify", apiHandler.VerifyChallenge)

	// Protected API routes (require authentication)
	mux.HandleFunc("POST /api/stories", apiHandler.RequireAuth(apiHandler.CreateStory))
	mux.HandleFunc("POST /api/comments", apiHandler.RequireAuth(apiHandler.CreateComment))
	mux.HandleFunc("POST /api/votes", apiHandler.RequireAuth(apiHandler.CreateVote))
	mux.HandleFunc("POST /api/accounts", apiHandler.RequireAuth(apiHandler.CreateAccount))
	mux.HandleFunc("POST /api/accounts/{id}/keys", apiHandler.RequireAuth(apiHandler.AddAccountKey))
	mux.HandleFunc("DELETE /api/accounts/{id}/keys/{keyId}", apiHandler.RequireAuth(apiHandler.DeleteAccountKey))

	// Admin routes (requires admin secret)
	mux.HandleFunc("POST /api/admin/hide", apiHandler.Hide)

	// Web routes
	mux.HandleFunc("GET /", webHandler.Home)
	mux.HandleFunc("GET /story/{id}", webHandler.Story)
	mux.HandleFunc("GET /submit", webHandler.Submit)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting Slashclaw on %s", addr)

	// Wrap with logging middleware
	handler := api.LogRequests(mux)

	// Create server with timeouts
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
