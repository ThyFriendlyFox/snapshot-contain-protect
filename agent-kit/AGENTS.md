# AGENTS.md — the binding contract

In effect whenever code in this repo is touched, by agent or human.
When this conflicts with intuition, this wins.

## Commands

```sh
{{BUILD_CMD}}      # build
{{TEST_CMD}}       # tests
{{LINT_CMD}}       # lint / format check
{{VERIFY_CMD}}     # full health gate — must pass before any push
```

Requires {{MIN_RUNTIME}}.

## Invariants — never regress these

<!-- SETUP fills this with the repo's 3–10 load-bearing decisions.
     Architecture, not style. If the repo already has an AGENTS.md,
     its invariants live here (or this section points there). -->

1. …

## Landmine map

<!-- Places where the obvious change is the wrong change. -->

| Area | Why it bites |
|---|---|
| `path/…` | … |

## House style

- Match the surrounding code's idiom, naming, and comment density.
- No demo scaffolding, no leftover diagnostics, no dead flags.
- Comments state constraints the code can't show — never narration.
- User-facing copy states the thing plainly; no reassurance microcopy.

## Process rules

- Branch from `{{DEFAULT_BRANCH}}`; never commit to it directly.
- `{{VERIFY_CMD}}` green before every push. Flaky gate → fix or
  quarantine in the same PR; never route around it.
- After adding/removing/renaming source files, run the stack's
  regeneration step (project gen, lockfile, tidy) and commit the result.
- Commit at boundaries; message says what changed and cites evidence.
- Docs move with behavior — same commit or PR.
- Report outcomes faithfully; failing is failing, with output.
