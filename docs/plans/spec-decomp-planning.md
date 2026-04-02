# Plan: Spec Decomposition & Planning Improvements

**Status:** Planned
**Branch:** TBD
**Date:** 2026-03-21 (updated 2026-04-02)

---

## 1. Problem Statement

fabrikk's planning pipeline (prepare → tech-spec → plan → compile) is structurally complete and gated, but the **quality of decomposition is low**. The compiler groups up to 4 requirements per task based on text proximity, infers file paths from keywords, and produces no symbol-level detail. The result: tasks are structurally correct but informationally thin — agents implementing them must rediscover the codebase from scratch.

Three specific gaps:

1. **Tasks are too large.** Grouping up to 4 requirements per task splits agent attention. A task with 4 requirements has ambiguous "done" conditions, tests that span multiple concerns, and reviews that cover too much surface area. One requirement per task keeps agents focused.

2. **No codebase exploration before planning.** The compiler works from requirement text alone. It infers paths via keyword matching ("compiler" → `internal/compiler`), never reads actual files to find function signatures, struct definitions, or reuse points. Slices get vague `FilesLikelyTouched` instead of concrete symbol-level detail.

3. **No council review of execution plans.** The structural review catches graph issues (cycles, duplicates, missing fields) but not design issues (scope gaps, false dependencies, missing file conflicts, underspecified acceptance criteria). A multi-model council review — like the one we already have for tech specs — would catch these before implementation begins.

### Prior Art

AgentOps `/plan` (studied in `.agents/research/2026-03-21-agentops-plan-decomposition.md`) demonstrates a 10-step planning workflow with:

- **Codebase exploration** via dispatched Explore agents requesting symbol-level detail
- **Pre-planning baseline audit** grounding the plan in verified numbers (wc, grep, ls)
- **Symbol-level implementation detail** for every file: exact function signatures, struct definitions, reuse points with `file:line` references, named test functions
- **File-conflict matrix** and **cross-wave shared file registry** for wave computation
- **Conformance checks** per issue: `files_exist`, `content_check`, `tests`, `command`
- **Boundaries:** Always (non-negotiable), Ask First (needs human), Never (out-of-scope)
- **Dependency necessity validation** removing false deps that only impose logical ordering
- **Issue granularity:** 1-2 files per issue, split at 3+

## 2. Design

### 2.1 Task Granularity: One Requirement Per Task

**Change:** `maxGroupSize` from 4 to 1 in `internal/compiler/compiler.go`.

```go
// Before:
const maxGroupSize = 4

// After:
const maxGroupSize = 1
```

**Exception:** Requirements that reference the same function/struct and can't be implemented independently remain grouped. The grouping logic in `shouldStartNewGroup` already handles this via theme/proximity checks — with `maxGroupSize = 1`, the theme check becomes the sole grouping mechanism.

**Impact on existing tests:** Tests that assert specific task counts will change. The compiler produces more, smaller tasks. Coverage remains 100% — every requirement still maps to exactly one task.

**Why:** An agent working on "Implement AT-FR-001" has one clear requirement, writes tests for that one thing, and gets reviewed on that scope. No ambiguity about what "done" means. Parallel execution improves because more tasks are independent.

### 2.2 Wave Computation with File-Conflict Analysis

**Currently:** No explicit wave computation. Tasks have `DependsOn` but no wave assignment. `GetPendingTasks()` returns all tasks with satisfied dependencies — implicit parallelism at execution time.

**Change:** Add explicit wave computation after task compilation, with file-conflict analysis.

#### Wave assignment algorithm

```go
// ComputeWaves assigns each task to a wave based on its dependency depth.
func ComputeWaves(tasks []state.Task) []Wave {
    depth := make(map[string]int) // task ID → wave number (0-indexed)

    // Topological sort to compute depth
    for _, task := range tasks {
        computeDepth(task.TaskID, tasks, depth)
    }

    // Group by depth
    waves := groupByDepth(tasks, depth)

    // Validate: no file conflicts within a wave
    for i := range waves {
        conflicts := detectFileConflicts(waves[i].Tasks)
        if len(conflicts) > 0 {
            // Serialize conflicting tasks: move later ones to next wave
            waves = resolveConflicts(waves, i, conflicts)
        }
    }

    return waves
}
```

