# Graph Report - echo  (2026-05-26)

## Corpus Check
- 28 files · ~20,953 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 331 nodes · 453 edges · 21 communities (16 shown, 5 thin omitted)
- Extraction: 86% EXTRACTED · 14% INFERRED · 0% AMBIGUOUS · INFERRED: 63 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_Community 0|Community 0]]
- [[_COMMUNITY_Community 1|Community 1]]
- [[_COMMUNITY_Community 2|Community 2]]
- [[_COMMUNITY_Community 3|Community 3]]
- [[_COMMUNITY_Community 4|Community 4]]
- [[_COMMUNITY_Community 5|Community 5]]
- [[_COMMUNITY_Community 6|Community 6]]
- [[_COMMUNITY_Community 7|Community 7]]
- [[_COMMUNITY_Community 8|Community 8]]
- [[_COMMUNITY_Community 9|Community 9]]
- [[_COMMUNITY_Community 10|Community 10]]
- [[_COMMUNITY_Community 11|Community 11]]
- [[_COMMUNITY_Community 12|Community 12]]
- [[_COMMUNITY_Community 13|Community 13]]
- [[_COMMUNITY_Community 14|Community 14]]
- [[_COMMUNITY_Community 15|Community 15]]
- [[_COMMUNITY_Community 16|Community 16]]
- [[_COMMUNITY_Community 17|Community 17]]
- [[_COMMUNITY_Community 18|Community 18]]

## God Nodes (most connected - your core abstractions)
1. `setupTestEnv()` - 16 edges
2. `NewSaveLearning()` - 15 edges
3. `newTestStore()` - 14 edges
4. `SQLiteFTS5Store` - 11 edges
5. `saveViaStore()` - 10 edges
6. `setupTestServer()` - 10 edges
7. `testLearning()` - 10 edges
8. `setupGitRepo()` - 10 edges
9. `4. Detailed Design` - 10 edges
10. `runServe()` - 9 edges

## Surprising Connections (you probably didn't know these)
- `runServe()` --calls--> `NewGitProjectDetector()`  [INFERRED]
  cmd/echo/main.go → internal/infrastructure/detector/git.go
- `runServe()` --calls--> `NewGitIdentityDetector()`  [INFERRED]
  cmd/echo/main.go → internal/infrastructure/detector/identity.go
- `runServe()` --calls--> `NewSQLiteFTS5Store()`  [INFERRED]
  cmd/echo/main.go → internal/infrastructure/store/sqlite_fts5.go
- `runServe()` --calls--> `NewSaveLearning()`  [INFERRED]
  cmd/echo/main.go → internal/usecase/save_learning.go
- `runServe()` --calls--> `NewSearchLearning()`  [INFERRED]
  cmd/echo/main.go → internal/usecase/search_learning.go

## Communities (21 total, 5 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.05
Nodes (40): 4.1 Data Model, 4.2 MCP Tools, 4.3 Project Detection & Normalization, 4.4 Admin CLI: Company Brain Management, 4.5 Duplicate Detection & Knowledge Evolution, 4.6 Firestore Indexes (Phase 3), 4.7 Embedding Integration (Phase 2+), 4.8 MCP Protocol Contract (+32 more)

### Community 1 - "Community 1"
Cohesion: 0.09
Nodes (18): TestMkdirAll(), Learning, NewLearning(), TestLearning_UpdatedAt(), TestLearningType_Valid(), TestNewLearning(), TestScope_Valid(), LearningType (+10 more)

### Community 2 - "Community 2"
Cohesion: 0.12
Nodes (23): SecretError, saveViaStore(), saveViaUsecase(), searchViaStore(), setupTestEnv(), TestE2E_AlwaysInjectPolicies(), TestE2E_BM25Ranking(), TestE2E_DeleteAndVerifyGone() (+15 more)

### Community 3 - "Community 3"
Cohesion: 0.09
Nodes (21): mockEmbedder, mockIdentityDetector, mockProjectDetector, cosineSimilarity(), NewSaveLearning(), TestCosineSimilarity(), TestCosineSimilarity_DifferentDimensions(), TestSaveLearning_Execute_EmptyAnswer() (+13 more)

### Community 4 - "Community 4"
Cohesion: 0.13
Nodes (20): DetectError, NewGitProjectDetector(), normalizeProjectURL(), runGit(), setupGitRepo(), TestGitIdentityDetector_Detect(), TestGitIdentityDetector_Detect_Email(), TestGitIdentityDetector_Detect_NotGitRepo() (+12 more)

