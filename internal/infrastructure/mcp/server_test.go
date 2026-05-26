package mcp

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/infrastructure/store"
	"github.com/company/echo/internal/usecase"
)

// setupTestServer creates a fully wired MCP server for integration testing.
func setupTestServer(t *testing.T) (*Server, *store.SQLiteFTS5Store) {
	t.Helper()

	// Create in-memory store.
	dbPath := ":memory:"
	textStore, err := store.NewSQLiteFTS5Store(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	// Create detectors with mock values.
	projDet := &mockProjectDetector{project: "github.com/test/repo"}
	identDet := &mockIdentityDetector{identity: "testuser"}

	// Create usecases (Phase 1: no embedder).
	saveUC := usecase.NewSaveLearning(textStore, nil, projDet, identDet)
	searchUC := usecase.NewSearchLearning(textStore, projDet)
	policyUC := usecase.NewGetPolicies(textStore, projDet)

	// Create logger.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	// Create MCP server.
	server := NewServer(saveUC, searchUC, policyUC, logger)

	return server, textStore
}

// mockProjectDetector for MCP tests.
type mockProjectDetector struct {
	project string
	err     error
}

func (m *mockProjectDetector) Detect() (string, error) {
	return m.project, m.err
}

// mockIdentityDetector for MCP tests.
type mockIdentityDetector struct {
	identity string
	err      error
}

func (m *mockIdentityDetector) Detect() (string, error) {
	return m.identity, m.err
}

func TestServer_SaveLearning(t *testing.T) {
	server, store := setupTestServer(t)
	defer store.Close()

	ctx := context.Background()

	// Call save_learning directly through the usecase (since MCP handler needs mcp.CallToolRequest).
	uc := server.saveUC

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to connect to the database?",
		Answer:   "Set DATABASE_URL=postgresql://localhost:5432/echo",
		Reasoning: "We use PostgreSQL for relational data",
		Location: ".env, config/database.go",
		Notes:    "Connection pool max is 20",
		Tags:     []string{"database", "postgresql"},
	}

	output, err := uc.Execute(ctx, input)
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
}

func TestServer_SearchLearning(t *testing.T) {
	server, store := setupTestServer(t)
	defer store.Close()

	ctx := context.Background()

	// First, save a learning.
	uc := server.saveUC

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "How to connect to the database?",
		Answer:   "Set DATABASE_URL=postgresql://localhost:5432/echo",
		Tags:     []string{"database", "postgresql"},
	}

	_, err := uc.Execute(ctx, input)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Now search for it.
	searchUC := server.searchUC

	searchOutput, err := searchUC.Execute(ctx, &domain.SearchQuery{
		Project: "github.com/test/repo",
		Query:   "database",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}

	if searchOutput.Count < 1 {
		t.Errorf("Search returned %d results, want >= 1", searchOutput.Count)
	}
}

func TestServer_GetPolicies(t *testing.T) {
	server, store := setupTestServer(t)
	defer store.Close()

	ctx := context.Background()

	// Save a policy with always_inject=true.
	l := &domain.Learning{
		ID:           "policy-1",
		Project:      "github.com/test/repo",
		Scope:        domain.ScopeProject, // Note: org scope is admin-only, using project for test
		AlwaysInject: true,
		Type:         domain.TypeProcess,
		Question:     "Deployment policy",
		Answer:       "Deployments only Tue-Thu",
		ResolvedBy:   "admin",
	}

	_, err := store.Save(ctx, l)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Get policies.
	policyUC := server.policyUC

	output, err := policyUC.Execute(ctx)
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

func TestServer_SaveLearning_SecretDetected(t *testing.T) {
	server, store := setupTestServer(t)
	defer store.Close()

	ctx := context.Background()

	uc := server.saveUC

	input := &domain.SaveInput{
		Type:     domain.TypeConfig,
		Question: "test",
		Answer:   "sk-12345678901234567890abcdef", // Looks like an API key.
	}

	_, err := uc.Execute(ctx, input)
	if err == nil {
		t.Fatal("Execute() should return error when secret is detected")
	}
}
