# AGENTS.md - Echo Development Guide

This file provides context for AI agents working on the Echo codebase.

## Project Overview

Echo is a shared team memory layer for AI agents. It's a Go MCP server that stores learnings in SQLite (Phase 1) with FTS5 full-text search, evolving to semantic search (Phase 2) and cloud shared memory (Phase 3).

## Architecture

Clean Architecture with dependency flow: `domain` → `usecase` → `infrastructure` → `cmd`.

```
cmd/echo/main.go          → Entry point, CLI (Cobra), dependency wiring
internal/domain/          → Interfaces (TextStore, Embedder, ProjectDetector, IdentityDetector)
internal/usecase/         → Business logic (SaveLearning, SearchLearning, GetPolicies)
internal/infrastructure/  → Implementations (SQLite FTS5, Git detectors, MCP server)
internal/pkg/secret/      → Secret detection patterns
internal/config/          → Configuration management
internal/e2e/             → End-to-end integration tests
```

## Key Decisions

1. **Phase 1 uses SQLite FTS5 (BM25), not vector search.** We start with lexical search to validate the product before adding ML complexity. BM25 covers ~70% of developer queries.

2. **Embedder interface is provider-agnostic.** It's `nil` in Phase 1, configurable in Phase 2 (Vertex AI, OpenAI, Cohere). The usecase checks `if s.embedder != nil` before generating embeddings.

3. **All data lives in `~/.config/echo/`** (XDG convention), not `~/.echo/`. Database: `~/.config/echo/echo.db`.

4. **FTS5 uses external content table pattern.** `learnings_data` stores all fields, `learnings_fts` indexes only searchable text columns. Triggers maintain sync.

5. **SQLite doesn't support `[]float32` directly.** Embeddings are serialized to JSON before storing, deserialized on read.

## Running Tests

```bash
go test ./...           # All 80 tests
go test ./internal/...  # Unit + integration tests
go test ./internal/e2e/... # End-to-end tests
```

## Adding a New Feature

1. Define interfaces in `internal/domain/`
2. Implement in `internal/infrastructure/`
3. Create usecase in `internal/usecase/`
4. Wire in `cmd/echo/main.go`
5. Add tests in the appropriate `_test.go` file or `internal/e2e/`

## MCP Tools

- `save_learning` — Save a learning (validates, detects project/identity, scans secrets, saves)
- `search_learning` — Search learnings (detects project, searches with BM25, returns ranked results)
- `get_critical_policies` — Return always_inject learnings for the current project

## Phase Status

| Phase | Status | Storage | Embeddings |
|-------|--------|---------|------------|
| Phase 1 | ✅ Done | SQLite FTS5 | None (noop) |
| Phase 2 | 🔲 Planned | SQLite + sqlite-vec | Configurable API |
| Phase 3 | 🔲 Planned | Firestore | Same as Phase 2 |
| Phase 4 | 🔲 Planned | Same as Phase 3 | Same as Phase 3 |

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, invoke the `skill` tool with `skill: "graphify"` before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
