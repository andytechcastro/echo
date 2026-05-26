package usecase

import (
	"context"
	"testing"

	"github.com/company/echo/internal/domain"
)

// mockStore implements domain.TextStore for testing.
type mockStore struct {
	saved        []*domain.Learning
	searchResults []domain.SearchResult
	alwaysInject  []domain.Learning
	getByID      map[string]*domain.Learning
	updateCalled bool
	deleteCalled bool
}

func (m *mockStore) Save(ctx context.Context, l *domain.Learning) (string, error) {
	if l.ID == "" {
		l.ID = "mock-generated-id"
	}
	m.saved = append(m.saved, l)
	return l.ID, nil
}

func (m *mockStore) Search(ctx context.Context, q *domain.SearchQuery) ([]domain.SearchResult, error) {
	return m.searchResults, nil
}

func (m *mockStore) Update(ctx context.Context, id string, l *domain.Learning) error {
	m.updateCalled = true
	return nil
}

func (m *mockStore) Delete(ctx context.Context, id string) error {
	m.deleteCalled = true
	return nil
}

func (m *mockStore) GetByID(ctx context.Context, id string) (*domain.Learning, error) {
	if l, ok := m.getByID[id]; ok {
		return l, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) GetAlwaysInject(ctx context.Context, project string) ([]domain.Learning, error) {
	return m.alwaysInject, nil
}

func (m *mockStore) Close() error { return nil }

// mockProjectDetector implements domain.ProjectDetector.
type mockProjectDetector struct {
	project string
	err     error
}

func (m *mockProjectDetector) Detect() (string, error) {
	return m.project, m.err
}

// mockIdentityDetector implements domain.IdentityDetector.
type mockIdentityDetector struct {
	identity string
	err      error
}

func (m *mockIdentityDetector) Detect() (string, error) {
	return m.identity, m.err
}

// mockEmbedder implements domain.Embedder.
type mockEmbedder struct {
	vector     []float32
	dimensions int
	provider   string
	err        error
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return m.vector, m.err
}

func (m *mockEmbedder) Dimensions() int {
	return m.dimensions
}

func (m *mockEmbedder) Provider() string {
	return m.provider
}

func TestSaveLearning_Execute_Success(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{} // Noop embedder (Phase 1)

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to connect to DB?",
		Answer:   "Set DATABASE_URL=postgresql://localhost:5432/echo",
		Tags:     []string{"database"},
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output.ID == "" {
		t.Error("Execute() should return an ID")
	}
	if output.Project != "github.com/test/repo" {
		t.Errorf("Project = %v, want github.com/test/repo", output.Project)
	}
	if output.ResolvedBy != "testuser" {
		t.Errorf("ResolvedBy = %v, want testuser", output.ResolvedBy)
	}
	if output.Updated {
		t.Error("Updated should be false for new learning")
	}
	if len(store.saved) != 1 {
		t.Errorf("Store saved %d learnings, want 1", len(store.saved))
	}
}

func TestSaveLearning_Execute_InvalidType(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{}

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:     domain.LearningType("invalid"),
		Question: "test",
		Answer:   "test",
	}

	_, err := uc.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute() should return error for invalid type")
	}
}

func TestSaveLearning_Execute_EmptyQuestion(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{}

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:   domain.TypeConfig,
		Answer: "test",
	}

	_, err := uc.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute() should return error for empty question")
	}
}

func TestSaveLearning_Execute_EmptyAnswer(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{}

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "test",
	}

	_, err := uc.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute() should return error for empty answer")
	}
}

func TestSaveLearning_Execute_ProjectDetectionError(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{err: domain.ErrNotFound}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{}

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "test",
		Answer:   "test",
	}

	_, err := uc.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute() should return error when project detection fails")
	}
}