#### New types

```go
// Wave groups tasks that can execute in parallel.
type Wave struct {
    WaveID int
    Tasks  []string // task IDs
}

// FileConflict records two tasks claiming the same file.
type FileConflict struct {
    FilePath string
    TaskA    string
    TaskB    string
}
```

**Note:** `ExecutionSlice` already has a `WaveID string` field. The wave computation populates this existing field rather than adding a new one.

#### File-conflict matrix

Before assigning tasks to waves, build a conflict matrix from `Scope.OwnedPaths`:

| File | Tasks | Conflict? |
|---|---|---|
| `internal/engine` | task-1, task-3 | YES → serialize |
| `internal/compiler` | task-2 | no |
| `cmd/fabrikk` | task-4 | no |

Conflicts are resolved by moving the later task (by Order) to the next wave. This preserves the dependency DAG while preventing parallel workers from touching the same files.

#### Cross-wave shared file registry

After wave assignment, build a registry of files that appear in multiple waves:

| File | Wave 1 Tasks | Wave 2+ Tasks | Mitigation |
|---|---|---|---|
| `internal/state/types.go` | task-1 | task-5 | Wave 2 worktree must branch from post-Wave-1 SHA |

This registry is displayed during `fabrikk plan review` and stored in the execution plan for the orchestrator to enforce base-SHA refresh between waves.

#### False dependency removal

For each `DependsOn` entry, verify the dependency is real:

1. Does the blocked task modify files that the blocker also modifies? → **Keep**
2. Does the blocked task read output produced by the blocker? → **Keep** (checked via `RequirementIDs` overlap)
3. Is the dependency only lane-based ordering (same requirement family)? → **Remove** if no file overlap

This increases parallelism by removing artificial serialization from the current lane-based dependency scheme.

#### Integration

- `CompileExecutionPlan` calls `ComputeWaves` after task creation
- Wave assignments stored on `ExecutionSlice.WaveID` (existing field, currently unpopulated)
- `renderExecutionPlanMarkdown` shows wave groupings
- `ReviewExecutionPlan` validates wave assignments (no intra-wave file conflicts)

### 2.3 Codebase Exploration Before Planning

**Currently:** `DraftExecutionPlan` calls `compiler.Compile(artifact)` which works from raw requirement text. No codebase exploration.

**Change:** Add a two-phase exploration step before plan drafting: deterministic structural analysis first, then LLM interpretation.

#### Hybrid approach: deterministic analysis + LLM interpretation

**Phase 1 — Deterministic structural analysis** (fast, cached, no LLM tokens):

Shell out to `code_intel.py` from the codereview skill. The script is discovered at runtime via `resolveCodeIntel()` which checks:
1. `FABRIKK_CODE_INTEL` environment variable (explicit path override)
2. `code_intel.py` on PATH
3. Common install locations: `~/.claude/skills/codereview/scripts/code_intel.py`, `~/workspaces/skill-codereview/scripts/code_intel.py`

The script provides:
- **Tree-sitter AST extraction** (8 languages, regex fallback) — function signatures, imports, exports with line ranges and visibility
- **Semantic similarity** — all-MiniLM-L6-v2 ONNX embeddings find related functions across files
- **Dependency graph** — import edges + cross-file call edges + semantic similarity scores
- **Complexity hotspots** — gocyclo/radon with tree-sitter fallback

All output as JSON. Tree-sitter is optional (graceful regex fallback). Results are cached by file mtime — subsequent runs only re-analyze changed files.

When `code_intel.py` is not available (no Python, script not found), fall back to pure LLM exploration (Phase 2 only).

**Phase 2 — LLM interpretation** (uses structural data as context):

Dispatch a CLI agent (via `agentcli`) with:
- The normalized requirements from the `RunArtifact`
- The structural analysis JSON from Phase 1 (if available) — injected as a `## Codebase Structure` section in the prompt
- Instructions to map requirements to code locations and return `ExplorationResult`

The LLM doesn't waste tokens rediscovering file structure — it interprets requirements against the structural data:
1. **Map requirements to symbols** — which existing functions/types each requirement touches
2. **Identify reuse points** — semantic similarity suggests related code the plan should build on
3. **Flag gaps** — requirements that don't map to any existing symbol need new files
4. **Suggest task grouping** — dependency graph edges inform wave computation

