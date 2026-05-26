package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/usecase"
)

const (
	serverName    = "echo"
	serverVersion = "0.1.0"
)

// Server wraps the MCP server with Echo-specific dependencies.
type Server struct {
	server   *mcp.Server
	saveUC   *usecase.SaveLearning
	searchUC *usecase.SearchLearning
	policyUC *usecase.GetPolicies
	logger   *slog.Logger
}

// NewServer creates a new Echo MCP server.
func NewServer(
	saveUC *usecase.SaveLearning,
	searchUC *usecase.SearchLearning,
	policyUC *usecase.GetPolicies,
	logger *slog.Logger,
) *Server {
	impl := &mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}

	srv := mcp.NewServer(impl, nil)

	s := &Server{
		server:   srv,
		saveUC:   saveUC,
		searchUC: searchUC,
		policyUC: policyUC,
		logger:   logger,
	}

	// Register tools.
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "save_learning",
		Description: "Save a resolved issue, config, pattern, or decision to the team knowledge base.",
	}, s.handleSaveLearning)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_learning",
		Description: "Search the team knowledge base for existing solutions related to a problem.",
	}, s.handleSearchLearning)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_critical_policies",
		Description: "Return organization-scoped policies that should be injected at session start.",
	}, s.handleGetPolicies)

	return s
}

// Run starts the MCP server over stdio transport.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting echo MCP server", "version", serverVersion)
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

// --- Tool Handlers ---

// parseTags converts a tags string (comma-separated or JSON array) to []string.
// This handles the case where MCP clients serialize arrays as JSON strings.
func parseTags(tags string) []string {
	if tags == "" {
		return nil
	}
	// Try parsing as JSON array first.
	var arr []string
	if err := json.Unmarshal([]byte(tags), &arr); err == nil {
		return arr
	}
	// Fall back to comma-separated.
	result := strings.Split(tags, ",")
	for i, t := range result {
		result[i] = strings.TrimSpace(t)
	}
	return result
}

// SaveLearningInput represents the input for save_learning tool.
type SaveLearningInput struct {
	Type      string `json:"type" jsonschema:"the type of learning: config, pattern, bugfix, decision, process, domain, gotcha"`
	Question  string `json:"question" jsonschema:"the problem that was solved"`
	Answer    string `json:"answer" jsonschema:"the solution"`
	Reasoning string `json:"reasoning" jsonschema:"why this solution was chosen (trade-offs, context)"`
	Location  string `json:"location" jsonschema:"affected files/modules (e.g., src/auth/middleware.ts)"`
	Notes     string `json:"notes" jsonschema:"gotchas, edge cases, warnings (lessons learned)"`
	Tags      string `json:"tags" jsonschema:"searchable tags, always in English (comma-separated or JSON array)"`
}

// SaveLearningOutput represents the output of save_learning tool.
type SaveLearningOutput struct {
	ID         string `json:"id"`
	Project    string `json:"project"`
	ResolvedBy string `json:"resolvedBy"`
	Message    string `json:"message"`
}

func (s *Server) handleSaveLearning(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input SaveLearningInput,
) (*mcp.CallToolResult, SaveLearningOutput, error) {
	s.logger.Info("save_learning called",
		"type", input.Type,
		"question", input.Question,
	)

	ucInput := &domain.SaveInput{
		Type:      domain.LearningType(input.Type),
		Question:  input.Question,
		Answer:    input.Answer,
		Reasoning: input.Reasoning,
		Location:  input.Location,
		Notes:     input.Notes,
		Tags:      parseTags(input.Tags),
	}

	output, err := s.saveUC.Execute(ctx, ucInput)
	if err != nil {
		// Check if it's a secret error.
		var secretErr *domain.SecretError
		if err != nil && fmt.Errorf("%w", secretErr) != nil {
			s.logger.Warn("save_learning rejected: secret detected", "error", err)
		}
		return nil, SaveLearningOutput{}, err
	}

	msg := "Learning saved successfully"
	if output.Updated {
		msg = "Learning updated successfully"
	}

	return nil, SaveLearningOutput{
		ID:         output.ID,
		Project:    output.Project,
		ResolvedBy: output.ResolvedBy,
		Message:    msg,
	}, nil
}

