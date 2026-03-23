# Claude Octopus — Codebase Review & Analysis

**Reviewed:** 2026-03-22
**Version:** 9.9.2
**Repository:** `~/workspaces/claude-octopus`
**Author:** Chris S (~1800 commits, sole contributor)
**License:** MIT

---

## 1. Project Overview

Claude Octopus is a Claude Code plugin that orchestrates **7 AI providers** through structured Double Diamond workflows. Rather than running providers in parallel and handing users separate answers, it assigns each provider a specific role, orchestrates execution, and enforces a 75% consensus quality gate before work advances.

### Core Value Proposition

- Coordinates Claude, Codex (OpenAI), Gemini (Google), Perplexity, OpenRouter, GitHub Copilot CLI, and Ollama
- Implements Double Diamond methodology: Discover → Define → Develop → Deliver
- 32 specialized personas auto-activate based on request context
- Works with just Claude alone (zero external providers required), scales up as providers are added
- Exposes functionality via Claude Code plugin, MCP server, and OpenClaw extension

### By the Numbers

| Metric | Value |
|--------|-------|
| Total Bash | ~29,000 lines across 52+ lib modules + orchestrate.sh |
| orchestrate.sh (main dispatcher) | 4,904 lines |
| Skills / Commands / Personas | 116 markdown definitions |
| Test files | 138 shell test scripts |
| Changelog | 1,078 lines |
| TypeScript (MCP server + OpenClaw) | ~500 lines |
| npm runtime deps | 2 (@modelcontextprotocol/sdk, zod) |
| Claude Code feature flags | ~80 SUPPORTS_* declarations |
| Version velocity | 9 releases on 2026-03-22 alone (v9.7.8 → v9.9.2) |

---

## 2. Architecture

### 2.1 High-Level Structure

```
claude-octopus/
├── scripts/orchestrate.sh       # Main coordinator (4,904 lines)
├── scripts/lib/                 # 52 modular Bash libraries (~22K lines)
├── scripts/*.sh                 # Supporting scripts (state, metrics, routing, teams)
├── agents/personas/             # 32 persona definitions (markdown + YAML frontmatter)
├── skills/                      # 45+ skill directories with execution contracts
├── commands/                    # 39 slash command definitions
├── mcp-server/src/index.ts      # MCP server adapter (~440 lines TypeScript)
├── openclaw/src/                # OpenClaw extension adapter
├── config/providers/            # Per-provider CLAUDE.md configs (codex, gemini, claude, ollama, copilot)
├── config/workflows/            # Double Diamond methodology config
├── config/blind-spots/          # 6 JSON weakness detection files
├── tests/                       # smoke / unit / integration / e2e / live / benchmark
├── .claude/                     # Claude Code plugin metadata, agents, skills, hooks
├── .claude-plugin/plugin.json   # Plugin registration (47 commands)
├── .factory-plugin/plugin.json  # Factory AI integration
├── workflows/embrace.yaml       # Full 4-phase workflow config
└── Makefile                     # Test orchestration
```

### 2.2 Provider Orchestration Layer

| Provider | Role | Cost Model | Required? |
|----------|------|------------|-----------|
| Claude (Sonnet 4.6) | Orchestrator, synthesis, quality gate | Included with Claude Code | Yes |
| Codex (OpenAI) | Implementation depth, code patterns | User's OPENAI_API_KEY | No |
| Gemini (Google) | Ecosystem breadth, security review | User's GEMINI_API_KEY | No |
| Perplexity | Live web search, CVE lookups | User's PERPLEXITY_API_KEY | No |
| OpenRouter | 100+ model routing, budget alternatives | User's OPENROUTER_API_KEY | No |
| Copilot (GitHub) | Zero-cost research via subscription | GitHub Copilot subscription | No |
| Ollama | Local LLM, offline, privacy fallback | Free (local) | No |

**Execution models:**
- **Parallel:** Research/discover phase — Codex & Gemini run simultaneously
- **Sequential:** Define phase — consensus building via Codex → Gemini → Claude
- **Adversarial:** Review phases — multi-model cross-checking with disagreement surfacing
- **Quality gates:** 75% consensus required before advancing phases

### 2.3 Double Diamond Workflow

```
┌─────────────┬────────────┬────────────┬────────────┐
│  DISCOVER   │  DEFINE    │  DEVELOP   │  DELIVER   │
│  (Probe)    │  (Grasp)   │  (Tangle)  │   (Ink)    │
├─────────────┼────────────┼────────────┼────────────┤
│ Diverge:    │ Converge:  │ Diverge:   │ Converge:  │
│ Research &  │ Build      │ Multi-AI   │ Quality    │
│ exploration │ consensus  │ implement  │ assurance  │
└─────────────┴────────────┴────────────┴────────────┘
```

Each phase is a skill (`flow-discover/SKILL.md`, etc.) with execution contracts, visual indicators, and validation gates.

### 2.4 Entry Points

| Entry Point | Mechanism | Description |
|-------------|-----------|-------------|
| `/octo:*` commands | Claude Code slash command | 47 registered commands |
| `octo <intent>` | Natural language prefix | Smart router classifies and dispatches |
| MCP Server | `.mcp.json` registration | 10 tools via Model Context Protocol |
| OpenClaw Extension | `openclaw.extensions` | 97 skill workflows |
| Factory AI Droid | `DROID_PLUGIN_ROOT` | Autonomous spec-in, software-out mode |

### 2.5 Key Design Decisions

**Bash as primary language.** The core is ~29K lines of Bash (3.2+ compatible for macOS). This is a deliberate choice — the orchestrator's job is to dispatch to external CLI tools (codex, gemini, copilot, ollama) and collect stdout. Bash is the natural language for CLI coordination.

**MCP server as thin adapter.** The TypeScript MCP server (~440 lines) shells out to orchestrate.sh for all logic. It adds parameter validation via Zod schemas, security-hardened env forwarding, and IDE context injection, but duplicates zero business logic.

**File-based state.** Run artifacts live in `.octo/STATE.md` or `~/.claude-octopus/`. Cross-session handoff via `/octo-continue.md`. No database, no transactions.

**Persona system.** 32 personas defined as markdown with YAML frontmatter (`name`, `description`, `readonly`, `tools`). Auto-activated based on request context classification. Tool policies enforce RBAC — readonly personas cannot use Write/Edit/Bash.

---

## 3. Strengths