When Phase 1 is unavailable, the LLM falls back to tool-based exploration (Grep/Glob/Read) to discover the same information — slower but still functional.

#### Why hybrid, not pure LLM or pure AST

- Pure LLM: wastes tokens on Grep/Read to rediscover what AST parsing finds instantly
- Pure AST: can't match requirements to code by intent, only by name
- Hybrid: deterministic analysis provides the facts, LLM provides the judgment

#### Types

```go
// StructuralAnalysis holds the output of code_intel.py graph analysis.
// Populated by Phase 1 (deterministic). Nil when code_intel.py is unavailable.
type StructuralAnalysis struct {
    Nodes []StructuralNode `json:"nodes"` // functions, types, methods
    Edges []StructuralEdge `json:"edges"` // imports, calls, semantic similarity
}

type StructuralNode struct {
    ID       string `json:"id"`       // "path/to/file.go::FunctionName"
    Kind     string `json:"kind"`     // "function", "class", "struct", "method"
    File     string `json:"file"`
    Line     int    `json:"line"`
    Exported bool   `json:"exported"`
}

type StructuralEdge struct {
    From  string  `json:"from"`
    To    string  `json:"to"`
    Type  string  `json:"type"`  // "imports", "calls", "semantic_similarity"
    Score float64 `json:"score"` // 0.0-1.0, only for semantic_similarity edges
}

// ExplorationResult holds the combined output of Phases 1+2.
// Populated by the LLM agent using structural data as context.
type ExplorationResult struct {
    FileInventory []FileInfo     `json:"file_inventory"`
    Symbols       []SymbolInfo   `json:"symbols"`
    TestFiles     []TestFileInfo `json:"test_files"`
    ReusePoints   []ReusePoint   `json:"reuse_points"`
}

type FileInfo struct {
    Path      string `json:"path"`
    Exists    bool   `json:"exists"`
    IsNew     bool   `json:"is_new"`
    LineCount int    `json:"line_count"`
    Language  string `json:"language"` // "go", "python", "typescript", etc.
}

type SymbolInfo struct {
    FilePath  string `json:"file_path"`
    Line      int    `json:"line"`
    Kind      string `json:"kind"`      // "func", "type", "interface", "method", "class", "trait"
    Signature string `json:"signature"` // e.g., "func (s *Store) Add(l *Learning) error"
}

type TestFileInfo struct {
    Path      string   `json:"path"`       // e.g., "internal/learning/store_test.go"
    TestNames []string `json:"test_names"` // e.g., ["TestStoreAdd", "TestStoreQuery"]
    Covers    string   `json:"covers"`     // the source file this test covers
}

type ReusePoint struct {
    FilePath  string `json:"file_path"`
    Line      int    `json:"line"`
    Symbol    string `json:"symbol"`
    Relevance string `json:"relevance"` // why this is relevant to the plan
}
```

#### Data flow

```
DraftExecutionPlan
  │
  ├─ Phase 1: runStructuralAnalysis(ctx)
  │    └─ Shell out to code_intel.py graph --root <workdir> --json
  │    └─ Returns *StructuralAnalysis (nil if unavailable)
  │
  ├─ Phase 2: exploreForPlan(ctx, artifact, structuralAnalysis)
  │    ├─ Build prompt: requirements + structural JSON (if available)
  │    ├─ Dispatch via agentcli.InvokeFunc (or daemon QueryFunc)
  │    └─ Parse JSON response into *ExplorationResult
  │
  ├─ Compile: compiler.CompileExecutionPlan(artifact, plan)
  │    └─ Enriched with ExplorationResult data
  │
  └─ Persist: write exploration result to run directory
```

**Prompt size management:** The structural analysis JSON for a typical project (50 files, 200 functions) is ~15KB — well within context limits. For larger projects (500+ files), the prompt builder truncates to the top 100 functions by semantic relevance to the requirements, with a note that the full analysis is available at a file path the agent can Read.

#### Implementation

