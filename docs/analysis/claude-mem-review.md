# Claude-Mem — Codebase Review & Analysis

**Reviewed:** 2026-03-22
**Version:** 10.6.2
**Repository:** `~/workspaces/claude-mem`
**Author:** Alex Newman (@thedotmack, ~1,294 commits, primary contributor)
**License:** AGPL-3.0

---

## 1. Project Overview

Claude-Mem is a Claude Code plugin that provides **persistent memory across sessions**. It observes tool usage during a session, sends those observations to a background LLM agent that extracts structured insights (observations and summaries), stores them in SQLite, and injects relevant context back into future sessions. It also supports semantic search via Chroma vector DB.

### Core Value Proposition

- Automatic observation capture — hooks into PostToolUse, SessionStart, SessionEnd lifecycle events
- LLM-powered compression — a separate Claude/Gemini/OpenRouter agent processes raw tool output into structured observations
- Context injection — on SessionStart, recent observations are assembled into a timeline and injected into Claude's context
- Semantic search — Chroma vector DB enables natural-language queries across session history
- Multi-IDE support — Claude Code (native), Cursor (hooks installer), OpenClaw (gateway plugin)
- Web viewer UI — real-time SSE-powered React dashboard at localhost:37777
- Mode system — domain-specific observation schemas (code, law-study, email-investigation)

### By the Numbers

| Metric | Value |
|--------|-------|
| Total TypeScript/TSX | ~35,700 lines across 185 files |
| Source directories | 15+ functional domains in src/ |
| Worker service (main orchestrator) | 1,272 lines |
| SessionStore (database layer) | 2,459 lines |
| SearchManager | 1,884 lines |
| Test files | 70 (Bun test runner) |
| Skills | 5 (/do, /make-plan, /mem-search, /smart-explore, /timeline-report) |
| npm runtime deps | 10 (Claude Agent SDK, MCP SDK, Express, React, etc.) |
| npm devDeps | 14 (tree-sitter parsers, esbuild, TypeScript, etc.) |
| i18n translations | 28 languages |
| Total commits | 1,493 (Sept 2025 – Mar 2026) |
| Contributors | ~25 (Alex Newman: 1,294; copilot-swe-agent[bot]: 39; others: <15 each) |
| Release velocity | v10.6.2 current; active patch/minor releases every 1–3 days |

---

## 2. Architecture

### 2.1 High-Level Structure

```
claude-mem/
├── src/
│   ├── cli/                     # Hook framework & event handling
│   │   ├── adapters/            # Platform adapters (Claude Code, Cursor, raw)
│   │   └── handlers/            # Event handlers (context, observation, summarize, etc.)
│   ├── sdk/                     # Claude Agent SDK integration (parser + prompt generation)
│   ├── servers/mcp-server.ts    # MCP stdio server (thin adapter → Worker HTTP API)
│   ├── services/
│   │   ├── worker-service.ts    # Main orchestrator & HTTP server startup
│   │   ├── context/             # Context building pipeline (ContextBuilder, renderers)
│   │   ├── sqlite/              # Database layer (SessionStore, migrations, search)
│   │   ├── sync/                # Chroma vector DB sync
│   │   ├── infrastructure/      # Process management, health monitoring, shutdown
│   │   ├── smart-file-read/     # AST-based code search (tree-sitter)
│   │   ├── worker/              # Business logic (agents, search, HTTP routes, formatting)
│   │   │   ├── agents/          # Response processing pipeline
│   │   │   ├── search/          # Strategy pattern (Chroma, SQLite, Hybrid)
│   │   │   └── http/routes/     # REST API route handlers
│   │   ├── domain/              # Mode management (observation type schemas)
│   │   └── integrations/        # Cursor IDE hooks installer
│   ├── shared/                  # Cross-module utilities (paths, env, settings)
│   ├── supervisor/              # Process supervision & health
│   ├── ui/viewer/               # React web viewer (SSE, observation/summary cards)
│   └── utils/                   # Logger, CLAUDE.md utils, tag stripping
├── plugin/                      # Built plugin distribution
│   ├── .claude-plugin/          # Plugin registration (plugin.json)
│   ├── hooks/hooks.json         # Hook lifecycle configuration
│   ├── scripts/                 # Compiled executables (worker-service.cjs, context-generator.cjs)
│   ├── skills/                  # 5 skill definitions (SKILL.md files)
│   ├── modes/                   # Observation type schemas (code.json, law-study.json)
│   └── ui/                      # Built React viewer (viewer.html + bundle)
├── openclaw/                    # OpenClaw gateway integration (installer, tests)
├── cursor-hooks/                # Cursor IDE hook scripts
├── ragtime/                     # Batch email corpus processor
├── tests/                       # 70 Bun test files
└── scripts/                     # 35+ build, migration, debug, i18n scripts
```