### 3.1 Thoughtful Security Model

The MCP server has a `BLOCKED_ENV_VARS` set preventing clients from overriding security-governing variables (`OCTOPUS_SECURITY_V870`, `OCTOPUS_GEMINI_SANDBOX`, `OCTOPUS_CODEX_SANDBOX`, `CLAUDE_OCTOPUS_AUTONOMY`). Provider environment is explicitly allowlisted rather than forwarded wholesale via `process.env`. API key leaks are sanitized from error output:

```typescript
const sanitized = msg.replace(/[A-Za-z_]+KEY=[^\s]+/g, "[REDACTED]");
```

Anti-injection nonces wrap untrusted content (`lib/secure.sh`). Path traversal is validated in both the MCP server and orchestrate.sh workspace path handling. Sandbox mode for Codex CLI is configurable with an allowlist (`workspace-write`, `write`, `read-only`) and rejects invalid values.

### 3.2 Defensive Bash Engineering

- Bash 3.2 compatibility maintained — no associative arrays, POSIX string helpers (`_ucfirst`, `_lowercase` via `tr` instead of `${var^^}`)
- `set -eo pipefail` at the top of orchestrate.sh
- Context budget enforcement (`enforce_context_budget`) prevents runaway token consumption — scales budget by role (implementer: 60%, verifier: 25%)
- Output guard (`guard_output`) detects >49KB content and writes to temp file instead of flooding Claude's context
- Search spiral guard in probe prompts: "If you find yourself searching more than 3 times in a row without writing analysis, STOP"
- Include guards on library modules (`[[ -n "${_OCTOPUS_SECURE_LOADED:-}" ]] && return 0`)

### 3.3 Well-Structured Decomposition

The v9.7.7 monolith extraction broke orchestrate.sh into 52 focused lib modules:

| Module | Lines | Responsibility |
|--------|-------|----------------|
| config-display.sh | 1,099 | Configuration rendering |
| smoke.sh | 1,066 | Smoke test helpers |
| doctor.sh | 1,063 | Health check diagnostics |
| workflows.sh | 990 | Double Diamond phase handlers |
| intelligence.sh | 952 | Context detection & classification |
| quality.sh | 939 | Quality gate evaluation |
| cost.sh | 818 | Cost tracking & reporting |
| spawn.sh | 761 | Agent lifecycle management |
| auto-route.sh | 725 | Intent classification & routing |
| debate.sh | 717 | Adversarial multi-model debate |

Each module has a clear header documenting what was extracted and what functions it provides.

### 3.4 Comprehensive Test Suite

138 test files organized in tiers with proper CI gating:

```
Smoke (<30s) → Unit (1-2min) → Integration (5-10min) → E2E (15-30min) → Live (real API)
```

Tests cover real integration issues — the v9.9.0 release caught debate flag placement bugs, quality_threshold being silently ignored, and env var allowlist gaps across MCP and OpenClaw adapters. The CI pipeline (`.github/workflows/test.yml`) has dependency ordering: smoke gates unit gates integration gates e2e.

### 3.5 Graceful Degradation

Works with zero external providers (Claude-only mode). Provider detection checks CLI availability + auth credentials. Circuit breaker prevents repeated calls to failing providers. Model restriction service (`validate_model_allowed`) supports per-provider allowlists with capability-aware fallback — if a blocked model has a capability match in the allowlist, it prefers that over naive first-in-list fallback.

### 3.6 Modular Configuration

Per-provider CLAUDE.md files loadable via `--add-dir`. Users only load the context they need:

```bash
claude --add-dir=config/providers/codex    # Working with Codex
claude --add-dir=config/providers/gemini   # Working with Gemini
claude --add-dir=config/workflows          # Double Diamond methodology
```

---

## 4. Concerns & Risks

### 4.1 CRITICAL: 48 Silent-Fail Sources

**All 48 library modules** are sourced with error suppression:

```bash
source "${SCRIPT_DIR}/lib/utils.sh" 2>/dev/null || true
source "${SCRIPT_DIR}/lib/secure.sh" 2>/dev/null || true
source "${SCRIPT_DIR}/lib/dispatch.sh" 2>/dev/null || true
```

If any module fails to load — syntax error, missing file, permission issue — orchestrate.sh proceeds silently with undefined functions. **A typo in `lib/secure.sh` would disable all security hardening with zero feedback.** A broken `lib/dispatch.sh` would cause every provider dispatch to fail at call time rather than at startup.

**Recommendation:** Fail-fast on critical modules:
```bash
source "${SCRIPT_DIR}/lib/secure.sh" || { echo "FATAL: security module failed to load" >&2; exit 1; }
source "${SCRIPT_DIR}/lib/dispatch.sh" || { echo "FATAL: dispatch module failed to load" >&2; exit 1; }
source "${SCRIPT_DIR}/lib/providers.sh" || { echo "FATAL: providers module failed to load" >&2; exit 1; }
```

Keep `|| true` only for genuinely optional modules (e.g., `semantic-cache.sh`, `copilot.sh`).

### 4.2 Orchestrate.sh Is Still 4,904 Lines

Despite extracting 52 modules, the main file remains large. The bottom ~2,000 lines are a giant `case` statement dispatching ~40 commands. Extracted comment tombstones remain:

```bash
# [EXTRACTED to lib/agents.sh]
```

Empty security sections (lines 150-170) are comment blocks with no code — the functions were extracted but the section headers weren't cleaned up.

**Recommendation:** Replace the monolithic case statement with a file-based command registry. Each command becomes a self-contained script in `scripts/commands/`, and the dispatcher does dynamic lookup:

```bash
cmd_file="${SCRIPT_DIR}/commands/${COMMAND}.sh"
[[ -f "$cmd_file" ]] && source "$cmd_file" || { echo "Unknown command: $COMMAND"; exit 1; }
```

### 4.3 JSON Generation via Heredocs Is Fragile

The `init-workflow` command (lines 4854-4886) builds JSON via string interpolation:

```bash
cat <<INIT_JSON
{
  "workflow": "$_init_workflow",
  "models": {
    "researcher": "$_model_researcher",
    ...
  }
}
INIT_JSON
```

If `_model_researcher` contains quotes, newlines, backslashes, or other special characters, this produces invalid JSON. `jq` is already a required dependency.

