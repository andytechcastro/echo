# Tech Spec: Echo

## 1. Context

Developers across the company use different AI agents (Cursor, Claude, Copilot, Windsurf) to assist with daily work. Each agent operates in isolation — knowledge discovered by one developer is lost to the rest of the team. This results in repeated investigation of the same problems, duplicated effort, and wasted tokens.

Echo is a shared team memory layer that sits between developers and their AI agents. When one developer resolves something (a config issue, a bug, a pattern, an architectural decision), Echo captures it. When another developer on the same project encounters the same situation, their agent retrieves the existing solution instantly.

## 2. Goals & Non-Goals

### Goals
- Enable AI agents to share knowledge across developers on the same project
- Provide a simple MCP server with `save_learning` and `search_learning` tools
- Store all data in English for universal searchability
- Filter results by project so teams only see relevant knowledge
- Start with **Phase 1: Local lexical search** (SQLite FTS5, zero dependencies)
- Evolve to **Phase 2: Local semantic search** (sqlite-vec + Vertex AI embeddings API)
- Scale to **Phase 3: Cloud shared memory** (GCP Firestore + Vertex AI)
- Demonstrate end-to-end flow on 2 machines with different agents as a proof of concept

### Non-Goals
- No UI/dashboard in v1 (CLI + agent integration only)
- No automatic bug detection (agent-triggered capture only)
- No cross-project knowledge sharing (project-scoped only)
- No translation service in Echo (translation is handled by the agent)
- No local embedding models (external API is used for quality and multilingual support)
- No vendor lock-in on embeddings (configurable `Embedder` interface)

## 3. Architecture

### 3.1 Sequence Diagram (Phase 3 — Cloud Mode)

> **Note:** Phase 1 (lexical) and Phase 2 (local vectors) follow the same flow but without GCP services. Phase 1 uses SQLite FTS5 only. Phase 2 adds a configurable `Embedder` (Vertex AI default) but stores in SQLite.

```mermaid
sequenceDiagram
    participant DevA as Dev A (Cursor)
    participant AgentA as Agent A
    participant MCP as MCP Server (Go)
    participant VA as Vertex AI (GCP)
    participant FS as Firestore (GCP)
    participant AgentB as Agent B
    participant DevB as Dev B (Claude/VS Code)

    DevA->>AgentA: "How do I connect to DB locally?"
    AgentA->>AgentA: Researches, finds solution
    AgentA->>MCP: save_learning(type, question, answer, tags)
    MCP->>VA: Generate embedding(question + answer)
    VA-->>MCP: Embedding vector (768 floats)
    MCP->>FS: Write document (with embedding)
    FS-->>MCP: OK
    MCP-->>AgentA: Learning saved

    DevB->>AgentB: "How do I connect to DB locally?"
    AgentB->>MCP: search_learning(query)
    MCP->>VA: Generate embedding(query)
    VA-->>MCP: Embedding vector
    MCP->>FS: findNearest(embedding, project filter)
    FS-->>MCP: Return top 5 matches
    MCP-->>AgentB: Found: "Dev A solved this. Solution: ..."
    AgentB->>DevB: "This was already solved by Dev A. Here's the config: ..."
```

### 3.2 High-Level Components (Phase 3 — Cloud Mode)

> **Note:** Phase 1 replaces GCP services with local SQLite. Phase 2 uses a configurable `Embedder` (Vertex AI default) for embeddings but stores in SQLite locally.

```
┌─────────────────────────────────────────────────────┐
│                   Developer Machine                 │
│                                                     │
│  ┌──────────┐    ┌─────────────┐    ┌─────────────┐ │
│  │ IDE/Agent│◄──►│ MCP Client  │◄──►│ MCP Server  │ │
│  │(Cursor,  │    │ (built-in)  │    │   (Go)      │ │
│  │ Claude)  │    └─────────────┘    │             │ │
│  └──────────┘                       │  Tools:     │ │
│                                     │  save_      │ │
│                                     │  search_    │ │
│                                     └─────┬───────┘ │
└───────────────────────────────────────────┼─────────┘
                                            │
                            ┌───────────────┼───────────────┐
                            │              GCP              │
                            │                               │
                    ┌───────▼───────┐       ┌─────────────┐ │
                    │   Firestore   │       │  Vertex AI  │ │
                    │ (Storage +    │       │ (Embeddings)│ │
                    │ Vector Search)│       │             │ │
                    │               │       │ text-embed- │ │
                    │  Collection:  │       │ ding-004    │ │
                    │  learnings    │       │             │ │
                    └───────────────┘       └─────────────┘ │
                            └───────────────────────────────┘
```

## 4. Detailed Design

### 4.1 Data Model

**Storage:** SQLite (Phase 1-2) / Firestore (Phase 3)

| Field | Type | Description |
|---|---|---|
| `id` | string (auto) | SQLite autoincrement / Firestore auto-generated ID |
| `project` | string | Git remote URL (e.g., `github.com/company/repo-x`) OR `*` for global scope |
| `scope` | string | `project` (default) or `organization` (company-wide rules) |
| `always_inject` | boolean | If true, this learning is returned automatically at session start (e.g., critical policies) |
| `type` | string | One of: `config`, `pattern`, `bugfix`, `decision`, `process`, `domain`, `gotcha` |

