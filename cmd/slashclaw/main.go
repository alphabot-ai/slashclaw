package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
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

	// API routes
	mux.HandleFunc("POST /api/stories", apiHandler.CreateStory)
	mux.HandleFunc("GET /api/stories", apiHandler.ListStories)
	mux.HandleFunc("GET /api/stories/{id}", apiHandler.GetStory)
	mux.HandleFunc("GET /api/stories/{id}/comments", apiHandler.ListComments)

	mux.HandleFunc("POST /api/comments", apiHandler.CreateComment)

	mux.HandleFunc("POST /api/votes", apiHandler.CreateVote)

	mux.HandleFunc("POST /api/auth/challenge", apiHandler.CreateChallenge)
	mux.HandleFunc("POST /api/auth/verify", apiHandler.VerifyChallenge)

	mux.HandleFunc("POST /api/accounts", apiHandler.CreateAccount)
	mux.HandleFunc("GET /api/accounts/{id}", apiHandler.GetAccount)
	mux.HandleFunc("POST /api/accounts/{id}/keys", apiHandler.AddAccountKey)
	mux.HandleFunc("DELETE /api/accounts/{id}/keys/{keyId}", apiHandler.DeleteAccountKey)

	mux.HandleFunc("POST /api/admin/hide", apiHandler.Hide)

	// Web routes
	mux.HandleFunc("GET /", webHandler.Home)
	mux.HandleFunc("GET /story/{id}", webHandler.Story)
	mux.HandleFunc("GET /submit", webHandler.Submit)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting Slashclaw on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Server error: %v", err)
		os.Exit(1)
	}
}
