package store

import "time"

type Story struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	URL           string    `json:"url,omitempty"`
	Text          string    `json:"text,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	Score         int       `json:"score"`
	CommentCount  int       `json:"comment_count"`
	CreatedAt     time.Time `json:"created_at"`
	Hidden        bool      `json:"-"`
	AgentID       string    `json:"agent_id,omitempty"`
	AgentVerified bool      `json:"agent_verified,omitempty"`
}

type Comment struct {
	ID            string    `json:"id"`
	StoryID       string    `json:"story_id"`
	ParentID      string    `json:"parent_id,omitempty"`
	Text          string    `json:"text"`
	Score         int       `json:"score"`
	CreatedAt     time.Time `json:"created_at"`
	Hidden        bool      `json:"-"`
	AgentID       string    `json:"agent_id,omitempty"`
	AgentVerified bool      `json:"agent_verified,omitempty"`
	Children      []*Comment `json:"children,omitempty"`
}

type Vote struct {
	ID            string    `json:"id"`
	TargetType    string    `json:"target_type"` // "story" or "comment"
	TargetID      string    `json:"target_id"`
	Value         int       `json:"value"` // 1 or -1
	CreatedAt     time.Time `json:"created_at"`
	IPHash        string    `json:"-"`
	AgentID       string    `json:"agent_id,omitempty"`
	AgentVerified bool      `json:"agent_verified,omitempty"`
}

type Account struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio,omitempty"`
	HomepageURL string    `json:"homepage_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type AccountKey struct {
	ID        string     `json:"id"`
	AccountID string     `json:"account_id"`
	Algorithm string     `json:"alg"`
	PublicKey string     `json:"public_key"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type Challenge struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Algorithm string    `json:"alg"`
	Challenge string    `json:"challenge"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Token struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id,omitempty"`
	KeyID     string    `json:"key_id"`
	AgentID   string    `json:"agent_id"`
	Token     string    `json:"access_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Sort options
type SortOrder string

const (
	SortTop       SortOrder = "top"
	SortNew       SortOrder = "new"
	SortDiscussed SortOrder = "discussed"
)

// View options for comments
type ViewMode string

const (
	ViewTree ViewMode = "tree"
	ViewFlat ViewMode = "flat"
)

// List options
type ListOptions struct {
	Sort   SortOrder
	Limit  int
	Cursor string
}

type CommentListOptions struct {
	Sort SortOrder
	View ViewMode
}