**Recommendation:** Use `jq -n` for all JSON generation:
```bash
jq -n \
  --arg workflow "$_init_workflow" \
  --arg researcher "$_model_researcher" \
  '{workflow: $workflow, models: {researcher: $researcher}}'
```

### 4.4 Feature Flag Sprawl (~80 SUPPORTS_* Flags)

Lines 196-288 of orchestrate.sh declare ~80 `SUPPORTS_*` boolean flags tracking Claude Code versions from v2.1.12 through v2.1.77+. Many are annotated as "METADATA" — declared but never checked in conditionals. They serve as a capability inventory for doctor diagnostics and logging.

This creates:
- **Maintenance burden:** Every Claude Code release adds 2-5 new flags
- **Readability cost:** 100 lines of flag declarations before any logic
- **Dead code risk:** Hard to audit which flags actually gate behavior vs. are purely decorative

**Recommendation:** Audit flags and separate into tiers:
1. **Gating flags** (checked in if-conditionals) — keep
2. **Diagnostic flags** (only read by doctor/status) — move to a capabilities registry file
3. **Dead flags** (never referenced after declaration) — remove

### 4.5 High Release Velocity Creates Fix-the-Fix Chains

Nine releases on 2026-03-22 (v9.7.8 through v9.9.2). The changelog shows clear fix-then-fix-the-fix patterns:

- **v9.9.0:** Adds Ollama and Copilot as providers
- **v9.9.1:** Fixes missing Ollama dispatch branch (v9.9.0 "wired detection but missed the dispatch branch"), fixes `detect_providers()` missing Perplexity/Ollama/Copilot
- **v9.9.2:** Fixes Ollama CLAUDE.md false claims, fixes AGENTS.md paths

This pattern suggests features ship before integration testing catches gaps. The test suite has the infrastructure (138 files, tiered CI) but the gap is likely in test coverage for new provider integration paths.

**Recommendation:** Before cutting a release that adds a new provider, require a mandatory provider integration test that verifies: detection, dispatch, model resolution, circuit breaker, doctor check, MCP forwarding, and OpenClaw registry sync. This would have caught all three v9.9.0 follow-up issues.

### 4.6 Lossy Context Budget Truncation

`enforce_context_budget` estimates tokens at 4 chars/token and hard-truncates:

```bash
local char_budget=$((budget * 4))
if [[ ${#prompt} -gt $char_budget ]]; then
    echo "${prompt:0:$char_budget}

[... truncated to fit context budget of ~$budget tokens ...]"
```

For prompts containing structured data, code blocks, or JSON, this will break mid-token, mid-JSON, or mid-instruction. A provider receiving a truncated JSON block or half an instruction may produce garbage output.

**Recommendation:** Truncate at paragraph or section boundaries. At minimum, ensure truncation doesn't break within a code fence or JSON block.

### 4.7 MCP Server Transport Security

The MCP server comment at line 429 explicitly notes:

```typescript
// SECURITY: stdio transport is scoped to the spawning process (local IDE only).
// If switching to HTTP/SSE/WebSocket, add bearer token authentication.
```

This is correct for the current stdio transport but is a latent risk. If the transport is ever changed (e.g., for remote IDE support), the lack of authentication would be a critical vulnerability. The comment serves as a reminder but is easy to miss during a transport migration.

### 4.8 Cost Tracking Is Estimate-Based

CLAUDE.md lists approximate per-query costs (Codex ~$0.01-0.15, Gemini ~$0.01-0.03, etc.) and `cost.sh` is 818 lines of tracking logic. However, actual API usage data (token counts, cached vs. uncached, etc.) is not available from CLI-based invocations. Users paying per-token for external providers may not get accurate cost reporting.

---

## 5. Orchestration System (Deep Dive)

### 5.1 Command Dispatch Architecture

orchestrate.sh's bottom ~2,000 lines are a monolithic `case` statement dispatching ~40 commands across 12 categories:

**Double Diamond Commands:**
| Command | Alias(es) | Handler | Description |
|---------|-----------|---------|-------------|
| `discover` | `research`, `probe` | `probe_discover()` | Parallel multi-perspective research (5-7 agents) |
| `define` | `grasp` | `grasp_define()` | 3-agent consensus building on problem definition |
| `develop` | `tangle` | `tangle_develop()` | Task decomposition + parallel subtask execution |
| `deliver` | `ink` | `ink_deliver()` | Quality gates + cross-model review scoring |
| `embrace` | — | `embrace_full_workflow()` | All 4 phases integrated end-to-end |
| `probe-single` | — | `probe_single_agent()` | Single-agent probe (v8.54.0, avoids 120s timeout) |
| `synthesize-probe` | — | standalone | Timeout recovery: synthesize already-collected results |

**Crossfire (Adversarial) Commands:**
| Command | Alias(es) | Handler | Description |
|---------|-----------|---------|-------------|
| `grapple` | `debate` | `grapple_debate()` | Multi-round Codex vs Gemini vs Sonnet debate |
| `squeeze` | `red-team` | `squeeze_test()` | Blue Team defense vs Red Team attack |
| `sentinel` | `watch` | `sentinel_watch()` | GitHub-aware issue/PR/CI triage monitor |

**Dark Factory Command:**
| Command | Alias(es) | Handler | Description |
|---------|-----------|---------|-------------|
| `factory` | `dark-factory` | `factory_run()` | Spec → scenarios → embrace → holdout → score → retry |

**Knowledge Worker Commands:**
| Command | Alias(es) | Handler |
|---------|-----------|---------|
| `empathize` | `empathy`, `ux-research` | `empathize_research()` |
| `advise` | `consult`, `strategy` | `advise_strategy()` |
| `synthesize` | `synthesis`, `lit-review` | `synthesize_research()` |
| `knowledge` | `km` | `toggle_knowledge_work_mode()` |

**Agent Dispatch Commands:**
| Command | Handler | Description |
|---------|---------|-------------|
| `spawn <agent> <prompt>` | `spawn_agent()` | Manual single-agent dispatch |
| `auto <prompt>` | `auto_route()` | Smart intent classification + routing |
| `parallel` | `parallel_execute()` | JSON task file → parallel execution |
| `fan-out` | `fan_out()` | Parallel dispatch to codex + gemini |
| `map-reduce` | `map_reduce()` | Decompose → parallel map → aggregate |

**Optimization Domain Commands (v4.2):**
`optimize-performance`, `optimize-cost`, `optimize-database`, `optimize-bundle`, `optimize-accessibility`, `optimize-seo`, `optimize-image`, `optimize-audit` (parallel 5-domain + synthesis)

