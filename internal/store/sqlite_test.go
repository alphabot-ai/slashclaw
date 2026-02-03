package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "slashclaw-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, cleanup
}

func TestStoryCreate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	story := &Story{
		Title:   "Test Story",
		URL:     "https://example.com",
		Tags:    []string{"test", "example"},
		AgentID: "test-agent",
	}

	err := store.CreateStory(ctx, story)
	if err != nil {
		t.Fatalf("failed to create story: %v", err)
	}

	if story.ID == "" {
		t.Error("story ID should be set after creation")
	}

	// Verify story was created
	fetched, err := store.GetStory(ctx, story.ID)
	if err != nil {
		t.Fatalf("failed to get story: %v", err)
	}

	if fetched.Title != story.Title {
		t.Errorf("title mismatch: got %q, want %q", fetched.Title, story.Title)
	}

	if fetched.URL != story.URL {
		t.Errorf("url mismatch: got %q, want %q", fetched.URL, story.URL)
	}

	if len(fetched.Tags) != len(story.Tags) {
		t.Errorf("tags count mismatch: got %d, want %d", len(fetched.Tags), len(story.Tags))
	}

	if fetched.AgentID != story.AgentID {
		t.Errorf("agent_id mismatch: got %q, want %q", fetched.AgentID, story.AgentID)
	}
}