**Type Definitions:**
| Type | Description | Example |
|---|---|---|
| `config` | Environment setup, credentials, connections | "How to connect to the staging DB" |
| `pattern` | Code structure, reusable patterns | "We use Repository pattern for DB access" |
| `bugfix` | Specific error resolution | "Fix for 502 Bad Gateway on /api/users" |
| `decision` | Architectural or tooling choices | "Why we chose Firestore over Mongo" |
| `process` | Team workflows, deployment rules | "Deployments must be approved by Ops after 4pm" |
| `domain` | Business logic, terminology | "'Invoice' includes both charges and credits" |
| `gotcha` | Non-obvious warnings, edge cases | "External API fails silently if payload > 50 items" |
| `question` | string | The original question or problem (English) |
| `answer` | string | The solution or resolution (English) |
| `reasoning` | string | **Why** this solution was chosen (trade-offs, alternatives rejected) |
| `location` | string | **Where** this applies (file paths, modules, e.g., `src/auth/middleware.ts`) |
| `notes` | string | **Learned** (gotchas, edge cases, warnings, "don't do X because Y") |
| `tags` | []string | Searchable tags, always in English |
| `embedding` | []float32 | Embedding vector (N dimensions, matches configured `Embedder`) for semantic search |
| `resolvedBy` | string | Developer identity (from GCP IAM or git config) |
| `createdAt` | timestamp | When the learning was saved |
| `updatedAt` | timestamp | Last time the learning was updated |

### 4.2 MCP Tools

#### `save_learning`

**Description:** Save a resolved issue, config, pattern, or decision to the team knowledge base.

**Input:**
```json
{
  "type": "config | pattern | bugfix | decision | process | domain | gotcha",
  "question": "string — the problem that was solved",
  "answer": "string — the solution",
  "reasoning": "string — why this solution was chosen (trade-offs, context)",
  "location": "string — affected files/modules (e.g., src/auth/middleware.ts)",
  "notes": "string — gotchas, edge cases, warnings (lessons learned)",
  "tags": ["string"] — always in English
}
```

**Output:**
```json
{
  "id": "string",
  "project": "string (auto-detected from git remote)",
  "resolvedBy": "string (auto-detected from IAM or git config)",
  "message": "Learning saved successfully"
}
```

**Behavior (Phase-Dependent):**

| Step | Phase 1 (Lexical) | Phase 2 (Local Vectors) | Phase 3 (Cloud) |
|---|---|---|---|
| 1 | Language normalization (agent translates to English) | Same | Same |
| 2 | Scope enforcement (`project` only) | Same | Same |
| 3 | Auto-detect `project` from git remote | Same | Same |
| 4 | Auto-detect `resolvedBy` | Same | Same |
| 5 | — (no embeddings) | Call configured `Embedder` (default: Vertex AI) | Call configured `Embedder` (default: Vertex AI) |
| 6 | **Lexical duplicate check:** exact question match | **Semantic duplicate check:** kNN pre-filter (top 20, cosine > 0.92) | **Semantic duplicate check:** Firestore `findNearest()` (top 20, cosine > 0.92) |
| 7 | Update if exact match, else create | Update if similarity > 0.92, else create | Same |
| 8 | Return success with document ID | Same | Same |

#### `search_learning`

**Description:** Search the team knowledge base for existing solutions related to a problem.

**Input:**
```json
{
  "query": "string — the problem or question (English)",
  "tags": ["string"] — optional filter tags (English)
}
```

**Output:**
```json
{
  "results": [
    {
      "id": "string",
      "project": "string",
      "type": "string",
      "question": "string",
      "answer": "string",
      "reasoning": "string",
      "location": "string",
      "notes": "string",
      "tags": ["string"],
      "resolvedBy": "string",
      "createdAt": "timestamp",
      "relevanceScore": "number"
    }
  ],
  "count": "number"
}
```

**Behavior (Phase-Dependent):**

| Step | Phase 1 (Lexical) | Phase 2 (Local Vectors) | Phase 3 (Cloud) |
|---|---|---|---|
| 1 | Auto-detect `project` from git remote | Same | Same |
| 2 | Language normalization (agent translates query to English) | Same | Same |
| 3 | — (no embeddings) | Call configured `Embedder` (default: Vertex AI) | Call configured `Embedder` (default: Vertex AI) |
| 4 | **FTS5 query:** `MATCH <query>` with BM25 ranking | **Vector search:** `vec_distance_cosine()` ORDER BY, limit 5 | **Firestore:** `findNearest(embedding, project filter, limit=5)` |
| 5 | Return top 5 results by BM25 score | Return top 5 results by cosine similarity | Return top 5 results by similarity score |
| 6 | Agent translates results to user's language | Same | Same |

### 4.3 Project Detection & Normalization

The system must generate a **canonical project ID** regardless of how the developer cloned the repository (SSH, HTTPS, Git protocol, or with credentials).

**Normalization Logic:**
1.  **Run:** `git remote get-url origin`
2.  **Strip Protocol & Auth:**
    - Remove `git@`, `https://`, `git://`, `ssh://`
    - Remove user:token prefixes (e.g., `user:ghp_123@`)
3.  **Standardize Separators:**
    - Replace `:` with `/` (common in SSH URLs like `git@github.com:org/repo.git`)
4.  **Remove Suffixes:**
    - Remove trailing `.git`
5.  **Lowercase:** Ensure consistency.

**Examples:**
| Input (git remote) | Normalized Project ID |
|---|---|
| `git@github.com:empresa/repo-x.git` | `github.com/empresa/repo-x` |
| `https://github.com/empresa/repo-x.git` | `github.com/empresa/repo-x` |
| `https://user:token@github.com/empresa/repo-x` | `github.com/empresa/repo-x` |
| `git@gitlab.internal:team/backend.git` | `gitlab.internal/team/backend` |

