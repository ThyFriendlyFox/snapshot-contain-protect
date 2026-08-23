# VERIFICATION.md — one command answers "is this repo healthy"

`{{VERIFY_CMD}}` runs, in order:

1. Lint / format check — `{{LINT_CMD}}`
2. Build — `{{BUILD_CMD}}`
3. Tests — `{{TEST_CMD}}`
4. <repo-specific gates — one script per gate, each independently runnable>

## Rules

- CI runs **the same command** as local. No CI-only logic.
- A gate that can't run in some environment **skips loudly**, never
  passes silently.
- New behavior lands with its gate in the same PR whenever feasible.
- A feature's completion promise (ROADMAP.md) should be backed by a gate
  here whenever it can be — evidence that keeps proving itself beats
  evidence produced once.
- Fixing a flaky or broken gate is always in scope, for any task.