### 2.2 Data Flow

```
Claude Session (user working)
  │
  ├─[SessionStart]──→ contextHandler ──→ Worker GET /api/context/inject
  │                                        └─→ ContextBuilder queries SQLite
  │                                        └─→ Renders timeline markdown
  │                                        └─→ Injected into Claude's context window
  │
  ├─[PostToolUse]──→ observationHandler ──→ Worker POST /api/sessions/observations
  │                                          └─→ Queued for SDK agent processing
  │                                          └─→ SDKAgent spawns Claude subprocess
  │                                          └─→ Agent extracts <observation> XML
  │                                          └─→ ResponseProcessor stores to SQLite
  │                                          └─→ Fire-and-forget Chroma sync
  │                                          └─→ SSE broadcast to web viewer
  │
  ├─[Stop]──→ summarizeHandler ──→ Worker triggers session summary
  │                                 └─→ Agent produces <summary> XML
  │                                 └─→ Stored alongside observations
  │
  └─[SessionEnd]──→ sessionCompleteHandler ──→ Marks session complete, cleanup
```

### 2.3 Observer Agent Architecture

The core innovation is a **separate LLM agent** that acts as an observer. For each tool execution in the user's session, raw tool input/output is sent to this observer agent, which extracts structured observations in XML format:

```xml
<observation>
  <type>bugfix</type>
  <title>Fixed null pointer in auth middleware</title>
  <subtitle>Missing guard on token refresh path</subtitle>
  <facts>
    <fact>Token refresh endpoint lacked null check on session.user</fact>
    <fact>Added guard clause at auth/middleware.ts:42</fact>
  </facts>
  <narrative>The auth middleware crashed on token refresh when...</narrative>
  <concepts><concept>problem-solution</concept><concept>gotcha</concept></concepts>
  <files_read><file>src/auth/middleware.ts</file></files_read>
  <files_modified><file>src/auth/middleware.ts</file></files_modified>
</observation>
```

Three agent implementations exist with fallback chain:
1. **SDKAgent** (default) — Claude subprocess via `@anthropic-ai/claude-agent-sdk`, all tools disallowed (observer-only)
2. **GeminiAgent** — Direct HTTP to Gemini API
3. **OpenRouterAgent** — Proxy to OpenRouter API

The observer agent is **tool-less** by design — it cannot Read, Write, Bash, etc. It only receives tool execution data and produces structured XML.

### 2.4 Context Injection Pipeline

On SessionStart, `ContextBuilder` assembles a timeline for Claude's context window:

1. Load config (observation count, session count, token display preferences)
2. Query SQLite for recent observations + summaries (supports worktree multi-project)
3. Calculate token economics (discovery_tokens spent to produce observations)
4. Render timeline: header → chronological observations → most recent summary → footer
5. Return as markdown (for Claude) and optionally colored (for terminal display)

### 2.5 Search Architecture (Strategy Pattern)

Three search strategies with automatic fallback:

| Strategy | When Used | Mechanism |
|----------|-----------|-----------|
| **ChromaSearch** | Query text + Chroma available | Vector semantic similarity |
| **HybridSearch** | Metadata filters + query text | SQL filter → then Chroma semantic |
| **SQLiteSearch** | No query text, or Chroma unavailable | Structured SQL filtering only |

