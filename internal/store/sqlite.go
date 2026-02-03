package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS stories (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		url TEXT,
		text TEXT,
		tags TEXT,
		score INTEGER DEFAULT 0,
		comment_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		hidden INTEGER DEFAULT 0,
		agent_id TEXT,
		agent_verified INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_stories_url ON stories(url) WHERE url IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_stories_created_at ON stories(created_at);
	CREATE INDEX IF NOT EXISTS idx_stories_score ON stories(score);

	CREATE TABLE IF NOT EXISTS comments (
		id TEXT PRIMARY KEY,
		story_id TEXT NOT NULL,
		parent_id TEXT,
		text TEXT NOT NULL,
		score INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		hidden INTEGER DEFAULT 0,
		agent_id TEXT,
		agent_verified INTEGER DEFAULT 0,
		FOREIGN KEY (story_id) REFERENCES stories(id)
	);

	CREATE INDEX IF NOT EXISTS idx_comments_story_id ON comments(story_id);
	CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id);

	CREATE TABLE IF NOT EXISTS votes (
		id TEXT PRIMARY KEY,
		target_type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		value INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		ip_hash TEXT,
		agent_id TEXT,
		agent_verified INTEGER DEFAULT 0,
		UNIQUE(target_type, target_id, ip_hash, agent_id)
	);

	CREATE INDEX IF NOT EXISTS idx_votes_target ON votes(target_type, target_id);

	CREATE TABLE IF NOT EXISTS accounts (
		id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		bio TEXT,
		homepage_url TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS account_keys (
		id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL,
		algorithm TEXT NOT NULL,
		public_key TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		revoked_at DATETIME,
		FOREIGN KEY (account_id) REFERENCES accounts(id),
		UNIQUE(algorithm, public_key)
	);

	CREATE INDEX IF NOT EXISTS idx_account_keys_account ON account_keys(account_id);
	CREATE INDEX IF NOT EXISTS idx_account_keys_pubkey ON account_keys(algorithm, public_key);

	CREATE TABLE IF NOT EXISTS challenges (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		algorithm TEXT NOT NULL,
		challenge TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_challenges_challenge ON challenges(challenge);

	CREATE TABLE IF NOT EXISTS tokens (
		id TEXT PRIMARY KEY,
		account_id TEXT,
		key_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Stories

func (s *SQLiteStore) CreateStory(ctx context.Context, story *Story) error {
	if story.ID == "" {
		story.ID = uuid.New().String()
	}
	if story.CreatedAt.IsZero() {
		story.CreatedAt = time.Now().UTC()
	}

	tagsJSON, _ := json.Marshal(story.Tags)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stories (id, title, url, text, tags, score, comment_count, created_at, hidden, agent_id, agent_verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, story.ID, story.Title, nullString(story.URL), nullString(story.Text), string(tagsJSON),
		story.Score, story.CommentCount, story.CreatedAt, boolToInt(story.Hidden),
		nullString(story.AgentID), boolToInt(story.AgentVerified))

	return err
}

func (s *SQLiteStore) GetStory(ctx context.Context, id string) (*Story, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, url, text, tags, score, comment_count, created_at, hidden, agent_id, agent_verified
		FROM stories WHERE id = ? AND hidden = 0
	`, id)

	story, err := scanStory(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return story, err
}

func (s *SQLiteStore) ListStories(ctx context.Context, opts ListOptions) ([]*Story, string, error) {
	if opts.Limit <= 0 || opts.Limit > 100 {
		opts.Limit = 30
	}

	var orderBy string
	switch opts.Sort {
	case SortNew:
		orderBy = "created_at DESC"
	case SortDiscussed:
		orderBy = "comment_count DESC, created_at DESC"
	default: // SortTop
		// Time-decay ranking: score / (hours + 2)^1.5
		// Simplified: using (hours + 2) * sqrt(hours + 2) as approximation for (hours + 2)^1.5
		// Or just use score - hours for MVP simplicity
		orderBy = "score - (CAST((julianday('now') - julianday(created_at)) * 24 AS REAL)) DESC"
	}

	query := fmt.Sprintf(`
		SELECT id, title, url, text, tags, score, comment_count, created_at, hidden, agent_id, agent_verified
		FROM stories WHERE hidden = 0
		ORDER BY %s
		LIMIT ?
	`, orderBy)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var stories []*Story
	for rows.Next() {
		story, err := scanStoryRows(rows)
		if err != nil {
			return nil, "", err
		}
		stories = append(stories, story)
	}

	var nextCursor string
	if len(stories) > opts.Limit {
		stories = stories[:opts.Limit]
		nextCursor = stories[len(stories)-1].ID
	}

	return stories, nextCursor, nil
}

func (s *SQLiteStore) FindStoryByURL(ctx context.Context, url string, since time.Time) (*Story, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, url, text, tags, score, comment_count, created_at, hidden, agent_id, agent_verified
		FROM stories WHERE url = ? AND created_at > ? AND hidden = 0
		ORDER BY created_at DESC LIMIT 1
	`, url, since)

	story, err := scanStory(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return story, err
}

func (s *SQLiteStore) GetLastStoryByAgent(ctx context.Context, agentID string) (*Story, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, url, text, tags, score, comment_count, created_at, hidden, agent_id, agent_verified
		FROM stories WHERE agent_id = ?
		ORDER BY created_at DESC LIMIT 1
	`, agentID)

	story, err := scanStory(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return story, err
}

func (s *SQLiteStore) UpdateStoryScore(ctx context.Context, id string, delta int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE stories SET score = score + ? WHERE id = ?`, delta, id)
	return err
}

func (s *SQLiteStore) UpdateStoryCommentCount(ctx context.Context, id string, delta int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE stories SET comment_count = comment_count + ? WHERE id = ?`, delta, id)
	return err
}

func (s *SQLiteStore) HideStory(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE stories SET hidden = 1 WHERE id = ?`, id)
	return err
}

// Comments

func (s *SQLiteStore) CreateComment(ctx context.Context, comment *Comment) error {
	if comment.ID == "" {
		comment.ID = uuid.New().String()
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO comments (id, story_id, parent_id, text, score, created_at, hidden, agent_id, agent_verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, comment.ID, comment.StoryID, nullString(comment.ParentID), comment.Text,
		comment.Score, comment.CreatedAt, boolToInt(comment.Hidden),
		nullString(comment.AgentID), boolToInt(comment.AgentVerified))

	return err
}

func (s *SQLiteStore) GetComment(ctx context.Context, id string) (*Comment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, story_id, parent_id, text, score, created_at, hidden, agent_id, agent_verified
		FROM comments WHERE id = ? AND hidden = 0
	`, id)

	comment, err := scanComment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return comment, err
}

func (s *SQLiteStore) ListComments(ctx context.Context, storyID string, opts CommentListOptions) ([]*Comment, error) {
	var orderBy string
	switch opts.Sort {
	case SortNew:
		orderBy = "created_at DESC"
	default:
		orderBy = "score DESC, created_at ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, story_id, parent_id, text, score, created_at, hidden, agent_id, agent_verified
		FROM comments WHERE story_id = ? AND hidden = 0
		ORDER BY %s
	`, orderBy)

	rows, err := s.db.QueryContext(ctx, query, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		comment, err := scanCommentRows(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}

	if opts.View == ViewTree {
		return buildCommentTree(comments), nil
	}

	return comments, nil
}

func buildCommentTree(comments []*Comment) []*Comment {
	byID := make(map[string]*Comment)
	for _, c := range comments {
		byID[c.ID] = c
	}

	var roots []*Comment
	for _, c := range comments {
		if c.ParentID == "" {
			roots = append(roots, c)
		} else if parent, ok := byID[c.ParentID]; ok {
			parent.Children = append(parent.Children, c)
		}
	}

	return roots
}

func (s *SQLiteStore) UpdateCommentScore(ctx context.Context, id string, delta int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE comments SET score = score + ? WHERE id = ?`, delta, id)
	return err
}

func (s *SQLiteStore) HideComment(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE comments SET hidden = 1 WHERE id = ?`, id)
	return err
}

// Votes

func (s *SQLiteStore) CreateVote(ctx context.Context, vote *Vote) error {
	if vote.ID == "" {
		vote.ID = uuid.New().String()
	}
	if vote.CreatedAt.IsZero() {
		vote.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO votes (id, target_type, target_id, value, created_at, ip_hash, agent_id, agent_verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, vote.ID, vote.TargetType, vote.TargetID, vote.Value, vote.CreatedAt,
		nullString(vote.IPHash), nullString(vote.AgentID), boolToInt(vote.AgentVerified))

	return err
}

func (s *SQLiteStore) GetVote(ctx context.Context, targetType, targetID, ipHash, agentID string) (*Vote, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, target_type, target_id, value, created_at, ip_hash, agent_id, agent_verified
		FROM votes WHERE target_type = ? AND target_id = ? AND (ip_hash = ? OR agent_id = ?)
	`, targetType, targetID, ipHash, agentID)

	var vote Vote
	var ipHashNull, agentIDNull sql.NullString
	err := row.Scan(&vote.ID, &vote.TargetType, &vote.TargetID, &vote.Value, &vote.CreatedAt,
		&ipHashNull, &agentIDNull, &vote.AgentVerified)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	vote.IPHash = ipHashNull.String
	vote.AgentID = agentIDNull.String
	return &vote, nil
}

func (s *SQLiteStore) UpdateVote(ctx context.Context, id string, value int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE votes SET value = ? WHERE id = ?`, value, id)
	return err
}

// Accounts

func (s *SQLiteStore) CreateAccount(ctx context.Context, account *Account) error {
	if account.ID == "" {
		account.ID = uuid.New().String()
	}
	if account.CreatedAt.IsZero() {
		account.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO accounts (id, display_name, bio, homepage_url, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, account.ID, account.DisplayName, nullString(account.Bio),
		nullString(account.HomepageURL), account.CreatedAt)

	return err
}

func (s *SQLiteStore) GetAccount(ctx context.Context, id string) (*Account, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, display_name, bio, homepage_url, created_at
		FROM accounts WHERE id = ?
	`, id)

	var account Account
	var bio, homepageURL sql.NullString
	err := row.Scan(&account.ID, &account.DisplayName, &bio, &homepageURL, &account.CreatedAt)
	if err != nil {
		return nil, err
	}

	account.Bio = bio.String
	account.HomepageURL = homepageURL.String
	return &account, nil
}

// Account Keys

func (s *SQLiteStore) CreateAccountKey(ctx context.Context, key *AccountKey) error {
	if key.ID == "" {
		key.ID = uuid.New().String()
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_keys (id, account_id, algorithm, public_key, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, key.ID, key.AccountID, key.Algorithm, key.PublicKey, key.CreatedAt, nil)

	return err
}

func (s *SQLiteStore) GetAccountKey(ctx context.Context, id string) (*AccountKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, algorithm, public_key, created_at, revoked_at
		FROM account_keys WHERE id = ?
	`, id)

	return scanAccountKey(row)
}

func (s *SQLiteStore) GetAccountKeyByPublicKey(ctx context.Context, alg, publicKey string) (*AccountKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, algorithm, public_key, created_at, revoked_at
		FROM account_keys WHERE algorithm = ? AND public_key = ? AND revoked_at IS NULL
	`, alg, publicKey)

	key, err := scanAccountKey(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return key, err
}

func (s *SQLiteStore) ListAccountKeys(ctx context.Context, accountID string) ([]*AccountKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, algorithm, public_key, created_at, revoked_at
		FROM account_keys WHERE account_id = ?
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*AccountKey
	for rows.Next() {
		var key AccountKey
		var revokedAt sql.NullTime
		err := rows.Scan(&key.ID, &key.AccountID, &key.Algorithm, &key.PublicKey, &key.CreatedAt, &revokedAt)
		if err != nil {
			return nil, err
		}
		if revokedAt.Valid {
			key.RevokedAt = &revokedAt.Time
		}
		keys = append(keys, &key)
	}

	return keys, nil
}

func (s *SQLiteStore) RevokeAccountKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE account_keys SET revoked_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

// Auth

func (s *SQLiteStore) CreateChallenge(ctx context.Context, challenge *Challenge) error {
	if challenge.ID == "" {
		challenge.ID = uuid.New().String()
	}

	// Format time in SQLite-compatible format for proper datetime comparison
	expiresAtStr := challenge.ExpiresAt.UTC().Format("2006-01-02 15:04:05")

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO challenges (id, agent_id, algorithm, challenge, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, challenge.ID, challenge.AgentID, challenge.Algorithm, challenge.Challenge, expiresAtStr)

	return err
}

func (s *SQLiteStore) GetChallenge(ctx context.Context, challengeStr string) (*Challenge, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, algorithm, challenge, expires_at
		FROM challenges WHERE challenge = ? AND expires_at > datetime('now')
	`, challengeStr)

	var c Challenge
	err := row.Scan(&c.ID, &c.AgentID, &c.Algorithm, &c.Challenge, &c.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (s *SQLiteStore) DeleteChallenge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM challenges WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) CreateToken(ctx context.Context, token *Token) error {
	if token.ID == "" {
		token.ID = uuid.New().String()
	}

	// Format time in SQLite-compatible format for proper datetime comparison
	expiresAtStr := token.ExpiresAt.UTC().Format("2006-01-02 15:04:05")

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tokens (id, account_id, key_id, agent_id, token, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, token.ID, nullString(token.AccountID), token.KeyID, token.AgentID, token.Token, expiresAtStr)

	return err
}

func (s *SQLiteStore) GetToken(ctx context.Context, tokenStr string) (*Token, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, key_id, agent_id, token, expires_at
		FROM tokens WHERE token = ? AND expires_at > datetime('now')
	`, tokenStr)

	var t Token
	var accountID sql.NullString
	err := row.Scan(&t.ID, &accountID, &t.KeyID, &t.AgentID, &t.Token, &t.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.AccountID = accountID.String
	return &t, nil
}

func (s *SQLiteStore) DeleteExpiredTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE expires_at < datetime('now')`)
	return err
}

