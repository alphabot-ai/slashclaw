package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alphabot-ai/slashclaw/internal/config"
	"github.com/alphabot-ai/slashclaw/internal/store"
)

func setupTestHandler(t *testing.T) (*Handler, *store.SQLiteStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "slashclaw-web-test-*.db")
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
		BaseURL: "http://localhost:8080",
	}

	handler, err := NewHandler(sqliteStore, cfg)
	if err != nil {
		sqliteStore.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create handler: %v", err)
	}

	cleanup := func() {
		sqliteStore.Close()
		os.Remove(tmpFile.Name())
	}

	return handler, sqliteStore, cleanup
}

func TestNewHandler(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	if handler == nil {
		t.Fatal("handler should not be nil")
	}
	if handler.templates == nil {
		t.Fatal("templates should not be nil")
	}
	if len(handler.templates) != 3 {
		t.Errorf("expected 3 templates, got %d", len(handler.templates))
	}
}

func TestHome(t *testing.T) {
	handler, sqliteStore, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create some test stories
	for i := 0; i < 3; i++ {
		story := &store.Story{
			Title: "Test Story",
			URL:   "https://example.com/" + string(rune('a'+i)),
		}
		sqliteStore.CreateStory(context.Background(), story)
	}

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantInBody []string
	}{
		{
			name:       "home page",
			path:       "/",
			wantStatus: http.StatusOK,
			wantInBody: []string{"Slashclaw", "Test Story", "Top", "New", "Discussed"},
		},
		{
			name:       "home with sort=new",
			path:       "/?sort=new",
			wantStatus: http.StatusOK,
			wantInBody: []string{"Slashclaw"},
		},
		{
			name:       "home with sort=discussed",
			path:       "/?sort=discussed",
			wantStatus: http.StatusOK,
			wantInBody: []string{"Slashclaw"},
		},
		{
			name:       "404 for other paths",
			path:       "/notfound",
			wantStatus: http.StatusNotFound,
			wantInBody: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.Home(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()
			for _, want := range tt.wantInBody {
				if !strings.Contains(body, want) {
					t.Errorf("body should contain %q", want)
				}
			}
		})
	}
}

func TestHomeJSON(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler.Home(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"stories"`) {
		t.Error("JSON response should contain stories key")
	}
}

func TestStory(t *testing.T) {
	handler, sqliteStore, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a test story
	story := &store.Story{
		Title: "Test Story Title",
		Text:  "Test story content",
	}
	sqliteStore.CreateStory(context.Background(), story)

	// Create a comment on the story
	comment := &store.Comment{
		StoryID: story.ID,
		Text:    "Test comment",
	}
	sqliteStore.CreateComment(context.Background(), comment)

	tests := []struct {
		name       string
		storyID    string
		wantStatus int
		wantInBody []string
	}{
		{
			name:       "existing story",
			storyID:    story.ID,
			wantStatus: http.StatusOK,
			wantInBody: []string{"Test Story Title", "Test story content", "Test comment"},
		},
		{
			name:       "non-existent story",
			storyID:    "non-existent-id",
			wantStatus: http.StatusNotFound,
			wantInBody: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/story/"+tt.storyID, nil)
			req.SetPathValue("id", tt.storyID)
			rec := httptest.NewRecorder()

			handler.Story(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()
			for _, want := range tt.wantInBody {
				if !strings.Contains(body, want) {
					t.Errorf("body should contain %q", want)
				}
			}
		})
	}
}

func TestStoryJSON(t *testing.T) {
	handler, sqliteStore, cleanup := setupTestHandler(t)
	defer cleanup()

	story := &store.Story{
		Title: "Test Story",
		URL:   "https://example.com",
	}
	sqliteStore.CreateStory(context.Background(), story)

	req := httptest.NewRequest(http.MethodGet, "/story/"+story.ID, nil)
	req.SetPathValue("id", story.ID)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler.Story(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"story"`) {
		t.Error("JSON response should contain story key")
	}
	if !strings.Contains(body, `"comments"`) {
		t.Error("JSON response should contain comments key")
	}
}

func TestSubmit(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/submit", nil)
	rec := httptest.NewRecorder()

	handler.Submit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	wantInBody := []string{"Submit", "Title", "URL", "Text"}
	for _, want := range wantInBody {
		if !strings.Contains(body, want) {
			t.Errorf("body should contain %q", want)
		}
	}
}

func TestSubmitJSON(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/submit", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler.Submit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"fields"`) {
		t.Error("JSON response should contain fields schema")
	}
}

func TestWantsJSON(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		query  string
		want   bool
	}{
		{"no header", "", "", false},
		{"html accept", "text/html", "", false},
		{"json accept", "application/json", "", true},
		{"json query param", "", "format=json", true},
		{"mixed", "text/html", "format=json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			got := wantsJSON(req)
			if got != tt.want {
				t.Errorf("wantsJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}
