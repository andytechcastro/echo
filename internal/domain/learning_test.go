package domain

import (
	"testing"
	"time"
)

func TestLearningType_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input LearningType
		want  bool
	}{
		{"config", TypeConfig, true},
		{"pattern", TypePattern, true},
		{"bugfix", TypeBugfix, true},
		{"decision", TypeDecision, true},
		{"process", TypeProcess, true},
		{"domain", TypeDomain, true},
		{"gotcha", TypeGotcha, true},
		{"invalid", LearningType("invalid"), false},
		{"empty", LearningType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScope_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input Scope
		want  bool
	}{
		{"project", ScopeProject, true},
		{"organization", ScopeOrganization, true},
		{"invalid", Scope("invalid"), false},
		{"empty", Scope(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewLearning(t *testing.T) {
	l := NewLearning(
		TypeBugfix,
		"How to fix 502?",
		"Restart the service",
		"Service was stuck",
		"src/server.go",
		"Check logs first",
		[]string{"bugfix", "server"},
	)

	if l.Type != TypeBugfix {
		t.Errorf("Type = %v, want %v", l.Type, TypeBugfix)
	}
	if l.Scope != ScopeProject {
		t.Errorf("Scope = %v, want %v", l.Scope, ScopeProject)
	}
	if l.AlwaysInject {
		t.Error("AlwaysInject should be false by default")
	}
	if l.Question != "How to fix 502?" {
		t.Errorf("Question = %v, want %v", l.Question, "How to fix 502?")
	}
	if len(l.Tags) != 2 {
		t.Errorf("Tags length = %v, want 2", len(l.Tags))
	}
	if l.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if l.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestSecretError(t *testing.T) {
	err := &SecretError{Field: "answer", Pattern: "connection-string"}

	if err.Error() != "potential secret detected in 'answer' field (pattern: connection-string)" {
		t.Errorf("Error() = %v", err.Error())
	}

	// Test Is() implementation.
	var target error = &SecretError{}
	if !err.Is(target) {
		t.Error("Is() should return true for SecretError target")
	}

	// Test Is() with different error type.
	if err.Is(ErrNotFound) {
		t.Error("Is() should return false for ErrNotFound")
	}
}

func TestDomainErrors(t *testing.T) {
	// Verify all domain errors are non-nil.
	errors := []error{
		ErrNotFound,
		ErrDuplicate,
		ErrSecretDetected,
		ErrInvalidType,
		ErrInvalidScope,
		ErrScopeForbidden,
		ErrEmptyField,
	}

	for i, err := range errors {
		if err == nil {
			t.Errorf("errors[%d] is nil", i)
		}
	}
}

func TestLearning_UpdatedAt(t *testing.T) {
	l := NewLearning(TypeConfig, "q", "a", "r", "l", "n", nil)

	// Simulate an update.
	l.UpdatedAt = time.Now().Add(1 * time.Hour)

	if !l.UpdatedAt.After(l.CreatedAt) {
		t.Error("UpdatedAt should be after CreatedAt after update")
	}
}
