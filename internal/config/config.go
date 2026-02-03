package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server
	Port        int
	Host        string
	BaseURL     string
	AdminSecret string

	// Database
	DatabasePath string

	// Rate Limiting
	StoryRateLimit   int           // per hour
	CommentRateLimit int           // per hour
	VoteRateLimit    int           // per hour
	RateLimitWindow  time.Duration

	// Auth
	ChallengeTTL time.Duration
	TokenTTL     time.Duration

	// Content
	DuplicateWindow time.Duration
}

func Load() *Config {
	return &Config{
		Port:             getEnvInt("PORT", 8080),
		Host:             getEnv("HOST", "0.0.0.0"),
		BaseURL:          getEnv("BASE_URL", "http://localhost:8080"),
		AdminSecret:      getEnv("ADMIN_SECRET", ""),
		DatabasePath:     getEnv("DATABASE_PATH", "slashclaw.db"),
		StoryRateLimit:   getEnvInt("STORY_RATE_LIMIT", 10),
		CommentRateLimit: getEnvInt("COMMENT_RATE_LIMIT", 60),
		VoteRateLimit:    getEnvInt("VOTE_RATE_LIMIT", 120),
		RateLimitWindow:  getEnvDuration("RATE_LIMIT_WINDOW", time.Hour),
		ChallengeTTL:     getEnvDuration("CHALLENGE_TTL", 5*time.Minute),
		TokenTTL:         getEnvDuration("TOKEN_TTL", 24*time.Hour),
		DuplicateWindow:  getEnvDuration("DUPLICATE_WINDOW", 30*24*time.Hour),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
