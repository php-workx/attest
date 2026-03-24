# Learning Systems Comparison: claude-mem vs. agentops vs. attest

**Date:** 2026-03-22

---

## 1. System Summaries

### claude-mem (v10.6.2)

A Claude Code **plugin** that runs a background LLM agent as an observer. Every PostToolUse event is sent to this agent, which extracts structured `<observation>` XML (type, title, facts, narrative, concepts, files). Observations are stored in SQLite + Chroma vector DB. On SessionStart, a timeline of recent observations is injected into Claude's context. Search via MCP tools with 3-layer progressive disclosure.

**Philosophy:** Capture everything automatically, compress with LLM, recall by relevance.

### agentops (v2.25.0)

A **skill ecosystem** that implements a tiered knowledge flywheel. Knowledge flows from session transcripts through a forge → learnings → patterns promotion pipeline. Scoring is based on confidence + citations + recency, with automatic promotion (score ≥ 6) to MEMORY.md. Active maintenance via Athena (mine/grow/defrag) and flywheel health monitoring.

**Philosophy:** Extract signal from noise through tiered promotion, compound through citation feedback.

### attest (internal/learning/)

A **Go library** integrated with attest's spec-driven verification pipeline. Learnings are extracted deterministically from council review findings, judge rejection rationales, and verifier failures. Each learning has an effectiveness score (success rate when attached to future tasks). Stored as JSONL with file-lock concurrency safety.

**Philosophy:** Only capture what survives verification, score by empirical success rate.

---

## 2. Side-by-Side Comparison

### 2.1 What Gets Captured

| Dimension | claude-mem | agentops | attest |
|-----------|-----------|----------|--------|
| **Trigger** | Every PostToolUse (automatic) | SessionEnd hook + manual skills | Council/verifier completion |
| **Source material** | Raw tool input/output | Session transcripts | Structured findings with IDs |
| **Extraction method** | LLM agent (Claude/Gemini/OpenRouter) | LLM via `/forge` skill | Deterministic Go code |
| **Output format** | XML → structured observation record | Markdown with YAML frontmatter | Go struct → JSONL |
| **Types** | 6 (bugfix, feature, refactor, change, discovery, decision) | Untyped (free-form markdown) | 5 categories (pattern, anti_pattern, tooling, codebase, process) |
| **Concepts** | 7 (how-it-works, why-it-exists, problem-solution, gotcha, pattern, trade-off, what-changed) | None (free text) | Tags (auto-extracted keywords + manual) |
| **File tracking** | files_read, files_modified per observation | None | SourcePaths (from task scope) |
| **Volume** | High (one+ per tool call) | Medium (batched per session) | Low (max 5 per review, gated by verification) |

### 2.2 Storage Architecture

| Dimension | claude-mem | agentops | attest |
|-----------|-----------|----------|--------|
| **Primary store** | SQLite (`~/.claude-mem/claude-mem.db`) | Markdown files (`.agents/learnings/`) | JSONL (`.attest/learnings/index.jsonl`) |
| **Secondary store** | Chroma vector DB (semantic embeddings) | MEMORY.md (auto-promoted summaries) | Tag index (`.attest/learnings/tags.json`) |
| **Schema** | 11 SQL tables, FTS5 full-text search | Flat files + JSONL citation log | Single JSONL + inverted tag index |
| **Concurrency** | SQLite WAL mode | File-level (no locking) | flock-based exclusive access |
| **Cross-session** | Yes (primary purpose) | Yes (file persistence + MEMORY.md) | Yes (file persistence) |
| **Cross-project** | Per-project SQLite filtering | Global `~/.agents/` + per-project `.agents/` | Per-project `.attest/learnings/` only |
| **Migration system** | 1,388 LOC migration runner | None (flat files) | None (append-only JSONL) |
| **Backup/restore** | Export/import scripts | Git (files are committed) | Repair from JSONL backup |

### 2.3 Retrieval & Context Injection

