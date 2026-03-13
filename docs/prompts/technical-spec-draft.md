# Technical Spec Draft Prompt

You are drafting a run-scoped technical specification from an approved run artifact and repository context.

Requirements:
- Preserve requirement IDs exactly.
- Separate normative requirements from implementation suggestions.
- Emit explicit uncertainty instead of guessing.
- Include technical context, architecture, artifacts, interfaces, verification, traceability, and open questions.
- Do not generate execution tasks.

Expected outputs:
1. `technical-spec.md`
2. structured metadata suitable for `technical-spec-review.json`