**Utility Commands:**
`init`, `config`, `detect-providers`, `status`, `doctor`, `clean`, `skills`, `cost`, `audit`, `init-workflow`, `help`, `auth`, `release`, `completion`

### 5.2 Phase Functions in Detail

**`probe_discover(prompt)` — Phase 1/4 (Discover)**

1. Generates 5 core perspectives (problem, solutions, edge cases, feasibility, cross-synthesis)
2. Adds +1 codebase analysis if in git repo (claude-sonnet)
3. Adds +1 web research if `PERPLEXITY_API_KEY` set
4. Injects blind spots from `config/blind-spots/` JSON manifest based on keyword triggers
5. Routes perspectives to providers based on task type (security/review/architecture/research)
6. Displays cost estimate, asks user before proceeding
7. Spawns 5-7 agents in parallel with per-agent status tracking (success/timeout/failed)
8. Synthesizes via single Gemini call (or fallback concatenation)
9. Creates timeout recovery marker: `probe-needs-synthesis-${task_group}.marker`

**`grasp_define(prompt, probe_results)` — Phase 2/4 (Define)**

1. Three parallel agents: Codex (problem statement), Gemini (success criteria), Claude Sonnet (constraints)
2. Single Gemini consensus call to unify perspectives
3. Fallback: concatenate raw perspectives if consensus fails
4. Output: `grasp-consensus-*.md`

**`tangle_develop(prompt, grasp_file)` — Phase 3/4 (Develop)**

1. Gemini decomposes into 2-6 subtasks (cohesion emphasis)
2. Parallel execution: codex for `[CODING]` subtasks, gemini for `[REASONING]`
3. Validation gate via `validate_tangle_results()`
4. Design review ceremony before subtask execution (v8.18.0)
5. Provider lockout reset between phases

**`ink_deliver(prompt, tangle_results)` — Phase 4/4 (Deliver)**

1. Pre-delivery checks (results exist, no critical failures)
2. Sonnet 4.6 quality review (security, reliability, performance, accessibility)
3. Cross-model review scoring via `score_cross_model_review()` (v8.19.0)
4. Optional strict 4x10 gate (all dimensions 10/10 when `OCTOPUS_REVIEW_4X10=true`)
5. Optional simplification review (identify over-engineering, v8.29.0)
6. Final Gemini synthesis + quality certification
7. Output: `delivery-*.md`

### 5.3 Agent Spawn Lifecycle (spawn.sh)

**Prompt Assembly Pipeline (9 stages, ordered):**