| Dimension | claude-mem | agentops | attest |
|-----------|-----------|----------|--------|
| **Auto-injection** | SessionStart hook → timeline in context window | SessionStart hook → MEMORY.md already loaded by Claude Code | Not yet (AssembleContext exists but not wired) |
| **Search** | Semantic (Chroma) + structured (SQLite) + MCP tools | `ao search` + `ao lookup` (file-based) | `store.Query(tags, paths, category)` |
| **Progressive disclosure** | 3-layer: index → timeline → full details | None (full content or nothing) | Token-budgeted (2000 tokens, max 8 learnings) |
| **Relevance scoring** | Vector similarity (Chroma) + recency | Recency-weighted decay (~17%/week) + citation count | Effectiveness (success/attach ratio) |
| **Token awareness** | Yes (discovery_tokens tracked, 10x savings claimed) | Yes (400-800 token budget for inject) | Yes (2000 token cap, ~500/learning) |

### 2.4 Quality & Promotion

| Dimension | claude-mem | agentops | attest |
|-----------|-----------|----------|--------|
| **Quality gate** | None — "always save, never skip" | Confidence scoring (0.0-1.0) + citation count | Verification gating (only captures from verified findings) |
| **Promotion tiers** | None (flat storage) | 3 tiers: Forge (T0) → Learnings (T1) → Patterns (T2) | None (flat, but effectiveness scoring ranks) |
| **Promotion criteria** | N/A | T0→T1: confidence ≥ 0.7 or cited ≥ 2; T1→T2: confidence ≥ 0.8 AND cited ≥ 3 AND age > 30d | N/A (effectiveness rises with success rate) |
| **Deduplication** | Content-hash (16-char SHA, 30s window) | Title match + keyword overlap > 80% | Tag overlap + similar summaries |
| **Contradiction detection** | None | None | Pattern vs. anti_pattern with shared tags |
| **Expiry/GC** | None visible | Staleness: >30d uncited → archive | Auto-expiry: >90d without attach → expired |
| **Feedback loop** | None | Citation tracking → score boost → promotion | Outcome recording → effectiveness score |

### 2.5 Maintenance & Health

| Dimension | claude-mem | agentops | attest |
|-----------|-----------|----------|--------|
| **Maintenance skill** | None | `/athena` (mine/grow/defrag), `/flywheel` (health monitor) | `store.Maintain()` (dedup, expiry, GC, index rebuild) |
| **Health metrics** | Web viewer shows observation counts | Velocity (learnings/week), staleness %, cache hit rate, consistency % | AttachCount, SuccessCount per learning |
| **Artifact consistency** | None | `artifact-consistency.sh` (broken ref scanner) | Corruption detection + `Repair()` |
| **Active synthesis** | None | Athena "Grow" phase (LLM validates + synthesizes) | None |

### 2.6 Cost Model

| Dimension | claude-mem | agentops | attest |
|-----------|-----------|----------|--------|
| **Per-observation cost** | 1 LLM API call per tool use (unbounded) | 1 LLM call per session via /forge (batched) | Zero (deterministic extraction) |
| **Search cost** | SQLite query (free) or Chroma query (free, local) | File scan (free) | JSONL scan (free) |
| **Maintenance cost** | None | LLM calls in Athena "Grow" phase | Zero (Go code) |
| **Storage cost** | SQLite + Chroma disk | Flat files in git | JSONL + tag index |
| **Cost tracking** | discovery_tokens per observation | None explicit | None |

---

## 3. Architectural Diagrams

### claude-mem: Continuous Observation Loop

```
Every Tool Call ──→ Hook ──→ Worker ──→ LLM Agent ──→ XML Parse ──→ SQLite + Chroma
                                                                        │
SessionStart ────→ ContextBuilder ←──── SQLite query ───────────────────┘
                        │
                        └──→ Timeline markdown injected into Claude context
```

### agentops: Tiered Promotion Flywheel

```
Session End ──→ /forge ──→ .agents/forge/ (Tier 0)
                              │ conf ≥ 0.7
                              ▼
                         .agents/learnings/ (Tier 1)
                              │ conf ≥ 0.8, cited ≥ 3, age > 30d
                              ▼
                         .agents/patterns/ (Tier 2)
                              │ score ≥ 6
                              ▼
                         MEMORY.md (auto-promoted, max 200 lines)

                    ┌──────────────────────┐
Citations ─────────→│ Backlog Processing   │──→ Score boost
Session refs ──────→│ (dedup, staleness)   │──→ Promotion
                    └──────────────────────┘

/athena ──→ Mine (git/code) → Grow (LLM validate) → Defrag (cleanup)
/flywheel ──→ Health report (velocity, staleness, consistency)
```