// Helpers

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanStory(row *sql.Row) (*Story, error) {
	var story Story
	var url, text, tags, agentID sql.NullString
	var hidden, agentVerified int

	err := row.Scan(&story.ID, &story.Title, &url, &text, &tags, &story.Score,
		&story.CommentCount, &story.CreatedAt, &hidden, &agentID, &agentVerified)
	if err != nil {
		return nil, err
	}

	story.URL = url.String
	story.Text = text.String
	story.AgentID = agentID.String
	story.Hidden = hidden == 1
	story.AgentVerified = agentVerified == 1

	if tags.Valid && tags.String != "" {
		json.Unmarshal([]byte(tags.String), &story.Tags)
	}

	return &story, nil
}

func scanStoryRows(rows *sql.Rows) (*Story, error) {
	var story Story
	var url, text, tags, agentID sql.NullString
	var hidden, agentVerified int

	err := rows.Scan(&story.ID, &story.Title, &url, &text, &tags, &story.Score,
		&story.CommentCount, &story.CreatedAt, &hidden, &agentID, &agentVerified)
	if err != nil {
		return nil, err
	}

	story.URL = url.String
	story.Text = text.String
	story.AgentID = agentID.String
	story.Hidden = hidden == 1
	story.AgentVerified = agentVerified == 1

	if tags.Valid && tags.String != "" {
		json.Unmarshal([]byte(tags.String), &story.Tags)
	}

	return &story, nil
}

