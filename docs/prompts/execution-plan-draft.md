# Execution Plan Draft Prompt

Produce a deterministic execution plan from the approved technical spec and run artifact.

Rules:
- preserve requirement IDs exactly
- create small, independently testable slices
- make dependencies explicit
- prefer narrow owned paths
- include concrete acceptance checks
- emit uncertainty in notes instead of silently guessing