### Community 5 - "Community 5"
Cohesion: 0.07
Nodes (26): 1. Context, 2. Goals & Non-Goals, 3.1 Sequence Diagram (Phase 3 — Cloud Mode), 3.2 High-Level Components (Phase 3 — Cloud Mode), 3. Architecture, 5. Alternative Solutions, 7. Implementation Plan, 8. Usage Scenario (+18 more)

### Community 6 - "Community 6"
Cohesion: 0.11
Nodes (16): Config, Default(), Load(), runServe(), mockIdentityDetector, mockProjectDetector, NewServer(), setupTestServer() (+8 more)

### Community 7 - "Community 7"
Cohesion: 0.09
Nodes (22): Admin CLI, code:bash (go install github.com/company/echo/cmd/echo@latest), code:json ({), code:json ({), code:bash (# Phase 1: Local lexical search (zero config, zero dependenc), code:bash (# Add a global rule (Phase 4)), code:block6 (cmd/echo/              - CLI entry point (Cobra)), code:bash (# Run all tests) (+14 more)

### Community 8 - "Community 8"
Cohesion: 0.13
Nodes (9): GetPoliciesInput, GetPoliciesOutput, PolicyResult, SaveLearningInput, SaveLearningOutput, SearchLearningInput, SearchLearningOutput, SearchLearningResult (+1 more)

### Community 9 - "Community 9"
Cohesion: 0.33
Nodes (14): newTestStore(), testLearning(), TestSQLiteFTS5Store_Delete(), TestSQLiteFTS5Store_Delete_NotFound(), TestSQLiteFTS5Store_GetAlwaysInject(), TestSQLiteFTS5Store_GetByID_NotFound(), TestSQLiteFTS5Store_SaveAndGetByID(), TestSQLiteFTS5Store_SaveAutoID() (+6 more)

### Community 10 - "Community 10"
Cohesion: 0.14
Nodes (14): 6.1 Authentication, 6.2 Authorization, 6.3 Observability, 6.4 Data Retention, 6.5 Privacy & Secret Sanitization, 6.6 Scalability & Performance, 6. Security & Observability, code:block21 (✅ GOOD:  DATABASE_URL=postgresql://user:<STAGING_DB_PASSWORD) (+6 more)

### Community 11 - "Community 11"
Cohesion: 0.17
Nodes (11): Adding a New Feature, AGENTS.md - Echo Development Guide, Architecture, code:block1 (cmd/echo/main.go          → Entry point, CLI (Cobra), depend), code:bash (go test ./...           # All 80 tests), graphify, Key Decisions, MCP Tools (+3 more)

### Community 14 - "Community 14"
Cohesion: 0.40
Nodes (4): SaveInput, SearchQuery, SearchResult, TextStore

### Community 15 - "Community 15"
Cohesion: 0.50
Nodes (3): MigrationPhase, DetectPhase(), MigrateToVector()

## Knowledge Gaps
- **99 isolated node(s):** `plugin`, `SaveOutput`, `PoliciesOutput`, `SearchOutput`, `Learning` (+94 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `runServe()` connect `Community 6` to `Community 1`, `Community 3`, `Community 4`?**
  _High betweenness centrality (0.119) - this node is a cross-community bridge._
- **Why does `NewSQLiteFTS5Store()` connect `Community 1` to `Community 9`, `Community 2`, `Community 6`?**
  _High betweenness centrality (0.115) - this node is a cross-community bridge._
- **Why does `NewSaveLearning()` connect `Community 3` to `Community 2`, `Community 6`?**
  _High betweenness centrality (0.105) - this node is a cross-community bridge._
- **Are the 14 inferred relationships involving `NewSaveLearning()` (e.g. with `runServe()` and `TestSaveLearning_Execute_Success()`) actually correct?**
  _`NewSaveLearning()` has 14 INFERRED edges - model-reasoned connections that need verification._
- **What connects `plugin`, `SaveOutput`, `PoliciesOutput` to the rest of the system?**
  _99 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.05 - nodes in this community are weakly interconnected._
- **Should `Community 1` be split into smaller, more focused modules?**
  _Cohesion score 0.08888888888888889 - nodes in this community are weakly interconnected._