```go
// runStructuralAnalysis shells out to code_intel.py for deterministic codebase analysis.
// Returns nil (not error) when the tool is unavailable — caller proceeds with LLM-only exploration.
func (e *Engine) runStructuralAnalysis(ctx context.Context) *StructuralAnalysis {
    codeIntelPath := resolveCodeIntel()
    if codeIntelPath == "" {
        return nil
    }

    cmd := exec.CommandContext(ctx, "python3", codeIntelPath, "graph",
        "--root", e.WorkDir, "--json")
    out, err := cmd.Output()
    if err != nil {
        fmt.Fprintf(os.Stderr, "  code_intel: %v (falling back to LLM-only exploration)\n", err)
        return nil
    }

    var analysis StructuralAnalysis
    if err := json.Unmarshal(out, &analysis); err != nil {
        fmt.Fprintf(os.Stderr, "  code_intel: parse error: %v\n", err)
        return nil
    }
    return &analysis
}

// exploreForPlan dispatches an LLM agent to map requirements to code using structural context.
func (e *Engine) exploreForPlan(ctx context.Context, artifact *state.RunArtifact, sa *StructuralAnalysis) (*ExplorationResult, error) {
    prompt := buildExplorationPrompt(artifact, sa) // includes structural JSON when sa != nil
    backend := agentcli.BackendFor(agentcli.BackendClaude, "")

    raw, err := agentcli.InvokeFunc(ctx, &backend, prompt, 180)
    if err != nil {
        return nil, fmt.Errorf("explore: %w", err)
    }

    var result ExplorationResult
    if err := json.Unmarshal([]byte(agentcli.ExtractJSONBlock(raw, 0)), &result); err != nil {
        return nil, fmt.Errorf("parse exploration result: %w", err)
    }
    return &result, nil
}
```

#### Integration

- `DraftExecutionPlan` calls `runStructuralAnalysis` then `exploreForPlan` before compilation
- Results populate `ExecutionSlice.FilesLikelyTouched` with verified paths
- Results populate `ExecutionSlice.ImplementationDetail` (§2.7) with symbol-level specs
- The exploration result is persisted to `<run-dir>/exploration.json` for council review

#### Future: native Go integration

The codereview skill is being migrated from Python to Go. Once complete:
- `code_intel.py` subprocess call becomes a native Go library import
- No Python dependency needed
- Tighter integration with the compiler — structural analysis feeds directly into task compilation

Additional optional accelerators:
- **Roam** (`roam-code`) — if `roam` is in PATH, use `roam search` / `roam context` / `roam impact` for pre-indexed structural analysis with 27-language support
- When neither `code_intel.py` nor Roam is available, pure LLM exploration remains the fallback

### 2.4 Conformance Checks Per Task

**Currently:** Tasks get `RequiredEvidence` (coarse: `test_pass`, `quality_gate_pass`, `file_exists`). No per-task `content_check` or `files_exist` validation blocks.

**Change:** Add a `ValidationChecks` field to `ExecutionSlice` and `Task` with typed assertions.

#### Types

```go
// ValidationCheck defines a mechanically verifiable assertion for a task.
type ValidationCheck struct {
    Type   string `json:"type"`   // "files_exist", "content_check", "tests", "command", "lint"
    // Type-specific fields:
    Paths   []string `json:"paths,omitempty"`   // files_exist: paths that must exist
    File    string   `json:"file,omitempty"`    // content_check: file to search
    Pattern string   `json:"pattern,omitempty"` // content_check: regex pattern
    Command string   `json:"command,omitempty"` // tests/command/lint: shell command to run
}
```

Examples:

```go
// files_exist: verify file was created
ValidationCheck{Type: "files_exist", Paths: []string{"internal/learning/index.go"}}

// content_check: verify function exists
ValidationCheck{Type: "content_check", File: "internal/learning/store.go", Pattern: "func.*AssembleContext"}

// tests: run specific test
ValidationCheck{Type: "tests", Command: "go test -run TestAssembleContext ./internal/learning/..."}
```

#### Derivation

The compiler derives validation checks from:

1. **Exploration results:** If a file is marked `IsNew`, add `files_exist` check
2. **Requirement text:** If requirement mentions "test", add `tests` check with inferred test command
3. **Symbol extraction:** If a new function is expected, add `content_check` with function signature pattern
4. **Quality gate:** Always include the project's quality gate command

#### Integration

- `ValidationChecks` field on `ExecutionSlice` and `Task`
- `ReviewExecutionPlan` validates that every slice has at least one check (warning if missing)
- The verifier can run these checks in addition to its existing pipeline
- `MarshalTicket` renders validation checks in a `## Validation` section