### attest: Verification-Gated Extraction

```
Council Review ──→ Applied findings ──→ learning.FromFinding() ──→ Store.Add()
                   Rejected findings ──→ learning.FromRejection() ──→ Store.Add()
Verifier ─────────→ Blocking findings ──→ learning.FromVerifier() ──→ Store.Add()
Manual ───────────→ attest learn "..." ──→ Store.Add()

                                    ┌─────────────────────┐
Future task ──→ AssembleContext() ←─│ Query(tags, paths)  │
                     │              │ Sort by effectiveness│
                     │              │ Cap at token budget  │
                     ▼              └─────────────────────┘
              ContextBundle
              (max 8 learnings,        RecordOutcome(ids, passed)
               2000 tokens,                    │
               + latest handoff)               ▼
                                    SuccessCount / AttachCount = Effectiveness
```

---

## 4. Strengths & Weaknesses Matrix

| Strength | claude-mem | agentops | attest |
|----------|:---------:|:--------:|:------:|
| Zero-config automatic capture | **Strong** | Medium (hook + forge) | Weak (requires run completion) |
| Signal-to-noise ratio | Weak (captures everything) | **Strong** (tiered promotion) | **Strong** (verification-gated) |
| Semantic search | **Strong** (Chroma vectors) | Weak (file scan) | Weak (tag-based) |
| Cross-session continuity | **Strong** (primary purpose) | **Strong** (MEMORY.md + files) | Medium (files persist, not auto-injected) |
| Cost efficiency | Weak (LLM per tool call) | Medium (LLM per session) | **Strong** (zero incremental) |
| Empirical effectiveness scoring | None | Medium (citation-based) | **Strong** (success rate tracking) |
| Active knowledge maintenance | None | **Strong** (Athena + flywheel) | Medium (Maintain() but not automated) |
| Structured output | **Strong** (typed observations) | Weak (free-form markdown) | **Strong** (Go structs) |
| Dependency footprint | Weak (SQLite, Chroma, Bun, Express) | Medium (ao CLI, skills) | **Strong** (pure Go, zero deps) |
| Multi-IDE support | **Strong** (Claude Code, Cursor, OpenClaw) | Medium (Claude Code only) | N/A (CLI tool) |

---

## 5. What Each System Does Best

### claude-mem excels at:
- **Recall completeness** — never misses a tool use, every action is recorded
- **Semantic retrieval** — "find the time I fixed the auth bug" actually works via vector search
- **Real-time visibility** — web viewer shows observations as they happen
- **Context injection UX** — seamless timeline appears in every new session

### agentops excels at:
- **Knowledge compounding** — learnings get better over time through citation feedback
- **Signal quality** — tiered promotion ensures only valuable insights persist
- **Active maintenance** — Athena proactively validates, synthesizes, and cleans up
- **Health monitoring** — flywheel metrics give visibility into knowledge system health
- **Cross-repo patterns** — global patterns in `~/.agents/` work across projects

### attest excels at:
- **Precision** — only captures findings that survived multi-model review or verifier checks
- **Empirical scoring** — effectiveness is measured by actual task success rates, not proxy metrics
- **Zero cost** — deterministic extraction, no LLM calls for learning capture
- **Traceability** — every learning links back to a specific finding, task, and run
- **Concurrency safety** — flock-based locking handles concurrent workers

---

## 6. Gaps in Each System

### claude-mem gaps:
- No quality scoring or promotion — all observations are equal
- No feedback loop — observations don't get better over time
- No maintenance/cleanup — database grows unbounded
- No effectiveness measurement — no way to know which observations were useful
- No dedup beyond 30-second content-hash window