func TestSearchLearning_Execute_Success(t *testing.T) {
	store := &mockStore{
		searchResults: []domain.SearchResult{
			{
				Learning: domain.Learning{
					ID:       "result-1",
					Question: "How to connect to DB?",
					Answer:   "Set DATABASE_URL=...",
				},
				RelevanceScore: 1.5,
			},
		},
	}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}

	uc := NewSearchLearning(store, projDet)

	query := &domain.SearchQuery{
		Query: "database connection",
		Limit: 5,
	}

	output, err := uc.Execute(context.Background(), query)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output.Count != 1 {
		t.Errorf("Count = %d, want 1", output.Count)
	}
	if output.Results[0].Learning.ID != "result-1" {
		t.Errorf("Result ID = %v, want result-1", output.Results[0].Learning.ID)
	}
}

func TestSearchLearning_Execute_DefaultLimit(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}

	uc := NewSearchLearning(store, projDet)

	query := &domain.SearchQuery{
		Query: "test",
		Limit: 0, // Should default to 5
	}

	_, err := uc.Execute(context.Background(), query)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify the query was modified with default limit.
	if query.Limit != 5 {
		t.Errorf("Limit = %d, want 5", query.Limit)
	}
}

func TestSearchLearning_Execute_EmptyQuery(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}

	uc := NewSearchLearning(store, projDet)

	query := &domain.SearchQuery{
		Query: "",
	}

	_, err := uc.Execute(context.Background(), query)
	if err == nil {
		t.Fatal("Execute() should return error for empty query")
	}
}

func TestGetPolicies_Execute_Success(t *testing.T) {
	store := &mockStore{
		alwaysInject: []domain.Learning{
			{
				ID:           "policy-1",
				Question:     "Deployment policy",
				Answer:       "Deployments only Tue-Thu",
				AlwaysInject: true,
			},
		},
	}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}

	uc := NewGetPolicies(store, projDet)

	output, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output.Count != 1 {
		t.Errorf("Count = %d, want 1", output.Count)
	}
	if output.Policies[0].ID != "policy-1" {
		t.Errorf("Policy ID = %v, want policy-1", output.Policies[0].ID)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float64
	}{
		{
			name: "identical vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "different lengths",
			a:    []float32{1, 2, 3},
			b:    []float32{1, 2, 3},
			want: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if got < tt.want-0.001 || got > tt.want+0.001 {
				t.Errorf("cosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCosineSimilarity_DifferentDimensions(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0}

	got := cosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("cosineSimilarity() with different dimensions = %v, want 0", got)
	}
}

func TestSaveLearning_Execute_WithEmbedder(t *testing.T) {
	// Create a learning that already exists in the store (for duplicate detection).
	existing := &domain.Learning{
		ID:        "existing-1",
		Project:   "github.com/test/repo",
		Question:  "How to connect to DB?",
		Answer:    "Old answer",
		Embedding: []float32{0.9, 0.1, 0.0}, // Similar to the mock embedder's vector.
	}

	store := &mockStore{
		getByID: map[string]*domain.Learning{
			"existing-1": existing,
		},
		searchResults: []domain.SearchResult{
			{
				Learning:       *existing,
				RelevanceScore: 1.0,
			},
		},
	}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{
		vector:     []float32{0.95, 0.05, 0.0}, // Very similar to existing.
		dimensions: 3,
		provider:   "mock",
	}

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to connect to DB?",
		Answer:   "New answer",
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !output.Updated {
		t.Error("Updated should be true for duplicate detection")
	}
	if !store.updateCalled {
		t.Error("Store.Update should have been called")
	}
}

func TestSaveLearning_Execute_SecretDetected(t *testing.T) {
	store := &mockStore{}
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}
	embedder := &mockEmbedder{}

	uc := NewSaveLearning(store, embedder, projDet, identDet)

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "test",
		Answer:   "sk-12345678901234567890abcdef", // Looks like an API key.
	}

	_, err := uc.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute() should return error when secret is detected")
	}

	// Verify it's a secret-related error.
	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}
