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
# Phase 1: Local lexical search (zero config, zero dependencies)
echo-mcp serve

# Phase 2: Local semantic search + HTTP server (requires embedding provider credentials)
echo-mcp serve --mode embeddings --embedder vertex-ai

# Phase 3: Cloud shared memory (requires GCP credentials)
echo-mcp serve --mode cloud
```

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
| `ECHO_EMBEDDER` | `--embedder` | `vertex-ai` | Embedding provider: `vertex-ai`, `openai`, `cohere` |
| `ECHO_LOG_LEVEL` | `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `ECHO_DATA_DIR` | `--data-dir` | `~/.config/echo` | Data directory |
| `ECHO_HTTP_ADDR` | `--http-addr` | `:7438` | HTTP server address (empty to disable) |

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
    store/             - SQLite FTS5 storage implementation
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
| **Phase 3** | 🔲 Planned | Cloud shared memory (Firestore + kNN vector search) |
| **Phase 4** | 🔲 Planned | Admin CLI, observability, production polish |

## Development

```bash
# Run all tests
go test ./...

# Build
go build ./cmd/echo

# Run locally
go run ./cmd/echo serve
```

## License

Apache 2.0