func scanComment(row *sql.Row) (*Comment, error) {
	var comment Comment
	var parentID, agentID sql.NullString
	var hidden, agentVerified int

	err := row.Scan(&comment.ID, &comment.StoryID, &parentID, &comment.Text, &comment.Score,
		&comment.CreatedAt, &hidden, &agentID, &agentVerified)
	if err != nil {
		return nil, err
	}

	comment.ParentID = parentID.String
	comment.AgentID = agentID.String
	comment.Hidden = hidden == 1
	comment.AgentVerified = agentVerified == 1

	return &comment, nil
}

func scanCommentRows(rows *sql.Rows) (*Comment, error) {
	var comment Comment
	var parentID, agentID sql.NullString
	var hidden, agentVerified int

	err := rows.Scan(&comment.ID, &comment.StoryID, &parentID, &comment.Text, &comment.Score,
		&comment.CreatedAt, &hidden, &agentID, &agentVerified)
	if err != nil {
		return nil, err
	}

	comment.ParentID = parentID.String
	comment.AgentID = agentID.String
	comment.Hidden = hidden == 1
	comment.AgentVerified = agentVerified == 1

	return &comment, nil
}

func scanAccountKey(row *sql.Row) (*AccountKey, error) {
	var key AccountKey
	var revokedAt sql.NullTime

	err := row.Scan(&key.ID, &key.AccountID, &key.Algorithm, &key.PublicKey, &key.CreatedAt, &revokedAt)
	if err != nil {
		return nil, err
	}

	if revokedAt.Valid {
		key.RevokedAt = &revokedAt.Time
	}

	return &key, nil
}

// Ensure SQLiteStore implements Store
var _ Store = (*SQLiteStore)(nil)
