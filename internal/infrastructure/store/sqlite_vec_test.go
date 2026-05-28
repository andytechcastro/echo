package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/company/echo/internal/domain"
)

func TestSQLiteFTS5Store_VectorSaveAndSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vec.db")

	// Create store with 384-dim vector support.
	store, err := NewSQLiteFTS5StoreWithDimensions(dbPath, 384)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	// Create test learnings with embeddings.
	embeddings := [][]float32{
		makeVector(384, 0.1), // "how to configure auth"
		makeVector(384, 0.2), // "database migration tips"
		makeVector(384, 0.3), // "testing best practices"
	}

	for i, emb := range embeddings {
		learning := &domain.Learning{
			ID:        fmt.Sprintf("learn_test_%d", i),
			Project:   "test-project",
			Scope:     domain.ScopeProject,
			Type:      domain.TypePattern,
			Question:  "test question",
			Answer:    "test answer",
			Embedding: emb,
		}
		_, err := store.Save(context.Background(), learning)
		if err != nil {
			t.Fatalf("save learning %d: %v", i, err)
		}
	}

	// Vector search should return results.
	queryVec := makeVector(384, 0.15)
	results, err := store.VectorSearch(context.Background(), queryVec, "test-project", 5)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results should be ordered by distance.
	for i := 1; i < len(results); i++ {
		if results[i].Distance < results[i-1].Distance {
			t.Errorf("results not ordered by distance: [%d].distance=%f < [%d].distance=%f",
				i, results[i].Distance, i-1, results[i-1].Distance)
		}
	}
}

func TestSQLiteFTS5Store_VectorSave_DimensionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vec.db")

	store, err := NewSQLiteFTS5StoreWithDimensions(dbPath, 384)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	// Try to save with wrong embedding dimensions.
	learning := &domain.Learning{
		ID:        "test-id",
		Project:   "test-project",
		Scope:     domain.ScopeProject,
		Type:      domain.TypePattern,
		Question:  "test",
		Answer:    "test",
		Embedding: makeVector(768, 0.1), // wrong dimensions
	}

	// Save should succeed (learning saved) but vector save should fail silently.
	_, err = store.Save(context.Background(), learning)
	if err != nil {
		t.Fatalf("save should succeed even with wrong embedding: %v", err)
	}

	// Vector search should return 0 results (vector not saved).
	results, err := store.VectorSearch(context.Background(), makeVector(384, 0.1), "test-project", 5)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 vector results, got %d", len(results))
	}
}

func TestSQLiteFTS5Store_VectorSearch_NoVectorTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vec.db")

	// Create store WITHOUT vector support.
	store, err := NewSQLiteFTS5Store(dbPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	// Vector search should fail gracefully.
	_, err = store.VectorSearch(context.Background(), makeVector(384, 0.1), "test-project", 5)
	if err == nil {
		t.Fatal("expected error when vector table not configured")
	}
}

func TestSQLiteFTS5Store_VectorSearch_QueryDimensionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vec.db")

	store, err := NewSQLiteFTS5StoreWithDimensions(dbPath, 384)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	// Try to search with wrong dimensions.
	_, err = store.VectorSearch(context.Background(), makeVector(768, 0.1), "test-project", 5)
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestSQLiteFTS5Store_Delete_AlsoDeletesVector(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vec.db")

	store, err := NewSQLiteFTS5StoreWithDimensions(dbPath, 384)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	// Save a learning with embedding.
	learning := &domain.Learning{
		ID:        "learn_test_123",
		Project:   "test-project",
		Scope:     domain.ScopeProject,
		Type:      domain.TypePattern,
		Question:  "test",
		Answer:    "test",
		Embedding: makeVector(384, 0.1),
	}
	_, err = store.Save(context.Background(), learning)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify vector exists.
	results, err := store.VectorSearch(context.Background(), makeVector(384, 0.1), "test-project", 5)
	if err != nil {
		t.Fatalf("vector search before delete: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result before delete, got %d", len(results))
	}

	// Delete the learning.
	err = store.Delete(context.Background(), "learn_test_123")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify vector is also deleted.
	results, err = store.VectorSearch(context.Background(), makeVector(384, 0.1), "test-project", 5)
	if err != nil {
		t.Fatalf("vector search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after delete, got %d", len(results))
	}
}

// makeVector creates a float32 slice of the given size with a base value.
func makeVector(size int, base float32) []float32 {
	v := make([]float32, size)
	for i := range v {
		v[i] = base + float32(i)*0.001
	}
	return v
}
