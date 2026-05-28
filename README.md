# Echo

**Shared team memory for AI agents.**

Echo is a shared team memory layer that sits between developers and their AI agents. When one developer resolves something (a config issue, a bug, a pattern, an architectural decision), Echo captures it. When another developer on the same project encounters the same situation, their agent retrieves the existing solution instantly.

## Quick Start

### Install

```bash
go install github.com/company/echo/cmd/echo@latest
```

### Configure your AI agent

**OpenCode (recommended):**
```bash
echo setup opencode
```

This creates a global plugin that injects Echo rules into every session and configures the MCP server automatically.

**Manual MCP configuration:**

**Cursor / VS Code (`.cursor/mcp.json` or `.vscode/mcp.json`):**
```json
{
  "mcpServers": {
    "echo": {
      "command": "echo-mcp",
      "args": ["serve"]
    }
  }
}
```

**Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):**
```json
{
  "mcpServers": {
    "echo": {
      "command": "echo-mcp",
      "args": ["serve"]
    }
  }
}
```

### Start the server

```bash
# Mode 1: Local lexical search (zero config, zero dependencies)
echo-mcp serve --mode local

# Mode 2: Local semantic search (requires ONNX Runtime, auto-downloads 90MB model)
echo-mcp serve --mode embeddings

# Mode 3: Cloud shared memory (requires GCP credentials, Phase 3b)
echo-mcp serve --mode cloud
```

**Using semantic search (`--mode embeddings`):**

First, install ONNX Runtime:

```bash
ORT_VERSION=1.23.0
wget https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz
tar -xzf onnxruntime-linux-x64-${ORT_VERSION}.tgz
sudo cp -r onnxruntime-linux-x64-${ORT_VERSION}/lib/* /usr/local/lib/
```

Then start Echo:

```bash
echo-mcp serve --mode embeddings
```

The model (all-MiniLM-L6-v2, 90MB) and tokenizer vocab are downloaded automatically on first run to `~/.config/echo/models/`.

### Sync learnings from git

```bash
# Import chunks from .echo/chunks directory
echo-mcp sync --import

# Specify a different project directory
echo-mcp sync --import /path/to/project
```

### Admin CLI

```bash
# Add a global rule (Phase 4)
echo-mcp admin add --scope organization --type process --question "Deployment policy" --answer "Deployments only Tue-Thu"

# List all global rules
echo-mcp admin list --scope organization
```

## Configuration

| Env Var | Flag | Default | Description |
|---------|------|---------|-------------|
| `ECHO_MODE` | `--mode` | `local` | Operating mode: `local`, `embeddings`, `cloud` |
| `ECHO_EMBEDDER` | `--embedder` | `local` | Embedding provider: `local`, `vertex-ai`, `openai`, `cohere` |
| `ECHO_LOG_LEVEL` | `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `ECHO_DATA_DIR` | `--data-dir` | `~/.config/echo` | Data directory |
| `ECHO_HTTP_ADDR` | `--http-addr` | `:7438` | HTTP server address (empty to disable) |
| `ECHO_MODEL_PATH` | `--model-path` | `~/.config/echo/models/all-MiniLM-L6-v2.onnx` | ONNX model path |
| `ECHO_VOCAB_PATH` | `--vocab-path` | `~/.config/echo/models/vocab.txt` | WordPiece vocab path |

## MCP Tools

### `save_learning`

Save a resolved issue, config, pattern, or decision to the team knowledge base.

**Input:**
| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `config`, `pattern`, `bugfix`, `decision`, `process`, `domain`, `gotcha` |
| `question` | string | The problem that was solved |
| `answer` | string | The solution |
| `reasoning` | string | Why this solution was chosen |
| `location` | string | Affected files/modules |
| `notes` | string | Gotchas, edge cases, warnings |
| `tags` | string | Searchable tags, comma-separated or JSON array (English) |

### `search_learning`

Search the team knowledge base for existing solutions.

**In `embeddings` mode:** Uses semantic vector search (cosine similarity) for better results. Falls back to BM25 lexical search if vector search fails.

**In `local` mode:** Uses BM25 lexical search (FTS5).

**Input:**
| Field | Type | Description |
|-------|------|-------------|
| `query` | string | The problem or question to search for |
| `tags` | string | Optional tag filters, comma-separated or JSON array |

### `get_critical_policies`

Return organization-scoped policies that should be injected at session start.

## HTTP Server (Phase 2)

Echo runs an HTTP server alongside the MCP server for plugin communication. The OpenCode plugin uses HTTP endpoints for:

- **Session lifecycle** (`POST /sessions`, `DELETE /sessions/:id`)
- **Prompt capture** (`POST /prompts`)
- **Passive observation extraction** (`POST /observations/passive`)
- **Project migration** (`POST /projects/migrate`)
- **Context injection** (`GET /context?project=X`)
- **Health check** (`GET /health`)

The HTTP server is enabled by default on port `7438`. Disable it with `--http-addr ""`.

## Git Sync (Phase 2)

Echo supports distributed team memory via git. Learnings can be exported as JSON chunks and shared via git commits:

```
.echo/
  manifest.json    # Tracks imported chunks (auto-generated)
  chunks/          # JSON files, one per learning
    learn_xxx.json
    learn_yyy.json
```

The plugin auto-imports chunks on load. Manual import: `echo-mcp sync --import`.

## Project Structure

```
cmd/echo/              - CLI entry point (Cobra)
internal/
  domain/              - Core entities and interfaces (zero dependencies)
  infrastructure/
    store/             - SQLite FTS5 + sqlite-vec storage implementation
    embedder/          - ONNX-based local embedding (all-MiniLM-L6-v2)
    detector/          - Git project and identity detection
    mcp/               - MCP server and tool handlers
    httpserver/        - HTTP server for plugin communication
  usecase/             - Business logic orchestration
  sync/                - Git sync (manifest + chunk import/export)
  pkg/secret/          - Secret detection patterns
  config/              - Configuration management
  setup/               - OpenCode plugin generation
  e2e/                 - End-to-end integration tests
```

## Roadmap

| Phase | Status | Description |
|-------|--------|-------------|
| **Phase 1** | ✅ Done | Local lexical search (SQLite FTS5, zero deps, complete service) |
| **Phase 2** | ✅ Done | HTTP server + plugin hooks + git sync + passive extraction |
| **Phase 3a** | ✅ Done | Local semantic search (ONNX 90MB + sqlite-vec, 384 dims, offline) |
| **Phase 3b** | 🔲 Planned | Cloud shared memory (Firestore kNN + external APIs) |
| **Phase 4** | 🔲 Planned | Admin CLI, observability, production polish |

## Development

```bash
# Run all tests (requires CGO for sqlite-vec)
CGO_ENABLED=1 go test -tags fts5 ./...

# Build (requires CGO)
CGO_ENABLED=1 go build -tags fts5 ./cmd/echo

# Run locally
CGO_ENABLED=1 go run -tags fts5 ./cmd/echo serve --mode local
```

**Requirements:**
- Go 1.26+
- CGO enabled (gcc/clang required)
- ONNX Runtime shared library (for `--mode embeddings`)

## License

Apache 2.0