Fallback: Chroma failure → SQLite. Always degrades gracefully.

### 2.6 MCP Integration

The MCP server (`mcp-server.cjs`) is a **thin stdio adapter** that delegates to the Worker HTTP API. Provides 3 tools:
- `search` — query index (~50–100 tokens/result)
- `timeline` — chronological context around an anchor
- `get_observations` — batch fetch full details by ID

Follows a 3-layer progressive disclosure pattern for token efficiency.

### 2.7 Mode System

Modes define domain-specific observation schemas as JSON files:

```json
{
  "observation_types": [
    { "id": "bugfix", "emoji": "🔴", "description": "Something broken, now fixed" },
    { "id": "feature", "emoji": "🟣", "description": "New capability added" },
    ...
  ],
  "observation_concepts": [
    { "id": "problem-solution", "description": "Issues and their fixes" },
    ...
  ],
  "prompts": {
    "system_identity": "Record what was LEARNED/BUILT/FIXED...",
    "recording_focus": "Deliverables & capabilities...",
    ...
  }
}
```

Modes: `code` (default, 28 i18n variants), `law-study`, `email-investigation`.

---

## 3. Strengths

### 3.1 Graceful Degradation Everywhere

Every hook handler follows the same pattern: if the worker is unavailable, return success with empty data. Observation storage failure does not block tool use. Context fetch failure returns empty context. Chroma sync is fire-and-forget. This means claude-mem **never breaks the user's workflow** even when its own components fail.

```typescript
// observation.ts — representative pattern
const workerReady = await ensureWorkerRunning();
if (!workerReady) {
  return { continue: true, suppressOutput: true, exitCode: HOOK_EXIT_CODES.SUCCESS };
}
```

This is the right tradeoff for a plugin that runs in someone else's workflow.

### 3.2 Observer-Only Agent Design

The SDK agent explicitly disallows all tools (Bash, Read, Write, Edit, Grep, Glob, WebFetch, WebSearch, Task). This is a critical security decision — the memory agent cannot modify the user's codebase, cannot leak information to external services, and cannot create infinite recursive tool loops. It's a pure consumer of tool execution data.

### 3.3 Token-Efficient Search Design

The 3-layer progressive disclosure pattern (search → timeline → get_observations) is well-designed:
- Layer 1: ~50–100 tokens/result (index only)
- Layer 2: Chronological context around interesting results
- Layer 3: ~500–1,000 tokens/result (full details, only for filtered IDs)

Claimed 10x token savings vs. fetching everything upfront. The `smart-explore` skill takes this further with AST-based code navigation (11–18x claimed savings over full file reads).

### 3.4 Well-Decomposed Service Architecture

Clear separation of concerns across ~15 functional domains. The `ResponseProcessor` extracts shared logic from three agent implementations. The search strategy pattern allows clean addition of new backends. The context builder uses a renderer pipeline (header → timeline → summary → footer). Database operations are organized into focused query modules (observations, sessions, summaries, prompts, timeline).

### 3.5 Robust Process Lifecycle

The infrastructure layer handles real production concerns:
- **ProcessManager** (802 LOC) — PID files, signal handling, child process management
- **HealthMonitor** — Port checks, readiness probes, version validation
- **GracefulShutdown** — Ordered teardown (PID → server → sessions → MCP → Chroma → DB)
- **ProcessRegistry** — Orphan reaper, stale session cleanup
- Bun runner with multi-path detection for fresh installs

### 3.6 Cross-Platform & Multi-IDE Support

- **Bun runner** handles fresh-install scenarios where Bun isn't in PATH
- **Smart installer** auto-provisions Bun and uv on first use
- **Windows compatibility** — PowerShell spawn, taskkill, extended timeouts, stdin buffering workaround for Linux Bun/libuv
- **Cursor IDE** — full hooks installer with context injection parity
- **OpenClaw** — gateway integration with observation feed to Telegram/Discord/Slack

