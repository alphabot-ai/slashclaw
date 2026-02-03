package auth

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/alphabot-ai/slashclaw/internal/store"
	"github.com/google/uuid"
)

var (
	ErrInvalidAlgorithm  = errors.New("invalid algorithm")
	ErrInvalidPublicKey  = errors.New("invalid public key")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrChallengeExpired  = errors.New("challenge expired or not found")
	ErrChallengeNotFound = errors.New("challenge not found")
)

// Algorithm constants
const (
	AlgEd25519   = "ed25519"
	AlgSecp256k1 = "secp256k1"
	AlgRSAPSS    = "rsa-pss"
	AlgRSASHA256 = "rsa-sha256"
)

// Service handles authentication operations
type Service struct {
	store        store.Store
	challengeTTL time.Duration
	tokenTTL     time.Duration
}

// NewService creates a new auth service
func NewService(s store.Store, challengeTTL, tokenTTL time.Duration) *Service {
	return &Service{
		store:        s,
		challengeTTL: challengeTTL,
		tokenTTL:     tokenTTL,
	}
}

// CreateChallenge generates a new challenge for an agent
func (s *Service) CreateChallenge(ctx context.Context, agentID, alg string) (*store.Challenge, error) {
	if !isValidAlgorithm(alg) {
		return nil, ErrInvalidAlgorithm
	}

	// Generate random challenge string
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return nil, err
	}

	challenge := &store.Challenge{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		Algorithm: alg,
		Challenge: base64.URLEncoding.EncodeToString(challengeBytes),
		ExpiresAt: time.Now().UTC().Add(s.challengeTTL),
	}

	if err := s.store.CreateChallenge(ctx, challenge); err != nil {
		return nil, err
	}

	return challenge, nil
}

// VerifyAndCreateToken verifies a signature and creates an access token
func (s *Service) VerifyAndCreateToken(ctx context.Context, agentID, alg, publicKey, challengeStr, signature string) (*store.Token, error) {
	// Get the challenge
	challenge, err := s.store.GetChallenge(ctx, challengeStr)
	if err != nil {
		return nil, err
	}
	if challenge == nil {
		return nil, ErrChallengeNotFound
	}
	if time.Now().After(challenge.ExpiresAt) {
		return nil, ErrChallengeExpired
	}
	if challenge.AgentID != agentID || challenge.Algorithm != alg {
		return nil, ErrChallengeNotFound
	}

	// Verify the signature
	valid, err := verifySignature(alg, publicKey, challengeStr, signature)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrInvalidSignature
	}

	// Delete the used challenge
	s.store.DeleteChallenge(ctx, challenge.ID)

	// Check if there's an existing account key
	accountKey, err := s.store.GetAccountKeyByPublicKey(ctx, alg, publicKey)
	if err != nil {
		return nil, err
	}

	// Generate token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}

	token := &store.Token{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		Token:     base64.URLEncoding.EncodeToString(tokenBytes),
		ExpiresAt: time.Now().UTC().Add(s.tokenTTL),
	}

	if accountKey != nil {
		token.AccountID = accountKey.AccountID
		token.KeyID = accountKey.ID
	} else {
		token.KeyID = "unregistered:" + publicKey[:16]
	}

	if err := s.store.CreateToken(ctx, token); err != nil {
		return nil, err
	}

	return token, nil
}

// ValidateToken checks if a token is valid and returns the token info
func (s *Service) ValidateToken(ctx context.Context, tokenStr string) (*store.Token, error) {
	token, err := s.store.GetToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// verifySignature verifies a signature based on the algorithm
func verifySignature(alg, publicKeyStr, message, signatureStr string) (bool, error) {
	switch alg {
	case AlgEd25519:
		return verifyEd25519(publicKeyStr, message, signatureStr)
	case AlgRSAPSS:
		return verifyRSAPSS(publicKeyStr, message, signatureStr)
	case AlgRSASHA256:
		return verifyRSASHA256(publicKeyStr, message, signatureStr)
	case AlgSecp256k1:
		// For MVP, we'll stub secp256k1 and implement later
		return false, fmt.Errorf("secp256k1 not yet implemented")
	default:
		return false, ErrInvalidAlgorithm
	}
}

func verifyEd25519(publicKeyStr, message, signatureStr string) (bool, error) {
	// Decode public key from base64
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyStr)
	if err != nil {
		return false, ErrInvalidPublicKey
	}

	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return false, ErrInvalidPublicKey
	}

	publicKey := ed25519.PublicKey(publicKeyBytes)

	// Decode signature from base64
	signatureBytes, err := base64.StdEncoding.DecodeString(signatureStr)
	if err != nil {
		return false, ErrInvalidSignature
	}

	return ed25519.Verify(publicKey, []byte(message), signatureBytes), nil
}

func verifyRSAPSS(publicKeyStr, message, signatureStr string) (bool, error) {
	publicKey, err := parseRSAPublicKey(publicKeyStr)
	if err != nil {
		return false, err
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureStr)
	if err != nil {
		return false, ErrInvalidSignature
	}

	hash := sha256.Sum256([]byte(message))
	err = rsa.VerifyPSS(publicKey, crypto.SHA256, hash[:], signatureBytes, nil)
	return err == nil, nil
}

func verifyRSASHA256(publicKeyStr, message, signatureStr string) (bool, error) {
	publicKey, err := parseRSAPublicKey(publicKeyStr)
	if err != nil {
		return false, err
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureStr)
	if err != nil {
		return false, ErrInvalidSignature
	}

	hash := sha256.Sum256([]byte(message))
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signatureBytes)
	return err == nil, nil
}

func parseRSAPublicKey(publicKeyStr string) (*rsa.PublicKey, error) {
	// Try PEM format first
	block, _ := pem.Decode([]byte(publicKeyStr))
	if block != nil {
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, ErrInvalidPublicKey
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, ErrInvalidPublicKey
		}
		return rsaPub, nil
	}

	// Try base64-encoded DER
	derBytes, err := base64.StdEncoding.DecodeString(publicKeyStr)
	if err != nil {
		return nil, ErrInvalidPublicKey
	}

	pub, err := x509.ParsePKIXPublicKey(derBytes)
	if err != nil {
		return nil, ErrInvalidPublicKey
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, ErrInvalidPublicKey
	}

	return rsaPub, nil
}

func isValidAlgorithm(alg string) bool {
	switch alg {
	case AlgEd25519, AlgSecp256k1, AlgRSAPSS, AlgRSASHA256:
		return true
	default:
		return false
	}
}

// HashIP creates a hash of an IP address for vote tracking
func HashIP(ip string) string {
	hash := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(hash[:16])
}