**Implementation (Go):**
```go
func detectProject() (string, error) {
    rawURL := exec.Command("git", "remote", "get-url", "origin").Output()
    // Regex to strip protocols, auth, and .git suffix
    // Result: "github.com/company/repo"
    return normalize(rawURL), nil
}
```

### 4.4 Admin CLI: Company Brain Management

Organization-scoped learnings (`scope: organization`) are **admin-only**. AI agents cannot create or modify them. This ensures company-wide rules are curated and controlled by Tech Leads, Architects, or Engineering Managers.

**Important:** The Admin CLI is **not an MCP tool**. It's a subcommand of the same `echo` binary that writes directly to the storage backend (SQLite or Firestore), bypassing the MCP server entirely. Agents can only **read** organization-scoped learnings via `get_critical_policies` — they cannot write them.

```
echo serve              → MCP server (agents use this)
echo admin add ...      → Admin CLI (humans use this, writes directly to storage)
echo admin list ...     → Admin CLI
echo admin update ...   → Admin CLI
echo admin delete ...   → Admin CLI
```

**Commands:**
```bash
# Add a global rule (requires admin IAM role)
echo admin add --scope organization \
  --type process \
  --question "Deployment windows policy" \
  --answer "Deployments to production are only allowed Tue-Thu, 10am-3pm UTC" \
  --always-inject

# List all global rules
echo admin list --scope organization

# Update a global rule
echo admin update <id> --answer "Updated policy text..."

# Delete a global rule
echo admin delete <id>
```

**Auth Enforcement:**
- Admin commands require `roles/echo.admin` IAM role.
- MCP tool `save_learning` rejects any `scope: organization` write attempt from agents.

**Always-Inject Behavior:**
- Learnings with `always_inject: true` are returned automatically when the agent calls `get_critical_policies` at session start.
- This ensures every developer's agent knows company rules before writing any code.

### 4.5 Duplicate Detection & Knowledge Evolution

**Problem:** Fetching all learnings for a project and comparing embeddings one-by-one is **O(n)** — it reads every document into memory before writing. With 100K+ documents, this is slow and expensive.

**Solution:** Use Firestore's native vector search as a **pre-filter**. Only compare the top-N closest results.

**Algorithm:**

1. Generate embedding from the new enriched content (`question + answer + reasoning + notes`).
2. **kNN Pre-Filter:** Query Firestore vector search for the **top 20 nearest neighbors** within the same `project`.
3. **Cosine Similarity Check:** Compare the new embedding against only those 20 candidates:

| Max Similarity (among top 20) | Action |
|---|---|
| **> 0.92** (Near-Duplicate) | **Update** the existing document. Overwrite `answer`, `reasoning`, `location`, `notes`. **Recalculate and replace the `embedding`** to reflect the new solution. Update `updatedAt`. |
| **0.70 – 0.92** (Related) | **Create new.** These are distinct but related topics (e.g., "DB connection" vs "DB optimization"). Both should coexist. |
| **< 0.70** (Unrelated) | **Create new.** No existing knowledge covers this topic. |

**Why top 20?** The threshold of 0.92 is very high — it means near-identical semantic content. If a near-duplicate exists, it will almost certainly appear in the top 20 nearest neighbors. Checking more yields diminishing returns while adding latency and cost.

**Evolution Logic (Update vs Create):**
- **Update:** When similarity > 0.92, the system recognizes this as the *same topic* with potentially *new information*. The embedding is recalculated because if the solution changes (e.g., from "restart service" to "apply patch v1.2"), the vector must move to the new semantic location so future searches for "patch" find it.
- **Create:** When similarity < 0.92, this is distinct knowledge worth preserving separately.

**Performance:**
- **Before (O(n)):** Read all project documents → compute N cosine similarities → decide. Cost: N reads + N compute ops.
- **After (O(k)):** kNN search (1 index read) → compute 20 cosine similarities → decide. Cost: ~1 read + 20 compute ops.
- **Improvement:** At 100K documents, this is **~5,000x fewer reads** per save operation.

### 4.6 Firestore Indexes (Phase 3)

| Collection | Fields | Type |
|---|---|---|
| `learnings` | `project` (ASC), `createdAt` (DESC) | Composite |
| `learnings` | `embedding` | **Vector Index** (N dims = Embedder.Dimensions(), COSINE distance) |

**Vector Search Configuration:**
- **Distance Metric:** Cosine similarity
- **Dimensions:** Matches configured `Embedder` (768 for Vertex AI, 1536 for OpenAI, etc.)
- **Filtering:** Combined with `project` equality filter for scoped search
- **Scalability:** Handles 100K+ documents efficiently without fetching all into memory

**Cost Efficiency:**
- Vector search is billed as index reads: 1 read per 100 kNN entries
- For typical usage (top 5 results), cost is **~1 read per query**
- With 50,000 free reads/day, this is **$0 for most teams**

### 4.7 Embedding Integration (Phase 2+)

Echo uses a **provider-agnostic `Embedder` interface**. The default is Vertex AI, but any embedding API works.

**Default Provider: Vertex AI `text-embedding-004`**

| Property | Value |
|---|---|
| **Dimensions** | 768 |
| **Max input** | 2,048 tokens |
| **Languages** | 100+ (multilingual) |
| **Pricing** | $0.000025 / 1,000 characters (online) |
| **Auth** | GCP ADC (`roles/aiplatform.user`) |

