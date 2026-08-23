# LOOP.md — creating and running a ralph loop

The ralph loop is the engine of the build phase. One short prompt fires
on every iteration until the work is done. All intelligence lives in the
kit files, not the prompt. This pattern shipped a 141-iteration,
110-commit run once; keep its shape.

## The loop file

Create `.claude/ralph-loop.local.md`:

```markdown
---
active: true
iteration: 1
max_iterations: 0
completion_promise: "<the ROADMAP item's promise, verbatim, or null>"
started_at: "<ISO 8601 UTC now>"
---

Ship the top ready item in agent-kit/ROADMAP.md: <feature name>.
The loop ends when its completion promise is true with the named
evidence. Obey AGENTS.md; verify per VERIFICATION.md and commit at
each boundary.
```

Field rules:

- `iteration` — the loop runner rewrites this every tick. Start at 1.
- `max_iterations` — `0` means unbounded; trust the completion promise.
  Set a number only for a bounded experiment.
- `completion_promise` — one testable sentence. `null` is allowed only
  when a `max_iterations` bound is set; never both null and 0.
- The prompt body is 2–3 sentences, no more. If the loop needs more
  instruction, the missing text belongs in AGENTS.md, the ROADMAP
  item's spec, or a plan file — not in the prompt. A fat loop prompt
  is the first sign of a thin spec.

## Untracked, always

The file carries runtime state and dirties the tree on every tick.
It never enters history. Ensure `.gitignore` has:

```
.claude/ralph-loop.local.md
.claude/*.local.md
```

(Mouse learned this twice — the file got committed, removed, then
accidentally re-added. Gitignore it at creation time.)

## Running

Each iteration: read the prompt → work → `{{VERIFY_CMD}}` → commit at
any boundary reached → increment `iteration`. The loop runs with
`WEEKLY.md` step 3 as its outer frame and stops when:

1. The completion promise is true with the named evidence — normal end.
2. `max_iterations` is reached — report state honestly, per TONE.md.
3. A human interrupts — leave the tree clean: commit or stash, note
   state in the loop file.

On any stop, set `active: false` and delete the file after the branch
merges. A dead loop file left behind confuses the next session.

## Changing the prompt mid-run

Allowed, rare. The 141-iteration run changed its body once, at a phase
boundary. A prompt that changes every few iterations means the promise
was wrong — stop, fix the ROADMAP item, restart the loop.

## Multi-agent variant

For two parallel streams (the Dual Path pattern):

- Write a plan file per stream under `plans/`, with a file-ownership
  table — two agents never edit the same file.
- Each agent runs in its own git worktree on its own branch.
- The orchestrator session launches agents, answers questions, merges
  per the plan's protocol, runs the FULL gate once after merge, and
  does all device/simulator verification itself — agents never touch
  shared hardware.
- Set `completion_promise` to the joint promise: both streams proven,
  full suite green, both merged.
- Before launching, commit a rollback point named in the loop file
  ("PreStart"). If the run tangles, reset to it and run the streams
  one at a time.
- Standing rules for the run (device handling, sequencing) go in the
  loop file body — this is the one case where the body grows past
  3 sentences.
