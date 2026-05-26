package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/company/echo/internal/domain"
)

// TestMkdirAll tests that the store creates directories.
func TestMkdirAll(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "subdir", "nested", "echo.db")

	store, err := NewSQLiteFTS5Store(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteFTS5Store() error: %v", err)
	}
	defer store.Close()

	// Verify the directory was created.
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Error("Directory should have been created")
	}
}

// newTestStore creates an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *SQLiteFTS5Store {
	t.Helper()

	store, err := NewSQLiteFTS5Store(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	return store
}

// testLearning creates a test learning with unique ID.
func testLearning(id string) *domain.Learning {
	return &domain.Learning{
		ID:           id,
		Project:      "github.com/test/repo",
		Scope:        domain.ScopeProject,
		Type:         domain.TypeConfig,
		Question:     "How to connect to the database?",
		Answer:       "Set DATABASE_URL=postgresql://localhost:5432/echo",
		Reasoning:    "We use PostgreSQL for relational data",
		Location:     ".env, config/database.go",
		Notes:        "Connection pool max is 20",
		Tags:         []string{"database", "postgresql", "config"},
		ResolvedBy:   "testuser",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
}

func TestSQLiteFTS5Store_SaveAndGetByID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	l := testLearning("test-1")

	id, err := store.Save(ctx, l)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if id != "test-1" {
		t.Errorf("Save() ID = %v, want %v", id, "test-1")
	}

	got, err := store.GetByID(ctx, "test-1")
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}

	if got.Question != l.Question {
		t.Errorf("Question = %v, want %v", got.Question, l.Question)
	}
	if got.Answer != l.Answer {
		t.Errorf("Answer = %v, want %v", got.Answer, l.Answer)
	}
	if got.Project != l.Project {
		t.Errorf("Project = %v, want %v", got.Project, l.Project)
	}
	if len(got.Tags) != len(l.Tags) {
		t.Errorf("Tags length = %v, want %v", len(got.Tags), len(l.Tags))
	}
}

func TestSQLiteFTS5Store_SaveAutoID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	l := testLearning("") // Empty ID should trigger auto-generation.

	id, err := store.Save(ctx, l)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if id == "" {
		t.Error("Save() should return a generated ID")
	}
}

func TestSQLiteFTS5Store_GetByID_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.GetByID(ctx, "nonexistent")
	if err != domain.ErrNotFound {
		t.Errorf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteFTS5Store_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	l := testLearning("test-update")
	_, err := store.Save(ctx, l)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Update the learning.
	l.Answer = "Updated answer"
	l.Notes = "Updated notes"
	l.UpdatedAt = time.Now().UTC()

	err = store.Update(ctx, "test-update", l)
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	got, err := store.GetByID(ctx, "test-update")
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}

	if got.Answer != "Updated answer" {
		t.Errorf("Answer = %v, want %v", got.Answer, "Updated answer")
	}
	if got.Notes != "Updated notes" {
		t.Errorf("Notes = %v, want %v", got.Notes, "Updated notes")
	}
}

func TestSQLiteFTS5Store_Update_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	l := testLearning("nonexistent")
	err := store.Update(ctx, "nonexistent", l)
	if err != domain.ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteFTS5Store_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	l := testLearning("test-delete")
	_, err := store.Save(ctx, l)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	err = store.Delete(ctx, "test-delete")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = store.GetByID(ctx, "test-delete")
	if err != domain.ErrNotFound {
		t.Errorf("GetByID() after delete = %v, want ErrNotFound", err)
	}
}

