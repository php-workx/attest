# Execution Plan Review Prompt

Review the execution plan for structural quality.

Check:
- every slice has requirement IDs
- every slice is independently testable
- dependencies are minimal and explicit
- owned paths are narrow enough for safe execution
- acceptance checks are concrete
- oversized or cross-cutting slices are called out
