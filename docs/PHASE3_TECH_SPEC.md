# Tech Spec: Phase 3 — Semantic Search with Local Embeddings

## Context

Echo currently supports lexical BM25 search via SQLite FTS5 (Phase 1) and HTTP/plugin infrastructure (Phase 2). The `Embedder` interface and `Learning.Embedding` field exist but are unused. Phase 3 introduces semantic vector search using a local ONNX embedding model (22MB) + sqlite-vec, with zero external dependencies.

## Goals

- Enable semantic search via vector embeddings (cosine similarity)
- Use local ONNX model (all-MiniLM-L6-v2, 22MB, 384 dims) — no APIs, no internet required
- Store and query vectors via sqlite-vec extension
- Graceful fallback to BM25 if embedding fails
- Maintain backward compatibility with existing SQLite FTS5 data

## Non-Goals

- External embedding APIs (Vertex AI, OpenAI, Cohere) — moved to Phase 3b
- Cloud shared memory (Firestore) — moved to Phase 3b
- Admin CLI / RBAC (Phase 4)
- Multi-tenant isolation beyond project scope

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│  SaveLearning Usecase                           │
│  1. Validate & detect project/identity          │
│  2. Embed question+answer via ONNX model        │
│  3. Save to SQLite (FTS5 + vec_learnings)       │
└──────────────────┬──────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────┐
│  SearchLearning Usecase                         │
│  1. Embed query via ONNX model                  │
│  2. Query vec_learnings (cosine similarity)     │
│  3. Fallback to BM25 if embedding fails         │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  Embedding Pipeline                              │
│                                                  │
│  Text → [ONNX Runtime] → float32[384] → sqlite  │
│         (all-MiniLM-L6-v2, 22MB)                │
└─────────────────────────────────────────────────┘
```

---

## Data Model

### Existing (unchanged)

```sql
-- learnings_data: stores all fields
-- learnings_fts: FTS5 virtual table for BM25
```

### New: sqlite-vec vector table

```sql
CREATE VIRTUAL TABLE vec_learnings USING vec0(
    embedding float[384],
    learning_id TEXT,        -- maps to learnings_data.id
    project TEXT,            -- filterable metadata
    scope TEXT,              -- filterable metadata
    type TEXT,               -- filterable metadata
    tags TEXT,               -- filterable metadata
    question TEXT,           -- searchable metadata
    answer TEXT              -- searchable metadata
);
```

### Triggers for sync

```sql
-- On save: insert into vec_learnings
-- On update: update vec_learnings
-- On delete: delete from vec_learnings
```

---

## Embedding Model: all-MiniLM-L6-v2

| Property | Value |
|----------|-------|
| **Format** | ONNX |
| **Size** | ~22MB |
| **Dimensions** | 384 |
| **Language** | English (primary) |
| **License** | Apache 2.0 |
| **Source** | Hugging Face (sentence-transformers) |

### Why this model

- Smallest viable model for semantic search (22MB vs 400MB+ for larger models)
- 384 dimensions is sufficient for code/technical text
- ONNX format runs via `onnxruntime_go` (CGO-based, compatible with our stack)
- No GPU required — runs on CPU in ~10-50ms per embedding
- Proven track record in production RAG systems

### Text to embed

```
question + " " + answer
```

We concatenate question and answer for the embedding. This captures both the problem and solution context.

---

## Config Additions

```go
// New fields in Config
type Config struct {
    // ... existing fields

    // EmbedderType is the embedding provider: "local", "vertex-ai", "openai", "cohere".
    // Default: "local" (Phase 3a)
    EmbedderType string

    // ModelPath is the path to the ONNX model file.
    // Default: ~/.config/echo/models/all-MiniLM-L6-v2.onnx
    ModelPath string
}
```

### Environment variables

```
ECHO_EMBEDDER=local          # "local" | "vertex-ai" | "openai" | "cohere"
ECHO_MODEL_PATH=/path/to/model.onnx  # optional override
```

---

## Mode Matrix (Updated)

| Mode | Embedder | Search | Use Case |
|------|----------|--------|----------|
| `local` | nil | BM25 only | Basic local usage (Phase 1) |
| `embeddings` | ONNX local | BM25 + vector (sqlite-vec) | Semantic search, offline (Phase 3a) |
| `cloud` | configured | Firestore kNN | Shared team memory (Phase 3b) |
| `hybrid` | configured | Cloud + local fallback | Production resilience (Phase 3b) |

---

## Implementation Details

### 1. ONNX Embedder Implementation

```go
// internal/infrastructure/embedder/onnx.go
package embedder

import (
    "context"
    "github.com/yalue/onnxruntime_go"
)

type ONNXEmbedder struct {
    session *onnxruntime_go.AdvancedSession
    tokenizer *Tokenizer // simple BPE or WordPiece tokenizer
    dimensions int
}

