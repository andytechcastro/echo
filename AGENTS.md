# AGENTS.md - Echo Development Guide

This file provides context for AI agents working on the Echo codebase.

## Project Overview

Echo is a shared team memory layer for AI agents. It's a Go binary (`echo-mcp`) that runs both an MCP server (stdio) and an HTTP server for plugin communication. Learnings are stored in SQLite with FTS5 full-text search and sqlite-vec for semantic vector search.

## Architecture

Clean Architecture with dependency flow: `domain` → `usecase` → `infrastructure` → `cmd`.

```
cmd/echo/main.go          → Entry point, CLI (Cobra), dependency wiring
internal/domain/          → Interfaces (TextStore, Embedder, ProjectDetector, IdentityDetector)
internal/usecase/         → Business logic (SaveLearning, SearchLearning, GetPolicies)
internal/infrastructure/  → Implementations
  store/                  → SQLite FTS5 + sqlite-vec storage
  embedder/               → ONNX-based local embedding (all-MiniLM-L6-v2)
  detector/               → Git project and identity detection
  mcp/                    → MCP server and tool handlers
  httpserver/             → HTTP server for plugin communication
internal/sync/            → Git sync (manifest + chunk import/export)
internal/pkg/secret/      → Secret detection patterns
internal/config/          → Configuration management
internal/setup/           → OpenCode plugin generation (plugin.ts embedded)
internal/e2e/             → End-to-end integration tests
```

## Key Decisions

1. **Phase 1 uses SQLite FTS5 (BM25), not vector search.** We start with lexical search to validate the product before adding ML complexity. BM25 covers ~70% of developer queries.

2. **Phase 3a uses local ONNX embeddings (all-MiniLM-L6-v2, ~90MB, 384 dims).** No external APIs required. The model runs via `onnxruntime_go` (CGO-based). Embeddings are stored in sqlite-vec `vec_learnings` virtual table.

3. **Phase 3b will add external APIs (Vertex AI, OpenAI, Cohere) and Firestore cloud.** Moved from Phase 3a to keep Phase 3a local-first and offline-capable.

4. **Embedder interface is provider-agnostic.** It's `nil` in Phase 1, local ONNX in Phase 3a, configurable cloud provider in Phase 3b+. The usecase checks `if s.embedder != nil` before generating embeddings.

5. **All data lives in `~/.config/echo/`** (XDG convention), not `~/.echo/`. Database: `~/.config/echo/echo.db`. Model: `~/.config/echo/models/all-MiniLM-L6-v2.onnx`.

6. **FTS5 uses external content table pattern.** `learnings_data` stores all fields, `learnings_fts` indexes only searchable text columns. Triggers maintain sync.

7. **SQLite doesn't support `[]float32` directly.** Embeddings are serialized to JSON before storing, deserialized on read. sqlite-vec handles vector storage separately.

8. **Hybrid HTTP + MCP architecture (Phase 2).** The binary runs both servers. The plugin uses HTTP for lifecycle, prompt capture, and passive extraction. The MCP server is used for search/save operations.

9. **Tags are strings, not arrays.** MCP clients serialize arrays inconsistently. We use `string` in input structs and `parseTags()` to handle both JSON array and comma-separated formats.

## Running Tests

```bash
CGO_ENABLED=1 go test -tags fts5 ./...           # All 99 tests
CGO_ENABLED=1 go test -tags fts5 ./internal/...  # Unit + integration tests
CGO_ENABLED=1 go test -tags fts5 ./internal/e2e/... # End-to-end tests
CGO_ENABLED=1 go test -tags fts5 ./internal/infrastructure/httpserver/... # HTTP server tests
```

**Note:** CGO is required for sqlite-vec and ONNX Runtime. The `-tags fts5` flag enables FTS5 in the CGO SQLite build.

## Adding a New Feature

1. Define interfaces in `internal/domain/`
2. Implement in `internal/infrastructure/`
3. Create usecase in `internal/usecase/`
4. Wire in `cmd/echo/main.go`
5. Add tests in the appropriate `_test.go` file or `internal/e2e/`

## MCP Tools

- `save_learning` — Save a learning (validates, detects project/identity, scans secrets, saves)
- `search_learning` — Search learnings (detects project, searches with BM25 or vector search, returns ranked results)
- `get_critical_policies` — Return always_inject learnings for the current project

## HTTP Server Endpoints (Phase 2)

- `GET /health` — Health check
- `POST /sessions` — Create session (idempotent)
- `DELETE /sessions/:id` — Delete session
- `POST /prompts` — Capture user prompt
- `POST /observations/passive` — Save passive observation (from Task tool output)
- `POST /projects/migrate` — Migrate project name
- `GET /context?project=X` — Get context for compaction

## Phase Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 | ✅ Done | SQLite FTS5, MCP server, BM25 search |
| Phase 2 | ✅ Done | HTTP server + plugin hooks + git sync + passive extraction |
| Phase 3a | ✅ Done | Local semantic search (ONNX 90MB + sqlite-vec, 384 dims) |
| Phase 3b | 🔲 Planned | Cloud shared memory (Firestore kNN + external APIs) |
| Phase 4 | 🔲 Planned | Admin CLI, observability, production polish |

See `docs/PHASE3_TECH_SPEC.md` for detailed architecture and `docs/tickets/` for implementation tickets.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, invoke the `skill` tool with `skill: "graphify"` before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
