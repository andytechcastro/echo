package usecase

import (
	"context"
	"fmt"

	"github.com/company/echo/internal/domain"
)

// SearchLearning is the usecase for searching learnings.
type SearchLearning struct {
	store   domain.TextStore
	projDet domain.ProjectDetector
}

// NewSearchLearning creates a new SearchLearning usecase.
func NewSearchLearning(
	store domain.TextStore,
	projDet domain.ProjectDetector,
) *SearchLearning {
	return &SearchLearning{
		store:   store,
		projDet: projDet,
	}
}

// Execute searches for learnings matching the query.
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

	// Execute search.
	results, err := s.store.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return &SearchOutput{
		Results: results,
		Count:   len(results),
	}, nil
}

// SearchOutput is the result of a successful search.
type SearchOutput struct {
	Results []domain.SearchResult
	Count   int
}
