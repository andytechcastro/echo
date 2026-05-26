package usecase

import (
	"context"
	"fmt"

	"github.com/company/echo/internal/domain"
)

// GetPolicies is the usecase for retrieving organization-scoped policies.
type GetPolicies struct {
	store   domain.TextStore
	projDet domain.ProjectDetector
}

// NewGetPolicies creates a new GetPolicies usecase.
func NewGetPolicies(
	store domain.TextStore,
	projDet domain.ProjectDetector,
) *GetPolicies {
	return &GetPolicies{
		store:   store,
		projDet: projDet,
	}
}

// Execute returns all always_inject learnings for the current project.
func (g *GetPolicies) Execute(ctx context.Context) (*PoliciesOutput, error) {
	// Auto-detect project.
	project, err := g.projDet.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect project: %w", err)
	}

	// Get always_inject learnings.
	learnings, err := g.store.GetAlwaysInject(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get always_inject: %w", err)
	}

	return &PoliciesOutput{
		Policies: learnings,
		Count:    len(learnings),
	}, nil
}

// PoliciesOutput is the result of a successful policies retrieval.
type PoliciesOutput struct {
	Policies []domain.Learning
	Count    int
}
