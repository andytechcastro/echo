package httpserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Suppress logs during tests
	}))

	srv, err := NewServer(":0", dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	return srv, func() { srv.Close() }
}

func TestHealth(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
}

func TestSessions(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	input := sessionInput{
		ID:        "test-session-1",
		Project:   "test-project",
		Directory: "/tmp/test",
	}

	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Test idempotency.
	req = httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.handleSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 on idempotent call, got %d", w.Code)
	}
}

func TestSessionsDelete(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create session first.
	input := sessionInput{
		ID:        "test-session-2",
		Project:   "test-project",
		Directory: "/tmp/test",
	}
	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSessions(w, req)

	// Delete session.
	req = httptest.NewRequest(http.MethodDelete, "/sessions/test-session-2", nil)
	w = httptest.NewRecorder()
	srv.handleSessionByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestPrompts(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	input := promptInput{
		SessionID: "test-session-3",
		Content:   "How do I set up authentication?",
		Project:   "test-project",
	}

	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePrompts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestObservationsPassive(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	input := observationInput{
		SessionID: "test-session-4",
		Content:   "Fixed auth middleware bug",
		Project:   "test-project",
		Source:    "task-complete",
	}

	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/observations/passive", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleObservationsPassive(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestProjectsMigrate(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create session with old project name.
	input := sessionInput{
		ID:        "test-session-5",
		Project:   "old-project",
		Directory: "/tmp/test",
	}
	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSessions(w, req)

	// Migrate project.
	migrateInput := migrateInput{
		OldProject: "old-project",
		NewProject: "new-project",
	}
	body, _ = json.Marshal(migrateInput)
	req = httptest.NewRequest(http.MethodPost, "/projects/migrate", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.handleProjectsMigrate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestContext(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Add some data.
	promptInput := promptInput{
		SessionID: "test-session-6",
		Content:   "How do I set up auth?",
		Project:   "test-project",
	}
	body, _ := json.Marshal(promptInput)
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handlePrompts(w, req)

	obsInput := observationInput{
		SessionID: "test-session-6",
		Content:   "Auth middleware fixed",
		Project:   "test-project",
		Source:    "task-complete",
	}
	body, _ = json.Marshal(obsInput)
	req = httptest.NewRequest(http.MethodPost, "/observations/passive", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.handleObservationsPassive(w, req)

	// Get context.
	req = httptest.NewRequest(http.MethodGet, "/context?project=test-project", nil)
	w = httptest.NewRecorder()
	srv.handleContext(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["context"] == "" {
		t.Error("expected non-empty context")
	}
}
