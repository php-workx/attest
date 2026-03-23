# Plan: Adopt Claude Octopus Patterns into attest

**Status:** Draft
**Date:** 2026-03-22
**Depends on:** Agent memory simplification (2026-03-22-learning-simplification.md)
**Branch:** TBD (create from `main` after memory simplification merges)

---

## 1. Problem Statement

attest's council pipeline uses 3 fixed backends (claude, codex, gemini) with hardcoded personas, but lacks:

1. **Provider reliability** — No circuit breaker, no backoff, no fallback. If Codex is down, the council stalls or fails.
2. **Blind spot coverage** — Persona perspectives are static. Known LLM weaknesses (e.g., supply-chain attacks, distributed consensus failures) are not systematically forced into reviews.
3. **Cost visibility** — Users have no idea what a council run costs. No per-phase or per-provider cost breakdown.
4. **Smart model routing** — `Task.DefaultModel` exists on the struct but is never read. `RoutingPolicy` has `DefaultImplementer`/`DefaultReviewer` but they're hardcoded to `"claude-sonnet"`.
5. **Provider health monitoring** — No tracking of which providers are healthy, slow, or rate-limited across runs.

These gaps were identified by comparing attest against Claude Octopus (see `docs/analysis/claude-octopus-review.md`), which solves all five at the Bash/CLI layer. This plan codifies the best Octopus patterns in Go, integrated into attest's existing architecture.

## 2. Design Principles

