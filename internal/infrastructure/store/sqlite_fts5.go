package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/company/echo/internal/domain"
)

// SQLiteFTS5Store implements domain.TextStore using SQLite with FTS5 extension.
// This is the Phase 1 storage backend — pure Go, no CGO, zero external dependencies.
type SQLiteFTS5Store struct {
	db *sql.DB
}

// NewSQLiteFTS5Store creates a new store, initializing the database file if needed.
// The dbPath should be the full path to the SQLite file (e.g., "~/.config/echo/echo.db").
func NewSQLiteFTS5Store(dbPath string) (*SQLiteFTS5Store, error) {
	// Ensure directory exists.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	// Open database with WAL mode for concurrent reads.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Configure connection pool.
	db.SetMaxOpenConns(1) // SQLite requires single writer.
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	store := &SQLiteFTS5Store{db: db}

	if err := store.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return store, nil
}

// initialize creates the schema if it doesn't exist.
func (s *SQLiteFTS5Store) initialize() error {
	schema := []string{
		schemaLearningsWithRowid,
		schemaLearningsFTSExternal,
		schemaIndexes,
		schemaTriggers,
	}

	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec schema: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}

// Save persists a learning. Returns the generated ID.
func (s *SQLiteFTS5Store) Save(ctx context.Context, learning *domain.Learning) (string, error) {
	if learning.ID == "" {
		learning.ID = generateID()
	}

	tagsJSON, err := json.Marshal(learning.Tags)
	if err != nil {
		return "", fmt.Errorf("marshal tags: %w", err)
	}

	// Serialize embedding to JSON for SQLite storage.
	// In Phase 1, Embedding is nil and stored as NULL.
	var embeddingJSON []byte
	if learning.Embedding != nil {
		embeddingJSON, err = json.Marshal(learning.Embedding)
		if err != nil {
			return "", fmt.Errorf("marshal embedding: %w", err)
		}
	}

	query := `
		INSERT INTO learnings_data (
			id, project, scope, always_inject, type,
			question, answer, reasoning, location, notes, tags,
			embedding, resolved_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		learning.ID,
		learning.Project,
		string(learning.Scope),
		boolToInt(learning.AlwaysInject),
		string(learning.Type),
		learning.Question,
		learning.Answer,
		learning.Reasoning,
		learning.Location,
		learning.Notes,
		string(tagsJSON),
		embeddingJSON, // JSON-serialized embedding (NULL in Phase 1)
		learning.ResolvedBy,
		learning.CreatedAt.Format(time.RFC3339),
		learning.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("insert learning: %w", err)
	}

	return learning.ID, nil
}

// Search returns learnings matching the query, ordered by BM25 relevance.
func (s *SQLiteFTS5Store) Search(ctx context.Context, query *domain.SearchQuery) ([]domain.SearchResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}

	// Build FTS5 query.
	ftsQuery := buildFTS5Query(query.Query)

	sqlQuery := `
		SELECT
			l.id, l.project, l.scope, l.always_inject, l.type,
			l.question, l.answer, l.reasoning, l.location, l.notes, l.tags,
			l.embedding, l.resolved_by, l.created_at, l.updated_at,
			rank
		FROM learnings_fts f
		JOIN learnings_data l ON l.rowid = f.rowid
		WHERE learnings_fts MATCH ?
		  AND l.project = ?
		ORDER BY rank
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, sqlQuery, ftsQuery, query.Project, limit)
	if err != nil {
		// If FTS5 query fails (e.g., syntax error), fall back to LIKE search.
		return s.fallbackSearch(ctx, query, limit)
	}
	defer rows.Close()

	return s.scanRows(rows)
}