func NewONNXEmbedder(modelPath string) (*ONNXEmbedder, error) {
    // Load ONNX model
    // Create session
    // Initialize tokenizer
    return &ONNXEmbedder{
        session: session,
        tokenizer: tokenizer,
        dimensions: 384,
    }, nil
}

func (e *ONNXEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
    // 1. Tokenize text
    // 2. Run ONNX inference
    // 3. Mean pooling + normalize
    // 4. Return float32[384]
}

func (e *ONNXEmbedder) Dimensions() int { return 384 }
func (e *ONNXEmbedder) Provider() string { return "local-onnx" }
```

### 2. sqlite-vec Store Implementation

```go
// internal/infrastructure/store/sqlite_vec.go
package store

import (
    "database/sql"
    "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

type SQLiteVecStore struct {
    db *sql.DB
    dimensions int
}

func NewSQLiteVecStore(dbPath string, dimensions int) (*SQLiteVecStore, error) {
    // Open SQLite DB
    // Load sqlite-vec extension
    // Create vec_learnings virtual table if not exists
    // Create sync triggers
    return &SQLiteVecStore{
        db: db,
        dimensions: dimensions,
    }, nil
}

func (s *SQLiteVecStore) SaveVector(ctx context.Context, learningID string, embedding []float32, metadata map[string]string) error {
    // INSERT INTO vec_learnings (learning_id, embedding, project, scope, ...)
}

func (s *SQLiteVecStore) VectorSearch(ctx context.Context, queryVec []float32, project string, limit int) ([]SearchResult, error) {
    // SELECT rowid, distance FROM vec_learnings
    // WHERE embedding MATCH ? AND project = ?
    // ORDER BY distance LIMIT ?
}
```

### 3. Hybrid Search (BM25 + Vector)

```go
// internal/usecase/search_learning.go
func (s *SearchLearning) Execute(ctx context.Context, query *domain.SearchQuery) ([]domain.SearchResult, error) {
    // Try vector search first if embedder is available
    if s.embedder != nil {
        queryVec, err := s.embedder.Embed(ctx, query.Query)
        if err == nil {
            results, err := s.store.VectorSearch(ctx, queryVec, query.Project, query.Limit)
            if err == nil && len(results) > 0 {
                return results, nil
            }
        }
        slog.Warn("vector search failed, falling back to BM25", "error", err)
    }

    // Fallback to BM25
    return s.store.Search(ctx, query)
}
```

### 4. Migration: FTS5 → vec_learnings

```bash
echo admin migrate-to-vectors [--model-path=/path/to/model.onnx]
```

Steps:
1. Read all learnings from `learnings_data`
2. For each learning, generate embedding via ONNX model
3. Insert into `vec_learnings` with metadata
4. Verify count match
5. Report summary

---

## Security & Observability

- **No external calls**: All embeddings generated locally, no data leaves the machine
- **Model integrity**: Verify ONNX model checksum on first load
- **Error handling**: Graceful fallback to BM25 if embedding fails
- **Logging**: All embedding operations logged with `slog` (level: debug)
- **Performance**: Embedding latency tracked (target: <50ms per text)

---

## Alternative Solutions Considered

| Solution | Rejected Because |
|----------|-----------------|
| External APIs (Phase 3a) | User wants local-first, offline capable |
| Larger models (bge-large, etc.) | Too big (400MB+), overkill for our use case |
| Pure BM25 (no vectors) | Doesn't solve semantic search problem |
| PostgreSQL + pgvector | Requires external DB, more ops complexity |

---

## Implementation Plan

### Phase 3a — Local Embeddings + sqlite-vec (8 tickets)

| # | Ticket | Type | Est. Effort |
|---|--------|------|-------------|
| 1 | Add sqlite-vec dependency + load extension | Infra | S |
| 2 | Create `vec_learnings` virtual table + triggers | Infra | S |
| 3 | Add ONNX runtime dependency + embedder implementation | Infra | M |
| 4 | Bundle all-MiniLM-L6-v2 ONNX model (22MB) | Infra | S |
| 5 | Implement vector save (populate vec_learnings on Save) | Infra | S |
| 6 | Implement vector search (cosine similarity query) | Infra | M |
| 7 | Add `--mode embeddings` wiring in main.go | Cmd | S |
| 8 | Integration tests with sqlite-vec + ONNX | Test | M |

### Phase 3b — External APIs + Cloud (moved to later)

| # | Ticket | Type | Est. Effort |
|---|--------|------|-------------|
| 1 | Add Vertex AI embedder implementation | Infra | M |
| 2 | Add OpenAI embedder implementation | Infra | S |
| 3 | Add Cohere embedder implementation | Infra | S |
| 4 | Implement FirestoreStore (Save, Search, kNN) | Infra | M |
| 5 | Implement HybridStore (cloud + local fallback) | Infra | M |
| 6 | Wire `--mode cloud` and `--mode hybrid` in main.go | Cmd | S |
| 7 | Create migration tool (SQLite → Firestore) | Tool | M |
| 8 | Integration tests with Firestore emulator | Test | M |
