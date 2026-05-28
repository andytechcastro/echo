package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/infrastructure/store"
	"github.com/company/echo/internal/usecase"
)

// --- Mock detectors ---

type mockProjectDetector struct {
	project string
	err     error
}

func (m *mockProjectDetector) Detect() (string, error) {
	return m.project, m.err
}

type mockIdentityDetector struct {
	identity string
	err      error
}

func (m *mockIdentityDetector) Detect() (string, error) {
	return m.identity, m.err
}

// noopEmbedder implements domain.Embedder returning nil/0 (Phase 1 behavior).
type noopEmbedder struct{}

func (n *noopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (n *noopEmbedder) Dimensions() int                                      { return 0 }
func (n *noopEmbedder) Provider() string                                     { return "noop" }

// --- Test environment ---

type testEnv struct {
	store    *store.SQLiteFTS5Store
	projDet  *mockProjectDetector
	identDet *mockIdentityDetector
	embedder *noopEmbedder
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := store.NewSQLiteFTS5Store(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return &testEnv{
		store:    s,
		projDet:  &mockProjectDetector{project: "github.com/test/echo"},
		identDet: &mockIdentityDetector{identity: "andres"},
		embedder: &noopEmbedder{},
	}
}

func saveViaStore(t *testing.T, env *testEnv, l *domain.Learning) string {
	t.Helper()
	id, err := env.store.Save(context.Background(), l)
	if err != nil {
		t.Fatalf("store.Save() error: %v", err)
	}
	return id
}

func searchViaStore(t *testing.T, env *testEnv, query string, tags []string) []domain.SearchResult {
	t.Helper()
	results, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: env.projDet.project,
		Query:   query,
		Tags:    tags,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("store.Search() error: %v", err)
	}
	return results
}

// --- 1. Full save → search flow ---

func TestE2E_SaveAndSearch(t *testing.T) {
	env := setupTestEnv(t)

	// Save.
	id := saveViaStore(t, env, &domain.Learning{
		ID:         "learn-1",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "How to configure the database connection pool?",
		Answer:     "Set DATABASE_URL and POOL_SIZE in .env",
		Reasoning:  "PostgreSQL connection pooling improves throughput",
		Location:   "config/db.go",
		Notes:      "Default pool size is 20",
		Tags:       []string{"database", "postgresql", "config"},
		ResolvedBy: "andres",
	})
	if id != "learn-1" {
		t.Errorf("Save() ID = %q, want %q", id, "learn-1")
	}

	// Search.
	results := searchViaStore(t, env, "database connection pool", nil)

	if len(results) < 1 {
		t.Fatalf("Search() returned 0 results, want >= 1")
	}
	if results[0].Learning.ID != "learn-1" {
		t.Errorf("Search() top result ID = %q, want %q", results[0].Learning.ID, "learn-1")
	}
	if results[0].RelevanceScore <= 0 {
		t.Errorf("Search() RelevanceScore = %v, want > 0", results[0].RelevanceScore)
	}
	if results[0].Learning.Answer != "Set DATABASE_URL and POOL_SIZE in .env" {
		t.Errorf("Search() answer = %q, want correct answer", results[0].Learning.Answer)
	}
}

// --- 2. Project isolation ---

func TestE2E_ProjectIsolation(t *testing.T) {
	env := setupTestEnv(t)

	// Save for project A.
	saveViaStore(t, env, &domain.Learning{
		ID:         "proj-a-1",
		Project:    "github.com/company/project-a",
		Scope:      domain.ScopeProject,
		Type:       domain.TypeBugfix,
		Question:   "Fix memory leak in worker",
		Answer:     "Close the channel after processing",
		ResolvedBy: "andres",
	})

	// Save for project B.
	saveViaStore(t, env, &domain.Learning{
		ID:         "proj-b-1",
		Project:    "github.com/company/project-b",
		Scope:      domain.ScopeProject,
		Type:       domain.TypeBugfix,
		Question:   "Fix memory leak in handler",
		Answer:     "Defer the cleanup function",
		ResolvedBy: "andres",
	})

	// Search project A — should only return project A results.
	resultsA, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: "github.com/company/project-a",
		Query:   "memory leak",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	for _, r := range resultsA {
		if r.Learning.Project != "github.com/company/project-a" {
			t.Errorf("Search() returned result from project %q, want project-a", r.Learning.Project)
		}
	}

	// Search project B — should only return project B results.
	resultsB, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: "github.com/company/project-b",
		Query:   "memory leak",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	for _, r := range resultsB {
		if r.Learning.Project != "github.com/company/project-b" {
			t.Errorf("Search() returned result from project %q, want project-b", r.Learning.Project)
		}
	}
}

// --- 3. Duplicate detection (lexical) ---

func TestE2E_DuplicateDetection(t *testing.T) {
	env := setupTestEnv(t)

	// First save succeeds.
	saveViaUsecase(t, env, &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to set up CORS in Express?",
		Answer:   "Use the cors middleware package",
	})

	// Second save with same question fails.
	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to set up CORS in Express?",
		Answer:   "Different answer this time",
	}
	uc := usecase.NewSaveLearning(env.store, env.embedder, env.projDet, env.identDet)
	_, err := uc.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute() should return error for duplicate question")
	}
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("Execute() error = %v, want ErrDuplicate", err)
	}
}