**Alternative Providers:**

| Provider | Model | Dimensions | Pricing |
|---|---|---|---|
| **OpenAI** | `text-embedding-3-small` | 1536 | $0.02 / 1M tokens |
| **Cohere** | `embed-multilingual-v3` | 1024 | $0.10 / 1M tokens |

**Role in Architecture:**
The embedding provider is used **exclusively for embedding generation** (converting text to vectors). It does NOT store data, perform search, or manage memory. All storage and retrieval is handled by the configured `TextStore` (SQLite or Firestore).

**Embedding Strategy:**
The embedding is generated from the **full enriched context** (`question + answer + reasoning + notes`). This ensures the semantic vector captures not just the solution, but the *trade-offs* and *lessons learned*, making duplicate detection and search significantly more accurate.

### 4.8 MCP Protocol Contract

Echo implements the **Model Context Protocol (MCP)** over **stdio** transport. This allows any MCP-compatible agent (Cursor, Claude Desktop, VS Code, Windsurf) to communicate with the Echo server without network configuration.

**Transport:** `stdio` (standard input/output)
**Protocol:** JSON-RPC 2.0

#### Connection Lifecycle

```
┌─────────────┐                          ┌─────────────┐
│  AI Agent    │                          │ Echo MCP    │
│  (Client)    │                          │  Server     │
├─────────────┤                          ├─────────────┤
│             │── initialize ────────────►│             │
│             │◄── capabilities response ─│             │
│             │                          │             │
│             │── tools/list ────────────►│             │
│             │◄── [save_learning,       │             │
│             │     search_learning,     │             │
│             │     get_critical_policies]│            │
│             │                          │             │
│             │── tools/call             │             │
│             │   (save_learning) ──────►│             │
│             │◄── result                │             │
│             │                          │             │
│             │── tools/call             │             │
│             │   (search_learning) ────►│             │
│             │◄── result                │             │
└─────────────┴──────────────────────────┴─────────────┘
```

#### Message Format

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "save_learning",
    "arguments": {
      "type": "config",
      "question": "How to connect to the staging database?",
      "answer": "Set DATABASE_URL=postgresql://...",
      "reasoning": "We use PostgreSQL for relational data...",
      "location": ".env.staging, config/database.go",
      "notes": "Connection pool max is 20. Do not exceed.",
      "tags": ["database", "postgresql", "staging", "connection"]
    }
  }
}
```

**Success Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"id\": \"abc123\", \"project\": \"github.com/company/repo\", \"resolvedBy\": \"andres\", \"message\": \"Learning saved successfully\"}"
      }
    ],
    "isError": false
  }
}
```

**Error Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"error\": \"scope 'organization' is admin-only. Use 'project' scope.\"}"
      }
    ],
    "isError": true
  }
}
```

#### Server Capabilities

The server declares these capabilities on `initialize`:

```json
{
  "protocolVersion": "2024-11-05",
  "capabilities": {
    "tools": {
      "listChanged": false
    }
  },
  "serverInfo": {
    "name": "echo",
    "version": "0.1.0"
  }
}
```

#### Configuration (Agent-side)

Agents connect to Echo via their MCP configuration file:

**Cursor / VS Code (`mcp.json`):**
```json
{
  "mcpServers": {
    "echo": {
      "command": "echo",
      "args": ["serve"]
    }
  }
}
```

**Local Mode (Phase 1 — default):**
```json
{
  "mcpServers": {
    "echo": {
      "command": "echo",
      "args": ["serve"]
    }
  }
}
```

**Local Semantic (Phase 2):**
```json
{
  "mcpServers": {
    "echo": {
      "command": "echo",
      "args": ["serve", "--embeddings"]
    }
  }
}
```

**Cloud Mode (Phase 3):**
```json
{
  "mcpServers": {
    "echo": {
      "command": "echo",
      "args": ["serve", "--cloud"]
    }
  }
}
```

### 4.9 Evolution Strategy: From Local to Cloud

Echo is built with **Hexagonal Architecture** from day one. The interfaces (`TextStore`, `Embedder`) never change — only the implementations do. This allows us to evolve the storage backend incrementally without rewriting the MCP server or tool logic.

```
Phase 1              Phase 2              Phase 3
───────────          ───────────          ───────────
SQLite + FTS5   →    SQLite + sqlite-vec  →  Firestore
  (lexical)            (+ API embeddings)     (cloud, shared)
  Zero deps            CGO (build only)       GCP IAM
  Single machine       Single machine         Multi-machine sync
  BM25 ranking         Cosine similarity      kNN vector search
