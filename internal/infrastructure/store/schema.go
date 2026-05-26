package store

// DDL statements for Phase 1: SQLite FTS5 (lexical BM25 search).
const (
	// schemaLearnings creates the main learnings table.
	schemaLearnings = `
		CREATE TABLE IF NOT EXISTS learnings (
			id           TEXT PRIMARY KEY,
			project      TEXT    NOT NULL,
			scope        TEXT    NOT NULL DEFAULT 'project',
			always_inject INTEGER NOT NULL DEFAULT 0,
			type         TEXT    NOT NULL,
			question     TEXT    NOT NULL,
			answer       TEXT    NOT NULL,
			reasoning    TEXT    NOT NULL DEFAULT '',
			location     TEXT    NOT NULL DEFAULT '',
			notes        TEXT    NOT NULL DEFAULT '',
			tags         TEXT    NOT NULL DEFAULT '[]',
			embedding    BLOB,
			resolved_by  TEXT    NOT NULL,
			created_at   TEXT    NOT NULL,
			updated_at   TEXT    NOT NULL
		);
	`

	// schemaLearningsFTS creates the FTS5 virtual table for BM25 ranking.
	// It indexes question, answer, reasoning, notes, and tags for full-text search.
	schemaLearningsFTS = `
		CREATE VIRTUAL TABLE IF NOT EXISTS learnings_fts USING fts5(
			question,
			answer,
			reasoning,
			notes,
			tags,
			content='learnings',
			content_rowid='rowid'
		);
	`

	// schemaLearningsRowid adds a hidden rowid column to learnings for FTS5 content_rowid linking.
	// SQLite FTS5 requires an integer rowid to link to the content table.
	// We use a separate shadow table approach: learnings keeps TEXT id, learnings_fts links via rowid.
	schemaLearningsWithRowid = `
		CREATE TABLE IF NOT EXISTS learnings_data (
			rowid        INTEGER PRIMARY KEY AUTOINCREMENT,
			id           TEXT    UNIQUE NOT NULL,
			project      TEXT    NOT NULL,
			scope        TEXT    NOT NULL DEFAULT 'project',
			always_inject INTEGER NOT NULL DEFAULT 0,
			type         TEXT    NOT NULL,
			question     TEXT    NOT NULL,
			answer       TEXT    NOT NULL,
			reasoning    TEXT    NOT NULL DEFAULT '',
			location     TEXT    NOT NULL DEFAULT '',
			notes        TEXT    NOT NULL DEFAULT '',
			tags         TEXT    NOT NULL DEFAULT '[]',
			embedding    BLOB,
			resolved_by  TEXT    NOT NULL,
			created_at   TEXT    NOT NULL,
			updated_at   TEXT    NOT NULL
		);
	`

	// schemaLearningsFTSExternal uses learnings_data.rowid as content_rowid.
	schemaLearningsFTSExternal = `
		CREATE VIRTUAL TABLE IF NOT EXISTS learnings_fts USING fts5(
			question,
			answer,
			reasoning,
			notes,
			tags,
			content='learnings_data',
			content_rowid='rowid'
		);
	`

	// schemaIndexes creates performance indexes.
	schemaIndexes = `
		CREATE INDEX IF NOT EXISTS idx_learnings_project ON learnings_data(project);
		CREATE INDEX IF NOT EXISTS idx_learnings_scope ON learnings_data(scope);
		CREATE INDEX IF NOT EXISTS idx_learnings_always_inject ON learnings_data(always_inject);
		CREATE INDEX IF NOT EXISTS idx_learnings_type ON learnings_data(type);
	`

	// schemaTriggers maintains FTS5 index on INSERT, UPDATE, DELETE.
	schemaTriggers = `
		CREATE TRIGGER IF NOT EXISTS learnings_ai AFTER INSERT ON learnings_data BEGIN
			INSERT INTO learnings_fts(rowid, question, answer, reasoning, notes, tags)
			VALUES (new.rowid, new.question, new.answer, new.reasoning, new.notes, new.tags);
		END;

		CREATE TRIGGER IF NOT EXISTS learnings_ad AFTER DELETE ON learnings_data BEGIN
			INSERT INTO learnings_fts(learnings_fts, rowid, question, answer, reasoning, notes, tags)
			VALUES ('delete', old.rowid, old.question, old.answer, old.reasoning, old.notes, old.tags);
		END;

		CREATE TRIGGER IF NOT EXISTS learnings_au AFTER UPDATE ON learnings_data BEGIN
			INSERT INTO learnings_fts(learnings_fts, rowid, question, answer, reasoning, notes, tags)
			VALUES ('delete', old.rowid, old.question, old.answer, old.reasoning, old.notes, old.tags);
			INSERT INTO learnings_fts(rowid, question, answer, reasoning, notes, tags)
			VALUES (new.rowid, new.question, new.answer, new.reasoning, new.notes, new.tags);
		END;
	`
)
