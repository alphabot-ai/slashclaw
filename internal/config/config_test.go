package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Clear any env vars that might interfere
	os.Unsetenv("PORT")
	os.Unsetenv("HOST")
	os.Unsetenv("DATABASE_PATH")

	cfg := Load()

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want \"0.0.0.0\"", cfg.Host)
	}
	if cfg.DatabasePath != "slashclaw.db" {
		t.Errorf("DatabasePath = %q, want \"slashclaw.db\"", cfg.DatabasePath)
	}
	if cfg.StoryRateLimit != 10 {
		t.Errorf("StoryRateLimit = %d, want 10", cfg.StoryRateLimit)
	}
	if cfg.CommentRateLimit != 60 {
		t.Errorf("CommentRateLimit = %d, want 60", cfg.CommentRateLimit)
	}
	if cfg.VoteRateLimit != 120 {
		t.Errorf("VoteRateLimit = %d, want 120", cfg.VoteRateLimit)
	}
	if cfg.RateLimitWindow != time.Hour {
		t.Errorf("RateLimitWindow = %v, want 1h", cfg.RateLimitWindow)
	}
	if cfg.DuplicateWindow != 30*24*time.Hour {
		t.Errorf("DuplicateWindow = %v, want 720h", cfg.DuplicateWindow)
	}
	if cfg.PostCooldown != 60*time.Second {
		t.Errorf("PostCooldown = %v, want 60s", cfg.PostCooldown)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set env vars
	os.Setenv("PORT", "3000")
	os.Setenv("HOST", "127.0.0.1")
	os.Setenv("DATABASE_PATH", "/tmp/test.db")
	os.Setenv("STORY_RATE_LIMIT", "5")
	os.Setenv("POST_COOLDOWN", "30s")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("HOST")
		os.Unsetenv("DATABASE_PATH")
		os.Unsetenv("STORY_RATE_LIMIT")
		os.Unsetenv("POST_COOLDOWN")
	}()

	cfg := Load()

	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want \"127.0.0.1\"", cfg.Host)
	}
	if cfg.DatabasePath != "/tmp/test.db" {
		t.Errorf("DatabasePath = %q, want \"/tmp/test.db\"", cfg.DatabasePath)
	}
	if cfg.StoryRateLimit != 5 {
		t.Errorf("StoryRateLimit = %d, want 5", cfg.StoryRateLimit)
	}
	if cfg.PostCooldown != 30*time.Second {
		t.Errorf("PostCooldown = %v, want 30s", cfg.PostCooldown)
	}
}

func TestGetEnvInvalidValues(t *testing.T) {
	// Invalid int should use default
	os.Setenv("PORT", "not-a-number")
	defer os.Unsetenv("PORT")

	cfg := Load()
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080 (default on invalid)", cfg.Port)
	}
}

func TestGetEnvDurationInvalid(t *testing.T) {
	// Invalid duration should use default
	os.Setenv("POST_COOLDOWN", "invalid")
	defer os.Unsetenv("POST_COOLDOWN")

	cfg := Load()
	if cfg.PostCooldown != 60*time.Second {
		t.Errorf("PostCooldown = %v, want 60s (default on invalid)", cfg.PostCooldown)
	}
}