```

#### Phase 1: Local Lexical Search (Complete Service)

**Storage:** SQLite with FTS5 virtual table (`modernc.org/sqlite` — pure Go)
**Search:** BM25 full-text ranking (lexical, not semantic)
**Dependencies:** Zero. No CGO, no external services, no API keys.

**Why start here:**
- **Validate the product** before adding complexity. Does the team actually use Echo?
- **Zero friction** for developers — `echo serve` and it works. No config, no credentials.
- **BM25 covers ~70% of developer queries** — technical terms, error codes, config keys are exact-match friendly.
- **Hexagonal interfaces** mean we can swap the backend later without touching MCP logic.

**Data Location:** `~/.config/echo/echo.db` (single SQLite file)

**Limitations:**
- "DB" does NOT find "database" — lexical matching only.
- No semantic understanding — queries must use similar vocabulary to stored learnings.
- Single machine only — no team sharing.

#### Phase 2: Local Semantic Search (Vectors via API)

**Storage:** SQLite + `sqlite-vec` extension (`mattn/go-sqlite3` + `asg017/sqlite-vec`)
**Embeddings:** External API — configurable provider (Vertex AI default, OpenAI, Cohere)
**Dependencies:** CGO (build time only), API credentials for the chosen embedding provider.

**Why sqlite-vec over LanceDB:**
- We already have SQLite from Phase 1 — sqlite-vec is a **natural extension**, not a replacement.
- Full SQL for metadata filtering, transactions, and ACID durability.
- Single backend covers Linux, macOS, and Windows.
- Brute-force cosine similarity = **recall 1.00** (exact, no approximate trade-offs).
- Performance (~1.7s for 100K vectors) is acceptable for a CLI tool used occasionally.

**Trade-offs:**
- CGO required at build time (gcc/clang). End users still get a clean single binary.
- Embeddings require an external API call — not fully offline. This is intentional: we prioritize embedding quality over offline capability.
- Brute-force search (no ANN index yet). Acceptable for <100K learnings per project.
- **Embedding provider is configurable** — Vertex AI (default), OpenAI, or Cohere. Swappable via `--embedder` flag.

**Data Location:** `~/.config/echo/echo.db` (same file, upgraded schema with vector tables)

**Upgrade Path from Phase 1:**
- Existing FTS5 data is preserved.
- New learnings get both FTS5 entries AND vector embeddings.
- Search queries check vectors first; fallback to FTS5 if no embeddings exist.

**Schema Migration (Phase 1 → Phase 2):**
- On first run with `--embeddings`, Echo detects the existing Phase 1 database and runs a migration:
  1. Creates `vec_learnings` virtual table with `FLOAT[N]` vector column (N = configured Embedder dimensions).
  2. Preserves existing `learnings` and `learnings_fts` tables intact.
  3. Backfills embeddings for existing learnings (calls Embedder API for each stored learning).
- If the user changes embedding providers (e.g., Vertex AI 768 dims → OpenAI 1536 dims), a **re-migration** is required:
  1. Drop existing `vec_learnings` table.
  2. Create new `vec_learnings` with updated dimensions.
  3. Re-embed all learnings with the new provider.
- Migration is automatic and logged. The FTS5 data is never affected — it survives any vector schema change.

#### Phase 3: Cloud Mode (Firestore + Shared Memory)

**Storage:** GCP Firestore with native `findNearest()` vector search.
**Embeddings:** Same configurable `Embedder` interface from Phase 2 (Vertex AI default).
**Auth:** GCP IAM (Application Default Credentials) for Firestore only — embedding provider auth is separate.
**Use Case:** Teams that want shared memory across machines, with automatic sync and zero maintenance.

**Why Firestore over local-only:**
- **Cross-machine sync** — Dev A saves, Dev B retrieves. This is Echo's core value proposition.
- **Native vector search** — `findNearest()` with kNN indexing, not brute-force.
- **Combined doc+vector storage** — no need for a separate vector database.
- **Cost-effective** — ~$13/mo for 300 developers.

**Auth separation:**
- **Firestore:** Requires GCP IAM (`roles/datastore.user`)
- **Embeddings:** Requires the configured provider's auth (GCP ADC for Vertex AI, API key for OpenAI/Cohere)
- These are independent — you can use Firestore + OpenAI, or Firestore + Vertex AI.

**Estimated Monthly Cost (300 Developers, Vertex AI default):**
| Component | Volume | Cost |
|---|---|---|
| **Embeddings (Vertex AI)** | ~15,000/day (50/dev) | ~$0.375/day → **$11.25/mo** |
| **Storage (Firestore)** | ~15,000 writes/day | ~$0.054/day → **$1.62/mo** |
| **Vector Search (Firestore)** | ~15,000 reads/day | ~150 reads/day → **$0.00 (Free Tier)** |
| **Total** | | **~$13/mo for 300 devs** |

> Costs vary by embedding provider. OpenAI `text-embedding-3-small` is ~$0.02/1M tokens (cheaper for short texts). Cohere is ~$0.10/1M tokens.

#### Architecture Comparison

| Aspect | Phase 1 (Lexical) | Phase 2 (Local Vectors) | Phase 3 (Cloud) |
|---|---|---|---|
| **Store Interface** | `SQLiteFTS5Store` | `SQLiteVecStore` | `FirestoreStore` |
| **Embedder** | `NoEmbedder` (noop) | `Embedder` (configurable: Vertex/OpenAI/Cohere) | `Embedder` (same as Phase 2) |
| **CGO** | ❌ No (pure Go) | ✅ Yes (build only) | ❌ No |
| **External API** | ❌ No | ✅ Embedding provider | ✅ Embedding provider + Firestore |
| **Search Type** | BM25 (lexical) | Cosine (semantic) | kNN (semantic) |
| **Sync** | None (local file) | None (local file) | Automatic (cloud) |
| **Latency (100K)** | ~5ms | ~1.7s | ~50ms |
| **Recall** | Lexical match | 1.00 (exact) | ~0.98 (ANN) |
| **Cost** | $0 | ~$0.04/1K embeddings | ~$13/mo (300 devs) |

#### Startup Flags

```bash
# Phase 1: Local lexical (default, zero config)
echo serve

# Phase 2: Local semantic (requires embedding provider auth: GCP ADC, OpenAI key, or Cohere key)
echo serve --embeddings

