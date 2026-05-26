package store

import (
	"context"
	"database/sql"
	"fmt"
)

// MigrationPhase represents the current database schema version.
type MigrationPhase int

const (
	PhaseUnknown MigrationPhase = iota
	Phase1Lexical               // FTS5 only, no vector tables
	Phase2Vector                // FTS5 + sqlite-vec vector tables
)

// DetectPhase inspects the database and returns the current migration phase.
func DetectPhase(db *sql.DB) (MigrationPhase, error) {
	// Check if vec_learnings table exists (Phase 2 indicator).
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'vec_learnings'
	`).Scan(&count)
	if err != nil {
		return PhaseUnknown, fmt.Errorf("check vec_learnings: %w", err)
	}

	if count > 0 {
		return Phase2Vector, nil
	}

	// Check if learnings_data table exists (Phase 1 indicator).
	err = db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'learnings_data'
	`).Scan(&count)
	if err != nil {
		return PhaseUnknown, fmt.Errorf("check learnings_data: %w", err)
	}

	if count > 0 {
		return Phase1Lexical, nil
	}

	return PhaseUnknown, nil
}

// MigrateToVector prepares the database for Phase 2 (sqlite-vec).
// This is called on first run with --embeddings flag.
// It creates the vector table schema but does NOT backfill embeddings.
func MigrateToVector(ctx context.Context, db *sql.DB, dimensions int) error {
	phase, err := DetectPhase(db)
	if err != nil {
		return err
	}

	if phase == Phase2Vector {
		return nil // Already migrated.
	}

	if phase == PhaseUnknown {
		return fmt.Errorf("cannot migrate: database not initialized (run Phase 1 first)")
	}

	// Phase 1 → Phase 2: create vec_learnings virtual table.
	// Note: sqlite-vec uses FLOAT[N] syntax for vector columns.
	vecSchema := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_learnings USING vec0(
			rowid    INTEGER PRIMARY KEY,
			embedding FLOAT[%d]
		);
	`, dimensions)

	_, err = db.ExecContext(ctx, vecSchema)
	if err != nil {
		return fmt.Errorf("create vec_learnings: %w", err)
	}

	return nil
}

// RemigrateProvider handles provider change (e.g., 768 dims → 1536 dims).
// It drops the existing vector table, recreates it, and returns the list of
// learning IDs that need re-embedding.
func RemigrateProvider(ctx context.Context, db *sql.DB, newDimensions int) ([]string, error) {
	// Drop existing vector table.
	_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS vec_learnings")
	if err != nil {
		return nil, fmt.Errorf("drop vec_learnings: %w", err)
	}

	// Recreate with new dimensions.
	vecSchema := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_learnings USING vec0(
			rowid    INTEGER PRIMARY KEY,
			embedding FLOAT[%d]
		);
	`, newDimensions)

	_, err = db.ExecContext(ctx, vecSchema)
	if err != nil {
		return nil, fmt.Errorf("recreate vec_learnings: %w", err)
	}

	// Return all learning IDs that need re-embedding.
	rows, err := db.QueryContext(ctx, "SELECT id FROM learnings_data")
	if err != nil {
		return nil, fmt.Errorf("query learning IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}