// --- 4. Secret detection end-to-end ---

func TestE2E_SecretDetection(t *testing.T) {
	env := setupTestEnv(t)
	uc := usecase.NewSaveLearning(env.store, env.embedder, env.projDet, env.identDet)

	tests := []struct {
		name    string
		input   *domain.SaveInput
		wantErr bool
	}{
		{
			name: "clean answer",
			input: &domain.SaveInput{
				Type:     domain.TypeConfig,
				Question: "How to configure API?",
				Answer:   "Set the base URL in config",
			},
			wantErr: false,
		},
		{
			name: "API key in answer",
			input: &domain.SaveInput{
				Type:     domain.TypeConfig,
				Question: "What is the API key?",
				Answer:   "Use sk-abcdefghijklmnopqrstuvwxyz1234567890",
			},
			wantErr: true,
		},
		{
			name: "AWS key in reasoning",
			input: &domain.SaveInput{
				Type:      domain.TypeConfig,
				Question:  "How to auth with AWS?",
				Answer:    "Use IAM roles",
				Reasoning: "The key is AKIAIOSFODNN7EXAMPLE1",
			},
			wantErr: true,
		},
		{
			name: "GitHub token in notes",
			input: &domain.SaveInput{
				Type:     domain.TypeConfig,
				Question: "How to clone private repo?",
				Answer:   "Use gh auth login",
				Notes:    "Token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			},
			wantErr: true,
		},
		{
			name: "JWT in answer",
			input: &domain.SaveInput{
				Type:     domain.TypeConfig,
				Question: "What does the token look like?",
				Answer:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc",
			},
			wantErr: true,
		},
		{
			name: "password in answer",
			input: &domain.SaveInput{
				Type:     domain.TypeConfig,
				Question: "What is the db password?",
				Answer:   "password: SuperSecretPassword123!",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.Execute(context.Background(), tt.input)
			if tt.wantErr && err == nil {
				t.Fatal("Execute() should return error when secret is detected")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
		})
	}
}

// --- 5. Always-inject policies ---

func TestE2E_AlwaysInjectPolicies(t *testing.T) {
	env := setupTestEnv(t)

	// Save a policy with always_inject=true.
	saveViaStore(t, env, &domain.Learning{
		ID:           "policy-1",
		Project:      env.projDet.project,
		Scope:        domain.ScopeProject,
		AlwaysInject: true,
		Type:         domain.TypeProcess,
		Question:     "Deployment window policy",
		Answer:       "Deployments only allowed Tuesday through Thursday",
		ResolvedBy:   "andres",
	})

	// Save a normal learning (always_inject=false).
	saveViaStore(t, env, &domain.Learning{
		ID:           "normal-1",
		Project:      env.projDet.project,
		Scope:        domain.ScopeProject,
		AlwaysInject: false,
		Type:         domain.TypeConfig,
		Question:     "How to configure logging?",
		Answer:       "Use structured JSON logging",
		ResolvedBy:   "andres",
	})

	// Save a policy for a different project.
	saveViaStore(t, env, &domain.Learning{
		ID:           "policy-other",
		Project:      "github.com/other/repo",
		Scope:        domain.ScopeProject,
		AlwaysInject: true,
		Type:         domain.TypeProcess,
		Question:     "Other project policy",
		Answer:       "Other policy content",
		ResolvedBy:   "andres",
	})

	// Get policies for current project.
	policies, err := env.store.GetAlwaysInject(context.Background(), env.projDet.project)
	if err != nil {
		t.Fatalf("GetAlwaysInject() error: %v", err)
	}

	if len(policies) != 1 {
		t.Fatalf("GetAlwaysInject() returned %d policies, want 1", len(policies))
	}
	if policies[0].ID != "policy-1" {
		t.Errorf("GetAlwaysInject() ID = %q, want %q", policies[0].ID, "policy-1")
	}
	if !policies[0].AlwaysInject {
		t.Error("GetAlwaysInject() returned policy with AlwaysInject=false")
	}
}

// --- 6. FTS5 special characters (fallback LIKE) ---

func TestE2E_FTSSpecialCharacters(t *testing.T) {
	env := setupTestEnv(t)

	saveViaStore(t, env, &domain.Learning{
		ID:         "special-1",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeBugfix,
		Question:   "Fix error: cannot connect to database",
		Answer:     "Check the connection string format",
		ResolvedBy: "andres",
	})

	// Search with special characters that could break FTS5 syntax.
	specialQueries := []string{
		"error: cannot",
		"cannot connect (timeout)",
		"database / connection",
	}

	for _, q := range specialQueries {
		t.Run(q, func(t *testing.T) {
			results, err := env.store.Search(context.Background(), &domain.SearchQuery{
				Project: env.projDet.project,
				Query:   q,
				Limit:   5,
			})
			if err != nil {
				t.Fatalf("Search(%q) error: %v", q, err)
			}
			// Should not error; may return 0 or more results via FTS5 or LIKE fallback.
			// The key is that it doesn't panic or return a syntax error.
			_ = results
		})
	}
}

// --- 7. Tags storage and searchability ---

func TestE2E_TagsStorageAndSearch(t *testing.T) {
	env := setupTestEnv(t)

	saveViaStore(t, env, &domain.Learning{
		ID:         "tag-db",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "Database setup guide",
		Answer:     "Follow the migration steps",
		Tags:       []string{"database", "setup"},
		ResolvedBy: "andres",
	})

	saveViaStore(t, env, &domain.Learning{
		ID:         "tag-api",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "API authentication guide",
		Answer:     "Use JWT tokens for auth",
		Tags:       []string{"api", "auth"},
		ResolvedBy: "andres",
	})

	saveViaStore(t, env, &domain.Learning{
		ID:         "tag-both",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "Database API connection",
		Answer:     "Connect to the database via API",
		Tags:       []string{"database", "api"},
		ResolvedBy: "andres",
	})

	// Verify tags are persisted correctly via GetByID.
	db, err := env.store.GetByID(context.Background(), "tag-db")
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}
	if len(db.Tags) != 2 || db.Tags[0] != "database" || db.Tags[1] != "setup" {
		t.Errorf("GetByID() tags = %v, want [database setup]", db.Tags)
	}

	// Verify tags are searchable via FTS (tags column is indexed).
	results, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: env.projDet.project,
		Query:   "database",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("Search() for 'database' returned %d results, want >= 2 (tag-db + tag-both)", len(results))
	}

	// Verify searching for tag term returns correct learning.
	apiResults, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: env.projDet.project,
		Query:   "auth",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(apiResults) < 1 {
		t.Errorf("Search() for 'auth' returned 0 results, want >= 1")
	}
	for _, r := range apiResults {
		if r.Learning.ID == "tag-api" {
			found := false
			for _, tag := range r.Learning.Tags {
				if tag == "auth" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Search() result tag-api has tags %v, expected 'auth' tag", r.Learning.Tags)
			}
		}
	}
}