# Phase 3: Cloud mode (shared memory, requires GCP credentials + embedding provider auth)
echo serve --cloud
```

#### Why Not Local Embedding Models (ONNX)?

- **Quality risk:** Lightweight models like `all-MiniLM-L6-v2` perform poorly on non-English text and lack code/technical understanding.
- **Maintenance burden:** Model files to manage, ONNX runtime dependencies, CGO complexity, platform-specific binaries.
- **Negligible savings:** Vertex AI costs ~$0.04/month for a 50-dev team. The cost savings of a local model don't justify the quality risk and maintenance overhead.
- **Phase 2 uses API embeddings** — configurable provider (Vertex AI, OpenAI, Cohere), same quality as Phase 3, zero local model management.
- **Provider is swappable** — if Vertex AI pricing changes or a better model appears, just implement a new `Embedder`. No storage changes needed.

## 5. Alternative Solutions

### Option A: SQLite (local file)
- **Adopted for Phase 1:** SQLite with FTS5 is the foundation. Zero dependencies, pure Go, BM25 ranking covers ~70% of developer queries. Scales to Phase 2 with sqlite-vec extension for vector search.

### Option B: MongoDB Atlas
- **Rejected because:** External dependency, separate auth system (API keys), additional network configuration (IP whitelisting). Firestore provides native GCP IAM integration, zero credential management, and the team already uses GCP.

### Option C: Local embedding models as default (ONNX)
- **Rejected because:** Lightweight models like `all-MiniLM-L6-v2` perform poorly on non-English text, have limited context length (~256 tokens), and lack code/technical understanding. Vertex AI `text-embedding-004` costs ~$0.04/month for a 50-dev team, supports 100+ languages natively, and requires zero maintenance. Phase 2 uses Vertex API for embeddings — same quality, zero local model management.

### Option D: Keyword-only search (FTS5 / Engram-style)
- **Adopted for Phase 1:** BM25 lexical search is the starting point. It's fast, zero-dependency, and covers most technical queries. Phase 2 adds semantic search on top — FTS5 is preserved as a fallback and for hybrid search.

### Option E: Automatic bug detection from conversation
- **Rejected because:** Requires an LLM in the middle to interpret conversations, which adds token cost and false positive risk. Agent-triggered capture (the agent already knows it solved something) is simpler, cheaper, and more reliable.

### Option F: Vertex AI Matching Engine (Dedicated Vector DB)
- **Rejected because:** While it offers higher throughput for massive datasets (10M+ vectors), it requires dedicated nodes costing $100+/month minimum. Firestore's native vector search (`findNearest()`) provides sufficient performance for our scale (<1M learnings) at near-zero cost, with the added benefit of combined document+vector storage in a single service.

### Option G: LanceDB (embedded vector database)
- **Evaluated for Phase 2 but not selected:** LanceDB-go offers faster vector search (6ms vs 1.7s for sqlite-vec) with IVF-PQ indexing. However, we already use SQLite in Phase 1, and sqlite-vec is a natural extension — same SQL interface, same data file, same ACID guarantees. LanceDB would require replacing the storage layer entirely. We may revisit if brute-force performance becomes a bottleneck at scale.

## 6. Security & Observability

### 6.1 Authentication

| Phase | Auth Method | Details |
|---|---|---|
| **Phase 1** | None | Local SQLite file, no credentials required |
| **Phase 2** | Embedding provider auth | GCP ADC (Vertex AI), API key (OpenAI/Cohere), or other provider |
| **Phase 3** | GCP IAM + Embedding provider | `roles/datastore.user` for Firestore + embedding provider auth (separate) |

- **Production (Phase 3):** Service account attached to Cloud Run or GKE workload

### 6.2 Authorization
- **Project scoping:** Every document is tagged with `project`. Queries always filter by detected project, preventing cross-project data leakage
- **No PII:** Developer identity is stored as name/email from git config or IAM, no sensitive data

### 6.3 Observability
- **Structured logging:** JSON logs with `level`, `message`, `project`, `tool`, `duration`, `phase`
- **Metrics (v2):** Prometheus counters for `save_learning_total`, `search_learning_total`, `search_hits_total`, `search_misses_total`
- **Error tracking:** Log all storage errors (SQLite/Firestore) with context for debugging
- **Phase-aware metrics:** Track which storage backend is active per request

### 6.4 Data Retention
- No automatic deletion for v1
- Manual cleanup via Firestore console
- v2: TTL policy for stale learnings (e.g., 90 days without access)

### 6.5 Privacy & Secret Sanitization

Secrets in Echo are dangerous because learnings are **shared across the team**. If Developer A saves a solution containing their actual database password, every developer's agent will see it — and it will be stored in plain text (local SQLite or Firestore).

**Defense in Depth — 4 Layers:**

#### Layer 1: Agent Instruction (Primary)

The AI agent is instructed via system prompt to **never include actual secrets** in Echo tools. Instead, use placeholders:

```
✅ GOOD:  DATABASE_URL=postgresql://user:<STAGING_DB_PASSWORD>@staging.db.internal:5432/echo
❌ BAD:   DATABASE_URL=postgresql://admin:r3alP@ssw0rd123@staging.db.internal:5432/echo
```

The agent knows the difference between a config pattern (which should be saved) and an actual credential (which should not).

#### Layer 2: Pattern-Based Detection (Safety Net)

Before saving, Echo scans `answer`, `reasoning`, and `notes` against known secret patterns. If a match is found, the save is **rejected** with a clear error.

**Detection patterns:**

| Category | Pattern | Example |
|---|---|---|
| **API Keys** | `sk-[a-zA-Z0-9]{20,}` | OpenAI, Anthropic keys |
| **AWS Credentials** | `AKIA[0-9A-Z]{16}` | AWS access key IDs |
| **GitHub Tokens** | `ghp_[a-zA-Z0-9]{36}` | GitHub personal access tokens |
| **Private Keys** | `-----BEGIN (RSA|EC|DSA|OPENSSH) PRIVATE KEY-----` | PEM-encoded keys |
| **Generic Secrets** | `(password|passwd|pwd)\s*[:=]\s*\S{8,}` | Inline passwords |
| **JWT Tokens** | `eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}` | JSON Web Tokens |
| **Connection Strings** | `://[^:]+:[^<]{8,}@` | URLs with embedded passwords (non-placeholder) |

