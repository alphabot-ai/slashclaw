package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/alphabot-ai/slashclaw/internal/store"
)

func setupTestStore(t *testing.T) (*store.SQLiteStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "slashclaw-auth-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	sqliteStore, err := store.NewSQLiteStore(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		sqliteStore.Close()
		os.Remove(tmpFile.Name())
	}

	return sqliteStore, cleanup
}

func TestCreateChallenge(t *testing.T) {
	sqliteStore, cleanup := setupTestStore(t)
	defer cleanup()

	service := NewService(sqliteStore, 5*time.Minute, 24*time.Hour)
	ctx := context.Background()

	t.Run("valid challenge", func(t *testing.T) {
		challenge, err := service.CreateChallenge(ctx, "test-agent", AlgEd25519)
		if err != nil {
			t.Fatalf("failed to create challenge: %v", err)
		}

		if challenge.Challenge == "" {
			t.Error("challenge string should not be empty")
		}

		if challenge.AgentID != "test-agent" {
			t.Errorf("agent_id = %q, want %q", challenge.AgentID, "test-agent")
		}

		if challenge.Algorithm != AlgEd25519 {
			t.Errorf("algorithm = %q, want %q", challenge.Algorithm, AlgEd25519)
		}

		if challenge.ExpiresAt.Before(time.Now()) {
			t.Error("challenge should not be expired")
		}
	})

	t.Run("invalid algorithm", func(t *testing.T) {
		_, err := service.CreateChallenge(ctx, "test-agent", "invalid-alg")
		if err != ErrInvalidAlgorithm {
			t.Errorf("expected ErrInvalidAlgorithm, got %v", err)
		}
	})
}

func TestVerifyEd25519(t *testing.T) {
	sqliteStore, cleanup := setupTestStore(t)
	defer cleanup()

	service := NewService(sqliteStore, 5*time.Minute, 24*time.Hour)
	ctx := context.Background()

	// Generate a key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKey)

	t.Run("valid signature", func(t *testing.T) {
		// Create a challenge
		challenge, err := service.CreateChallenge(ctx, "test-agent", AlgEd25519)
		if err != nil {
			t.Fatalf("failed to create challenge: %v", err)
		}

		// Sign the challenge
		signature := ed25519.Sign(privateKey, []byte(challenge.Challenge))
		signatureB64 := base64.StdEncoding.EncodeToString(signature)

		// Verify and create token
		token, err := service.VerifyAndCreateToken(ctx, "test-agent", AlgEd25519, publicKeyB64, challenge.Challenge, signatureB64)
		if err != nil {
			t.Fatalf("failed to verify: %v", err)
		}

		if token.AgentID != "test-agent" {
			t.Errorf("agent_id = %q, want %q", token.AgentID, "test-agent")
		}

		if token.Token == "" {
			t.Error("token should not be empty")
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		challenge, _ := service.CreateChallenge(ctx, "test-agent", AlgEd25519)

		// Wrong signature
		_, err := service.VerifyAndCreateToken(ctx, "test-agent", AlgEd25519, publicKeyB64, challenge.Challenge, "invalidsignature")
		if err != ErrInvalidSignature {
			t.Errorf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("expired challenge", func(t *testing.T) {
		// Create a challenge that expires immediately
		expiredService := NewService(sqliteStore, -1*time.Second, 24*time.Hour)
		challenge, _ := expiredService.CreateChallenge(ctx, "test-agent", AlgEd25519)

		signature := ed25519.Sign(privateKey, []byte(challenge.Challenge))
		signatureB64 := base64.StdEncoding.EncodeToString(signature)

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		_, err := service.VerifyAndCreateToken(ctx, "test-agent", AlgEd25519, publicKeyB64, challenge.Challenge, signatureB64)
		if err != ErrChallengeNotFound && err != ErrChallengeExpired {
			t.Errorf("expected challenge error, got %v", err)
		}
	})

	t.Run("wrong agent_id", func(t *testing.T) {
		challenge, _ := service.CreateChallenge(ctx, "test-agent", AlgEd25519)

		signature := ed25519.Sign(privateKey, []byte(challenge.Challenge))
		signatureB64 := base64.StdEncoding.EncodeToString(signature)

		// Use different agent_id
		_, err := service.VerifyAndCreateToken(ctx, "different-agent", AlgEd25519, publicKeyB64, challenge.Challenge, signatureB64)
		if err != ErrChallengeNotFound {
			t.Errorf("expected ErrChallengeNotFound, got %v", err)
		}
	})
}

func TestValidateToken(t *testing.T) {
	sqliteStore, cleanup := setupTestStore(t)
	defer cleanup()

	service := NewService(sqliteStore, 5*time.Minute, 24*time.Hour)
	ctx := context.Background()

	// Generate a key pair and create a token
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKey)

	challenge, _ := service.CreateChallenge(ctx, "test-agent", AlgEd25519)
	signature := ed25519.Sign(privateKey, []byte(challenge.Challenge))
	signatureB64 := base64.StdEncoding.EncodeToString(signature)

	token, _ := service.VerifyAndCreateToken(ctx, "test-agent", AlgEd25519, publicKeyB64, challenge.Challenge, signatureB64)

	t.Run("valid token", func(t *testing.T) {
		validated, err := service.ValidateToken(ctx, token.Token)
		if err != nil {
			t.Fatalf("failed to validate: %v", err)
		}

		if validated.AgentID != "test-agent" {
			t.Errorf("agent_id = %q, want %q", validated.AgentID, "test-agent")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		validated, err := service.ValidateToken(ctx, "nonexistent-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if validated != nil {
			t.Error("invalid token should return nil")
		}
	})
}

func TestHashIP(t *testing.T) {
	tests := []struct {
		ip1 string
		ip2 string
	}{
		{"192.168.1.1", "192.168.1.1"},
		{"10.0.0.1", "10.0.0.1"},
	}

	for _, tt := range tests {
		hash1 := HashIP(tt.ip1)
		hash2 := HashIP(tt.ip2)

		if hash1 != hash2 {
			t.Errorf("HashIP(%q) != HashIP(%q)", tt.ip1, tt.ip2)
		}

		if len(hash1) != 32 { // 16 bytes hex encoded
			t.Errorf("hash length = %d, want 32", len(hash1))
		}
	}

	// Different IPs should have different hashes
	hash1 := HashIP("192.168.1.1")
	hash2 := HashIP("192.168.1.2")
	if hash1 == hash2 {
		t.Error("different IPs should have different hashes")
	}
}

func TestIsValidAlgorithm(t *testing.T) {
	validAlgs := []string{AlgEd25519, AlgSecp256k1, AlgRSAPSS, AlgRSASHA256}
	for _, alg := range validAlgs {
		if !isValidAlgorithm(alg) {
			t.Errorf("%q should be valid", alg)
		}
	}

	invalidAlgs := []string{"invalid", "", "ed25519-invalid", "rsa"}
	for _, alg := range invalidAlgs {
		if isValidAlgorithm(alg) {
			t.Errorf("%q should be invalid", alg)
		}
	}
}
