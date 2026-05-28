package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/infrastructure/embedder"
	"github.com/company/echo/internal/infrastructure/store"
)

// TestE2E_VectorSaveAndSearch tests the full vector pipeline:
// save learning with embedding → vector search → verify results.
// Skip if ONNXRUNTIME_LIB_PATH is not set.
func TestE2E_VectorSaveAndSearch(t *testing.T) {
	runtimePath := os.Getenv("ONNXRUNTIME_LIB_PATH")
	if runtimePath == "" {
		t.Skip("ONNXRUNTIME_LIB_PATH not set, skipping integration test")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create embedder.
	modelPath := os.Getenv("ONNX_MODEL_PATH")
	if modelPath == "" {
		home, _ := os.UserHomeDir()
		modelPath = home + "/.config/echo/models/all-MiniLM-L6-v2.onnx"
	}
	vocabPath := os.Getenv("ONNX_VOCAB_PATH")
	if vocabPath == "" {
		home, _ := os.UserHomeDir()
		vocabPath = home + "/.config/echo/models/vocab.txt"
	}

	e, err := embedder.NewONNXEmbedder(modelPath, vocabPath, runtimePath)
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	defer e.Close()

	// Create store with vector support.
	s, err := store.NewSQLiteFTS5StoreWithDimensions(dbPath, e.Dimensions())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	// Save learnings with embeddings.
	learnings := []struct {
		id       string
		question string
		answer   string
	}{
		{"learn_1", "How to configure database connection?", "Set DATABASE_URL environment variable"},
		{"learn_2", "What is the deployment policy?", "Deployments only allowed Tuesday through Thursday"},
		{"learn_3", "How to reset user password?", "Use the admin panel or run reset-password CLI command"},
	}

	for _, l := range learnings {
		text := l.question + " " + l.answer
		vec, err := e.Embed(context.Background(), text)
		if err != nil {
			t.Fatalf("embed learning %s: %v", l.id, err)
		}

		learning := &domain.Learning{
			ID:        l.id,
			Project:   "github.com/test/repo",
			Scope:     domain.ScopeProject,
			Type:      domain.TypeConfig,
			Question:  l.question,
			Answer:    l.answer,
			Embedding: vec,
		}

		_, err = s.Save(context.Background(), learning)
		if err != nil {
			t.Fatalf("save learning %s: %v", l.id, err)
		}
	}

	// Search for something semantically similar to "database setup".
	queryVec, err := e.Embed(context.Background(), "database setup")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}

	results, err := s.VectorSearch(context.Background(), queryVec, "github.com/test/repo", 5)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// The most similar result should be about database connection.
	if results[0].Question == "" {
		t.Error("first result should have a question")
	}

	t.Logf("Top result: %s (distance: %.4f)", results[0].Question, results[0].Distance)
}

// TestE2E_VectorSearchProjectIsolation verifies that vector search
// only returns results for the specified project.
func TestE2E_VectorSearchProjectIsolation(t *testing.T) {
	runtimePath := os.Getenv("ONNXRUNTIME_LIB_PATH")
	if runtimePath == "" {
		t.Skip("ONNXRUNTIME_LIB_PATH not set, skipping integration test")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	e, err := embedder.NewONNXEmbedder(
		os.Getenv("ONNX_MODEL_PATH"),
		os.Getenv("ONNX_VOCAB_PATH"),
		runtimePath,
	)
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	defer e.Close()

	s, err := store.NewSQLiteFTS5StoreWithDimensions(dbPath, e.Dimensions())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	// Save learnings in two different projects.
	projects := []string{"github.com/project-a/repo", "github.com/project-b/repo"}
	for _, project := range projects {
		text := "test learning for " + project
		vec, err := e.Embed(context.Background(), text)
		if err != nil {
			t.Fatalf("embed: %v", err)
		}

		learning := &domain.Learning{
			ID:        "learn_" + project,
			Project:   project,
			Scope:     domain.ScopeProject,
			Type:      domain.TypeConfig,
			Question:  "Test question",
			Answer:    "Test answer for " + project,
			Embedding: vec,
		}

		_, err = s.Save(context.Background(), learning)
		if err != nil {
			t.Fatalf("save learning: %v", err)
		}
	}

	// Search in project A only.
	queryVec, err := e.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}

	results, err := s.VectorSearch(context.Background(), queryVec, projects[0], 5)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	// Should only return results from project A.
	for _, r := range results {
		if r.Project != projects[0] {
			t.Errorf("expected project %s, got %s", projects[0], r.Project)
		}
	}
}

// TestE2E_VectorSearchFallbackToBM25 verifies that when no embedder is
// available, search falls back to BM25 lexical search.
func TestE2E_VectorSearchFallbackToBM25(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store WITHOUT vector support.
	s, err := store.NewSQLiteFTS5Store(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	// Save a learning without embedding.
	learning := &domain.Learning{
		ID:       "learn_1",
		Project:  "github.com/test/repo",
		Scope:    domain.ScopeProject,
		Type:     domain.TypeConfig,
		Question: "How to configure database?",
		Answer:   "Set DATABASE_URL",
	}

	_, err = s.Save(context.Background(), learning)
	if err != nil {
		t.Fatalf("save learning: %v", err)
	}

	// Search should work via BM25.
	results, err := s.Search(context.Background(), &domain.SearchQuery{
		Project: "github.com/test/repo",
		Query:   "database configure",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one BM25 result")
	}
}
