package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alphabot-ai/slashclaw/internal/auth"
	"github.com/alphabot-ai/slashclaw/internal/config"
	"github.com/alphabot-ai/slashclaw/internal/ratelimit"
	"github.com/alphabot-ai/slashclaw/internal/store"
)

type testServer struct {
	handler *Handler
	store   *store.SQLiteStore
	cleanup func()
}

func setupTestServer(t *testing.T) *testServer {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "slashclaw-api-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	sqliteStore, err := store.NewSQLiteStore(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	cfg := &config.Config{
		StoryRateLimit:   100,
		CommentRateLimit: 100,
		VoteRateLimit:    100,
		RateLimitWindow:  time.Hour,
		ChallengeTTL:     5 * time.Minute,
		TokenTTL:         24 * time.Hour,
		DuplicateWindow:  30 * 24 * time.Hour,
		AdminSecret:      "test-admin-secret",
	}

	limiter := ratelimit.NewMemoryLimiter()
	authService := auth.NewService(sqliteStore, cfg.ChallengeTTL, cfg.TokenTTL)
	handler := NewHandler(sqliteStore, authService, limiter, cfg)

	cleanup := func() {
		sqliteStore.Close()
		os.Remove(tmpFile.Name())
	}

	return &testServer{
		handler: handler,
		store:   sqliteStore,
		cleanup: cleanup,
	}
}

