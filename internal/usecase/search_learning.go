package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/company/echo/internal/domain"
)

// SearchLearning is the usecase for searching learnings.
type SearchLearning struct {
	store    domain.TextStore
	projDet  domain.ProjectDetector
	embedder domain.Embedder
}

// NewSearchLearning creates a new SearchLearning usecase.
func NewSearchLearning(
	store domain.TextStore,
	projDet domain.ProjectDetector,
	embedder domain.Embedder,
) *SearchLearning {
	return &SearchLearning{
		store:    store,
		projDet:  projDet,
		embedder: embedder,
	}
}

// Execute searches for learnings matching the query.
// If an embedder is available, it uses vector search (cosine similarity).
// Falls back to BM25 lexical search if vector search fails or embedder is nil.
func (s *SearchLearning) Execute(ctx context.Context, query *domain.SearchQuery) (*SearchOutput, error) {
	// Auto-detect project if not provided.
	if query.Project == "" {
		var err error
		query.Project, err = s.projDet.Detect()
		if err != nil {
			return nil, fmt.Errorf("detect project: %w", err)
		}
	}

	if query.Query == "" {
		return nil, fmt.Errorf("%w: query", domain.ErrEmptyField)
	}

	// Set default limit.
	if query.Limit <= 0 {
		query.Limit = 5
	}

	// Try vector search if embedder is available.
	if s.embedder != nil {
		results, err := s.vectorSearch(ctx, query)
		if err == nil && len(results) > 0 {
			return &SearchOutput{
				Results: results,
				Count:   len(results),
			}, nil
		}
		slog.Warn("vector search failed or returned no results, falling back to BM25", "error", err)
	}

	// Fallback to BM25 lexical search.
	results, err := s.store.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return &SearchOutput{
		Results: results,
		Count:   len(results),
	}, nil
}

// vectorSearch performs semantic search using embeddings.
func (s *SearchLearning) vectorSearch(ctx context.Context, query *domain.SearchQuery) ([]domain.SearchResult, error) {
	// Check if store supports vector search.
	vecStore, ok := s.store.(interface {
		VectorSearch(ctx context.Context, queryVec []float32, project string, limit int) ([]domain.VectorSearchResult, error)
	})
	if !ok {
		return nil, fmt.Errorf("store does not support vector search")
	}

	// Generate embedding for query.
	queryVec, err := s.embedder.Embed(ctx, query.Query)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	// Execute vector search.
	vecResults, err := vecStore.VectorSearch(ctx, queryVec, query.Project, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	// Convert vector results to domain.SearchResult.
	results := make([]domain.SearchResult, len(vecResults))
	for i, vr := range vecResults {
		// Distance is cosine distance; convert to similarity score.
		score := 1.0 - vr.Distance
		if score < 0 {
			score = 0
		}

		results[i] = domain.SearchResult{
			Learning: domain.Learning{
				Project: vr.Project,
				Scope:   domain.Scope(vr.Scope),
				Type:    domain.LearningType(vr.Type),
				Question: vr.Question,
				Answer:  vr.Answer,
			},
			RelevanceScore: score,
		}
	}

	return results, nil
}

// SearchOutput is the result of a successful search.
type SearchOutput struct {
	Results []domain.SearchResult
	Count   int
}