func TestSQLiteFTS5Store_Delete_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if err != domain.ErrNotFound {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteFTS5Store_GetAlwaysInject(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Save a learning with always_inject=true.
	l1 := testLearning("always-1")
	l1.AlwaysInject = true
	_, err := store.Save(ctx, l1)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Save a learning with always_inject=false.
	l2 := testLearning("normal-1")
	_, err = store.Save(ctx, l2)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Save a learning for a different project.
	l3 := testLearning("other-project")
	l3.AlwaysInject = true
	l3.Project = "github.com/other/repo"
	_, err = store.Save(ctx, l3)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	results, err := store.GetAlwaysInject(ctx, "github.com/test/repo")
	if err != nil {
		t.Fatalf("GetAlwaysInject() error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("GetAlwaysInject() returned %d results, want 1", len(results))
	}
	if results[0].ID != "always-1" {
		t.Errorf("GetAlwaysInject() ID = %v, want always-1", results[0].ID)
	}
}

func TestSQLiteFTS5Store_Search_FTS5(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Save multiple learnings.
	learnings := []*domain.Learning{
		{
			ID:         "search-1",
			Project:    "github.com/test/repo",
			Scope:      domain.ScopeProject,
			Type:       domain.TypeConfig,
			Question:   "How to connect to the database?",
			Answer:     "Set DATABASE_URL=postgresql://localhost:5432/echo",
			Reasoning:  "We use PostgreSQL",
			Notes:      "Connection pool max is 20",
			Tags:       []string{"database", "postgresql"},
			ResolvedBy: "testuser",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		},
		{
			ID:         "search-2",
			Project:    "github.com/test/repo",
			Scope:      domain.ScopeProject,
			Type:       domain.TypeBugfix,
			Question:   "Fix 502 Bad Gateway error",
			Answer:     "Restart the nginx service",
			Reasoning:  "Nginx was stuck",
			Notes:      "Check /var/log/nginx/error.log",
			Tags:       []string{"bugfix", "nginx"},
			ResolvedBy: "testuser",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		},
		{
			ID:         "search-3",
			Project:    "github.com/other/repo",
			Scope:      domain.ScopeProject,
			Type:       domain.TypeConfig,
			Question:   "How to connect to Redis?",
			Answer:     "Set REDIS_URL=redis://localhost:6379",
			Reasoning:  "We use Redis for caching",
			Notes:      "",
			Tags:       []string{"redis", "cache"},
			ResolvedBy: "testuser",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		},
	}

	for _, l := range learnings {
		_, err := store.Save(ctx, l)
		if err != nil {
			t.Fatalf("Save() error for %s: %v", l.ID, err)
		}
	}

	tests := []struct {
		name        string
		query       string
		wantCount   int
		wantFirstID string
	}{
		{
			name:        "search for database",
			query:       "database",
			wantCount:   1, // Only search-1 matches (search-3 is different project)
			wantFirstID: "search-1",
		},
		{
			name:        "search for nginx",
			query:       "nginx",
			wantCount:   1,
			wantFirstID: "search-2",
		},
		{
			name:      "search for postgresql",
			query:     "postgresql",
			wantCount: 1,
		},
		{
			name:      "no results for random term",
			query:     "xyzrandom123",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.Search(ctx, &domain.SearchQuery{
				Project: "github.com/test/repo",
				Query:   tt.query,
				Limit:   5,
			})
			if err != nil {
				t.Fatalf("Search() error: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Search() returned %d results, want %d", len(results), tt.wantCount)
				return
			}

			if tt.wantCount > 0 && tt.wantFirstID != "" {
				if results[0].Learning.ID != tt.wantFirstID {
					t.Errorf("Search() first ID = %v, want %v", results[0].Learning.ID, tt.wantFirstID)
				}
				if results[0].RelevanceScore <= 0 {
					t.Errorf("Search() RelevanceScore = %v, want > 0", results[0].RelevanceScore)
				}
			}
		})
	}
}

func TestSQLiteFTS5Store_Search_Fallback(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	l := testLearning("fallback-1")
	l.Question = "How to configure the database connection pool?"
	_, err := store.Save(ctx, l)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Search with a term that should match via LIKE fallback.
	results, err := store.Search(ctx, &domain.SearchQuery{
		Project: "github.com/test/repo",
		Query:   "connection pool",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("Search() returned %d results, want >= 1", len(results))
	}
}

func TestSQLiteFTS5Store_Search_ProjectIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Save learnings for two different projects.
	l1 := testLearning("proj-a-1")
	l1.Project = "github.com/proj-a/repo"
	l1.Question = "How to deploy?"
	_, err := store.Save(ctx, l1)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	l2 := testLearning("proj-b-1")
	l2.Project = "github.com/proj-b/repo"
	l2.Question = "How to deploy?"
	_, err = store.Save(ctx, l2)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Search for proj-a should only return proj-a results.
	resultsA, err := store.Search(ctx, &domain.SearchQuery{
		Project: "github.com/proj-a/repo",
		Query:   "deploy",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	for _, r := range resultsA {
		if r.Learning.Project != "github.com/proj-a/repo" {
			t.Errorf("Search() returned result from project %v, want github.com/proj-a/repo", r.Learning.Project)
		}
	}

	// Same for proj-b.
	resultsB, err := store.Search(ctx, &domain.SearchQuery{
		Project: "github.com/proj-b/repo",
		Query:   "deploy",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	for _, r := range resultsB {
		if r.Learning.Project != "github.com/proj-b/repo" {
			t.Errorf("Search() returned result from project %v, want github.com/proj-b/repo", r.Learning.Project)
		}
	}
}

func TestSQLiteFTS5Store_Search_DefaultLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Save 10 learnings.
	for i := 0; i < 10; i++ {
		l := testLearning("limit-" + string(rune('0'+i)))
		l.Question = "Test learning number"
		_, err := store.Save(ctx, l)
		if err != nil {
			t.Fatalf("Save() error: %v", err)
		}
	}

	// Search with limit=0 should default to 5.
	results, err := store.Search(ctx, &domain.SearchQuery{
		Project: "github.com/test/repo",
		Query:   "test",
		Limit:   0, // Should default to 5
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) > 5 {
		t.Errorf("Search() returned %d results with limit=0, want <= 5", len(results))
	}
}