**Implementation:** Go `regexp` package, compiled once at startup, applied to each field before save.

#### Layer 3: Rejection with Guidance

When a secret is detected, Echo returns:

```json
{
  "error": "Potential secret detected in 'answer' field (pattern: connection-string). Replace actual credentials with placeholders (e.g., <DB_PASSWORD>) and retry.",
  "field": "answer",
  "pattern": "connection-string"
}
```

The agent receives this error, redacts the secret, and retries the save. This is an **automatic loop** — no human intervention needed.

#### Layer 4: Audit Log

All rejected saves are logged (without the secret content) for review:

```json
{
  "level": "warn",
  "message": "save_learning rejected: potential secret detected",
  "project": "github.com/company/repo",
  "field": "answer",
  "pattern": "connection-string",
  "resolvedBy": "andres"
}
```

**What we do NOT do:**
- We do NOT store rejected content (the secret is never persisted)
- We do NOT attempt to auto-redact (the agent must do it — we can't guess what's a secret vs a legitimate value)
- We do NOT scan `question` or `tags` (these are typically short and unlikely to contain secrets; `location` is file paths only)

**Phase-specific behavior:**
| Phase | Secret Sanitization |
|---|---|
| **Phase 1** | Layers 1 + 2 + 3 (agent instruction + pattern detection + rejection) |
| **Phase 2** | Same as Phase 1 + Layer 4 (audit log) |
| **Phase 3** | Same as Phase 2 + Firestore audit trail (rejected saves stored in separate `audit_rejects` collection) |

### 6.6 Scalability & Performance

| Phase | Storage | Max Documents | Search Latency | Notes |
|---|---|---|---|---|
| **Phase 1** | SQLite FTS5 | ~500K | ~5ms | BM25, lexical only |
| **Phase 2** | SQLite + sqlite-vec | ~100K | ~1.7s | Brute-force cosine, no ANN |
| **Phase 3** | Firestore | ~1M | ~50ms | kNN with `findNearest()` |
| **Future** | Vertex AI Vector Search | 10M+ | ~50ms | Dedicated vector index |

- **Token Savings vs AGENTS.md:**
  - Reading a 500KB `AGENTS.md` consumes ~125,000 tokens per query.
  - Echo retrieves only the relevant snippet (~500 tokens).
  - **Savings:** ~99.6% reduction in context tokens per query.

## 7. Implementation Plan

### Phase 1: Local Lexical Search (Complete Service)

**Goal:** A fully working Echo service — complete MCP server, agent integration, persistence. The only limitation is lexical (BM25) search instead of semantic.

**What "complete" means:**
- MCP server running over stdio, compatible with Cursor, Claude Desktop, VS Code
- `save_learning` and `search_learning` tools fully functional
- Project detection, duplicate detection, `resolvedBy` tracking
- Data persistence in `~/.config/echo/echo.db`
- Agent can save a learning on Machine A and search for it on Machine A
- **The only limitation:** search is lexical (BM25), not semantic. "DB" does not find "database".

- [ ] Initialize Go module with `modelcontextprotocol/go-sdk`
- [ ] Define interfaces: `TextStore` (search, save), `Embedder` (Phase 1: noop)
- [ ] Implement `SQLiteFTS5Store` using `modernc.org/sqlite` (pure Go, no CGO)
  - FTS5 virtual table for BM25 ranking
  - Schema: `learnings` table + `learnings_fts` virtual table
- [ ] Implement `NoEmbedder` (noop — placeholder for Phase 2)
- [ ] Implement `InMemoryStore` (for unit tests)
- [ ] Implement `save_learning` and `search_learning` MCP tools
- [ ] Add project detection and normalization logic (`git remote get-url origin`)
- [ ] Add `resolvedBy` detection from IAM/git config
- [ ] Implement duplicate detection (lexical: exact question match)
- [ ] Data persistence in `~/.config/echo/echo.db`
- [ ] Configure MCP in Cursor and VS Code — verify end-to-end agent integration
- [ ] Test: save a learning, search for it, verify BM25 ranking
- [ ] Test: persistence across restarts

**Deliverable:** `echo serve` starts a fully functional MCP server. Agents can save and search learnings locally. The service is production-ready for single-machine lexical search.

### Phase 2: Local Semantic Search (Vectors via Embedding API)

**Goal:** Add semantic search via an external embedding API. The storage upgrades to sqlite-vec, and the `Embedder` interface connects to any embedding provider (Vertex AI, OpenAI, Cohere, etc.).

**Key design decision:** The `Embedder` interface is **provider-agnostic**. Phase 2 ships with a `VertexAIEmbedder` as the default implementation, but the architecture supports swapping providers without changing storage or MCP logic.

