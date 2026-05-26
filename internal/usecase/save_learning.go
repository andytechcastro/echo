package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/pkg/secret"
)

// SaveLearning is the usecase for saving a learning.
type SaveLearning struct {
	store    domain.TextStore
	embedder domain.Embedder
	projDet  domain.ProjectDetector
	identDet domain.IdentityDetector
}

// NewSaveLearning creates a new SaveLearning usecase.
func NewSaveLearning(
	store domain.TextStore,
	embedder domain.Embedder,
	projDet domain.ProjectDetector,
	identDet domain.IdentityDetector,
) *SaveLearning {
	return &SaveLearning{
		store:    store,
		embedder: embedder,
		projDet:  projDet,
		identDet: identDet,
	}
}

// Execute saves a learning with auto-detected project and identity.
func (s *SaveLearning) Execute(ctx context.Context, input *domain.SaveInput) (*SaveOutput, error) {
	// Validate input.
	if err := s.validate(input); err != nil {
		return nil, err
	}

	// Auto-detect project if not provided.
	project := input.Project
	if project == "" {
		var err error
		project, err = s.projDet.Detect()
		if err != nil {
			return nil, fmt.Errorf("detect project: %w", err)
		}
	}

	// Auto-detect identity if not provided.
	resolvedBy := input.ResolvedBy
	if resolvedBy == "" {
		var err error
		resolvedBy, err = s.identDet.Detect()
		if err != nil {
			return nil, fmt.Errorf("detect identity: %w", err)
		}
	}

	// Check for secrets in the input.
	if err := secret.Scan(input.Answer); err != nil {
		return nil, err
	}
	if err := secret.Scan(input.Reasoning); err != nil {
		return nil, err
	}
	if err := secret.Scan(input.Notes); err != nil {
		return nil, err
	}

	// Create the learning entity.
	learning := domain.NewLearning(
		input.Type,
		input.Question,
		input.Answer,
		input.Reasoning,
		input.Location,
		input.Notes,
		input.Tags,
	)
	learning.Project = project
	learning.ResolvedBy = resolvedBy

	// Generate embedding if embedder is available (Phase 2+).
	if s.embedder != nil && s.embedder.Dimensions() > 0 {
		embeddingText := input.Question + " " + input.Answer + " " + input.Reasoning + " " + input.Notes
		vec, err := s.embedder.Embed(ctx, embeddingText)
		if err != nil {
			return nil, fmt.Errorf("generate embedding: %w", err)
		}
		learning.Embedding = vec

		// Check for semantic duplicates.
		existing, err := s.findDuplicate(ctx, project, vec)
		if err != nil {
			return nil, fmt.Errorf("check duplicates: %w", err)
		}
		if existing != nil {
			// Update existing learning instead of creating new.
			existing.Answer = learning.Answer
			existing.Reasoning = learning.Reasoning
			existing.Notes = learning.Notes
			existing.Location = learning.Location
			existing.Tags = learning.Tags
			existing.Embedding = vec // Recalculate embedding.
			existing.UpdatedAt = learning.UpdatedAt

			if err := s.store.Update(ctx, existing.ID, existing); err != nil {
				return nil, fmt.Errorf("update existing learning: %w", err)
			}

			return &SaveOutput{
				ID:         existing.ID,
				Project:    project,
				ResolvedBy: resolvedBy,
				Updated:    true,
			}, nil
		}
	}

	// Check for lexical duplicates (Phase 1 fallback).
	if s.embedder == nil || s.embedder.Dimensions() == 0 {
		existing, err := s.findLexicalDuplicate(ctx, project, input.Question)
		if err != nil {
			return nil, fmt.Errorf("check lexical duplicates: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("%w: learning with same question already exists (id: %s)", domain.ErrDuplicate, existing.ID)
		}
	}

	// Save the learning.
	id, err := s.store.Save(ctx, learning)
	if err != nil {
		return nil, fmt.Errorf("save learning: %w", err)
	}

	return &SaveOutput{
		ID:         id,
		Project:    project,
		ResolvedBy: resolvedBy,
		Updated:    false,
	}, nil
}

// validate checks the input for required fields.
func (s *SaveLearning) validate(input *domain.SaveInput) error {
	if !input.Type.Valid() {
		return fmt.Errorf("%w: %q", domain.ErrInvalidType, input.Type)
	}
	if input.Question == "" {
		return fmt.Errorf("%w: question", domain.ErrEmptyField)
	}
	if input.Answer == "" {
		return fmt.Errorf("%w: answer", domain.ErrEmptyField)
	}
	return nil
}

// findDuplicate checks for semantic duplicates using vector similarity.
// Returns the existing learning if similarity > 0.92, nil otherwise.
func (s *SaveLearning) findDuplicate(ctx context.Context, project string, vec []float32) (*domain.Learning, error) {
	// Get all learnings for the project and compare embeddings.
	// In Phase 2, this uses sqlite-vec. In Phase 3, this uses Firestore findNearest.
	// For now, we do a simple search and compare.
	results, err := s.store.Search(ctx, &domain.SearchQuery{
		Project: project,
		Query:   "", // Empty query to get all results (handled by store).
		Limit:   20,
	})
	if err != nil {
		return nil, err
	}

	for _, r := range results {
		if r.Learning.Embedding == nil {
			continue
		}
		similarity := cosineSimilarity(vec, r.Learning.Embedding)
		if similarity > 0.92 {
			return &r.Learning, nil
		}
	}

	return nil, nil
}

// findLexicalDuplicate checks for exact question match (Phase 1).
func (s *SaveLearning) findLexicalDuplicate(ctx context.Context, project, question string) (*domain.Learning, error) {
	results, err := s.store.Search(ctx, &domain.SearchQuery{
		Project: project,
		Query:   question,
		Limit:   1,
	})
	if err != nil {
		return nil, err
	}

	for _, r := range results {
		if r.Learning.Question == question {
			return &r.Learning, nil
		}
	}

	return nil, nil
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProd, normA, normB float64
	for i := range a {
		dotProd += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProd / (math.Sqrt(normA) * math.Sqrt(normB))
}

// SaveOutput is the result of a successful save.
type SaveOutput struct {
	ID         string
	Project    string
	ResolvedBy string
	Updated    bool // true if an existing learning was updated
}