### 3.7 Privacy Controls

`<private>content</private>` tags strip content at the hook layer (edge processing) before data reaches the worker or database. Project exclusion lists prevent tracking specific directories. Exit code 0 philosophy prevents Windows Terminal tab accumulation from hook errors.

---

## 4. Concerns & Risks

### 4.1 CRITICAL: Observer Agent Cost Is Unbounded

Every PostToolUse event spawns or messages the SDK agent, which calls Claude (or Gemini/OpenRouter) to extract observations. In a session with hundreds of tool calls, this means **hundreds of LLM API calls running in the background**. The `discovery_tokens` tracking shows awareness of this cost, but there's no circuit breaker, rate limiter, or budget cap visible in the hook/agent flow.

A user running `grep` across a large codebase (producing dozens of tool calls in seconds) would generate a burst of API calls. Each observation prompt includes tool input + output, which for file reads could be substantial.

**Impact:** Silent cost accumulation. Users may not realize claude-mem is making API calls per tool use.

**Recommendation:** Add configurable per-session or per-hour token/cost budgets. Implement tool-level filtering (e.g., skip Glob results, batch rapid-fire tool calls). Surface cumulative session cost in the web viewer.

### 4.2 CRITICAL: XML Parsing via Regex Is Fragile

The observation parser (`sdk/parser.ts`) uses regex to extract XML fields:

```typescript
const observationRegex = /<observation>([\s\S]*?)<\/observation>/g;
const regex = new RegExp(`<${fieldName}>([\\s\\S]*?)</${fieldName}>`);
```

Non-greedy `[\s\S]*?` will match the **first** closing tag it encounters. If a `<narrative>` field contains XML-like content (e.g., code samples with angle brackets, HTML snippets, or LLM responses about XML), the parser will truncate at the wrong boundary.

The code comments reference "Issue #798" for this exact class of bug and the non-greedy match was the fix — but it's still fundamentally regex-based XML parsing.

**Impact:** Observation data loss or corruption when tool output contains XML-like content.

**Recommendation:** Either enforce CDATA wrapping in the prompt format, implement a simple stack-based XML parser that handles nesting, or switch to a structured output format (JSON) for agent responses.

### 4.3 HIGH: Single-Writer SQLite Under Concurrent Load

The worker service handles multiple concurrent sessions, each producing observations that write to the same SQLite database. SQLite in WAL mode handles concurrent reads well, but writes are still serialized. The `transactions.ts` file (254 LOC) handles atomic multi-record operations, but there's no connection pooling or write queue visible.

The `SessionStore` at 2,459 lines is the largest single file and handles all database operations. Under heavy load (multiple sessions, rapid tool calls), write contention could cause `SQLITE_BUSY` errors.

**Recommendation:** Implement a write queue with batching, or at minimum, add retry logic with exponential backoff for SQLITE_BUSY. Consider whether the `pending_messages` table already serves as a write buffer.

### 4.4 HIGH: 60MB Compiled Binary + Heavy Runtime Dependencies

The plugin distributes a 60MB compiled binary (`plugin/scripts/claude-mem`), plus a 1.7MB minified worker service, and requires auto-installation of both Bun and uv (Python package manager) for Chroma. The tree-sitter dependency suite (10 language parsers) adds significant weight.

For a plugin that runs alongside Claude Code, this is a heavy footprint:
- ~60MB binary
- Bun runtime (~80MB if fresh-installed)
- uv + Python (for Chroma)
- Tree-sitter native modules (10 parsers)
- Express HTTP server on port 37777

**Impact:** Slow first-start experience, large disk footprint, potential conflicts with user's existing Bun/Python/Node installations.

### 4.5 HIGH: Background LLM Sessions Are Hard to Debug