// SearchLearningInput represents the input for search_learning tool.
type SearchLearningInput struct {
	Query string `json:"query" jsonschema:"the problem or question to search for"`
	Tags  string `json:"tags" jsonschema:"optional tag filters (comma-separated or JSON array)"`
}

// SearchLearningResult represents a single search result.
type SearchLearningResult struct {
	ID             string   `json:"id"`
	Project        string   `json:"project"`
	Type           string   `json:"type"`
	Question       string   `json:"question"`
	Answer         string   `json:"answer"`
	Reasoning      string   `json:"reasoning"`
	Location       string   `json:"location"`
	Notes          string   `json:"notes"`
	Tags           []string `json:"tags"`
	ResolvedBy     string   `json:"resolvedBy"`
	CreatedAt      string   `json:"createdAt"`
	RelevanceScore float64  `json:"relevanceScore"`
}

// SearchLearningOutput represents the output of search_learning tool.
type SearchLearningOutput struct {
	Results []SearchLearningResult `json:"results"`
	Count   int                    `json:"count"`
}

func (s *Server) handleSearchLearning(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input SearchLearningInput,
) (*mcp.CallToolResult, SearchLearningOutput, error) {
	s.logger.Info("search_learning called", "query", input.Query)

	output, err := s.searchUC.Execute(ctx, &domain.SearchQuery{
		Query: input.Query,
		Tags:  parseTags(input.Tags),
		Limit: 5,
	})
	if err != nil {
		return nil, SearchLearningOutput{}, err
	}

	results := make([]SearchLearningResult, len(output.Results))
	for i, r := range output.Results {
		results[i] = SearchLearningResult{
			ID:             r.Learning.ID,
			Project:        r.Learning.Project,
			Type:           string(r.Learning.Type),
			Question:       r.Learning.Question,
			Answer:         r.Learning.Answer,
			Reasoning:      r.Learning.Reasoning,
			Location:       r.Learning.Location,
			Notes:          r.Learning.Notes,
			Tags:           r.Learning.Tags,
			ResolvedBy:     r.Learning.ResolvedBy,
			CreatedAt:      r.Learning.CreatedAt.Format("2006-01-02T15:04:05Z"),
			RelevanceScore: r.RelevanceScore,
		}
	}

	return nil, SearchLearningOutput{
		Results: results,
		Count:   output.Count,
	}, nil
}

// GetPoliciesInput represents the input for get_critical_policies tool.
type GetPoliciesInput struct{}

// GetPoliciesOutput represents the output of get_critical_policies tool.
type GetPoliciesOutput struct {
	Policies []PolicyResult `json:"policies"`
	Count    int            `json:"count"`
}

// PolicyResult represents a single policy.
type PolicyResult struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Type     string `json:"type"`
}

func (s *Server) handleGetPolicies(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input GetPoliciesInput,
) (*mcp.CallToolResult, GetPoliciesOutput, error) {
	s.logger.Info("get_critical_policies called")

	output, err := s.policyUC.Execute(ctx)
	if err != nil {
		return nil, GetPoliciesOutput{}, err
	}

	policies := make([]PolicyResult, len(output.Policies))
	for i, p := range output.Policies {
		policies[i] = PolicyResult{
			ID:       p.ID,
			Question: p.Question,
			Answer:   p.Answer,
			Type:     string(p.Type),
		}
	}

	return nil, GetPoliciesOutput{
		Policies: policies,
		Count:    output.Count,
	}, nil
}

// --- Error handling ---

// formatError converts an error into an MCP error response.
func formatError(err error) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, err.Error())},
		},
		IsError: true,
	}, nil
}

// --- JSON helpers ---

// toJSON marshals a value to JSON string.
func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
