# ROADMAP.md — the source of all work

**This file is not optional.** Every feature the agent builds flows down
from here. If it isn't on this roadmap, it doesn't get built; if it needs
building, it gets added here first. One item ships per weekly cycle
(see `WEEKLY.md`).

## North star

<!-- One paragraph: what this project is becoming. The queue below must
     visibly serve this. -->

{{PROJECT_NAME}} is …

## Feature Queue — ordered; top unblocked item ships next

<!-- RULES:
     · Always ≥3 ready items. Refilling the queue is part of every weekly
       cycle (WEEKLY.md step 7) — a starving queue is a failed cycle.
     · Order is priority. The agent takes the TOP unblocked item and may
       not reorder without recording why (below, under "Queue changes").
     · Every item carries a completion promise: ONE testable sentence
       that is unambiguously true or false. No promise, not ready.
     · "Evidence" names how the promise will be proven: which gate,
       screenshot, benchmark, or user-visible behavior. -->

### 1. <feature name>
- **Promise:** <one sentence that is provably true when done>
- **Evidence:** <the gate/artifact that proves it>
- **Use case:** <the docs/USE-CASES.md case this serves — required; no case, add one or don't build it>
- **Scope guard:** <what this item explicitly does NOT include>
- **Status:** ready | blocked on <what> | in progress (week of <date>)

### 2. <feature name>
- **Promise:** …
- **Evidence:** …
- **Scope guard:** …
- **Status:** ready

### 3. <feature name>
- **Promise:** …
- **Evidence:** …
- **Scope guard:** …
- **Status:** ready

## Later — candidates, not yet specced

- <idea> — <one line why it might matter>

## Shipped

<!-- Move queue items here when done, newest first, with the release tag
     and the evidence link. This is the project's real history of intent. -->

| Week | Feature | Release | Evidence |
|---|---|---|---|

## Explicitly not doing

- <declined idea> — <one line why; saves re-litigating it>

## Queue changes

<!-- Any reorder, insertion above position 3, or item removal gets one
     line here: date, what changed, why. -->
