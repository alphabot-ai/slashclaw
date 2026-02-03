package store

import (
	"context"
	"time"
)

// Store defines the interface for data persistence
type Store interface {
	// Stories
	CreateStory(ctx context.Context, story *Story) error
	GetStory(ctx context.Context, id string) (*Story, error)
	ListStories(ctx context.Context, opts ListOptions) ([]*Story, string, error) // returns stories and next cursor
	FindStoryByURL(ctx context.Context, url string, since time.Time) (*Story, error)
	UpdateStoryScore(ctx context.Context, id string, delta int) error
	UpdateStoryCommentCount(ctx context.Context, id string, delta int) error
	HideStory(ctx context.Context, id string) error

	// Comments
	CreateComment(ctx context.Context, comment *Comment) error
	GetComment(ctx context.Context, id string) (*Comment, error)
	ListComments(ctx context.Context, storyID string, opts CommentListOptions) ([]*Comment, error)
	UpdateCommentScore(ctx context.Context, id string, delta int) error
	HideComment(ctx context.Context, id string) error

	// Votes
	CreateVote(ctx context.Context, vote *Vote) error
	GetVote(ctx context.Context, targetType, targetID, ipHash, agentID string) (*Vote, error)
	UpdateVote(ctx context.Context, id string, value int) error

	// Accounts
	CreateAccount(ctx context.Context, account *Account) error
	GetAccount(ctx context.Context, id string) (*Account, error)

	// Account Keys
	CreateAccountKey(ctx context.Context, key *AccountKey) error
	GetAccountKey(ctx context.Context, id string) (*AccountKey, error)
	GetAccountKeyByPublicKey(ctx context.Context, alg, publicKey string) (*AccountKey, error)
	ListAccountKeys(ctx context.Context, accountID string) ([]*AccountKey, error)
	RevokeAccountKey(ctx context.Context, id string) error

	// Auth
	CreateChallenge(ctx context.Context, challenge *Challenge) error
	GetChallenge(ctx context.Context, challengeStr string) (*Challenge, error)
	DeleteChallenge(ctx context.Context, id string) error
	CreateToken(ctx context.Context, token *Token) error
	GetToken(ctx context.Context, tokenStr string) (*Token, error)
	DeleteExpiredTokens(ctx context.Context) error

	// Lifecycle
	Close() error
}