### 2.5 Council Review of Execution Plans

**Currently:** `ReviewExecutionPlan` runs deterministic structural checks only. No multi-model council review.

**Change:** Wire council review for execution plans, similar to `CouncilReviewTechnicalSpec`.

#### Review perspectives

| Persona | Focus | Backend |
|---|---|---|
| Scope Reviewer | Are slices well-scoped? File conflicts? Scope gaps? | claude (or codex if available) |
| Dependency Reviewer | Are dependencies correct? False deps? Missing deps? | claude (or codex if available) |
| Completeness Reviewer | Does every requirement have a covering slice? Acceptance criteria sufficient? | claude |
| Feasibility Reviewer | Are slices implementable? Too large? Missing context? | claude |

**Note:** Persona backends fall back to Claude via `BackendFor` when a backend is disabled in `fabrikk.yaml`. This is transparent — the review proceeds with the same personas regardless of backend availability.

#### Integration

- New engine method: `CouncilReviewExecutionPlan(ctx context.Context, cfg councilflow.CouncilConfig) (*councilflow.CouncilResult, error)`
- Called from `fabrikk plan review --council` (opt-in initially, default later)
- The council receives: execution plan markdown + exploration results + tech spec
- Council findings become `ReviewFinding` entries on the `ExecutionPlanReview`
- Blocking council findings prevent plan approval
- Custom personas can be injected via `--persona <path>` (same as tech-spec review)

#### MVP mode (default)

- 1 round, 4 reviewers, `--skip-approval` for personas
- Structural review runs first; if it fails, council is skipped
- Council runs only if structural review passes

### 2.6 Boundaries on RunArtifact

**Currently:** No boundary concept. No mechanism to inject cross-cutting constraints.

**Change:** Add `Boundaries` to `RunArtifact`.

```go
type Boundaries struct {
    Always   []string `json:"always,omitempty"`    // Non-negotiable constraints
    AskFirst []string `json:"ask_first,omitempty"` // Need human input
    Never    []string `json:"never,omitempty"`     // Explicit out-of-scope
}
```

- `Always` constraints are injected into every task's `Constraints` field during enrichment
- `AskFirst` items cause `DraftExecutionPlan` to return an `ErrHumanInputRequired` error with the pending items. The CLI prints them and exits — the user adds the answers to the spec, then re-runs the command. No interactive blocking inside the engine.
- `Never` items are validated during verification — if a worker touches out-of-scope paths, verification produces a warning finding
- Boundaries can be set during `fabrikk prepare` or parsed from a `## Boundaries` section in the tech spec

### 2.7 Implementation Detail on Execution Slices

**Currently:** Slices have `Title`, `Goal`, `Notes` (prose) but no structured implementation guidance.

**Change:** Add `ImplementationDetail` to `ExecutionSlice`.

```go
type ImplementationDetail struct {
    FilesToModify []FileChange  `json:"files_to_modify,omitempty"`
    SymbolsToAdd  []string      `json:"symbols_to_add,omitempty"`  // "func (s *Store) Maintain() error"
    SymbolsToUse  []string      `json:"symbols_to_use,omitempty"`  // "withLock at store.go:380"
    TestsToAdd    []string      `json:"tests_to_add,omitempty"`    // "TestMaintain_DedupMerge"
}

type FileChange struct {
    Path   string `json:"path"`
    Change string `json:"change"` // "Add Maintain method", "Modify GarbageCollect"
    IsNew  bool   `json:"is_new,omitempty"`
}
```

- Populated by the exploration step (§2.3)
- Rendered in the execution plan markdown for human review
- Rendered in ticket body for worker agents
- Council reviewers see this detail and can validate feasibility

## 3. Files to Modify