The observer agent runs as a subprocess via `@anthropic-ai/claude-agent-sdk`. When it produces malformed XML, fails to respond, or generates off-topic observations, debugging requires:
1. Checking worker logs (`~/.claude-mem/logs/`)
2. Understanding the prompt chain (init → observation → summary)
3. Correlating contentSessionId → memorySessionId → agent subprocess

The code has extensive logging (`logger.ts` at 409 LOC), but the indirection between hook → HTTP API → agent subprocess → XML parsing → database makes it hard to trace a single observation from tool use to storage.

### 4.6 MEDIUM: Mode System Prompt Injection Surface

The mode JSON files embed full LLM prompts (`system_identity`, `recording_focus`, `type_guidance`, etc.) that are interpolated directly into the SDK agent's system prompt:

```typescript
return `${mode.prompts.system_identity}
  ...
  ${mode.prompts.observer_role}
  ${mode.prompts.spatial_awareness}
  ${mode.prompts.recording_focus}`;
```

Custom modes (or modified mode files) could inject arbitrary instructions into the observer agent. Since the observer is tool-less this limits blast radius, but a modified mode could cause the observer to produce misleading observations that pollute future context injection.

### 4.7 MEDIUM: Schema Migration Runner Is Complex

The migration system spans `migrations.ts` (522 LOC) + `migrations/runner.ts` (866 LOC) = **1,388 lines** for schema management. The runner includes:
- Automatic schema repair for malformed databases
- Python `sqlite3` fallback for corruption recovery
- Column existence checks and safe ALTER TABLE
- Migration version tracking

This complexity suggests a history of schema issues in the field. The repair logic is defensive but makes the migration path hard to reason about.

### 4.8 MEDIUM: Commented-Out Validation Is Concerning

In `parser.ts`, the summary validation code is explicitly commented out with an author note:

```typescript
// NOTE FROM THEDOTMACK: 100% of the time we must SAVE the summary, even if fields are missing. 10/24/2025
// NEVER DO THIS NONSENSE AGAIN.
```

The "never skip" philosophy (also applied to observations) means malformed or partial data is always stored. While this maximizes data capture, it means downstream consumers (context injection, search) must handle null/missing fields throughout. The author's frustration suggests this was a painful lesson, but the blanket "never validate" approach trades data quality for data completeness.

### 4.9 LOW: Version Drift Between package.json and plugin.json

Root `package.json` shows version `10.6.2`, but `plugin/.claude-plugin/plugin.json` shows `10.4.1`. This means the plugin registry metadata is stale relative to the actual codebase. Users checking the plugin metadata see an older version than what's running.

---

## 5. Code Quality Observations

### 5.1 TypeScript Style

- Clean module boundaries with explicit imports
- Well-documented with JSDoc headers on all modules
- Consistent error handling pattern (log + graceful fallback)
- Interface-first design (EventHandler, SearchStrategy, ContextConfig)
- `bun:sqlite` used natively (no ORM overhead)

### 5.2 Testing

- 70 test files using Bun's built-in test runner
- Good coverage of core paths: SQLite CRUD, search strategies, context compilation, infrastructure lifecycle
- Mocking via `mock.module()` and `spyOn()` — consistent patterns
- Gap: limited end-to-end tests with real LLM integration (agent tests are mocked)
- Gap: no stress/load tests for concurrent session handling

### 5.3 Documentation

- CLAUDE.md provides clear architecture overview and build commands
- README.md is polished with i18n (28 languages)
- Skills have detailed SKILL.md files with workflow descriptions
- Extensive docs/ directory with architecture evolution, hooks reference, etc.
- External docs site at docs.claude-mem.ai (Mintlify)

### 5.4 Build System

- esbuild for bundling (fast)
- Single `npm run build-and-sync` workflow for development
- Automatic changelog generation
- `np` for npm release automation
- Marketplace sync script for Claude Code plugin distribution

---

## 6. Comparison to attest's Learning System

Both systems capture knowledge from AI agent sessions, but they serve fundamentally different purposes and operate at different layers:

| Aspect | claude-mem | attest (internal/learning/) |
|--------|-----------|----------------------------|
| **What it captures** | Raw tool usage → LLM-compressed observations | Explicit learnings from council/verifier findings |
| **When it captures** | Every tool call (automatic, continuous) | Post-verification (deliberate, gated) |
| **How it captures** | Background LLM agent extracts structured XML | Deterministic extraction from findings |
| **Storage** | SQLite + Chroma vector DB | File-based (JSON/YAML) |
| **Retrieval** | Context injection on SessionStart + MCP search | Manual query or plan-time lookup |
| **Cost model** | Per-observation LLM call (continuous cost) | Zero incremental cost (rule-based) |
| **Observation types** | 6 types (bugfix, feature, refactor, change, discovery, decision) | Learnings tied to requirement IDs |
| **Search** | Semantic (Chroma) + structured (SQLite) | TBD |
| **Cross-session** | Yes (primary purpose) | Yes (file-based persistence) |
| **Scope** | All tool usage in any project | Attest run artifacts only |
| **Dependencies** | Claude Agent SDK, Express, SQLite, Chroma, Bun, uv | None (pure Go) |

### Key Philosophical Differences

1. **Automatic vs. deliberate.** Claude-mem captures everything automatically (every PostToolUse). Attest's learning system captures only findings that survive verification. Claude-mem optimizes for recall completeness; attest optimizes for signal quality.

2. **LLM-in-the-loop vs. deterministic.** Claude-mem uses an LLM to compress tool output into observations. Attest uses rule-based extraction from structured findings. Claude-mem's approach is richer but expensive and non-deterministic; attest's is free and reproducible.

3. **Plugin vs. integrated.** Claude-mem is a standalone plugin that works with any Claude Code session. Attest's learning system is tightly integrated with its spec-verification pipeline. Claude-mem has broader reach; attest has deeper integration with its quality gates.

4. **Timeline vs. requirement-linked.** Claude-mem organizes observations chronologically with concepts. Attest links learnings to specific requirement IDs (AT-FR-***, AT-TS-***). Claude-mem answers "what happened when"; attest answers "what did we learn about requirement X."

### What attest Could Learn From claude-mem

- **Automatic observation capture**: Currently attest's learning extraction is manual/triggered. Auto-capturing observations from tool use during runs could surface patterns that deliberate extraction misses.
- **Semantic search**: Chroma integration for natural-language queries over learnings. Currently attest has no search capability.
- **Context injection**: Automatically surfacing relevant past learnings at run start time (analogous to SessionStart context injection).
- **Progressive disclosure**: The 3-layer search pattern (index → timeline → details) is a good token-efficiency model.
- **Token economics tracking**: Knowing the cost of producing vs. retrieving knowledge is valuable for optimization.

### What claude-mem Could Learn From attest

- **Quality gates on observations**: Not all tool use produces useful observations. Filtering or scoring before storage could improve signal-to-noise.
- **Requirement linkage**: Tying observations to specific spec requirements adds traceability.
- **Zero-cost extraction**: Rule-based extraction from structured data avoids per-observation LLM costs.
- **Deterministic reproduction**: Same inputs → same learnings, important for verification.

---

## 7. Summary of Recommendations

### Must-Fix (Reliability)

1. **Add observer agent cost controls** — per-session token budget, tool-type filtering, rapid-fire batching
2. **Harden XML parsing** — CDATA wrapping or stack-based parser for nested XML in observations

### Should-Fix (Maintainability)

3. **Add SQLite write queue** — batch writes and retry on SQLITE_BUSY under concurrent load
4. **Fix version drift** — sync plugin.json version with package.json on build
5. **Add agent tracing** — correlate tool use → observation → database storage with a single trace ID

### Nice-to-Have (Quality)

6. **Reduce binary size** — tree-sitter parsers are only needed for smart-explore, consider lazy loading
7. **Add observation quality scoring** — not all tool calls produce useful observations; skip or batch low-signal events
8. **Stress test concurrent sessions** — validate SQLite behavior under realistic multi-session load