### agentops gaps:
- Free-form markdown makes structured queries hard
- No file-path tracking (can't query "learnings about src/auth/")
- LLM-dependent extraction introduces non-determinism
- No semantic search (file scan only)
- Citation tracking is the only feedback signal (no empirical task success measurement)

### attest gaps:
- Not yet auto-injected into task context (AssembleContext exists but isn't wired)
- No semantic search (tag + path queries only)
- No active maintenance automation (Maintain() must be called explicitly)
- No cross-project knowledge sharing
- Low capture volume (only from completed council/verifier runs)
- No real-time visibility into learning state

---

## 7. Synthesis: What attest Should Take From Each

### From claude-mem (selective adoption):

| Idea | Value | Adaptation for attest |
|------|-------|-----------------------|
| **Auto-capture from tool use** | Medium | Too noisy for attest's philosophy. But: auto-extract from engine events (task started, wave completed, verification passed/failed) without LLM cost. |
| **Context injection on session start** | **High** | Wire `AssembleContext()` into the engine's task dispatch. When a worker picks up a task, inject relevant learnings for that task's paths/tags. This is the single highest-value improvement. |
| **Semantic search** | Medium | Overkill for attest's learning volume (~76 learnings). Tag + path queries suffice. Revisit if volume exceeds 500+. |
| **Progressive disclosure** | Medium | The 3-layer pattern (index → summary → detail) is good UX if attest ever gets a search CLI. |
| **Token economics tracking** | Low | Attest learnings are free to produce. Useful only for context assembly budgeting. |

### From agentops (selective adoption):

| Idea | Value | Adaptation for attest |
|------|-------|-----------------------|
| **Tiered promotion** | **High** | Currently all learnings are equal. Adopt a 2-tier system: learnings (default) → patterns (high effectiveness, proven across multiple tasks). Auto-promote when effectiveness > 0.8 and AttachCount > 3. |
| **Active maintenance (Athena)** | **High** | Run `store.Maintain()` automatically at run start or on a cron. Add contradiction detection between patterns and anti-patterns. |
| **Citation tracking** | Medium | Already have AttachCount/SuccessCount. Could add explicit citation logging for audit trail. |
| **Flywheel health metrics** | Medium | Expose learning count, effectiveness distribution, stale count, and last-maintained timestamp via `attest status`. |
| **MEMORY.md promotion** | Low | Attest is a CLI tool, not a Claude Code plugin. MEMORY.md is Claude Code's mechanism. |
| **Cross-repo patterns** | Low | Attest is project-scoped by design. Could add a `--global` flag to `attest learn` for personal cross-project patterns. |

### Concrete recommendations for attest (priority order):

1. **Wire AssembleContext into task dispatch** — This is the #1 gap. The code exists (`store.AssembleContext()`), it just needs to be called when a worker begins a task. The token-budgeted context bundle is already designed correctly.

2. **Add tiered promotion** — Learnings with effectiveness > 0.8 and AttachCount > 3 auto-promote to a `patterns` category. Patterns get higher priority in AssembleContext queries. This makes the system compound.

3. **Automate Maintain()** — Call `store.Maintain(90 * 24 * time.Hour)` at engine startup or after each run completes. This handles dedup, contradiction detection, expiry, and index rebuild without manual intervention.

4. **Add `attest learnings` CLI** — Surface learning health: count by category, effectiveness distribution, top/bottom performers, stale count. Equivalent to agentops' `/flywheel`.

5. **Record outcomes automatically** — After verifier completes, call `RecordOutcome()` with the learning IDs that were in the task's context bundle. This closes the feedback loop without manual intervention. The code exists but isn't wired.

---

## 8. Should attest adopt claude-mem instead?

**No.** The systems solve different problems at different layers:

| | claude-mem | attest learnings |
|-|-----------|-----------------|
| **Scope** | All Claude Code sessions, any project | Attest runs only |
| **Signal quality** | Low (captures everything) | High (verification-gated) |
| **Cost** | Continuous LLM spend | Zero |
| **Dependencies** | SQLite, Chroma, Bun, Express, Agent SDK | None (pure Go) |
| **Integration** | Plugin (external) | Library (internal) |

Claude-mem is a **general-purpose session memory** plugin. Attest's learning system is a **domain-specific knowledge accumulator** tied to spec verification. They're complementary:

- A user could run claude-mem alongside attest to get broad session memory
- Attest's learnings would be a high-signal subset focused on what matters for verification quality
- Replacing attest's learning system with claude-mem would lose: effectiveness scoring, verification gating, zero-cost extraction, and the tight integration with the council/verifier pipeline

**The right move is to close attest's wiring gaps (recommendations 1-5 above), not to adopt a different system.**