```go
type Embedder interface {
    // Embed returns a vector of the configured dimension (e.g., 768).
    // The provider (Vertex AI, OpenAI, Cohere) is an implementation detail.
    Embed(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
}
```

**Supported embedding providers (Phase 2):**

| Provider | Model | Dimensions | Auth |
|---|---|---|---|
| **Vertex AI** (default) | `text-embedding-004` | 768 | GCP ADC |
| **OpenAI** | `text-embedding-3-small` | 1536 | API key |
| **Cohere** | `embed-multilingual-v3` | 1024 | API key |

> The `Embedder` is configured at startup via flag or env var. The storage layer (`sqlite-vec`) stores whatever vector dimensions the provider returns.

- [ ] Switch SQLite driver to `mattn/go-sqlite3` (CGO required)
- [ ] Implement schema migration logic (Phase 1 → Phase 2):
  - Detect existing Phase 1 database on first `--embeddings` run
  - Create `vec_learnings` virtual table with configured dimensions
  - Backfill embeddings for existing learnings (call Embedder API for each)
  - Preserve FTS5 tables intact — they survive any vector schema change
- [ ] Implement provider-change re-migration (drop + recreate + re-embed)
- [ ] Add `sqlite-vec` extension (`asg017/sqlite-vec`)
  - Schema: `vec_learnings` virtual table with FLOAT[N] vector column (N = provider dimensions)
  - Preserve existing FTS5 tables for hybrid search
- [ ] Implement `SQLiteVecStore` adapter
  - Save: insert into both FTS5 and vec tables
  - Search: cosine similarity via `vec_distance_cosine()`, fallback to FTS5 if no embeddings
  - kNN pre-filter for duplicate detection (top 20, threshold 0.92)
- [ ] Implement `Embedder` interface with provider implementations:
  - `VertexAIEmbedder` (default — GCP `text-embedding-004`)
  - `OpenAIEmbedder` (optional — `text-embedding-3-small`)
  - `CohereEmbedder` (optional — `embed-multilingual-v3`)
- [ ] Add `--embeddings` flag and `--embedder` provider flag
- [ ] Configure embedding provider via env var or flag:
  ```bash
  # Default: Vertex AI
  echo serve --embeddings

  # OpenAI
  echo serve --embeddings --embedder openai

  # Cohere
  echo serve --embeddings --embedder cohere
  ```
- [ ] Test: save learning with embedding, semantic search with different wording, verify recall
- [ ] Test: duplicate detection (near-duplicate updates existing learning)
- [ ] Test: persistence across restarts
- [ ] Test: provider swap (same data, different embedder)

**Deliverable:** `echo serve --embeddings` starts MCP server with semantic search. Queries like "DB connection" find learnings about "database setup". The embedding provider is configurable and swappable.

### Phase 3: Cloud Mode (Firestore + Shared Memory)

**Goal:** Cross-machine sync with Firestore. Team shared memory.

- [ ] Implement `FirestoreStore` adapter
  - Collection: `learnings`
  - Vector index: N dims (matches configured Embedder), COSINE distance, combined with `project` filter
- [ ] Reuse `Embedder` interface from Phase 2 (same provider, same config)
- [ ] Add `--cloud` flag to enable Phase 3 mode
- [ ] Implement semantic duplicate detection with kNN pre-filter (Firestore `findNearest()`)
- [ ] Implement cross-machine sync test (Dev A saves → Dev B retrieves)
- [ ] Configure Firestore Vector Index in GCP
- [ ] IAM: `roles/datastore.user` for Firestore (embedding provider auth is separate)
- [ ] Test: end-to-end flow with GCP services
- [ ] Test: project scoping (no cross-project data leakage)

**Deliverable:** `echo serve --cloud` starts MCP server with shared team memory. Learnings sync across machines automatically.

### Phase 4: Admin CLI, Observability & Polish

**Goal:** Production-ready tooling: admin CLI for organization rules, observability, and documentation.

- [ ] Implement `echo admin` CLI commands (add, list, update, delete)
  - Writes directly to storage (SQLite/Firestore), NOT through MCP
  - IAM enforcement for organization-scoped learnings
- [ ] Implement `get_critical_policies` MCP tool (returns `always_inject: true` learnings)
- [ ] Structured logging with JSON output
- [ ] Prometheus metrics integration
- [ ] README and setup documentation

## 8. Usage Scenario

**Scenario:** Two developers setting up a new project locally.

1. **Machine 1 (Cursor):** Developer A asks *"How do I connect to the database locally?"* → Agent researches, finds the config → Agent calls `save_learning` → *"Learning saved for the team."*

2. **Machine 2 (Claude/VS Code):** Developer B asks the same question → Agent calls `search_learning` → Returns: *"This was already solved by Developer A. Here's the configuration: `DATABASE_URL=...`"* → Developer B gets the answer instantly instead of spending 10 minutes investigating.

3. **Knowledge Evolution:** Developer C encounters the same DB issue but with a different root cause (e.g., a new auth method). Agent calls `save_learning` → System detects near-duplicate (>0.92 similarity) → **Updates** the existing learning with the new auth method and recalculates the embedding. Future searches now find the updated solution.

4. **Company Policy Enforcement:** Developer D starts a new session → Agent calls `get_critical_policies` → Returns organization-scoped rules: *"Deployments to production are only allowed Tue-Thu, 10am-3pm UTC"* → Agent incorporates this into its planning before suggesting any deployment commands.
