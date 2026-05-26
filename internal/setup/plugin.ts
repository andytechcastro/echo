/**
 * Echo — OpenCode plugin adapter
 *
 * Connects OpenCode's event system to the Echo HTTP server.
 * Echo runs both an MCP server (stdio) and an HTTP server for plugin communication.
 *
 * Flow:
 *   OpenCode events → this plugin → HTTP calls → echo HTTP server → SQLite
 *
 * Session resilience:
 *   Uses `ensureSession()` before any DB write. Sessions are created on-demand —
 *   even if the plugin was loaded after the session started. The session ID comes
 *   from OpenCode's hooks (input.sessionID) rather than relying on session.created.
 */

import type { Plugin } from "@opencode-ai/plugin"

// ─── Configuration ───────────────────────────────────────────────────────────

const ECHO_PORT = parseInt(process.env.ECHO_PORT ?? "7438")
const ECHO_URL = `http://127.0.0.1:${ECHO_PORT}`
const ECHO_BIN = process.env.ECHO_BIN ?? Bun.which("echo-mcp") ?? "/usr/bin/echo-mcp"

// Echo's own MCP tools — don't count these as "tool calls" for session stats.
const ECHO_TOOLS = new Set([
  "save_learning",
  "search_learning",
  "get_critical_policies",
])

// ─── Memory Instructions ─────────────────────────────────────────────────────

const MEMORY_INSTRUCTIONS = `## Echo — Shared Team Memory

You have access to the 'echo' MCP server for shared team knowledge.

### When to use:
- **save_learning**: When you resolve a config, bugfix, pattern, decision, or discover a gotcha.
- **search_learning**: Before starting work or when a developer asks about project setup.
- **get_critical_policies**: At session start to inject organization rules.

### RULE: PROPOSE BEFORE SAVING
When you resolve something worth saving, DO NOT call save_learning directly.
Instead, propose it to the user:

  "💡 I found something worth saving for the team: <brief description>. Save it?"

Only call save_learning AFTER the user approves.

### Rules:
- Always use English for question, answer, reasoning, notes, and tags.
- NEVER include actual secrets. Use placeholders: <DB_PASSWORD>, <API_KEY>.
- Project and identity are auto-detected from git.
- Only write to scope: project. Organization scope is admin-only.
`

// ─── HTTP Client ─────────────────────────────────────────────────────────────

async function echoFetch(
  path: string,
  opts: { method?: string; body?: any } = {}
): Promise<any> {
  try {
    const res = await fetch(`${ECHO_URL}${path}`, {
      method: opts.method ?? "GET",
      headers: opts.body ? { "Content-Type": "application/json" } : undefined,
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    })
    return await res.json()
  } catch {
    // Echo HTTP server not running — silently fail
    return null
  }
}

