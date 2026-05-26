package domain

import (
	"time"
)

// LearningType represents the category of a learning.
type LearningType string

const (
	TypeConfig   LearningType = "config"
	TypePattern  LearningType = "pattern"
	TypeBugfix   LearningType = "bugfix"
	TypeDecision LearningType = "decision"
	TypeProcess  LearningType = "process"
	TypeDomain   LearningType = "domain"
	TypeGotcha   LearningType = "gotcha"
)

// Valid reports whether the learning type is one of the allowed values.
func (t LearningType) Valid() bool {
	switch t {
	case TypeConfig, TypePattern, TypeBugfix, TypeDecision, TypeProcess, TypeDomain, TypeGotcha:
		return true
	}
	return false
}

// Scope represents the visibility scope of a learning.
type Scope string

const (
	ScopeProject     Scope = "project"
	ScopeOrganization Scope = "organization"
)

// Valid reports whether the scope is one of the allowed values.
func (s Scope) Valid() bool {
	switch s {
	case ScopeProject, ScopeOrganization:
		return true
	}
	return false
}

// Learning represents a piece of shared team knowledge.
// This is the core domain entity — storage-agnostic.
type Learning struct {
	ID           string
	Project      string
	Scope        Scope
	AlwaysInject bool
	Type         LearningType
	Question     string
	Answer       string
	Reasoning    string
	Location     string
	Notes        string
	Tags         []string
	Embedding    []float32 // nil in Phase 1, populated in Phase 2+
	ResolvedBy   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewLearning creates a new learning with default values.
func NewLearning(
	learningType LearningType,
	question, answer, reasoning, location, notes string,
	tags []string,
) *Learning {
	now := time.Now().UTC()
	return &Learning{
		Scope:      ScopeProject,
		Type:       learningType,
		Question:   question,
		Answer:     answer,
		Reasoning:  reasoning,
		Location:   location,
		Notes:      notes,
		Tags:       tags,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
