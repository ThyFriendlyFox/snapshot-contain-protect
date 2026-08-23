# Architecture

<!-- The map a new contributor (or agent) reads before the first change.
     Tables over prose. Keep it current: WEEKLY.md step 5 includes this
     file when a feature moves a boundary. -->

{{PROJECT_NAME}} is <one sentence: kind of program, runtime>. The code has <N> parts.

| Part | Folder | Task |
|---|---|---|
| <part> | `src/…` | <one line> |

## Data flow

<!-- Number the steps of ONE representative end-to-end path, from user
     action to visible result. An agent should be able to place any file
     it opens onto one of these steps. -->

1. …
2. …

## <Part name>

| File | Task |
|---|---|
| `…` | <one line> |

## Boundaries

<!-- The interfaces between parts, and the rule at each: what may cross,
     what must not. These are AGENTS.md-invariant material — if a
     boundary is load-bearing, list it in AGENTS.md invariants too. -->

| Boundary | Rule |
|---|---|
| <part ↔ part> | <what crosses; what never does> |
