# attest

A spec-driven autonomous run system for coding agents. Written in Go — stdlib only, single binary.

## The Problem

Coding agents are powerful, but they struggle to deliver complete, spec-conformant implementations without constant human oversight. The failure mode isn't lack of capability — it's lack of structure:

- **Drift.** Agents wander from the original spec, building things that weren't asked for while leaving actual requirements unfinished.
- **False completion.** Agents claim work is done when it isn't. Tests are missing, edge cases are ignored, requirements are quietly dropped.
- **Lost focus.** In skill-based or prompt-driven workflows, the LLM has too much latitude. It decides what to do next, which stages to skip, and when "good enough" is good enough. The result is inconsistent quality and incomplete implementations.

These problems get worse with parallelism. Multiple agents working concurrently multiply the opportunities for drift and conflicting changes.

## How attest Solves This

attest uses a **rigid skeleton with convergence-driven loops**.

The **skeleton** defines fixed stages that are coded in Go, not left to LLM judgment:

```
Spec Review → Work Plan → Implementation → Code Review
```

No agent gets to skip a stage, invent a new one, or decide the workflow. The stage boundaries and exit criteria are enforced by the orchestrator.

The **loops within each stage** are autonomous and self-correcting. They don't run a fixed number of times — they run until their exit criteria are met:

- Implementation nudge reveals gaps? Keep going.
- Code review finds critical findings? Keep going.
- Same issues reappearing without progress? Escalate to a stronger reasoning path.

The spec is the anchor. Every loop iteration checks against the *same* approved spec, not whatever the agent has drifted toward. Work is only complete when it traces back to approved requirements, passes verification, and clears independent review.

Agents are autonomous *within* their stage, but they don't control the process. The skeleton contains the autonomy.

## Document Set

- `docs/specs/attest-functional-spec.md`
- `docs/specs/attest-technical-spec.md`
- `docs/plans/2026-03-12-attest-v1-implementation-plan.md`