async function isEchoRunning(): Promise<boolean> {
  try {
    const res = await fetch(`${ECHO_URL}/health`, {
      signal: AbortSignal.timeout(500),
    })
    return res.ok
  } catch {
    return false
  }
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function extractProjectName(directory: string): string {
  // Try git remote origin URL
  try {
    const result = Bun.spawnSync(["git", "-C", directory, "remote", "get-url", "origin"])
    if (result.exitCode === 0) {
      const url = result.stdout?.toString().trim()
      if (url) {
        const name = url.replace(/\.git$/, "").split(/[/:]/).pop()
        if (name) return name
      }
    }
  } catch {}

  // Fallback: git root directory name (works in worktrees)
  try {
    const result = Bun.spawnSync(["git", "-C", directory, "rev-parse", "--show-toplevel"])
    if (result.exitCode === 0) {
      const root = result.stdout?.toString().trim()
      if (root) return root.split("/").pop() ?? "unknown"
    }
  } catch {}

  // Final fallback: cwd basename
  return directory.split("/").pop() ?? "unknown"
}

function truncate(str: string, max: number): string {
  if (!str) return ""
  return str.length > max ? str.slice(0, max) + "..." : str
}

/**
 * Strip <private>...</private> tags before sending to echo.
 * Double safety: the Go binary also strips, but we strip here too
 * so sensitive data never even hits the wire.
 */
function stripPrivateTags(str: string): string {
  if (!str) return ""
  return str.replace(/<private>[\s\S]*?<\/private>/gi, "[REDACTED]").trim()
}

// ─── Plugin Export ───────────────────────────────────────────────────────────

export const Echo: Plugin = async (ctx) => {
  const oldProject = ctx.directory.split("/").pop() ?? "unknown"
  const project = extractProjectName(ctx.directory)

  // Track tool counts per session (in-memory only, not critical)
  const toolCounts = new Map<string, number>()

  // Track which sessions we've already ensured exist in echo
  const knownSessions = new Set<string>()

  // Track sub-agent session IDs so we can suppress their registrations.
  // Sub-agents (Task() calls) have a parentID or a title ending in " subagent)".
  // We must not register them as top-level Echo sessions — they cause session
  // inflation (e.g. 170 sessions for 1 real conversation).
  const subAgentSessions = new Set<string>()

  /**
   * Ensure a session exists in echo. Idempotent — calls POST /sessions
   * which uses INSERT OR IGNORE. Safe to call multiple times.
   *
   * Silently skips sub-agent sessions (tracked in `subAgentSessions`).
   */
  async function ensureSession(sessionId: string): Promise<void> {
    if (!sessionId || knownSessions.has(sessionId)) return
    if (subAgentSessions.has(sessionId)) return
    knownSessions.add(sessionId)
    await echoFetch("/sessions", {
      method: "POST",
      body: {
        id: sessionId,
        project,
        directory: ctx.directory,
      },
    })
  }

  // Try to start echo HTTP server if not running.
  const running = await isEchoRunning()
  if (!running && ECHO_BIN) {
    try {
      Bun.spawn([ECHO_BIN, "serve", "--http-addr", `:${ECHO_PORT}`], {
        stdout: "ignore",
        stderr: "ignore",
        stdin: "ignore",
      })
      await new Promise((r) => setTimeout(r, 500))
    } catch {
      // Binary not found or can't start — plugin will silently no-op
    }
  }

  // Migrate project name if it changed (one-time, idempotent)
  if (oldProject !== project) {
    await echoFetch("/projects/migrate", {
      method: "POST",
      body: { old_project: oldProject, new_project: project },
    })
  }

  // Auto-import: if .echo/manifest.json exists in the project repo,
  // run `echo sync --import` to load any new chunks into the local DB.
  // This is how git-synced memories get loaded when cloning a repo or
  // pulling changes. Each chunk is imported only once (tracked by ID).
  try {
    const manifestFile = `${ctx.directory}/.echo/manifest.json`
    const file = Bun.file(manifestFile)
    if (await file.exists()) {
      Bun.spawn([ECHO_BIN, "sync", "--import"], {
        cwd: ctx.directory,
        stdout: "ignore",
        stderr: "ignore",
        stdin: "ignore",
      })
    }
  } catch {
    // Manifest doesn't exist or binary not found — silently skip
  }

  return {
    // ─── Event Listeners ───────────────────────────────────────────

    event: async ({ event }) => {
      // --- Session Created ---
      if (event.type === "session.created") {
        const info = (event.properties as any)?.info
        const sessionId = info?.id
        const parentID = info?.parentID
        const title: string = info?.title ?? ""

        // Sub-agent sessions (created via Task()) must NOT be registered as
        // top-level Echo sessions. They cause massive session inflation.
        const isSubAgent = !!parentID || title.endsWith(" subagent)")

        if (sessionId && !isSubAgent) {
          await ensureSession(sessionId)
        } else if (sessionId && isSubAgent) {
          subAgentSessions.add(sessionId)
        }
      }

      // --- Session Deleted ---
      if (event.type === "session.deleted") {
        const info = (event.properties as any)?.info
        const sessionId = info?.id
        if (sessionId) {
          toolCounts.delete(sessionId)
          knownSessions.delete(sessionId)
          subAgentSessions.delete(sessionId)
        }
      }
    },

    // ─── User Prompt Capture ──────────────────────────────────────
    // chat.message is called once per user message, before the LLM sees it.

    "chat.message": async (input, output) => {
      // Skip sub-agent sessions
      if (subAgentSessions.has(input.sessionID)) return

      const sessionId = input.sessionID

      // Extract text from parts (type:"text")
      const content = output.parts
        .filter((p) => p.type === "text")
        .map((p) => (p as any).text ?? "")
        .join("\n")
        .trim()

      // Also fallback to summary if parts yield nothing
      const fallback = !content && output.message.summary
        ? `${output.message.summary.title ?? ""}\n${output.message.summary.body ?? ""}`.trim()
        : ""

      const finalContent = content || fallback

      // Only capture non-trivial prompts (>10 chars)
      if (finalContent.length > 10) {
        await ensureSession(sessionId)
        await echoFetch("/prompts", {
          method: "POST",
          body: {
            session_id: sessionId,
            content: stripPrivateTags(truncate(finalContent, 2000)),
            project,
          },
        })
      }
    },

    // ─── Tool Execution Hook ─────────────────────────────────────
    // Count tool calls per session (for session end stats).
    // Also ensures the session exists — handles plugin reload / reconnect.
    // Passive capture: when a Task tool completes, POST its output to
    // the passive capture endpoint so the server extracts learnings.

    "tool.execute.after": async (input, output) => {
      if (ECHO_TOOLS.has(input.tool.toLowerCase())) return

      const sessionId = input.sessionID
      if (sessionId) {
        await ensureSession(sessionId)
        toolCounts.set(sessionId, (toolCounts.get(sessionId) ?? 0) + 1)
      }

      // Passive capture: extract learnings from Task tool output
      if (input.tool === "Task" && output && sessionId) {
        const text = typeof output === "string" ? output : JSON.stringify(output)
        if (text.length > 50) {
          await echoFetch("/observations/passive", {
            method: "POST",
            body: {
              session_id: sessionId,
              content: stripPrivateTags(text),
              project,
              source: "task-complete",
            },
          })
        }
      }
    },

    // ─── System Prompt: Always-on memory instructions ──────────

    "experimental.chat.system.transform": async (_input, output) => {
      if (output.system.length > 0) {
        output.system[output.system.length - 1] += "\n\n" + MEMORY_INSTRUCTIONS
      } else {
        output.system.push(MEMORY_INSTRUCTIONS)
      }
    },

    // ─── Compaction Hook: Persist memory + inject context ──────────

    "experimental.session.compacting": async (input, output) => {
      if (input.sessionID) {
        await ensureSession(input.sessionID)
      }

      // Inject context from previous sessions
      const data = await echoFetch(
        `/context?project=${encodeURIComponent(project)}`
      )
      if (data?.context) {
        output.context.push(data.context)
      }

      // Tell the compressor to instruct the new agent to persist the
      // compacted summary to Echo.
      output.context.push(
        `CRITICAL INSTRUCTION FOR COMPACTED SUMMARY:\n` +
        `The agent has access to Echo shared team memory via MCP tools.\n` +
        `You MUST include the following instruction at the TOP of the compacted summary:\n\n` +
        `"FIRST ACTION REQUIRED: Call get_critical_policies to inject organization rules. ` +
        `Use project: '${project}'. This preserves Echo context before any other work."\n\n` +
        `Also remember: ALWAYS propose before saving learnings. Never call save_learning without user approval.`
      )
    },
  }
}
