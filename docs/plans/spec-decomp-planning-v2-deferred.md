# Spec Decomposition — Deferred Improvements (Post-MVP)

These findings were raised during the council review of the spec-decomp plan (run-1775278624, 2026-04-04) and intentionally deferred from MVP. Each is a valid improvement that should be revisited after the MVP ships and real usage patterns emerge.

---

## UX Improvements

### 1. Exploration status in plan markdown (ux-002)
**What:** When exploration is unavailable or fails, the plan markdown contains no indication. A user reviewing the plan days later can't tell whether exploration ran.
**Suggested fix:** Add an `ExplorationStatus` field ("full", "structural-only", "llm-only", "unavailable") rendered in the plan header.
**Why deferred:** During the run, stderr shows the status. Adding the enum + plumbing through rendering is scope creep for MVP.

### 2. LLM exploration caching (ux-003)
**What:** Phase 2 (LLM exploration) has no caching. Every re-run of `plan draft` re-invokes the agent (~3 min, token cost).
**Suggested fix:** Cache ExplorationResult by artifact hash. Invalidate on spec change.
**Why deferred:** MVP users run `plan draft` a handful of times. Cache invalidation logic adds complexity that should be designed after real usage.

### 3. Plan summary header (ux-005)
**What:** With maxGroupSize=1, plan markdown produces 20+ slice headings with no summary. User can't quickly see "5 waves, 22 tasks."
**Suggested fix:** Add a summary block at the top of the rendered plan markdown.
**Why deferred:** Wave groupings already provide structure. A summary is formatting convenience with no type/API changes.

## Architecture Refinements

### 4. Move structural analysis types to engine package (arch-003)
**What:** StructuralAnalysis, StructuralNode, StructuralEdge are ephemeral analysis results produced/consumed only in engine/explore.go. Placing them in state/types.go creates false coupling.
**Suggested fix:** Move to engine-local types or a dedicated exploration package.
**Why deferred:** The types compile and function correctly regardless of package. Import-graph cleanliness is a quality concern, not a functional blocker.

### 5. Parallelize implementation waves 2 and 3 (arch-005)
**What:** Waves 2 (wave computation) and 3 (exploration) touch disjoint packages and could run in parallel, reducing critical path from 5 to 4 waves.
**Suggested fix:** Merge into a single wave or mark as parallelizable in the spec.
**Why deferred:** Serial execution produces identical results. Implementers can parallelize at their discretion.

## Security Hardening

### 6. Script trust boundary for code_intel.py (sec-002)
**What:** The spec permits execution from env var, PATH, or heuristic locations without defining trust levels. A poisoned PATH gives arbitrary code execution.
**Suggested fix:** Path canonicalization, ownership checks, or hash verification.
**Why deferred:** Requires local machine compromise as prerequisite — attacker doesn't need fabrikk to execute code. Same trust model as git, docker, kubectl. Appropriate for multi-user/CI phases.

### 7. Never boundaries as blocking failures (sec-003)
**What:** Never violations only produce warnings. Workers can touch out-of-scope paths unless a human notices.
**Suggested fix:** Make Never a blocking failure in verification.
**Why deferred:** Intentional MVP design — risk of false positives blocking legitimate work. Blocking enforcement appropriate when boundaries are well-calibrated in future phases.

## Algorithm Polish

### 8. Termination proof and max-wave limit (graph-005)
**What:** The spec doesn't prove the conflict resolution loop terminates or specify a max-wave limit.
**Suggested fix:** Add a formal proof sketch and defensive `if iterations > len(tasks) { return error }` guard.
**Why deferred:** The iterate-until-stable loop (applied in graph-001) is self-evidently terminating — each iteration moves at least one task to a later wave, bounded by N tasks. Implementers can add a guard trivially.

### 9. Conflict resolution tiebreaker rationale (graph-006)
**What:** Order-based tiebreaking is arbitrary. Fan-out-based tiebreaking could reduce wave depth.
**Suggested fix:** Document rationale or implement fan-out heuristic.
**Why deferred:** Not a correctness issue — produces valid waves either way. Order-based is deterministic and sufficient for MVP.

## Prompt Engineering

### 10. buildExplorationPrompt template in spec (LLM-005)
**What:** The spec references buildExplorationPrompt but provides zero detail on contents — no output instructions, no JSON schema in prompt, no examples.
**Suggested fix:** Include the prompt template verbatim in the spec.
**Why deferred:** The ExplorationResult Go types with json tags define the output contract. Robust JSON extraction (ExtractFromCodeFence + first-brace fallback) reduces dependence on prompt precision. Prompt text will iterate rapidly during development — spec-level templates become stale.

## Testing

### 11. Verification matrix (test-005)
**What:** The verification plan doesn't define which behaviors are unit-tested vs integration-tested vs manual.
**Suggested fix:** Add a formal verification matrix categorizing each check.
**Why deferred:** Items 1-3 are clearly automated (build, test, lint), items 4-11 are labeled "Manual:". Fixture-backed testing requirements (from test-002) establish the boundary. A matrix would recategorize the same information.