func TestCreateStoryAPI(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
		wantError  bool
	}{
		{
			name: "valid link story",
			body: map[string]any{
				"title": "Test Story Title",
				"url":   "https://example.com",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid text story",
			body: map[string]any{
				"title": "Test Text Post",
				"text":  "This is the content",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid story with tags",
			body: map[string]any{
				"title": "Test Story With Tags",
				"url":   "https://example.com/tags",
				"tags":  []string{"test", "example"},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "title too short",
			body: map[string]any{
				"title": "Short",
				"url":   "https://example.com",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "missing url and text",
			body: map[string]any{
				"title": "Test Story Title",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "both url and text",
			body: map[string]any{
				"title": "Test Story Title",
				"url":   "https://example.com",
				"text":  "Also has text",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "invalid url",
			body: map[string]any{
				"title": "Test Story Title",
				"url":   "not-a-valid-url",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "too many tags",
			body: map[string]any{
				"title": "Test Story Title",
				"url":   "https://example.com/too-many-tags-test",
				"tags":  []string{"1", "2", "3", "4", "5", "6"},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/stories", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			ts.handler.CreateStory(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var resp map[string]any
			json.Unmarshal(rec.Body.Bytes(), &resp)

			if tt.wantError {
				if _, ok := resp["error"]; !ok {
					t.Error("expected error in response")
				}
			} else {
				if _, ok := resp["id"]; !ok {
					t.Error("expected id in response")
				}
			}
		})
	}
}

func TestDuplicateURLDetection(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create first story
	body1, _ := json.Marshal(map[string]any{
		"title": "Original Story",
		"url":   "https://example.com/duplicate",
	})
	req1 := httptest.NewRequest(http.MethodPost, "/api/stories", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	ts.handler.CreateStory(rec1, req1)

	var resp1 CreateStoryResponse
	json.Unmarshal(rec1.Body.Bytes(), &resp1)
	originalID := resp1.ID

	// Try to create duplicate
	body2, _ := json.Marshal(map[string]any{
		"title": "Duplicate Story",
		"url":   "https://example.com/duplicate",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/stories", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	ts.handler.CreateStory(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("duplicate should return 200 OK, got %d", rec2.Code)
	}

	var resp2 CreateStoryResponse
	json.Unmarshal(rec2.Body.Bytes(), &resp2)

	if resp2.ID != originalID {
		t.Errorf("duplicate should return original ID %s, got %s", originalID, resp2.ID)
	}

	if !resp2.Existing {
		t.Error("duplicate should have existing=true")
	}
}

func TestListStoriesAPI(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create some stories
	for i := 0; i < 3; i++ {
		story := &store.Story{
			Title: "Test Story",
			Text:  "Content",
		}
		ts.store.CreateStory(context.Background(), story)
	}

	tests := []struct {
		name       string
		query      string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "default list",
			query:      "",
			wantCount:  3,
			wantStatus: http.StatusOK,
		},
		{
			name:       "sort by new",
			query:      "?sort=new",
			wantCount:  3,
			wantStatus: http.StatusOK,
		},
		{
			name:       "sort by discussed",
			query:      "?sort=discussed",
			wantCount:  3,
			wantStatus: http.StatusOK,
		},
		{
			name:       "limit results",
			query:      "?limit=2",
			wantCount:  2,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/stories"+tt.query, nil)
			rec := httptest.NewRecorder()
			ts.handler.ListStories(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var resp ListStoriesResponse
			json.Unmarshal(rec.Body.Bytes(), &resp)

			if len(resp.Stories) != tt.wantCount {
				t.Errorf("story count = %d, want %d", len(resp.Stories), tt.wantCount)
			}
		})
	}
}

func TestGetStoryAPI(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create a story
	story := &store.Story{Title: "Test Story", Text: "Content"}
	ts.store.CreateStory(context.Background(), story)

	t.Run("existing story", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/stories/"+story.ID, nil)
		req.SetPathValue("id", story.ID)
		rec := httptest.NewRecorder()
		ts.handler.GetStory(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var resp store.Story
		json.Unmarshal(rec.Body.Bytes(), &resp)

		if resp.ID != story.ID {
			t.Errorf("id = %s, want %s", resp.ID, story.ID)
		}
	})

	t.Run("non-existent story", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/stories/nonexistent", nil)
		req.SetPathValue("id", "nonexistent")
		rec := httptest.NewRecorder()
		ts.handler.GetStory(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestCreateCommentAPI(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create a story
	story := &store.Story{Title: "Test Story", Text: "Content"}
	ts.store.CreateStory(context.Background(), story)

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
	}{
		{
			name: "valid comment",
			body: map[string]any{
				"story_id": story.ID,
				"text":     "This is a comment",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing story_id",
			body: map[string]any{
				"text": "This is a comment",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing text",
			body: map[string]any{
				"story_id": story.ID,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "non-existent story",
			body: map[string]any{
				"story_id": "nonexistent",
				"text":     "This is a comment",
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/comments", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			ts.handler.CreateComment(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestVoteAPI(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create a story
	story := &store.Story{Title: "Test Story", Text: "Content"}
	ts.store.CreateStory(context.Background(), story)

	t.Run("upvote story", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"target_type": "story",
			"target_id":   story.ID,
			"value":       1,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/votes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.1:12345"

		rec := httptest.NewRecorder()
		ts.handler.CreateVote(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		// Verify score updated
		updated, _ := ts.store.GetStory(context.Background(), story.ID)
		if updated.Score != 1 {
			t.Errorf("score = %d, want 1", updated.Score)
		}
	})

	t.Run("change vote", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"target_type": "story",
			"target_id":   story.ID,
			"value":       -1,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/votes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.1:12345" // Same IP as before

		rec := httptest.NewRecorder()
		ts.handler.CreateVote(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		// Score should change by -2 (from +1 to -1)
		updated, _ := ts.store.GetStory(context.Background(), story.ID)
		if updated.Score != -1 {
			t.Errorf("score = %d, want -1", updated.Score)
		}
	})

	t.Run("invalid target_type", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"target_type": "invalid",
			"target_id":   story.ID,
			"value":       1,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/votes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		ts.handler.CreateVote(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"target_type": "story",
			"target_id":   story.ID,
			"value":       5,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/votes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		ts.handler.CreateVote(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestAdminHideAPI(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create a story
	story := &store.Story{Title: "Test Story", Text: "Content"}
	ts.store.CreateStory(context.Background(), story)

	t.Run("unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"target_type": "story",
			"target_id":   story.ID,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/hide", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		ts.handler.Hide(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"target_type": "story",
			"target_id":   story.ID,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/hide", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", "test-admin-secret")

		rec := httptest.NewRecorder()
		ts.handler.Hide(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		// Verify story is hidden
		hidden, _ := ts.store.GetStory(context.Background(), story.ID)
		if hidden != nil {
			t.Error("story should be hidden")
		}
	})
}

func TestAgentIDHeader(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	body, _ := json.Marshal(map[string]any{
		"title": "Story from Agent",
		"url":   "https://example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/stories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Id", "test-agent-v1")

	rec := httptest.NewRecorder()
	ts.handler.CreateStory(rec, req)

	var resp CreateStoryResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// Verify agent ID was saved
	story, _ := ts.store.GetStory(context.Background(), resp.ID)
	if story.AgentID != "test-agent-v1" {
		t.Errorf("agent_id = %q, want %q", story.AgentID, "test-agent-v1")
	}
}