// fallbackSearch uses LIKE when FTS5 syntax fails (e.g., special characters).
func (s *SQLiteFTS5Store) fallbackSearch(ctx context.Context, query *domain.SearchQuery, limit int) ([]domain.SearchResult, error) {
	likePattern := "%" + escapeLike(query.Query) + "%"

	sqlQuery := `
		SELECT
			l.id, l.project, l.scope, l.always_inject, l.type,
			l.question, l.answer, l.reasoning, l.location, l.notes, l.tags,
			l.embedding, l.resolved_by, l.created_at, l.updated_at,
			0.0 as rank
		FROM learnings_data l
		WHERE l.project = ?
		  AND (
			l.question LIKE ? OR
			l.answer LIKE ? OR
			l.reasoning LIKE ? OR
			l.notes LIKE ?
		  )
		ORDER BY l.updated_at DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, sqlQuery,
		query.Project,
		likePattern, likePattern, likePattern, likePattern,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fallback search: %w", err)
	}
	defer rows.Close()

	return s.scanRows(rows)
}

// Update modifies an existing learning by ID.
func (s *SQLiteFTS5Store) Update(ctx context.Context, id string, learning *domain.Learning) error {
	tagsJSON, err := json.Marshal(learning.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	// Serialize embedding to JSON for SQLite storage.
	var embeddingJSON []byte
	if learning.Embedding != nil {
		embeddingJSON, err = json.Marshal(learning.Embedding)
		if err != nil {
			return fmt.Errorf("marshal embedding: %w", err)
		}
	}

	query := `
		UPDATE learnings_data SET
			project = ?, scope = ?, always_inject = ?, type = ?,
			question = ?, answer = ?, reasoning = ?, location = ?, notes = ?, tags = ?,
			embedding = ?, resolved_by = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		learning.Project,
		string(learning.Scope),
		boolToInt(learning.AlwaysInject),
		string(learning.Type),
		learning.Question,
		learning.Answer,
		learning.Reasoning,
		learning.Location,
		learning.Notes,
		string(tagsJSON),
		embeddingJSON,
		learning.ResolvedBy,
		learning.UpdatedAt.Format(time.RFC3339),
		id,
	)
	if err != nil {
		return fmt.Errorf("update learning: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// Delete removes a learning by ID.
func (s *SQLiteFTS5Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM learnings_data WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete learning: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// GetByID retrieves a single learning by ID.
func (s *SQLiteFTS5Store) GetByID(ctx context.Context, id string) (*domain.Learning, error) {
	query := `
		SELECT
			id, project, scope, always_inject, type,
			question, answer, reasoning, location, notes, tags,
			embedding, resolved_by, created_at, updated_at
		FROM learnings_data
		WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanRow(row)
}

// GetAlwaysInject returns all learnings with always_inject=true for the given project.
func (s *SQLiteFTS5Store) GetAlwaysInject(ctx context.Context, project string) ([]domain.Learning, error) {
	query := `
		SELECT
			id, project, scope, always_inject, type,
			question, answer, reasoning, location, notes, tags,
			embedding, resolved_by, created_at, updated_at
		FROM learnings_data
		WHERE always_inject = 1 AND project = ?
		ORDER BY updated_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, project)
	if err != nil {
		return nil, fmt.Errorf("query always_inject: %w", err)
	}
	defer rows.Close()

	var result []domain.Learning
	for rows.Next() {
		l, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *l)
	}

	return result, rows.Err()
}

// Close releases the database connection.
func (s *SQLiteFTS5Store) Close() error {
	return s.db.Close()
}

// --- Internal helpers ---

// buildFTS5Query converts a user query into FTS5 syntax.
// It wraps each term in quotes to avoid FTS5 operator interpretation.
func buildFTS5Query(query string) string {
	terms := strings.Fields(query)
	for i, term := range terms {
		// Escape FTS5 special characters and wrap in quotes.
		terms[i] = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
	}
	return strings.Join(terms, " ")
}

// escapeLike escapes LIKE special characters.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// scanRows scans multiple rows into SearchResults.
func (s *SQLiteFTS5Store) scanRows(rows *sql.Rows) ([]domain.SearchResult, error) {
	var results []domain.SearchResult
	for rows.Next() {
		var l domain.Learning
		var tagsJSON string
		var scopeStr, typeStr string
		var alwaysInject int
		var createdAt, updatedAt string
		var rank float64
		var embeddingRaw []byte // JSON-serialized embedding

		err := rows.Scan(
			&l.ID, &l.Project, &scopeStr, &alwaysInject, &typeStr,
			&l.Question, &l.Answer, &l.Reasoning, &l.Location, &l.Notes, &tagsJSON,
			&embeddingRaw, &l.ResolvedBy, &createdAt, &updatedAt,
			&rank,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		l.Scope = domain.Scope(scopeStr)
		l.Type = domain.LearningType(typeStr)
		l.AlwaysInject = alwaysInject == 1
		l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		l.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if tagsJSON != "" {
			_ = json.Unmarshal([]byte(tagsJSON), &l.Tags)
		}

		// Deserialize embedding from JSON.
		if len(embeddingRaw) > 0 {
			_ = json.Unmarshal(embeddingRaw, &l.Embedding)
		}

		// BM25 rank is negative; convert to positive score.
		score := 0.0
		if rank < 0 {
			score = -rank
		}

		results = append(results, domain.SearchResult{
			Learning:       l,
			RelevanceScore: score,
		})
	}

	return results, rows.Err()
}

// rowScanner abstracts sql.Row and sql.Rows for single-row scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanRow scans a single row into a Learning.
func scanRow(row rowScanner) (*domain.Learning, error) {
	var l domain.Learning
	var tagsJSON string
	var scopeStr, typeStr string
	var alwaysInject int
	var createdAt, updatedAt string
	var embeddingRaw []byte // JSON-serialized embedding

	err := row.Scan(
		&l.ID, &l.Project, &scopeStr, &alwaysInject, &typeStr,
		&l.Question, &l.Answer, &l.Reasoning, &l.Location, &l.Notes, &tagsJSON,
		&embeddingRaw, &l.ResolvedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan row: %w", err)
	}

	l.Scope = domain.Scope(scopeStr)
	l.Type = domain.LearningType(typeStr)
	l.AlwaysInject = alwaysInject == 1
	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	l.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &l.Tags)
	}

	// Deserialize embedding from JSON.
	if len(embeddingRaw) > 0 {
		_ = json.Unmarshal(embeddingRaw, &l.Embedding)
	}

	return &l, nil
}

// boolToInt converts bool to SQLite INTEGER (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// generateID creates a simple unique ID.
// In production, use ulid or uuid. This is a placeholder for Phase 1.
func generateID() string {
	return fmt.Sprintf("learn_%d", time.Now().UnixNano())
}

// Ensure SQLiteFTS5Store implements domain.TextStore.
var _ domain.TextStore = (*SQLiteFTS5Store)(nil)