func TestStoryList(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple stories
	for i := 0; i < 5; i++ {
		story := &Story{
			Title: "Test Story",
			Text:  "Content",
			Score: i * 10,
		}
		if err := store.CreateStory(ctx, story); err != nil {
			t.Fatalf("failed to create story %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Test listing
	stories, cursor, err := store.ListStories(ctx, ListOptions{Sort: SortNew, Limit: 10})
	if err != nil {
		t.Fatalf("failed to list stories: %v", err)
	}

	if len(stories) != 5 {
		t.Errorf("expected 5 stories, got %d", len(stories))
	}

	if cursor != "" {
		t.Errorf("expected no cursor for small result set, got %q", cursor)
	}

	// Verify sorted by newest first
	for i := 1; i < len(stories); i++ {
		if stories[i].CreatedAt.After(stories[i-1].CreatedAt) {
			t.Errorf("stories not sorted by newest first")
		}
	}
}

func TestStoryFindByURL(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	story := &Story{
		Title: "Test Story",
		URL:   "https://example.com/unique",
	}

	if err := store.CreateStory(ctx, story); err != nil {
		t.Fatalf("failed to create story: %v", err)
	}

	// Find existing URL
	found, err := store.FindStoryByURL(ctx, story.URL, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("failed to find story: %v", err)
	}

	if found == nil {
		t.Fatal("expected to find story by URL")
	}

	if found.ID != story.ID {
		t.Errorf("found wrong story: got %q, want %q", found.ID, story.ID)
	}

	// Non-existent URL
	found, err = store.FindStoryByURL(ctx, "https://nonexistent.com", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if found != nil {
		t.Error("expected nil for non-existent URL")
	}
}

func TestStoryScore(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	story := &Story{Title: "Test", Text: "Content"}
	if err := store.CreateStory(ctx, story); err != nil {
		t.Fatalf("failed to create story: %v", err)
	}

	// Update score
	if err := store.UpdateStoryScore(ctx, story.ID, 5); err != nil {
		t.Fatalf("failed to update score: %v", err)
	}

	fetched, _ := store.GetStory(ctx, story.ID)
	if fetched.Score != 5 {
		t.Errorf("score mismatch: got %d, want 5", fetched.Score)
	}

	// Update again
	if err := store.UpdateStoryScore(ctx, story.ID, -2); err != nil {
		t.Fatalf("failed to update score: %v", err)
	}

	fetched, _ = store.GetStory(ctx, story.ID)
	if fetched.Score != 3 {
		t.Errorf("score mismatch: got %d, want 3", fetched.Score)
	}
}

func TestStoryHide(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	story := &Story{Title: "Test", Text: "Content"}
	if err := store.CreateStory(ctx, story); err != nil {
		t.Fatalf("failed to create story: %v", err)
	}

	// Hide the story
	if err := store.HideStory(ctx, story.ID); err != nil {
		t.Fatalf("failed to hide story: %v", err)
	}

	// Story should not be found
	fetched, err := store.GetStory(ctx, story.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fetched != nil {
		t.Error("hidden story should not be returned")
	}
}

func TestCommentCreate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a story first
	story := &Story{Title: "Test", Text: "Content"}
	store.CreateStory(ctx, story)

	// Create a comment
	comment := &Comment{
		StoryID: story.ID,
		Text:    "Test comment",
		AgentID: "test-agent",
	}

	if err := store.CreateComment(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.ID == "" {
		t.Error("comment ID should be set after creation")
	}

	// Verify comment was created
	fetched, err := store.GetComment(ctx, comment.ID)
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}

	if fetched.Text != comment.Text {
		t.Errorf("text mismatch: got %q, want %q", fetched.Text, comment.Text)
	}
}

func TestCommentTree(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a story
	story := &Story{Title: "Test", Text: "Content"}
	store.CreateStory(ctx, story)

	// Create root comment
	root := &Comment{StoryID: story.ID, Text: "Root comment"}
	store.CreateComment(ctx, root)

	// Create child comment
	child := &Comment{StoryID: story.ID, ParentID: root.ID, Text: "Child comment"}
	store.CreateComment(ctx, child)

	// Create grandchild comment
	grandchild := &Comment{StoryID: story.ID, ParentID: child.ID, Text: "Grandchild comment"}
	store.CreateComment(ctx, grandchild)

	// Get tree view
	comments, err := store.ListComments(ctx, story.ID, CommentListOptions{
		Sort: SortTop,
		View: ViewTree,
	})
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}

	if len(comments) != 1 {
		t.Errorf("expected 1 root comment, got %d", len(comments))
	}

	if len(comments[0].Children) != 1 {
		t.Errorf("expected 1 child, got %d", len(comments[0].Children))
	}

	if len(comments[0].Children[0].Children) != 1 {
		t.Errorf("expected 1 grandchild, got %d", len(comments[0].Children[0].Children))
	}

	// Get flat view
	flatComments, err := store.ListComments(ctx, story.ID, CommentListOptions{
		Sort: SortTop,
		View: ViewFlat,
	})
	if err != nil {
		t.Fatalf("failed to list flat comments: %v", err)
	}

	if len(flatComments) != 3 {
		t.Errorf("expected 3 flat comments, got %d", len(flatComments))
	}
}

func TestVoteCreate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a story
	story := &Story{Title: "Test", Text: "Content"}
	store.CreateStory(ctx, story)

	// Create a vote
	vote := &Vote{
		TargetType: "story",
		TargetID:   story.ID,
		Value:      1,
		IPHash:     "hash123",
		AgentID:    "test-agent",
	}

	if err := store.CreateVote(ctx, vote); err != nil {
		t.Fatalf("failed to create vote: %v", err)
	}

	// Retrieve the vote
	fetched, err := store.GetVote(ctx, "story", story.ID, "hash123", "test-agent")
	if err != nil {
		t.Fatalf("failed to get vote: %v", err)
	}

	if fetched == nil {
		t.Fatal("expected to find vote")
	}

	if fetched.Value != 1 {
		t.Errorf("value mismatch: got %d, want 1", fetched.Value)
	}
}

func TestVoteUpdate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	story := &Story{Title: "Test", Text: "Content"}
	store.CreateStory(ctx, story)

	vote := &Vote{
		TargetType: "story",
		TargetID:   story.ID,
		Value:      1,
		IPHash:     "hash123",
	}
	store.CreateVote(ctx, vote)

	// Update vote value
	if err := store.UpdateVote(ctx, vote.ID, -1); err != nil {
		t.Fatalf("failed to update vote: %v", err)
	}

	fetched, _ := store.GetVote(ctx, "story", story.ID, "hash123", "")
	if fetched.Value != -1 {
		t.Errorf("value mismatch: got %d, want -1", fetched.Value)
	}
}

func TestAccountCreate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	account := &Account{
		DisplayName: "Test Agent",
		Bio:         "A test agent",
		HomepageURL: "https://example.com",
	}

	if err := store.CreateAccount(ctx, account); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	if account.ID == "" {
		t.Error("account ID should be set after creation")
	}

	fetched, err := store.GetAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}

	if fetched.DisplayName != account.DisplayName {
		t.Errorf("display_name mismatch: got %q, want %q", fetched.DisplayName, account.DisplayName)
	}
}