1. Task classification & role determination
2. Routing rule override check (JSON config)
3. Checkpoint loading (crash-recovery from prior runs)
4. Persona application with pack overrides
5. Skill context injection (for curated agents)
6. Provider history context + heuristic context (file co-occurrence patterns from past successes)
7. Earned skills context
8. Search spiral guard (researchers: stop after 3 consecutive searches)
9. Context budget enforcement — **always last** (v8.10.0, Issue #25)

**Execution Dispatch (two paths):**

*Agent Teams path* (when `SUPPORTS_AGENT_TEAMS=true`, v8.5.0):
- Writes structured agent instruction JSON to `agent-teams/${task_id}.json`
- Outputs `AGENT_TEAMS_DISPATCH:...` marker for Claude Code native dispatch
- Returns immediately (native dispatch takes over)
- SubagentStop hook captures results (v2.1.72+)

*Legacy path* (Bash subprocess for Codex/Gemini):
- Spawns background process with isolated credential environment (`build_provider_env()`)
- Stdin-based prompt delivery (avoids `ARG_MAX` "Argument list too long", v9.2.2)
- Auth-aware retry loop with exponential backoff on 401/403
- Output capture: writes to temp file, filters CLI envelope, preserves partial on timeout
- Timeout handling via `run_with_timeout()` (exit code 124 = timeout, 143 = killed)

### 5.4 Smart Router (auto-route.sh)

`auto_route(prompt)` classifies along 5 dimensions before dispatching:

1. **Task type:** `classify_task()` → research | coding | review | design | copywriting | image | general
2. **Complexity:** `estimate_complexity()` → 1 (trivial), 2 (standard), 3 (premium)
3. **Cynefin domain:** `classify_cynefin()` → obvious | complicated | complex | chaotic
4. **Context:** `detect_context()` → on-site | remote | hybrid | asynchronous
5. **Response mode:** `detect_response_mode()` → direct | lightweight | standard

**Short-circuits:**
- `direct` mode: Claude handles natively (no external providers)
- `lightweight` mode: Single cross-check from fastest provider
- Trivial task detection → `handle_trivial_task()` (skips expensive LLMs)

**Routing targets:**
- Double Diamond: `diamond-discover`, `diamond-define`, `diamond-develop`, `diamond-deliver`
- Crossfire: `crossfire-grapple` (debate), `crossfire-squeeze` (security)
- Knowledge work: `knowledge-empathize`, `knowledge-advise`, `knowledge-synthesize`
- Optimization: 7 specialized workflows + full audit (5 parallel domains + synthesis)

### 5.5 Provider Reliability (provider-router.sh)

**Circuit breaker pattern:**
- `record_provider_failure()` tracks consecutive transient failures
- After `OCTO_CB_FAILURE_THRESHOLD` (default 3) → opens circuit
- Cooldown: `OCTO_CB_COOLDOWN_SECS` (default 300s)
- Half-open: after cooldown expires, allows one probe request
- `record_provider_success()` clears failure log and closes circuit

**Error classification:**
- *Transient:* timeout (124), rate limits (429), server errors (5xx), network
- *Permanent:* auth failures (401/403), billing, invalid requests (400/404)

**Provider selection modes** (via `OCTOPUS_ROUTING_MODE`):
- `round-robin`: Simple rotation (file-based state)
- `fastest`: Lowest average latency from metrics
- `cheapest`: Lowest average cost from metrics
- `scored`: Highest provider score (considers reliability + latency + cost)

### 5.6 Task Ledger Bridge (agent-teams-bridge.sh)

Synchronizes bash-spawned agents with Claude Code's native Agent Teams:

- `bridge_init_ledger()` — Creates JSON ledger with workflow, phases, tasks, gates, memory
- `bridge_register_task()` / `bridge_mark_task_complete()` — Atomic task lifecycle (lockfile-protected)
- `bridge_evaluate_gate()` — Phase completion ratio vs. threshold (default 0.75)
- `bridge_store_agent_id()` / `bridge_get_agent_id()` — Maps tasks to Claude Code agentIds for resume
- `bridge_write_warm_start_memory()` — Phase summaries for warm-start in subsequent phases
- `bridge_cleanup()` — Archives ledger to `history/ledger-*.json`

### 5.7 State & Metrics

**State Manager (state-manager.sh, 505 lines):**
- Tracks current workflow, phase, decisions (with rationale), blockers (active/resolved), metrics
- Atomic writes: temp file → validate → backup → move
- Generates human-readable `STATE.md` alongside JSON state

**Metrics Tracker (metrics-tracker.sh, 472 lines):**
- Per-agent: start time, input tokens (estimated 4 chars/tok), output tokens, duration, cost
- Uses native Claude Code Task tool metrics (v2.1.30+) when available, falls back to estimation
- Per-phase and per-provider breakdowns with native indicator (*) and fast mode indicator (lightning bolt)
- Session totals with history save

### 5.8 Dark Factory Pipeline

7-phase autonomous pipeline (`factory_run()`):

1. **Parse spec** → Extract satisfaction target
2. **Score spec quality** → NQS gate (minimum 85/100)
3. **Generate scenarios** → Test scenarios from spec
4. **Split holdout** → Deterministic 80/20 split (visible vs. blind test set, min 1, max 20%)
5. **Embrace workflow** → Full 4-phase with visible scenarios (autonomous mode, no cost prompts)
6. **Holdout tests** → Blind evaluation: PASS (1.0), PARTIAL (0.5), FAIL (0.0) per scenario
7. **Score satisfaction** → PASS/WARN/FAIL verdict
8. **Retry loop** → If FAIL, re-run phases 3-4 with remediation context (up to `max_retries`)
9. **Generate report** → `factory-report.md`

### 5.9 Key Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `OCTOPUS_ROUTING_MODE` | Provider selection: round-robin, fastest, cheapest, scored | round-robin |
| `OCTOPUS_DISPATCH_STRATEGY` | Provider count: full, minimal, smart | smart |
| `OCTOPUS_CODEX_SANDBOX` | Codex sandbox: workspace-write, write, read-only | workspace-write |
| `OCTOPUS_COST_TIER` | Cost awareness: balanced, budget, premium | balanced |
| `OCTOPUS_CONTEXT_BUDGET` | Max prompt tokens before truncation | 12000 |
| `OCTOPUS_REVIEW_4X10` | Enable strict all-10/10 quality gate | false |
| `OCTOPUS_PERSONA_PACKS` | Persona override mode: auto, off | auto |
| `OCTOPUS_HEURISTIC_LEARNING` | File co-occurrence heuristics | on |
| `OCTOPUS_FACTORY_HOLDOUT_RATIO` | Holdout test set ratio | 0.20 |
| `OCTOPUS_FACTORY_MAX_RETRIES` | Max remediation retry attempts | 1 |
| `OCTO_CB_FAILURE_THRESHOLD` | Circuit breaker failure count to trip | 3 |
| `OCTO_CB_COOLDOWN_SECS` | Circuit breaker cooldown duration | 300 |

---

## 6. Skills System (Deep Dive)

### 6.1 Skill Locations

Skills exist in two locations with different roles:
- **`skills/`** — 41 skill directories, each containing `SKILL.md` with full execution contracts
- **`.claude/skills/`** — 50 skill markdown files, enhanced versions with trigger patterns and Claude Code integration metadata

### 6.2 Execution Contract Pattern

All ENFORCED skills follow a mandatory multi-step execution contract:

```yaml
# Skill frontmatter
execution_mode: enforced
pre_execution_contract:
  - interactive_questions_answered
  - visual_indicators_displayed
validation_gates:
  - orchestrate_sh_executed
  - synthesis_file_exists
```

**Contract stages:**
1. Display visual indicators banner (provider status)
2. Read prior context from state files
3. Ask clarifying questions (sometimes via AskUserQuestion tool)
4. Execute `orchestrate.sh` via Bash tool (**MANDATORY — cannot simulate**)
5. Verify output artifact exists on disk
6. Update state (metrics, decisions)
7. Present results to user

**Hard gates** are marked with `<HARD-GATE>` and cannot be skipped. If a hard gate fails: stop, report error, show logs, do not substitute.

### 6.3 Complete Skill Inventory

#### Flow Skills (Double Diamond) — ENFORCED

| Skill | Command | Phase | Cost | Time |
|-------|---------|-------|------|------|
| `flow-discover` | `/octo:discover` | 1: Diverge (Research) | $0.01-0.08 | 2-5 min |
| `flow-define` | `/octo:define` | 2: Converge (Clarify) | $0.01-0.05 | 2-5 min |
| `flow-develop` | `/octo:develop` | 3: Diverge (Build) | $0.02-0.10 | 3-7 min |
| `flow-deliver` | `/octo:deliver` | 4: Converge (Ship) | $0.02-0.08 | 3-7 min |
| `flow-spec` | `/octo:spec` | Pre-flow (NLSpec) | varies | varies |
| `flow-parallel` | `/octo:teams` | Meta (team coordination) | varies | varies |

#### Debate Skill — ENFORCED

| Skill | Command | Participants | Rounds |
|-------|---------|-------------|--------|
| `skill-debate` | `/debate` | Codex, Gemini, Claude, Sonnet | 1-10 |

Debate modes: `cross-critique` (ACH falsification, default) or `blinded` (independent evaluation, prevents anchoring bias). Quality scoring: 100 pts = 25 length + 25 citations + 25 code examples + 25 engagement.

#### Core Implementation Skills — ENFORCED

| Skill | Description |
|-------|-------------|
| `skill-tdd` | Red-Green-Refactor cycle. Hard gate: "NO PRODUCTION CODE WITHOUT FAILING TEST FIRST" |
| `skill-factory` | Dark Factory: Parse → Scenarios → Embrace → Holdout → Score. Cost: $0.50-2.00, Time: 15-45 min |
| `skill-verify` | Evidence-based verification. Hard gate: cannot claim completion without fresh command output |
| `skill-debug` | Systematic debugging with methodical investigation |
| `skill-code-review` | Multi-LLM code review pipeline |

#### Workflow & Orchestration Skills — ENFORCED

| Skill | Description |
|-------|-------------|
| `skill-staged-review` | Two-stage spec-then-quality review pipeline |
| `skill-writing-plans` | Zero-context implementation plans with bite-sized tasks |
| `skill-parallel-agents` | Multi-tentacled orchestration using Double Diamond |
| `skill-finish-branch` | Post-implementation: verify tests, merge/PR/keep/discard |

#### Quality & Security Skills

| Skill | Description | Mode |
|-------|-------------|------|
| `skill-audit` | Systematic codebase auditing | Default |
| `octopus-security-audit` | OWASP compliance, vulnerability scanning | Enforced |
| `skill-security-framing` | URL validation & content wrapping (internal utility) | Default |
| `skill-claw` | OpenClaw instance administration | Enforced |

#### Knowledge & Research Skills

| Skill | Description |
|-------|-------------|
| `skill-prd` | AI-optimized PRD creation (100-point scoring) |
| `skill-decision-support` | Options presentation with trade-offs |
| `skill-thought-partner` | Creative brainstorming with Pattern Spotting & Paradox Hunting |
| `skill-meta-prompt` | Generate optimized prompts using proven techniques |
| `skill-deep-research` | In-depth research with cross-provider validation |
| `octopus-architecture` | System architecture & API design with consensus |
| `skill-extract` | Design system & product reverse-engineering |
| `skill-content-pipeline` | Extract patterns from URLs |
| `skill-ui-ux-design` | UI/UX design with multi-AI consensus |

#### Documentation & Delivery Skills

| Skill | Description |
|-------|-------------|
| `skill-doc-delivery` | Convert markdown → DOCX, PPTX, XLSX |
| `skill-deck` | Generate slide decks from briefs |
| `skill-resume` | Restore context from previous session |

#### Project Management Skills

| Skill | Description |
|-------|-------------|
| `skill-task-management` | Task management via TaskCreate/TaskUpdate |
| `skill-status` | Project progress dashboard & next steps |
| `skill-issues` | Track & manage project issues |
| `skill-iterative-loop` | Execute tasks in loops until goals met |
| `skill-rollback` | Rollback to git checkpoint tags |

#### Utility Skills

| Skill | Description |
|-------|-------------|
| `skill-doctor` | Environment diagnostics & provider check |
| `sys-configure` | Configure Claude Octopus providers & preferences |
| `skill-intent-contract` | Capture user goals & validate against them |
| `skill-context-detection` | Auto-detect work context (Dev vs Knowledge) |
| `skill-knowledge-work` | Override context auto-detection |
| `skill-visual-feedback` | Process screenshot UI/UX feedback |
| `octopus-quick` | Quick execution without workflow overhead |
| `skill-cost-projections` | Project remaining workflow cost |
| `skill-coverage-audit` | Trace codepaths in diffs, auto-generate tests |
| `skill-copilot-provider` | GitHub Copilot CLI as optional zero-cost provider |

### 6.4 Skill Loading & Activation

**Discovery process (OpenClaw):**
1. `openclaw/src/skill-loader.ts` scans `.claude/skills/*.md`
2. Parses YAML frontmatter: `name`, `description`, `aliases`, `execution_mode`, `validation_gates`
3. Generates TypeBox parameter schemas
4. Registers each skill as an OpenClaw `AgentTool`

**Activation triggers** (defined in `.claude/skills/*.md` frontmatter):
```yaml
trigger: |
  AUTOMATICALLY ACTIVATE when user says:
  "research X" or "explore Y"
```

**Execution modes:**
| Mode | Behavior |
|------|----------|
| `enforced` | Mandatory multi-step contract, blocking gates, cannot skip |
| `default` | Standard execution, adaptable by user |
| `supervised` | Requires user approval between phases |
| `autonomous` | Runs without intervention |

### 6.5 Blind Spots Library

`config/blind-spots/` contains 5 domain-specific JSON files with systematic perspectives that LLMs typically miss:

| Domain | Keywords | Example Blind Spot |
|--------|----------|--------------------|
| Security | auth, credential, token, vulnerability | Supply-chain attacks via compromised integrations |
| Architecture | microservice, scaling, distributed | Distributed consensus failure modes |
| Data | database, schema, storage, cache | Data migration rollback strategies |
| API | rest, graphql, endpoint, webhook | API versioning deprecation paths |
| Operations | deploy, ci/cd, kubernetes, monitor | Canary deployment failure isolation |

These inject additional perspectives into probe's edge-case and synthesis agents based on keyword matching against the user's prompt.

---

## 7. Commands System (Deep Dive)

### 7.1 Complete Command Inventory (40 Commands)

#### Core Routing
| Command | Description |
|---------|-------------|
| `/octo:auto` | Smart router — intent classification → workflow dispatch. Confidence >80% auto-routes, 70-80% suggests, <70% asks |
| `/octo:octo` | Legacy redirect to `/octo:auto` |

#### Double Diamond Workflow
| Command | Phase | Emoji |
|---------|-------|-------|
| `/octo:discover` | 1: Research & exploration | 🔍 |
| `/octo:define` | 2: Clarify & scope | 🎯 |
| `/octo:develop` | 3: Implementation | 🛠️ |
| `/octo:deliver` | 4: Validation & review | ✅ |
| `/octo:embrace` | All 4 phases | 🐙 |

#### Planning & Specification
| Command | Description |
|---------|-------------|
| `/octo:plan` | Strategic execution plan (doesn't execute). Captures 5-question intent, maps to phase weights, saves `.claude/session-plan.md` |
| `/octo:spec` | NLSpec authoring from multi-AI research |
| `/octo:prd` | Product Requirements Document generation |
| `/octo:prd-score` | PRD quality and completeness scoring |

#### Code Quality
| Command | Description |
|---------|-------------|
| `/octo:review` | Multi-LLM code review with inline PR comments. Targets: staged, working-tree, PR number, file path. Focus areas: correctness, security, performance, architecture, style, tests |
| `/octo:tdd` | Test-Driven Development automation |
| `/octo:security` | OWASP audit with threat modeling. Models: standard web, high-value, compliance, API-focused |
| `/octo:debug` | Systematic debugging |
| `/octo:staged-review` | Two-stage spec-then-quality review |

#### Research & Knowledge
| Command | Description |
|---------|-------------|
| `/octo:research` | Deep multi-perspective investigation |
| `/octo:brainstorm` | Collaborative idea generation |
| `/octo:debate` | Four-way AI debate (Claude, Sonnet, Gemini, Codex) |
| `/octo:km` | Knowledge Mode toggle (Dev 🔧 ↔ Knowledge 🎓) |

#### Delivery & Documentation
| Command | Description |
|---------|-------------|
| `/octo:docs` | Document delivery (export to PPTX, DOCX, PDF) |
| `/octo:deck` | Presentation deck generation |
| `/octo:design-ui-ux` | UI/UX design with multi-AI consensus |

#### Orchestration
| Command | Description |
|---------|-------------|
| `/octo:parallel` | Team of Teams — decompose + parallel `claude -p` workers |
| `/octo:multi` | Multi-instance coordination and aggregation |
| `/octo:pipeline` | CI/CD pipeline orchestration |

#### Monitoring & Automation
| Command | Description |
|---------|-------------|
| `/octo:sentinel` | GitHub-aware triage (issues, PRs, CI). Triage-only, never auto-executes |
| `/octo:scheduler` | Task scheduler with cron |
| `/octo:schedule` | Schedule tasks and workflows |
| `/octo:loop` | Iterative loop automation |

#### Advanced
| Command | Description |
|---------|-------------|
| `/octo:factory` | Dark Factory — spec-in, software-out autonomous pipeline |
| `/octo:claw` | OpenClaw integration admin |
| `/octo:extract` | Code extraction and refactoring |
| `/octo:meta-prompt` | Meta-prompt engineering |

#### Configuration
| Command | Description |
|---------|-------------|
| `/octo:setup` | Guided setup wizard |
| `/octo:model-config` | Model provider configuration |
| `/octo:quick` | Quick tasks without workflow overhead |
| `/octo:dev` | Development mode activation |
| `/octo:doctor` | System diagnostics and health check |

### 7.2 Command Execution Pattern

All commands enforce **MANDATORY COMPLIANCE** directives:

```markdown
When the user invokes `/octo:plan`, you MUST execute the structured
planning workflow below. You are PROHIBITED from doing the task directly,
skipping the intent capture questions, or deciding the task is "too simple"
for structured planning. The user chose this command deliberately —
respect that choice.
```

Common command structure:
1. Ask clarifying questions (scope, focus, autonomy level)
2. Display visual indicator banner with provider status
3. Execute skill/workflow via orchestrate.sh
4. Present results with quality scores
5. Ask "What would you like to do next?"

### 7.3 Embrace Workflow (Full Integration)

`/octo:embrace` asks 4 clarifying questions (scope, focus, autonomy, debate gates) then:

1. **Discover** → Multi-provider research
2. *[Optional debate gate: discuss approach risks]*
3. **Define** → Consensus building
4. **Develop** → Implementation with quality gates
5. *[Optional debate gate: validate implementation]*
6. **Deliver** → Final validation
7. Present results and phase artifacts

Autonomy modes: `supervised` (default), `semi-autonomous`, `autonomous`

---

## 8. Personas System (Deep Dive)

### 8.1 Persona Frontmatter Schema

```yaml
---
name: <persona-id>
description: <one-line expertise summary>
model: inherit | opus | sonnet | <specific-model>
memory: project | user | local | thread
tools: [Read, Glob, Grep, Bash, Task(...), WebSearch, WebFetch, ...]
readonly: true | false
when_to_use: |
  - Use case descriptions
avoid_if: |
  - Anti-pattern descriptions
examples:
  - prompt: "Example request"
    outcome: "Expected output"
hooks:
  PostToolUse:
    - matcher:
        tool: Bash
      command: "${CLAUDE_PLUGIN_ROOT}/hooks/<gate-hook>.sh"
---
```

### 8.2 Complete Persona Inventory (32 Personas)

#### Software Engineering (11)

| Persona | Model | Memory | Readonly | Description |
|---------|-------|--------|----------|-------------|
| `backend-architect` | inherit | project | — | Scalable API design, microservices, distributed systems. REST/GraphQL/gRPC, event-driven architectures, service mesh |
| `frontend-developer` | — | — | — | Frontend development and UI implementation |
| `code-reviewer` | opus | project | false | AI-powered code analysis, security vulnerabilities, performance optimization, production reliability |
| `debugger` | sonnet | project | — | Root cause analysis, intermittent bugs, race conditions |
| `tdd-orchestrator` | — | — | — | TDD lifecycle orchestration and test automation |
| `database-architect` | opus | project | — | Data layer design, technology selection, schema modeling, normalization, migration planning |
| `cloud-architect` | opus | local | — | AWS/Azure/GCP multi-cloud, IaC (Terraform/OpenTofu/CDK), FinOps, serverless, disaster recovery |
| `performance-engineer` | inherit | project | — | OpenTelemetry, distributed tracing, load testing, caching, Core Web Vitals |
| `graphql-architect` | — | — | — | GraphQL schema design and optimization |
| `ai-engineer` | inherit | local | — | Production LLM apps, RAG systems, intelligent agents, vector search, multimodal AI |
| `deployment-engineer` | — | — | — | Deployment automation and release management |

#### Documentation (5)

| Persona | Description |
|---------|-------------|
| `docs-architect` | Documentation strategy and content architecture |
| `academic-writer` | Academic writing and research communication |
| `exec-communicator` | Executive communication and leadership |
| `product-writer` | Product documentation and technical writing |
| `mermaid-expert` | Diagram creation and visualization |

#### Research & Strategy (3)

| Persona | Description |
|---------|-------------|
| `strategy-analyst` | Strategic business analysis and competitive intelligence |
| `research-synthesizer` | Literature review, research synthesis, academic writing |
| `business-analyst` | Business process analysis and requirements |

#### Business (3)

| Persona | Description |
|---------|-------------|
| `marketing-strategist` | Marketing strategy and demand generation |
| `finance-analyst` | Financial analysis and business modeling |
| `legal-compliance-advisor` | Legal and compliance framework implementation |

#### Security & Operations (3)

| Persona | Model | Description |
|---------|-------|-------------|
| `security-auditor` | opus | DevSecOps, vulnerability assessment, threat modeling, OWASP, OAuth2/OIDC |
| `incident-responder` | — | Incident management and crisis response |
| `context-manager` | — | Context awareness and state management |

#### Design & UX (2)

| Persona | Model | Description |
|---------|-------|-------------|
| `ux-researcher` | opus | User research synthesis, journey mapping, persona creation, usability evaluation |
| `ui-ux-designer` | — | UI/UX design and interaction design |

#### Language-Specific (3)

| Persona | Description |
|---------|-------------|
| `python-pro` | Python development expertise |
| `typescript-pro` | TypeScript development expertise |
| `test-automator` | AI-powered test automation, self-healing tests, CI/CD integration |

#### Admin (1)

| Persona | Description |
|---------|-------------|
| `openclaw-admin` | OpenClaw administration and integration |

### 8.3 Persona Activation Mechanics

**Persona loading** (`scripts/lib/personas.sh` + `persona-loader.sh`):

1. `get_persona_instruction(role)` — Loads persona markdown for a given role
2. `apply_persona(role, prompt, skip_persona, agent_name)` — Combines persona with prompt
3. Persona packs can override standard personas — searched in:
   - `./.octopus/personas` (project-local)
   - `~/.claude-octopus/personas` (user-global)
   - Custom paths via `OCTOPUS_PERSONA_PACKS` env var (colon-separated)
4. `get_agent_readonly(agent_name)` — Extracts `readonly: true` from frontmatter for RBAC enforcement

**Tool policy enforcement** (`apply_tool_policy()`):
| Policy | Allowed Tools | Applied When |
|--------|---------------|--------------|
| `read_search` | Read, Glob, Grep, WebSearch, WebFetch | readonly personas |
| `read_exec` | Read, Glob, Grep, Bash (read-only) | reviewer/verifier roles |
| `read_communicate` | Read, Glob, Grep only | synthesizer/release roles |
| `full` | All tools | implementer/developer roles |

**PostToolUse hooks** (selected personas):
- `architecture-gate.sh` — Architecture review after Bash
- `security-gate.sh` — Security check after Bash
- `perf-gate.sh` — Performance check after Bash
- `code-quality-gate.sh` — Quality check after Bash

### 8.4 Quality Principles (agents/principles/)

Four critique frameworks guide persona behavior:

**security.md** — 12 requirements: No SQL injection (parameterized queries), No XSS (proper encoding), No command injection (safe APIs), No CSRF (token validation), secure authentication (bcrypt/argon2), least privilege, secure defaults (fail closed), sensitive data handling (no logging), secure session management, input validation (server-side), error handling (no stack traces), dependency security (audit/update)

**performance.md** — 12 requirements: No N+1 queries, proper indexing, query optimization, strategic caching, memory efficiency, lazy loading, async I/O, connection pooling, parallel processing, algorithmic efficiency, minimize allocations, early exit

**maintainability.md** — Code quality and long-term sustainability

**general.md** — Foundational development practices

### 8.5 Factory AI Droids (agents/droids/)

10 condensed persona definitions optimized for Dark Factory mode:

`octo-backend-architect`, `octo-cloud-architect`, `octo-code-reviewer`, `octo-database-architect`, `octo-debugger`, `octo-docs-architect`, `octo-frontend-developer`, `octo-performance-engineer`, `octo-security-auditor`, `octo-tdd-orchestrator`

These inherit from full personas but are compressed for autonomous spec-in, software-out pipelines where context budget is tighter.

---

## 9. Code Quality Observations

### 9.1 Bash Style

- Consistent use of `local` for all function variables
- Proper quoting throughout (`"$variable"`, not `$variable`)
- Functions are well-named and follow a clear naming convention (`get_agent_command`, `validate_model_allowed`, `enforce_context_budget`)
- Logging is structured via a `log` function with levels (INFO, WARN, ERROR, DEBUG)
- POSIX compatibility maintained for macOS (Bash 3.2)

### 9.2 TypeScript Style (MCP Server)

- Clean separation: thin adapter that delegates to orchestrate.sh
- Proper use of Zod for parameter validation
- Explicit env allowlisting (not `...process.env`)
- Error handling with API key sanitization
- Type-safe with `as const` assertions

### 9.3 Documentation

Extensive and well-organized:
- README.md (19.1 KB) — feature guide and quickstart
- CLAUDE.md (12.3 KB) — system instructions with visual indicators
- ARCHITECTURE.md — provider flow and execution models
- COMMAND-REFERENCE.md — all 47 commands
- AGENTS.md — 32 personas catalog
- CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md
- Per-provider config docs (5 CLAUDE.md files)
- Test README with tier descriptions

The v9.9.2 documentation consolidation (removed 9 stale files) shows active maintenance.

---

## 10. Comparison to attest

Both projects share a philosophy of **structured, verifiable AI workflows** but approach it differently:

| Aspect | claude-octopus | attest |
|--------|---------------|--------|
| Language | Bash + TypeScript | Go |
| Approach | Multi-provider orchestration | Spec-driven verification |
| State | File-based (markdown, JSON) | File-based (JSON/JSONL in RunDir) |
| Quality gates | 75% consensus across providers | Council review + verifier pipeline |
| Concurrency | CLI subprocess parallelism | File locking via gofrs/flock |
| Dependencies | bash, jq, git + optional CLIs | 4 Go deps (lipgloss, yaml.v3, flock, term) |
| Binary size | N/A (interpreted) | Single binary |
| Testing | Shell test scripts (138 files) | Go test with race detector |
| Complexity control | Context budget (token estimate) | gocognit max 30, cyclop max 22 |

**Key philosophical difference:** Octopus trusts multiple models to catch each other's blind spots (adversarial consensus). Attest trusts deterministic compilation + evidence-based verification (spec traceability). Both are valid approaches to the "AI output quality" problem.

---

## 11. Summary of Recommendations

### Must-Fix (Security/Reliability)

1. **Make critical module sourcing fail-loud** — at minimum secure.sh, dispatch.sh, providers.sh
2. **Use jq for JSON generation** — prevent injection and malformed output from interpolated heredocs

### Should-Fix (Maintainability)

3. **Prune dead SUPPORTS_* flags** — audit which gate behavior vs. decorative, move diagnostics-only to a registry file
4. **Clean up orchestrate.sh** — remove tombstone comments, empty sections, consider file-based command dispatch
5. **Add provider integration smoke test gate** — prevent fix-the-fix chains on new provider additions

### Nice-to-Have (Quality)

6. **Improve context budget truncation** — paragraph/section-boundary aware, don't break structured content
7. **Track actual vs. estimated costs** — where CLI output provides token counts, use them
8. **Add transport security TODO** — make MCP auth a tracked issue, not just a comment
