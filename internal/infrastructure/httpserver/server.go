package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Server is the Echo HTTP server that runs alongside the MCP server.
// It handles session lifecycle, prompt capture, passive extraction,
// and context injection for the OpenCode plugin.
type Server struct {
	addr   string
	db     *sql.DB
	logger *slog.Logger
	srv    *http.Server
	mu     sync.RWMutex
}

// NewServer creates a new HTTP server.
func NewServer(addr string, dbPath string, logger *slog.Logger) (*Server, error) {
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s := &Server{
		addr:   addr,
		db:     db,
		logger: logger,
	}

	// Run migrations.
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// migrate creates the required tables if they don't exist.
func (s *Server) migrate() error {
	for _, stmt := range FullSchema() {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec schema: %w", err)
		}
	}
	return nil
}

// Start begins listening on the configured address.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check.
	mux.HandleFunc("/health", s.handleHealth)

	// Session management.
	mux.HandleFunc("/sessions", s.handleSessions)
	mux.HandleFunc("/sessions/", s.handleSessionByID)

	// Prompt capture.
	mux.HandleFunc("/prompts", s.handlePrompts)

	// Passive observation extraction.
	mux.HandleFunc("/observations/passive", s.handleObservationsPassive)

	// Project migration.
	mux.HandleFunc("/projects/migrate", s.handleProjectsMigrate)

	// Context injection for compaction.
	mux.HandleFunc("/context", s.handleContext)

	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	// Start in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("echo HTTP server starting", "addr", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Close gracefully shuts down the HTTP server.
func (s *Server) Close() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

// --- Helper functions ---

func (s *Server) readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil {
		http.Error(w, "empty body", http.StatusBadRequest)
		return fmt.Errorf("empty body")
	}
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return err
	}
	return nil
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// sanitizeProject cleans project names to prevent injection.
func sanitizeProject(p string) string {
	return strings.TrimSpace(p)
}

// isLocalhost checks if the request is from localhost.
func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