// --- 8. Update and re-search ---

func TestE2E_UpdateAndReSearch(t *testing.T) {
	env := setupTestEnv(t)

	// Save.
	saveViaStore(t, env, &domain.Learning{
		ID:         "update-1",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "How to configure Redis?",
		Answer:     "Set REDIS_URL=redis://localhost:6379",
		Reasoning:  "Redis is used for caching",
		Location:   "config/redis.go",
		Notes:      "Default port is 6379",
		Tags:       []string{"redis", "cache"},
		ResolvedBy: "andres",
	})

	// Update.
	updated := &domain.Learning{
		ID:         "update-1",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "How to configure Redis?",
		Answer:     "Set REDIS_URL=redis://cluster.example.com:6380",
		Reasoning:  "Redis is used for caching and sessions",
		Location:   "config/redis.go",
		Notes:      "Updated to use cluster mode on port 6380",
		Tags:       []string{"redis", "cache", "cluster"},
		ResolvedBy: "andres",
		UpdatedAt:  time.Now().UTC(),
	}
	if err := env.store.Update(context.Background(), "update-1", updated); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	// Re-search — should return updated content.
	results := searchViaStore(t, env, "redis cluster", nil)

	if len(results) < 1 {
		t.Fatalf("Search() returned 0 results after update, want >= 1")
	}

	found := false
	for _, r := range results {
		if r.Learning.ID == "update-1" {
			found = true
			if r.Learning.Answer != "Set REDIS_URL=redis://cluster.example.com:6380" {
				t.Errorf("Search() answer = %q, want updated answer", r.Learning.Answer)
			}
			if r.Learning.Notes != "Updated to use cluster mode on port 6380" {
				t.Errorf("Search() notes = %q, want updated notes", r.Learning.Notes)
			}
			if len(r.Learning.Tags) != 3 {
				t.Errorf("Search() tags count = %d, want 3", len(r.Learning.Tags))
			}
		}
	}
	if !found {
		t.Error("Search() did not find updated learning")
	}
}