func TestAccountKeyCreate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create account first
	account := &Account{DisplayName: "Test"}
	store.CreateAccount(ctx, account)

	key := &AccountKey{
		AccountID: account.ID,
		Algorithm: "ed25519",
		PublicKey: "base64encodedkey",
	}

	if err := store.CreateAccountKey(ctx, key); err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	// Get by public key
	fetched, err := store.GetAccountKeyByPublicKey(ctx, "ed25519", "base64encodedkey")
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	if fetched.AccountID != account.ID {
		t.Errorf("account_id mismatch: got %q, want %q", fetched.AccountID, account.ID)
	}

	// List keys
	keys, err := store.ListAccountKeys(ctx, account.ID)
	if err != nil {
		t.Fatalf("failed to list keys: %v", err)
	}

	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestAccountKeyRevoke(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	account := &Account{DisplayName: "Test"}
	store.CreateAccount(ctx, account)

	key := &AccountKey{
		AccountID: account.ID,
		Algorithm: "ed25519",
		PublicKey: "testkey",
	}
	store.CreateAccountKey(ctx, key)

	// Revoke the key
	if err := store.RevokeAccountKey(ctx, key.ID); err != nil {
		t.Fatalf("failed to revoke key: %v", err)
	}

	// Revoked key should not be found by public key
	fetched, err := store.GetAccountKeyByPublicKey(ctx, "ed25519", "testkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fetched != nil {
		t.Error("revoked key should not be returned")
	}
}

func TestChallengeCreateAndGet(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	challenge := &Challenge{
		AgentID:   "test-agent",
		Algorithm: "ed25519",
		Challenge: "randomchallengestring",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	if err := store.CreateChallenge(ctx, challenge); err != nil {
		t.Fatalf("failed to create challenge: %v", err)
	}

	// Get the challenge
	fetched, err := store.GetChallenge(ctx, "randomchallengestring")
	if err != nil {
		t.Fatalf("failed to get challenge: %v", err)
	}

	if fetched == nil {
		t.Fatal("expected to find challenge")
	}

	if fetched.AgentID != challenge.AgentID {
		t.Errorf("agent_id mismatch: got %q, want %q", fetched.AgentID, challenge.AgentID)
	}

	// Delete the challenge
	if err := store.DeleteChallenge(ctx, challenge.ID); err != nil {
		t.Fatalf("failed to delete challenge: %v", err)
	}

	// Should no longer find it
	fetched, _ = store.GetChallenge(ctx, "randomchallengestring")
	if fetched != nil {
		t.Error("deleted challenge should not be returned")
	}
}

func TestTokenCreateAndGet(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	token := &Token{
		AgentID:   "test-agent",
		KeyID:     "key123",
		Token:     "secrettoken",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	if err := store.CreateToken(ctx, token); err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Get the token
	fetched, err := store.GetToken(ctx, "secrettoken")
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	if fetched.AgentID != token.AgentID {
		t.Errorf("agent_id mismatch: got %q, want %q", fetched.AgentID, token.AgentID)
	}
}
