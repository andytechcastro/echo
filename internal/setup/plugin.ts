/**
 * Echo — OpenCode plugin adapter
 *
 * Injects Echo instructions into the system prompt so the agent
 * always knows about the shared team memory tools.
 *
 * Unlike Engram, Echo runs as an MCP server (not HTTP), so this plugin
 * focuses on system prompt injection and compaction context.
 */

import type { Plugin } from "@opencode-ai/plugin"

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

// ─── Helpers ─────────────────────────────────────────────────────────────────

function extractProjectName(directory: string): string {
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

  try {
    const result = Bun.spawnSync(["git", "-C", directory, "rev-parse", "--show-toplevel"])
    if (result.exitCode === 0) {
      const root = result.stdout?.toString().trim()
      if (root) return root.split("/").pop() ?? "unknown"
    }
  } catch {}

  return directory.split("/").pop() ?? "unknown"
}

// ─── Plugin Export ───────────────────────────────────────────────────────────

export const Echo: Plugin = async (ctx) => {
  const project = extractProjectName(ctx.directory)

  return {
    // ─── System Prompt: Always-on Echo instructions ──────────

    "experimental.chat.system.transform": async (_input, output) => {
      if (output.system.length > 0) {
        output.system[output.system.length - 1] += "\n\n" + MEMORY_INSTRUCTIONS
      } else {
        output.system.push(MEMORY_INSTRUCTIONS)
      }
    },

    // ─── Compaction Hook: Inject context ──────────

    "experimental.session.compacting": async (_input, output) => {
      output.context.push(
        `CRITICAL INSTRUCTION FOR COMPACTED SUMMARY:\n` +
        `The agent has access to Echo shared team memory via MCP tools (save_learning, search_learning, get_critical_policies).\n` +
        `You MUST include the following instruction at the TOP of the compacted summary:\n\n` +
        `"FIRST ACTION REQUIRED: Call get_critical_policies to inject organization rules. ` +
        `Use project: '${project}'. This preserves Echo context before any other work."\n\n` +
        `Also remember: ALWAYS propose before saving learnings. Never call save_learning without user approval.`
      )
    },
  }
}
