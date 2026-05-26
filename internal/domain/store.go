package domain

import "context"

// SearchQuery represents a search request.
type SearchQuery struct {
	Project string // auto-detected from git remote
	Query   string // the search text (English)
	Tags    []string // optional tag filters
	Limit   int // max results (default: 5)
}

// SearchResult wraps a learning with its relevance score.
type SearchResult struct {
	Learning       Learning
	RelevanceScore float64
}

// SaveInput represents the input for saving a learning.
type SaveInput struct {
	Project   string // auto-detected from git remote
	ResolvedBy string // auto-detected from IAM or git config
	Type      LearningType
	Question  string
	Answer    string
	Reasoning string
	Location  string
	Notes     string
	Tags      []string
}

// TextStore is the port for learning persistence.
// Phase 1: SQLite FTS5 (lexical BM25 search).
// Phase 2: SQLite + sqlite-vec (semantic cosine search).
// Phase 3: Firestore (cloud kNN search).
type TextStore interface {
	// Save persists a learning. Returns the generated ID.
	Save(ctx context.Context, input *Learning) (string, error)

	// Search returns learnings matching the query, ordered by relevance.
	Search(ctx context.Context, query *SearchQuery) ([]SearchResult, error)

	// Update modifies an existing learning by ID.
	Update(ctx context.Context, id string, input *Learning) error

	// Delete removes a learning by ID.
	Delete(ctx context.Context, id string) error

	// GetByID retrieves a single learning by ID.
	GetByID(ctx context.Context, id string) (*Learning, error)

	// GetAlwaysInject returns all learnings with always_inject=true for the given project/scope.
	GetAlwaysInject(ctx context.Context, project string) ([]Learning, error)

	// Close releases any resources held by the store.
	Close() error
}
