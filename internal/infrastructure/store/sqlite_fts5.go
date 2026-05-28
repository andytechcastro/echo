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

	// CGO SQLite driver with sqlite-vec extension.
	_ "github.com/mattn/go-sqlite3"
	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/company/echo/internal/domain"
)

func init() {
	// Register sqlite-vec extension with the SQLite driver.
	// This must be called before any SQLite connections are opened.
	vec.Auto()
}

// SQLiteFTS5Store implements domain.TextStore using SQLite with FTS5 and sqlite-vec extensions.
// Uses CGO (mattn/go-sqlite3) to support loadable extensions (FTS5 + vec0).
type SQLiteFTS5Store struct {
	db         *sql.DB
	dimensions int // vector dimensions (0 = no vector table)
}

// NewSQLiteFTS5Store creates a new store, initializing the database file if needed.
// The dbPath should be the full path to the SQLite file (e.g., "~/.config/echo/echo.db").
func NewSQLiteFTS5Store(dbPath string) (*SQLiteFTS5Store, error) {
	return NewSQLiteFTS5StoreWithDimensions(dbPath, 0)
}

// NewSQLiteFTS5StoreWithDimensions creates a store with vector support.
// If dimensions > 0, the vec_learnings table is created for semantic search.
func NewSQLiteFTS5StoreWithDimensions(dbPath string, dimensions int) (*SQLiteFTS5Store, error) {
	// Ensure directory exists.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	// Open database with CGO SQLite driver (sqlite-vec enabled).
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Configure connection pool.
	db.SetMaxOpenConns(1) // SQLite requires single writer.
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	store := &SQLiteFTS5Store{
		db:         db,
		dimensions: dimensions,
	}

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

	// Create vec_learnings table if dimensions configured.
	if s.dimensions > 0 {
		vecSchema := fmt.Sprintf(`
			CREATE VIRTUAL TABLE IF NOT EXISTS vec_learnings USING vec0(
				embedding float[%d],
				learning_id TEXT,
				project TEXT,
				scope TEXT,
				type TEXT,
				tags TEXT,
				question TEXT,
				answer TEXT
			)
		`, s.dimensions)

		if _, err := s.db.Exec(vecSchema); err != nil {
			return fmt.Errorf("create vec_learnings table: %w", err)
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
		embeddingJSON,
		learning.ResolvedBy,
		learning.CreatedAt.Format(time.RFC3339),
		learning.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("insert learning: %w", err)
	}

	// Also save to vec_learnings if embedding exists and vector table is configured.
	if learning.Embedding != nil && s.dimensions > 0 {
		if err := s.saveVector(ctx, learning.ID, learning.Embedding, learning); err != nil {
			// Log but don't fail — learning is saved, just without vector index.
			// In production, use slog here.
		}
	}

	return learning.ID, nil
}

// saveVector inserts or updates a vector entry in vec_learnings.
func (s *SQLiteFTS5Store) saveVector(ctx context.Context, learningID string, embedding []float32, learning *domain.Learning) error {
	if len(embedding) != s.dimensions {
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d", s.dimensions, len(embedding))
	}

	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	// Upsert: delete existing, then insert.
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM vec_learnings WHERE learning_id = ?",
		learningID,
	); err != nil {
		return fmt.Errorf("delete existing vector: %w", err)
	}

	tagsStr := strings.Join(learning.Tags, ",")

	query := `
		INSERT INTO vec_learnings (
			learning_id, embedding, project, scope, type, tags, question, answer
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		learningID,
		string(embeddingJSON),
		learning.Project,
		string(learning.Scope),
		string(learning.Type),
		tagsStr,
		learning.Question,
		learning.Answer,
	)
	if err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}

	return nil
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

// VectorSearch performs kNN search using cosine similarity on vec_learnings.
// Returns results ordered by distance (most similar first).
func (s *SQLiteFTS5Store) VectorSearch(ctx context.Context, queryVec []float32, project string, limit int) ([]VectorSearchResult, error) {
	if s.dimensions == 0 {
		return nil, fmt.Errorf("vector search not available: store created without vector dimensions")
	}

	if len(queryVec) != s.dimensions {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", s.dimensions, len(queryVec))
	}

	if limit <= 0 {
		limit = 5
	}

	queryVecJSON, err := json.Marshal(queryVec)
	if err != nil {
		return nil, fmt.Errorf("marshal query vector: %w", err)
	}

	query := `
		SELECT
			learning_id,
			distance,
			project,
			scope,
			type,
			tags,
			question,
			answer
		FROM vec_learnings
		WHERE embedding MATCH ?
		  AND project = ?
		ORDER BY distance
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, string(queryVecJSON), project, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []VectorSearchResult
	for rows.Next() {
		var r VectorSearchResult
		err := rows.Scan(
			&r.LearningID,
			&r.Distance,
			&r.Project,
			&r.Scope,
			&r.Type,
			&r.Tags,
			&r.Question,
			&r.Answer,
		)
		if err != nil {
			return nil, fmt.Errorf("scan vector result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return results, nil
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

	// Update vector index if embedding exists.
	if learning.Embedding != nil && s.dimensions > 0 {
		if err := s.saveVector(ctx, id, learning.Embedding, learning); err != nil {
			// Log but don't fail.
		}
	}

	return nil
}

// Delete removes a learning by ID.
func (s *SQLiteFTS5Store) Delete(ctx context.Context, id string) error {
	// Delete from vec_learnings first (if configured).
	if s.dimensions > 0 {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM vec_learnings WHERE learning_id = ?", id)
	}

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
func buildFTS5Query(query string) string {
	terms := strings.Fields(query)
	for i, term := range terms {
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
		var embeddingRaw []byte

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

		if len(embeddingRaw) > 0 {
			_ = json.Unmarshal(embeddingRaw, &l.Embedding)
		}

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
	var embeddingRaw []byte

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
func generateID() string {
	return fmt.Sprintf("learn_%d", time.Now().UnixNano())
}

// VectorSearchResult represents a single result from vector search.
type VectorSearchResult struct {
	LearningID string
	Distance   float64
	Project    string
	Scope      string
	Type       string
	Tags       string
	Question   string
	Answer     string
}

// Ensure SQLiteFTS5Store implements domain.TextStore.
var _ domain.TextStore = (*SQLiteFTS5Store)(nil)