| File | Change | Est. lines |
|------|--------|-----------|
| `internal/compiler/compiler.go` | Change `maxGroupSize` to 1, add `ComputeWaves`, `detectFileConflicts`, `resolveConflicts`, false dep removal | +120 |
| `internal/compiler/compiler_test.go` | Tests for wave computation, conflict detection, single-requirement tasks | +150 |
| `internal/engine/explore.go` | **NEW** — Hybrid codebase exploration: `runStructuralAnalysis` (code_intel.py dispatch), `exploreForPlan` (LLM agent with structural context), `resolveCodeIntel`, `buildExplorationPrompt` | ~200 |
| `internal/engine/explore_test.go` | **NEW** — Exploration tests (stubbed agent responses, structural analysis parsing) | ~120 |
| `internal/state/types.go` | Add `StructuralAnalysis`, `StructuralNode`, `StructuralEdge`, `ExplorationResult`, `FileInfo`, `SymbolInfo`, `TestFileInfo`, `ReusePoint`, `FileConflict`, `ValidationCheck`, `Boundaries`, `ImplementationDetail`, `FileChange` types. Add `ValidationChecks` to `Task`. Add `Boundaries` to `RunArtifact`. | +80 |
| `internal/engine/plan.go` | Update `DraftExecutionPlan` to call exploration + wave computation. Add `CouncilReviewExecutionPlan`. Update `ReviewExecutionPlan` to validate waves and run optional council. | +150 |
| `internal/engine/plan_test.go` | Tests for exploration integration, council review wiring, wave validation | +100 |
| `internal/ticket/format.go` | Render `ValidationChecks` and `ImplementationDetail` in ticket body | +30 |
| `cmd/fabrikk/main.go` | Add `--council` flag to `plan review`, add `boundaries` command | +20 |

**Estimated total:** ~970 new/modified lines.

## 4. Implementation Order

### Wave 1: Foundation (no dependencies between these)

1. **Task granularity** — Change `maxGroupSize` to 1. Update tests. This is a one-line change with test adjustments.

2. **Validation checks type** — Add `ValidationCheck` to `state/types.go` and `ExecutionSlice`. Wire through `MarshalTicket`. No behavioral change yet — just the type and rendering.

3. **Boundaries type** — Add `Boundaries` to `RunArtifact` in `state/types.go`. No behavioral enforcement yet — just the type.

### Wave 2: Wave computation (depends on Wave 1)

4. **Wave computation** — Implement `ComputeWaves`, `detectFileConflicts`, `resolveConflicts` in `compiler.go`. Populate `ExecutionSlice.WaveID`. Update `DraftExecutionPlan` to call wave computation. Update `renderExecutionPlanMarkdown` to show wave groupings.

5. **False dependency removal** — Implement dependency necessity validation. Remove lane-based deps where no file overlap exists.

6. **Wave validation in review** — Update `ReviewExecutionPlan` to validate no intra-wave file conflicts. Add cross-wave shared file registry to review output.

### Wave 3: Exploration (depends on Wave 1)

7. **Structural analysis** — Implement `runStructuralAnalysis` and `resolveCodeIntel` in `engine/explore.go`. Shell out to `code_intel.py`, parse `StructuralAnalysis` JSON. Graceful fallback when unavailable.

8. **LLM exploration** — Implement `exploreForPlan` and `buildExplorationPrompt` in `engine/explore.go`. Build prompt with requirements + structural context. Dispatch via `agentcli.InvokeFunc`. Parse `ExplorationResult`.

9. **Implementation detail** — Add `ImplementationDetail` to `ExecutionSlice`. Populate from exploration results. Render in ticket body.

10. **Conformance check derivation** — Derive `ValidationChecks` from exploration results (files_exist for new files, content_check for new functions, tests for test files).

### Wave 4: Council review (depends on Waves 2-3)

11. **Council review wiring** — Implement `CouncilReviewExecutionPlan`. Wire personas (scope, dependency, completeness, feasibility). MVP mode: 1 round, 4 reviewers.

12. **CLI integration** — Add `--council` flag to `fabrikk plan review`. Default to MVP mode (1 round). Display findings.

### Wave 5: Boundaries (depends on Wave 1)

13. **Boundary enforcement** — `Always` constraints injected into task `Constraints`. `Never` items validated during verification. `AskFirst` returns `ErrHumanInputRequired` from `DraftExecutionPlan`.

14. **CLI command** — `fabrikk boundaries set --always "..." --never "..."` or parse from tech spec `## Boundaries` section.

## 5. Design Decisions

### Hybrid exploration: deterministic analysis + LLM interpretation

We use a two-phase approach for codebase exploration before planning:
1. **Phase 1:** Shell out to `code_intel.py` (tree-sitter AST + semantic embeddings) for deterministic structural analysis — fast, cached, language-agnostic across 8 languages
2. **Phase 2:** Dispatch an LLM agent with the structural data as context — the agent maps requirements to code by intent, not just by name