- **No new dependencies.** All new code uses Go stdlib only (consistent with attest's 4-dep philosophy).
- **File-based state.** Provider health, cost logs, and blind spot manifests are JSON/JSONL files in `.attest/` — no databases, no external services.
- **Deterministic where possible.** Circuit breaker thresholds, cost estimates, and health scoring are pure functions. Only the blind spot keyword matching uses string heuristics.
- **Additive to existing types.** Extend `Persona`, `RoutingPolicy`, `RunStatus` — don't replace them. All changes must be backward-compatible with existing run artifacts.
- **Fail-open by default.** If provider health data is missing, assume healthy. If cost data is missing, skip cost display. If blind spot manifest is absent, use empty set. No new failure modes.

## 3. Scope

### In scope (this plan)

1. Provider circuit breaker with health tracking
2. Blind spot injection into council personas
3. Per-run cost tracking and display
4. Model routing from task metadata
5. Provider health dashboard (`attest providers` command)

### Out of scope (future work)

- Multi-vendor model diversity (adding new backends beyond claude/codex/gemini)
- Cost-based routing (choosing cheapest healthy provider)
- Smart task-type classification (auto-detecting research vs coding vs review)
- Provider selection modes (round-robin, fastest, cheapest, scored)
- Cost budget gates (block execution if estimated cost exceeds threshold)

---

## 4. Issue 1: Provider Circuit Breaker

### What

Track provider health across runs. After N consecutive transient failures, open the circuit (skip that provider). After a cooldown, probe with one request before fully reopening.

### Why

Octopus pattern: `provider-router.sh` classifies errors as transient/permanent, tracks consecutive failures per provider, and opens a circuit breaker after 3 failures with a 300s cooldown. attest has zero provider reliability — a flaky Codex endpoint causes the entire council to fail with an opaque subprocess error.

### Where

New package: `internal/provider/`

```
internal/provider/
    health.go      # ProviderHealth type, circuit breaker logic
    health_test.go
    classify.go    # Error classification (transient vs permanent)
    classify_test.go
```

### Types

```go
package provider

// Name identifies a council backend.
type Name string

const (
    Claude  Name = "claude"
    Codex   Name = "codex"
    Gemini  Name = "gemini"
)

// State is the circuit breaker state.
type State string

const (
    StateClosed   State = "closed"    // healthy, accepting requests
    StateOpen     State = "open"      // tripped, rejecting requests
    StateHalfOpen State = "half_open" // cooldown expired, probing
)

// Health tracks per-provider reliability.
type Health struct {
    Provider          Name      `json:"provider"`
    State             State     `json:"state"`
    ConsecutiveFails  int       `json:"consecutive_fails"`
    LastFailure       time.Time `json:"last_failure,omitempty"`
    LastSuccess       time.Time `json:"last_success,omitempty"`
    CooldownUntil     time.Time `json:"cooldown_until,omitempty"`
    TotalCalls        int       `json:"total_calls"`
    TotalFailures     int       `json:"total_failures"`
    AvgLatencyMs      int64     `json:"avg_latency_ms"`
}

// HealthStore persists provider health to disk.
type HealthStore struct {
    Path string // .attest/provider-health.json
}

// Config holds circuit breaker thresholds.
type Config struct {
    FailureThreshold int           // consecutive transient failures to trip (default 3)
    CooldownDuration time.Duration // how long to wait before half-open probe (default 5m)
}
```

### Functions

```go
// RecordSuccess resets consecutive failures, closes circuit, updates latency.
func (s *HealthStore) RecordSuccess(name Name, latencyMs int64) error

// RecordFailure classifies error and updates state.
// Transient errors increment ConsecutiveFails; permanent errors are logged but don't trip.
// Returns the updated Health for the caller to check state.
func (s *HealthStore) RecordFailure(name Name, err error) (Health, error)

// IsAvailable returns true if provider is closed or half-open (eligible for probe).
func (s *HealthStore) IsAvailable(name Name) bool

// AvailableFrom filters a list of provider names to those not in open state.
func (s *HealthStore) AvailableFrom(names []Name) []Name

// LoadAll returns health for all known providers.
func (s *HealthStore) LoadAll() ([]Health, error)

// ClassifyError determines if an error is transient (retryable) or permanent.
// Transient: timeout, rate limit (429), server error (5xx), network errors.
// Permanent: auth failure (401/403), bad request (400/404), billing errors.
func ClassifyError(err error) ErrorClass
```

### Integration with councilflow

Modify `runner.go` `runSingleReview()`:

```go
// Before invoking backend:
if !healthStore.IsAvailable(provider.Name(persona.Backend)) {
    return ReviewOutput{}, fmt.Errorf("provider %s circuit open: %w", persona.Backend, provider.ErrCircuitOpen)
}

// After successful invocation:
healthStore.RecordSuccess(provider.Name(persona.Backend), elapsed.Milliseconds())

// After failed invocation:
health, _ := healthStore.RecordFailure(provider.Name(persona.Backend), err)
if health.State == provider.StateOpen {
    log.Printf("circuit breaker tripped for %s after %d consecutive failures", persona.Backend, health.ConsecutiveFails)
}
```

Modify `RunRound()` to skip personas whose backend is unavailable rather than failing the entire round. A round can proceed with reduced perspectives (log a warning). Only fail the round if ALL backends are unavailable.

### File location

`.attest/provider-health.json` — persists across runs, shared by all runs in the workspace.

### Acceptance criteria

- `go test ./internal/provider/...` passes
- 3 consecutive timeout errors → circuit opens → `IsAvailable` returns false
- After cooldown → `IsAvailable` returns true (half-open)
- 1 success in half-open → circuit closes
- Permanent errors (401) logged but don't trip circuit
- Council round with 1 unavailable provider proceeds with remaining 2+

---

## 5. Issue 2: Blind Spot Injection

### What

Domain-specific JSON manifests that inject additional review perspectives based on keyword matching against the spec content. Forces coverage of known LLM weaknesses that static personas miss.

### Why

Octopus pattern: `config/blind-spots/` contains 5 domain JSON files (security, architecture, data, api, operations) with trigger keywords and injection prompts. When a spec mentions "authentication," the security blind spots (supply-chain attacks, token rotation, session fixation) are injected into the edge-case and synthesis perspectives.

attest's fixed personas cover security/performance/testability/architecture — but they use the same instructions regardless of spec content. A spec about database migrations gets the same security review as one about API authentication.

### Where

```
internal/councilflow/
    blindspot.go       # Manifest loading, keyword matching, perspective injection
    blindspot_test.go
config/
    blind-spots/
        manifest.json  # Master index with trigger keywords per domain
        security.json
        architecture.json
        data.json
        api.json
        operations.json
        concurrency.json  # Go-specific: goroutines, channels, mutexes
```

### Types

```go
package councilflow

// BlindSpot is a single known LLM weakness with its injection prompt.
type BlindSpot struct {
    ID              string   `json:"id"`
    Topic           string   `json:"topic"`
    TriggerKeywords []string `json:"trigger_keywords"` // regex patterns
    InjectionPrompt string   `json:"injection_prompt"` // appended to persona instructions
}

// BlindSpotDomain groups blind spots by category.
type BlindSpotDomain struct {
    Domain    string      `json:"domain"`
    BlindSpots []BlindSpot `json:"blind_spots"`
}

// BlindSpotManifest is the master index.
type BlindSpotManifest struct {
    Domains []BlindSpotDomain `json:"domains"`
}
```

### Functions

```go
// LoadManifest reads the blind spot manifest from configDir.
// Returns empty manifest (not error) if files don't exist.
func LoadManifest(configDir string) (BlindSpotManifest, error)

// MatchBlindSpots scans specContent for trigger keywords and returns
// matching blind spots across all domains.
func (m BlindSpotManifest) MatchBlindSpots(specContent string) []BlindSpot

// InjectBlindSpots appends matched blind spot prompts to persona instructions.
// Only injects into personas whose perspective aligns with the blind spot domain.
// Returns modified persona slice (does not mutate input).
func InjectBlindSpots(personas []Persona, blindSpots []BlindSpot) []Persona
```

### Integration with councilflow

Modify `council.go` `buildPersonaSet()`:

```go
func (c *Council) buildPersonaSet(round int) []Persona {
    personas := FixedPersonas()
    if !c.Config.SkipDynPersonas {
        dynPersonas, _ := GeneratePersonas(...)
        personas = append(personas, dynPersonas...)
    }

    // NEW: Inject blind spots based on spec content
    if manifest, err := LoadManifest(c.configDir); err == nil {
        matched := manifest.MatchBlindSpots(c.specContent)
        if len(matched) > 0 {
            personas = InjectBlindSpots(personas, matched)
            log.Printf("injected %d blind spot perspectives from %d matches", len(matched), len(matched))
        }
    }

    return personas
}
```

### Keyword matching rules

- Match is case-insensitive
- `trigger_keywords` are compiled as `regexp.Regexp` patterns (not plain strings)
- A domain matches if ANY of its trigger keywords match the spec
- Multiple domains can match simultaneously
- Injection is additive — blind spot prompts are appended to existing persona instructions, not replacing them

### Persona-domain alignment

| Blind spot domain | Injected into persona |
|-------------------|-----------------------|
| security | Security Engineer |
| architecture | Architecture Reviewer |
| data | Architecture Reviewer + Performance Engineer |
| api | Security Engineer + Architecture Reviewer |
| operations | Performance Engineer |
| concurrency | Performance Engineer + Testability Reviewer |

### Example manifest entry

```json
{
  "domain": "security",
  "blind_spots": [
    {
      "id": "sec-supply-chain",
      "topic": "Supply-chain attacks via compromised dependencies",
      "trigger_keywords": ["dependency", "import", "require", "go\\.mod", "vendor"],
      "injection_prompt": "Additionally review for supply-chain attack vectors: compromised transitive dependencies, typosquatting on package names, malicious post-install hooks, and dependency confusion between internal and public registries."
    },
    {
      "id": "sec-token-lifecycle",
      "topic": "Token lifecycle gaps",
      "trigger_keywords": ["token", "jwt", "session", "auth", "claim", "lease"],
      "injection_prompt": "Additionally review token lifecycle: rotation policy, revocation propagation delay, replay window after expiry, and handling of tokens issued before a key rotation."
    }
  ]
}
```

### Acceptance criteria

- `go test ./internal/councilflow/...` passes with blind spot tests
- Spec containing "authentication" triggers security blind spots
- Spec containing "goroutine" triggers concurrency blind spots
- Spec with no keyword matches produces no injection (no-op)
- Missing manifest files produce empty manifest (no error)
- Injected prompts appear in review prompt output (verifiable via review artifact files)

---

## 6. Issue 3: Per-Run Cost Tracking

### What

Track estimated cost per provider invocation. Display per-phase and per-provider breakdowns after council runs. Persist to run directory for historical analysis.

### Why

Octopus pattern: `metrics-tracker.sh` (472 lines) tracks tokens, duration, and estimated cost per agent call. Displays phase and provider breakdowns with native-vs-estimated indicators. Users see cost before and after each workflow phase.

attest has zero cost visibility. A council run invokes 4-7 reviewers across 3 providers plus a judge pass — users have no idea what this costs.

### Where

```
internal/provider/
    cost.go       # Cost estimation, tracking, display
    cost_test.go
```

### Types

```go
package provider

// InvocationRecord captures one backend call for cost tracking.
type InvocationRecord struct {
    Provider    Name      `json:"provider"`
    Model       string    `json:"model"`
    Phase       string    `json:"phase"`       // "review", "judge", "persona-gen", "nudge"
    PersonaID   string    `json:"persona_id,omitempty"`
    StartedAt   time.Time `json:"started_at"`
    DurationMs  int64     `json:"duration_ms"`
    InputChars  int       `json:"input_chars"`  // prompt length
    OutputChars int       `json:"output_chars"` // response length
    EstTokensIn int       `json:"est_tokens_in"`  // input_chars / 4
    EstTokensOut int      `json:"est_tokens_out"` // output_chars / 4
    EstCostUSD  float64   `json:"est_cost_usd"`
    Success     bool      `json:"success"`
}

// CostSummary aggregates cost across a run.
type CostSummary struct {
    TotalEstUSD     float64            `json:"total_est_usd"`
    ByProvider      map[Name]float64   `json:"by_provider"`
    ByPhase         map[string]float64 `json:"by_phase"`
    InvocationCount int                `json:"invocation_count"`
    TotalDurationMs int64              `json:"total_duration_ms"`
}

// CostTracker appends invocation records to a JSONL file.
type CostTracker struct {
    Path string // .attest/runs/<run-id>/cost-log.jsonl
}

// ModelPricing holds per-1K-token cost estimates.
type ModelPricing struct {
    InputPer1K  float64
    OutputPer1K float64
}
```

### Pricing table

```go
var KnownPricing = map[string]ModelPricing{
    // Claude
    "opus":           {InputPer1K: 0.015, OutputPer1K: 0.075},
    "sonnet":         {InputPer1K: 0.003, OutputPer1K: 0.015},
    "haiku":          {InputPer1K: 0.001, OutputPer1K: 0.005},
    // OpenAI
    "gpt-5.4":        {InputPer1K: 0.0025, OutputPer1K: 0.015},
    "gpt-5.3-codex":  {InputPer1K: 0.00175, OutputPer1K: 0.014},
    // Google
    "gemini-3-pro-preview":  {InputPer1K: 0.00125, OutputPer1K: 0.005},
    "gemini-3-flash-preview": {InputPer1K: 0.000075, OutputPer1K: 0.0003},
}
```

Pricing is best-effort estimates. Unknown models use a conservative fallback ($0.01/$0.03 per 1K).

### Functions

```go
// Record appends an invocation record to the cost log.
func (t *CostTracker) Record(rec InvocationRecord) error

// Summarize reads the cost log and returns aggregated summary.
func (t *CostTracker) Summarize() (CostSummary, error)

// EstimateCost computes estimated USD from char counts and model.
func EstimateCost(model string, inputChars, outputChars int) float64
```

### Integration with councilflow

Modify `runner.go` `runSingleReview()` to wrap invocations:

```go
start := time.Now()
raw, err := r.InvokeFunc(ctx, backend, prompt)
elapsed := time.Since(start)

if r.CostTracker != nil {
    r.CostTracker.Record(provider.InvocationRecord{
        Provider:    provider.Name(persona.Backend),
        Model:       backend.Model(),
        Phase:       "review",
        PersonaID:   persona.PersonaID,
        StartedAt:   start,
        DurationMs:  elapsed.Milliseconds(),
        InputChars:  len(prompt),
        OutputChars: len(raw),
        EstTokensIn: len(prompt) / 4,
        EstTokensOut: len(raw) / 4,
        EstCostUSD:  provider.EstimateCost(backend.Model(), len(prompt), len(raw)),
        Success:     err == nil,
    })
}
```

Similarly for judge invocations, nudge passes, and persona generation.

### Display

After `RunCouncil()` completes, print summary:

```
Cost estimate:
  claude (opus)   3 calls  ~$0.42  (judge + architecture-reviewer + persona-gen)
  codex (gpt-5.4) 2 calls  ~$0.08  (security-engineer + testability-reviewer)
  gemini (3-pro)   1 call   ~$0.03  (performance-engineer)
  ─────────────────────────────────
  Total            6 calls  ~$0.53  (2m 14s)
```

### File location

`.attest/runs/<run-id>/cost-log.jsonl` — per-run, append-only.

### Acceptance criteria

- `go test ./internal/provider/...` passes
- Council run produces `cost-log.jsonl` with one entry per invocation
- `Summarize()` correctly aggregates by provider and phase
- Unknown models use fallback pricing (no panic)
- Missing cost tracker (nil) is no-op (no cost tracking, no errors)

---

## 7. Issue 4: Model Routing from Task Metadata

### What

Honor `Task.DefaultModel` and `RoutingPolicy` when dispatching council reviews and future task implementation. Route high-risk tasks to stronger models.

### Why

Octopus pattern: `auto-route.sh` classifies tasks by type and complexity, then routes to the optimal provider/model combination. `dispatch.sh` resolves models from a configurable precedence chain. attest has the fields (`DefaultModel`, `RoutingPolicy.DefaultImplementer`, `RoutingPolicy.DefaultReviewer`) but never reads them.

### Where

Modify existing files — no new package needed.

```
internal/provider/
    routing.go      # Model resolution logic
    routing_test.go
internal/councilflow/
    personas.go     # Read RoutingPolicy for backend assignment
internal/engine/
    engine.go       # Read DefaultModel for task dispatch
```

### Types

Extend existing `RoutingPolicy` in `state/types.go`:

```go
// RoutingPolicy controls model selection. Currently in types.go lines 130-133.
type RoutingPolicy struct {
    DefaultImplementer string            `json:"default_implementer,omitempty"`
    DefaultReviewer    string            `json:"default_reviewer,omitempty"`
    RiskOverrides      map[string]string `json:"risk_overrides,omitempty"` // NEW: risk_level → model
}
```

### Functions

```go
package provider

// ResolveModel determines the model for a given context.
// Precedence: task.DefaultModel > riskOverride > routingPolicy default > hardcoded fallback.
func ResolveModel(task state.Task, policy state.RoutingPolicy, phase string) string

// ResolveReviewerBackend determines the backend for a council persona.
// Precedence: persona.Backend (if set) > routingPolicy.DefaultReviewer > "claude".
func ResolveReviewerBackend(persona councilflow.Persona, policy state.RoutingPolicy) string
```

### Risk-based routing

```go
var DefaultRiskOverrides = map[string]string{
    "critical": "opus",    // highest-risk tasks get strongest model
    "high":     "opus",
    "medium":   "sonnet",  // default
    "low":      "sonnet",
}
```

### Integration

In `engine.go` `enrichTaskWithLearnings()` (or a new `resolveTaskModel()` step), set `Task.DefaultModel` based on risk level if not already set:

```go
if task.DefaultModel == "" {
    task.DefaultModel = provider.ResolveModel(task, artifact.RoutingPolicy, "implement")
}
```

In `councilflow/runner.go` `BackendFor()`, consult the routing policy:

```go
func BackendFor(persona Persona, policy state.RoutingPolicy) CLIBackend {
    backendName := provider.ResolveReviewerBackend(persona, policy)
    // ... existing lookup in KnownBackends
}
```

### Acceptance criteria

- `go test ./internal/provider/...` passes
- Critical-risk task resolves to "opus" model
- Task with explicit `DefaultModel` overrides risk-based routing
- `RoutingPolicy.RiskOverrides` overrides defaults
- Empty/missing routing policy falls back to current hardcoded behavior
- Backward-compatible: existing run artifacts without `RiskOverrides` work unchanged

---

## 8. Issue 5: Provider Health Dashboard

### What

New CLI command `attest providers` that displays provider health, circuit breaker state, cost history, and availability.

### Why

Octopus pattern: `/octo:doctor` displays per-provider state (closed/OPEN/half-open), cooldown remaining, failure counts, and installed CLI versions. attest has no visibility into provider health.

### Where

```
cmd/attest/
    providers.go    # CLI command implementation
```

### Output format

```
Provider Health
───────────────────────────────────────────────────
  claude   closed    ✓  avg 8.2s   42 calls   0 failures
  codex    closed    ✓  avg 12.1s  38 calls   2 failures
  gemini   OPEN      ✗  cooldown 2m31s remaining (3 consecutive timeouts)
───────────────────────────────────────────────────

Recent Cost (last 5 runs)
───────────────────────────────────────────────────
  run-a1b2  2026-03-22  $0.53  (6 calls, 2m14s)
  run-c3d4  2026-03-21  $0.41  (5 calls, 1m52s)
  run-e5f6  2026-03-21  $0.62  (8 calls, 3m01s)
───────────────────────────────────────────────────
```

### Flags

- `--json` — machine-readable output
- `--reset <provider>` — manually reset circuit breaker for a provider
- `--reset-all` — reset all circuit breakers

### Acceptance criteria

- `attest providers` displays health for all 3 providers
- `attest providers --json` outputs valid JSON
- `attest providers --reset codex` clears codex circuit breaker
- Works with empty/missing health file (shows defaults)

---

## 9. Dependency Order

```
Issue 1 (Circuit Breaker)     ← foundation, no deps
Issue 3 (Cost Tracking)       ← foundation, no deps
Issue 4 (Model Routing)       ← depends on Issue 1 (needs provider.Name type)
Issue 2 (Blind Spots)         ← independent of Issues 1/3/4
Issue 5 (Provider Dashboard)  ← depends on Issues 1 + 3

Wave 1: Issues 1, 2, 3 (parallel — independent foundations)
Wave 2: Issues 4, 5 (depend on Wave 1)
```

---

## 10. Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Cost estimates diverge from actual billing | High | Low | Label all costs as "estimated." Add note: "Check provider dashboards for actual usage." |
| Blind spot keyword matching produces false positives | Medium | Low | Keep trigger patterns specific. Log matches so users can audit. No blind spots cause review failures — they only add perspective. |
| Circuit breaker too aggressive (trips on transient network blip) | Medium | Medium | Default threshold of 3 consecutive failures is conservative. Half-open probe allows recovery. Manual reset available via `attest providers --reset`. |
| `RoutingPolicy.RiskOverrides` adds complexity to run artifacts | Low | Low | Field is `omitempty` — absent by default. Existing artifacts unchanged. |
| Provider health file corrupted by concurrent runs | Low | Medium | Use `state.WriteAtomic()` for all writes. Health file is small (<1KB) — atomic rename is safe. |

---

## 11. What This Plan Does NOT Do

This plan adopts 5 specific patterns from Octopus. It explicitly does NOT adopt:

- **Octopus's 4,900-line Bash orchestrator** — attest's Go engine is architecturally sounder
- **80 SUPPORTS_* feature flags** — attest doesn't need Claude Code version detection
- **Session-scoped state** — attest already has persistent run directories
- **Heredoc JSON generation** — attest uses `encoding/json` (type-safe)
- **Silent-fail module sourcing** — attest uses Go's compile-time import checking
- **Double Diamond methodology** — attest has its own pipeline (spec review → plan → compile → implement → verify → council)
- **Dark Factory autonomous mode** — future work, requires the engine to drive implementation (not yet built)

The plan takes Octopus's best infrastructure patterns (reliability, observability, adaptability) and implements them as Go packages that integrate cleanly with attest's existing architecture.