// --- 9. Delete and verify gone ---

func TestE2E_DeleteAndVerifyGone(t *testing.T) {
	env := setupTestEnv(t)

	// Save.
	saveViaStore(t, env, &domain.Learning{
		ID:         "delete-1",
		Project:    env.projDet.project,
		Scope:      domain.ScopeProject,
		Type:       domain.TypeConfig,
		Question:   "How to configure logging?",
		Answer:     "Use structured JSON format",
		ResolvedBy: "andres",
	})

	// Verify it exists.
	results := searchViaStore(t, env, "logging", nil)
	if len(results) < 1 {
		t.Fatal("Search() should find the learning before delete")
	}

	// Delete.
	if err := env.store.Delete(context.Background(), "delete-1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Verify it's gone.
	resultsAfter, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: env.projDet.project,
		Query:   "logging",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("Search() after delete error: %v", err)
	}
	for _, r := range resultsAfter {
		if r.Learning.ID == "delete-1" {
			t.Error("Search() still found deleted learning")
		}
	}

	// GetByID should return ErrNotFound.
	_, err = env.store.GetByID(context.Background(), "delete-1")
	if err != domain.ErrNotFound {
		t.Errorf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

// --- 10. Empty database search ---

func TestE2E_EmptyDatabaseSearch(t *testing.T) {
	env := setupTestEnv(t)

	// Search on empty database — should return empty results, not error.
	results, err := env.store.Search(context.Background(), &domain.SearchQuery{
		Project: env.projDet.project,
		Query:   "anything",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("Search() on empty DB error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search() on empty DB returned %d results, want 0", len(results))
	}
}

// --- Additional: BM25 ranking verification ---

func TestE2E_BM25Ranking(t *testing.T) {
	env := setupTestEnv(t)

	// Save learnings with varying relevance to "database connection".
	learnings := []*domain.Learning{
		{
			ID:         "rank-high",
			Project:    env.projDet.project,
			Scope:      domain.ScopeProject,
			Type:       domain.TypeConfig,
			Question:   "How to set up database connection pooling?",
			Answer:     "Configure the database connection pool with max connections",
			Reasoning:  "Database connection pooling is critical for performance",
			Notes:      "Database pool settings",
			ResolvedBy: "andres",
		},
		{
			ID:         "rank-med",
			Project:    env.projDet.project,
			Scope:      domain.ScopeProject,
			Type:       domain.TypeConfig,
			Question:   "How to configure the database connection?",
			Answer:     "Set up the database URL in environment variables",
			Reasoning:  "Environment configuration for database connection",
			Notes:      "",
			ResolvedBy: "andres",
		},
		{
			ID:         "rank-low",
			Project:    env.projDet.project,
			Scope:      domain.ScopeProject,
			Type:       domain.TypeBugfix,
			Question:   "Fix deployment timeout",
			Answer:     "Increase the timeout value in the config",
			Reasoning:  "The default timeout was too short",
			Notes:      "",
			ResolvedBy: "andres",
		},
	}

	for _, l := range learnings {
		saveViaStore(t, env, l)
	}

	results := searchViaStore(t, env, "database connection", nil)

	if len(results) < 2 {
		t.Fatalf("Search() returned %d results, want >= 2", len(results))
	}

	// rank-high should be first (most occurrences of "database" and "connection").
	if results[0].Learning.ID != "rank-high" {
		t.Errorf("Search() top result = %q, want %q (BM25 should rank by relevance)",
			results[0].Learning.ID, "rank-high")
	}

	// Verify scores are descending.
	for i := 1; i < len(results); i++ {
		if results[i].RelevanceScore > results[i-1].RelevanceScore {
			t.Errorf("Search() results not sorted by relevance: result[%d].Score=%v > result[%d].Score=%v",
				i, results[i].RelevanceScore, i-1, results[i-1].RelevanceScore)
		}
	}
}

// --- Additional: Usecase-level save → search integration ---

func TestE2E_UsecaseSaveAndSearch(t *testing.T) {
	env := setupTestEnv(t)

	// Save via usecase.
	saveUsecase := usecase.NewSaveLearning(env.store, env.embedder, env.projDet, env.identDet)
	saveOutput, err := saveUsecase.Execute(context.Background(), &domain.SaveInput{
		Type:      domain.TypeBugfix,
		Question:  "How to fix N+1 query in user list?",
		Answer:    "Use JOIN instead of separate queries",
		Reasoning: "N+1 queries cause performance issues",
		Location:  "internal/handler/users.go",
		Notes:     "Add eager loading",
		Tags:      []string{"performance", "database", "gorm"},
	})
	if err != nil {
		t.Fatalf("SaveLearning.Execute() error: %v", err)
	}
	if saveOutput.ID == "" {
		t.Fatal("SaveLearning.Execute() should return an ID")
	}
	if saveOutput.Project != env.projDet.project {
		t.Errorf("SaveLearning.Project = %q, want %q", saveOutput.Project, env.projDet.project)
	}
	if saveOutput.ResolvedBy != "andres" {
		t.Errorf("SaveLearning.ResolvedBy = %q, want %q", saveOutput.ResolvedBy, "andres")
	}

	// Search via usecase.
	searchUsecase := usecase.NewSearchLearning(env.store, env.projDet, nil)
	searchOutput, err := searchUsecase.Execute(context.Background(), &domain.SearchQuery{
		Query: "N+1 query user list",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchLearning.Execute() error: %v", err)
	}
	if searchOutput.Count < 1 {
		t.Fatal("SearchLearning.Execute() returned 0 results, want >= 1")
	}

	found := false
	for _, r := range searchOutput.Results {
		if r.Learning.Question == "How to fix N+1 query in user list?" {
			found = true
			if r.Learning.Answer != "Use JOIN instead of separate queries" {
				t.Errorf("Search result answer = %q, want correct answer", r.Learning.Answer)
			}
		}
	}
	if !found {
		t.Error("SearchLearning.Execute() did not find the saved learning")
	}
}

// --- Additional: Usecase-level policies integration ---

func TestE2E_UsecaseGetPolicies(t *testing.T) {
	env := setupTestEnv(t)

	// Save a policy directly (bypassing usecase validation for always_inject).
	saveViaStore(t, env, &domain.Learning{
		ID:           "e2e-policy-1",
		Project:      env.projDet.project,
		Scope:        domain.ScopeProject,
		AlwaysInject: true,
		Type:         domain.TypeProcess,
		Question:     "Code review requirements",
		Answer:       "All PRs require 2 approvals before merge",
		ResolvedBy:   "andres",
	})

	// Get policies via usecase.
	policiesUsecase := usecase.NewGetPolicies(env.store, env.projDet)
	output, err := policiesUsecase.Execute(context.Background())
	if err != nil {
		t.Fatalf("GetPolicies.Execute() error: %v", err)
	}
	if output.Count < 1 {
		t.Fatal("GetPolicies.Execute() returned 0 policies, want >= 1")
	}

	found := false
	for _, p := range output.Policies {
		if p.ID == "e2e-policy-1" {
			found = true
			if !p.AlwaysInject {
				t.Error("Policy AlwaysInject should be true")
			}
		}
	}
	if !found {
		t.Error("GetPolicies.Execute() did not find the saved policy")
	}
}

// --- Additional: Cross-project search isolation at usecase level ---

func TestE2E_UsecaseProjectIsolation(t *testing.T) {
	env := setupTestEnv(t)

	projA := &mockProjectDetector{project: "github.com/company/project-a"}
	projB := &mockProjectDetector{project: "github.com/company/project-b"}

	// Save for project A.
	saveUsecaseA := usecase.NewSaveLearning(env.store, env.embedder, projA, env.identDet)
	_, err := saveUsecaseA.Execute(context.Background(), &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to deploy project A?",
		Answer:   "Use the A-specific deployment script",
	})
	if err != nil {
		t.Fatalf("SaveLearning (A) error: %v", err)
	}

	// Save for project B.
	saveUsecaseB := usecase.NewSaveLearning(env.store, env.embedder, projB, env.identDet)
	_, err = saveUsecaseB.Execute(context.Background(), &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to deploy project B?",
		Answer:   "Use the B-specific deployment script",
	})
	if err != nil {
		t.Fatalf("SaveLearning (B) error: %v", err)
	}

	// Search from project A context — should only find A's learning.
	searchUsecaseA := usecase.NewSearchLearning(env.store, projA, nil)
	outputA, err := searchUsecaseA.Execute(context.Background(), &domain.SearchQuery{
		Query: "deploy",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchLearning (A) error: %v", err)
	}
	for _, r := range outputA.Results {
		if r.Learning.Project != "github.com/company/project-a" {
			t.Errorf("Search (A) returned result from project %q, want project-a", r.Learning.Project)
		}
	}

	// Search from project B context — should only find B's learning.
	searchUsecaseB := usecase.NewSearchLearning(env.store, projB, nil)
	outputB, err := searchUsecaseB.Execute(context.Background(), &domain.SearchQuery{
		Query: "deploy",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchLearning (B) error: %v", err)
	}
	for _, r := range outputB.Results {
		if r.Learning.Project != "github.com/company/project-b" {
			t.Errorf("Search (B) returned result from project %q, want project-b", r.Learning.Project)
		}
	}
}

// --- Helper: save via usecase ---

func saveViaUsecase(t *testing.T, env *testEnv, input *domain.SaveInput) *usecase.SaveOutput {
	t.Helper()
	uc := usecase.NewSaveLearning(env.store, env.embedder, env.projDet, env.identDet)
	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("SaveLearning.Execute() error: %v", err)
	}
	return output
}