This is better than either approach alone. The LLM doesn't waste tokens rediscovering file structure. The deterministic analysis doesn't miss intent-level connections.

When `code_intel.py` is unavailable (no Python), Phase 2 runs standalone using tool-based exploration (Grep/Glob/Read). The codereview skill is being migrated to Go — once complete, the subprocess call becomes a native library import.

### Wave computation at planning time, not execution time

Currently, waves are implicit (computed at execution time by `GetPendingTasks`). We move wave computation to planning time so:
- The plan document shows explicit wave groupings for human review
- The council can validate wave assignments
- File conflicts are caught before any work begins
- The orchestrator knows the expected number of waves upfront

### Council review as opt-in initially

`fabrikk plan review` defaults to structural-only (current behavior). `fabrikk plan review --council` adds multi-model review. This lets us iterate on council personas and prompts without breaking the existing workflow. Once stabilized, council becomes default (like tech-spec review).

### MVP review mode: 1 round, skip persona approval

Following user preference: MVP mode with a single review round and auto-approved personas. Deeper reviews available via `--mode standard --round 2` or `--mode production --round 3`.

## 6. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Single-requirement tasks produce too many tasks | Task queue is large, overhead per task | Task count increases but each task is faster to complete. Net time is similar or better due to parallelism. |
| LLM exploration produces inconsistent results | Agent may miss symbols or hallucinate file paths | Response schema is strict; results are validated against filesystem before use. Exploration is advisory — the compiler still produces valid tasks without it. |
| `code_intel.py` not available on all systems | Phase 1 skipped, LLM explores without structural context | Graceful fallback: Phase 2 runs standalone. Slower but functional. |
| Wave computation is NP-hard in worst case | Conflict resolution takes too long | Practical task graphs are sparse DAGs. Greedy conflict resolution (move later task to next wave) is O(n²) worst case, fast in practice. |
| Council review adds latency to planning | Slower plan cycle | MVP mode (1 round) takes ~3-5 minutes. Structural review alone takes <1 second. User chooses when to invest in council review. |
| False dependency removal breaks execution ordering | Tasks execute in wrong order | Only remove deps where no file overlap exists. Conservative: if uncertain, keep the dependency. |
| Boundaries enforcement is too rigid | Blocks legitimate out-of-scope work | `Never` boundaries produce verification warnings, not hard failures. `AskFirst` returns an error — the user provides input and re-runs. |
| Structural analysis JSON too large for prompt | Context overflow on large projects (500+ files) | Truncate to top 100 functions by semantic relevance. Full analysis available at a file path the agent can Read. |

## 7. Verification

1. `go build ./...` — compiles
2. `go test -race ./internal/compiler/... ./internal/engine/... ./cmd/fabrikk/...`
3. `golangci-lint run`
4. Manual: `fabrikk plan draft <run-id>` with a spec containing 10+ requirements → verify single-requirement tasks
5. Manual: verify wave groupings in execution plan markdown
6. Manual: verify file-conflict matrix in review output
7. Manual: `fabrikk plan review --council <run-id>` → verify council findings
8. Manual: verify exploration results populate `ImplementationDetail` on slices
9. Manual: verify `ValidationChecks` render in ticket body
10. Manual: compile with exploration → verify `## Validation` section in tickets
11. Manual: run with `code_intel.py` unavailable → verify graceful fallback to LLM-only

## 8. Relationship to Memory System

This plan complements the agent memory system (docs/plans/done/agent-memory-system.md):

- **Phase 2 extraction:** Council review of execution plans will produce findings that feed into the learning store (same extraction logic as tech-spec council review)
- **Phase 1 enrichment:** Tasks compiled from the improved plan will be enriched with learnings from the store (existing behavior, unaffected)
- **Prevention checks:** Mature learnings compiled into prevention checks (Phase 3) will be loaded as context during council review of execution plans
- **Boundaries:** `Always` constraints from boundaries can reference learnings (e.g., "Use withLock for all state mutations — see lrn-abc123")

The planning improvements and memory system form a feedback loop: better plans → fewer implementation issues → fewer findings → better learnings → better future plans.
