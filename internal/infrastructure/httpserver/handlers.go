package httpserver

import (
	"net/http"
	"strings"
)

// --- Health Check ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Session Management ---

type sessionInput struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Directory string `json:"directory"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input sessionInput
	if err := s.readJSON(w, r, &input); err != nil {
		return
	}

	if input.ID == "" || input.Project == "" {
		s.writeError(w, http.StatusBadRequest, "id and project are required")
		return
	}

	// INSERT OR IGNORE for idempotency.
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO sessions (id, project, directory) VALUES (?, ?, ?)`,
		input.ID, sanitizeProject(input.Project), input.Directory,
	)
	if err != nil {
		s.logger.Error("failed to create session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	s.logger.Debug("session created", "id", input.ID, "project", input.Project)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "created", "id": input.ID})
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /sessions/{id}
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" || id == r.URL.Path {
		s.writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		s.deleteSession(w, r, id)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, id string) {
	// Delete session and associated data.
	tx, err := s.db.Begin()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	_, _ = tx.Exec(`DELETE FROM prompts WHERE session_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM observations WHERE session_id = ?`, id)
	result, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}

	if err := tx.Commit(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	rows, _ := result.RowsAffected()
	s.logger.Debug("session deleted", "id", id, "rows", rows)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// --- Prompt Capture ---

type promptInput struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	Project   string `json:"project"`
}

func (s *Server) handlePrompts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input promptInput
	if err := s.readJSON(w, r, &input); err != nil {
		return
	}

	if input.SessionID == "" || input.Content == "" {
		s.writeError(w, http.StatusBadRequest, "session_id and content are required")
		return
	}

	// Ensure session exists (idempotent).
	_, _ = s.db.Exec(
		`INSERT OR IGNORE INTO sessions (id, project, directory) VALUES (?, ?, '')`,
		input.SessionID, sanitizeProject(input.Project),
	)

	_, err := s.db.Exec(
		`INSERT INTO prompts (session_id, project, content) VALUES (?, ?, ?)`,
		input.SessionID, sanitizeProject(input.Project), input.Content,
	)
	if err != nil {
		s.logger.Error("failed to save prompt", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to save prompt")
		return
	}

	s.logger.Debug("prompt captured", "session", input.SessionID, "len", len(input.Content))
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// --- Passive Observation Extraction ---

type observationInput struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	Project   string `json:"project"`
	Source    string `json:"source"`
}

func (s *Server) handleObservationsPassive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input observationInput
	if err := s.readJSON(w, r, &input); err != nil {
		return
	}

	if input.SessionID == "" || input.Content == "" {
		s.writeError(w, http.StatusBadRequest, "session_id and content are required")
		return
	}

	source := input.Source
	if source == "" {
		source = "passive"
	}

	// Ensure session exists.
	_, _ = s.db.Exec(
		`INSERT OR IGNORE INTO sessions (id, project, directory) VALUES (?, ?, '')`,
		input.SessionID, sanitizeProject(input.Project),
	)

	_, err := s.db.Exec(
		`INSERT INTO observations (session_id, project, content, source) VALUES (?, ?, ?, ?)`,
		input.SessionID, sanitizeProject(input.Project), input.Content, source,
	)
	if err != nil {
		s.logger.Error("failed to save observation", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to save observation")
		return
	}

	s.logger.Debug("observation saved", "session", input.SessionID, "source", source, "len", len(input.Content))
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// --- Project Migration ---

type migrateInput struct {
	OldProject string `json:"old_project"`
	NewProject string `json:"new_project"`
}

func (s *Server) handleProjectsMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input migrateInput
	if err := s.readJSON(w, r, &input); err != nil {
		return
	}

	if input.OldProject == "" || input.NewProject == "" {
		s.writeError(w, http.StatusBadRequest, "old_project and new_project are required")
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	newProj := sanitizeProject(input.NewProject)
	oldProj := sanitizeProject(input.OldProject)

	_, _ = tx.Exec(`UPDATE sessions SET project = ? WHERE project = ?`, newProj, oldProj)
	_, _ = tx.Exec(`UPDATE prompts SET project = ? WHERE project = ?`, newProj, oldProj)
	_, _ = tx.Exec(`UPDATE observations SET project = ? WHERE project = ?`, newProj, oldProj)

	if err := tx.Commit(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to commit migration")
		return
	}

	s.logger.Info("project migrated", "old", oldProj, "new", newProj)
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":      "migrated",
		"old_project": oldProj,
		"new_project": newProj,
	})
}

// --- Context Injection for Compaction ---

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	if project == "" {
		s.writeError(w, http.StatusBadRequest, "project query parameter is required")
		return
	}

	// Get recent observations for the project.
	rows, err := s.db.Query(
		`SELECT content, source FROM observations WHERE project = ? ORDER BY created_at DESC LIMIT 10`,
		sanitizeProject(project),
	)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to query observations")
		return
	}
	defer rows.Close()

	var observations []string
	for rows.Next() {
		var content, source string
		if err := rows.Scan(&content, &source); err != nil {
			continue
		}
		observations = append(observations, "["+source+"] "+content)
	}

	// Get recent prompts for the project.
	promptRows, err := s.db.Query(
		`SELECT content FROM prompts WHERE project = ? ORDER BY created_at DESC LIMIT 5`,
		sanitizeProject(project),
	)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to query prompts")
		return
	}
	defer promptRows.Close()

	var prompts []string
	for promptRows.Next() {
		var content string
		if err := promptRows.Scan(&content); err != nil {
			continue
		}
		prompts = append(prompts, content)
	}

	// Get active sessions.
	sessionRows, err := s.db.Query(
		`SELECT id, tool_count FROM sessions WHERE project = ? ORDER BY created_at DESC LIMIT 5`,
		sanitizeProject(project),
	)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to query sessions")
		return
	}
	defer sessionRows.Close()

	var sessions []string
	for sessionRows.Next() {
		var id string
		var toolCount int
		if err := sessionRows.Scan(&id, &toolCount); err != nil {
			continue
		}
		sessions = append(sessions, "Session "+id+" (tool_calls: "+string(rune(toolCount))+")")
	}

	// Build context string.
	var ctx strings.Builder
	ctx.WriteString("## Echo Context for Project: " + project + "\n\n")

	if len(observations) > 0 {
		ctx.WriteString("### Recent Observations\n")
		for _, obs := range observations {
			ctx.WriteString("- " + obs + "\n")
		}
		ctx.WriteString("\n")
	}

	if len(prompts) > 0 {
		ctx.WriteString("### Recent Prompts\n")
		for _, p := range prompts {
			// Truncate long prompts.
			if len(p) > 200 {
				p = p[:200] + "..."
			}
			ctx.WriteString("- " + p + "\n")
		}
		ctx.WriteString("\n")
	}

	if len(sessions) > 0 {
		ctx.WriteString("### Active Sessions\n")
		for _, sess := range sessions {
			ctx.WriteString("- " + sess + "\n")
		}
		ctx.WriteString("\n")
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"context": ctx.String()})
}
