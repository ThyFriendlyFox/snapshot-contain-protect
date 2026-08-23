# AUTOMATION.md — the machine that ships one update a week, per repo

`WEEKLY.md` defines one cycle. This file defines how many cycles run, what
starts them, where their work comes from, and what stops a bad one from
merging. It is the layer above `WEEKLY.md` and the layer below the human.

Precedence: `SECURITY.md` > `AGENTS.md` / `TONE.md` > `ROADMAP.md` >
`VERIFICATION.md` > this file > everything else. This file schedules work.
It never overrides an invariant.

---

## 1. The repo register

One row per repo. Each repo ships one update per week on its own day. A
staggered schedule keeps human review load flat at 1 repo per day.

| Repo | Day | Gate command | Feedback sources |
|---|---|---|---|
| `momentum-swing` | Monday | `./verify/verify.sh` | own issues |
| `mouse` | Tuesday | `./verify/verify.sh` | App Store reviews, TestFlight, issues |
| `kern` | Wednesday | `./verify/verify.sh` | issues, Play Store reviews |
| `blackarrow` | Thursday | `./verify/verify.sh` | design partner email, issues |
| — | Friday | — | reserved for overflow and releases |
| `snapshot-contain-protect` | unassigned | `./verify/verify.sh` | own issues |

`snapshot-contain-protect` finished `SETUP.md` on 2026-08-23 with a green
gate and a queue of 4 ready items. It has no day yet: every weekday except
Friday is taken, and Friday is reserved. The human assigns its day.

Rules for the register:

1. Every repo has exactly 1 gate command. The name is the same in every repo.
2. A repo with no ready queue item does not get a day. It gets a queue-refill
   cycle instead, per `WEEKLY.md` failure modes.
3. A repo joins the register only after `SETUP.md` completes and the gate is
   green. A red baseline is that repo's first feature.

---

## 2. The three run tiers

| Tier | Use it for | Survives machine off | Local files |
|---|---|---|---|
| Cloud routine | Feedback intake, nightly rot check | yes | no |
| Desktop scheduled task | The weekly cycle trigger | no | yes |
| `/loop` in session | The build phase inside a cycle | no | yes |

The build phase needs local files, a warm cache, and your credentials. It runs
as `/loop`. The trigger does not need any of that. It runs as a Desktop task.
Feedback intake touches no source tree at all. It runs in the cloud.

---

## 3. Feedback intake — how user words become roadmap items

Feedback never becomes a feature directly. It becomes a **candidate**. A
candidate becomes a queue item only after it has a testable promise and a use
case. This is the gate that keeps the app from becoming a pile of requests.

### 3.1 The intake routine

Create 1 cloud routine per repo. Schedule it 24 hours before that repo's build
day, so the human has a night to re-rank.

```
/schedule collect feedback for <repo>, cluster it, and open one issue
per cluster labeled feedback-candidate
```

The routine reads the repo's feedback sources, then writes candidates as
GitHub issues. It does not edit `ROADMAP.md`. Only the build cycle does that.

### 3.2 The candidate shape

Every `feedback-candidate` issue uses this body. The routine fills it.

```markdown
## Signal
<N> reports. Sources: <where>. First seen <date>.

## What users said
<2 to 4 paraphrased reports. No marketing words. No quotes longer than
one sentence.>

## What they actually want
<1 sentence. The underlying need, not the requested solution.>

## Use case
<Names an existing case in docs/USE-CASES.md, or states that none fits.>

## Proposed promise
<1 testable sentence, in the shape ROADMAP.md items use.>

## Vision check
<1 sentence: why this belongs in the app, per the north star.>
```

### 3.3 The promotion rule

At `WEEKLY.md` step 1, the agent reads open `feedback-candidate` issues. It
promotes a candidate to the Feature Queue only when all 4 hold:

1. The promise is 1 testable sentence with named evidence.
2. It traces to a case in `docs/USE-CASES.md`, or it adds one in the same PR.
3. It does not contradict the `ROADMAP.md` north star.
4. It fits the repo's existing seams. A candidate that needs a new abstraction
   gets split: the seam is its own item, first.

A candidate that fails any of the 4 gets a comment saying which one, then
`wontfix` or `blocked`. Never close it silently.

### 3.4 The vision guard

Volume is not a vote. 20 users asking for a feature that contradicts the north
star is 20 users asking for a different product. The agent states this plainly
in the issue and moves on. Record the decision in `ROADMAP.md` under
"Explicitly not doing" so the next cycle does not relitigate it.

---

## 4. The weekly machine

Per repo, per its day.

### 4.1 Trigger — Desktop scheduled task

Create 1 local task per repo in the Desktop app under Routines. Point it at
that repo's folder. The prompt is 3 sentences. All intelligence lives in the
kit files.

```markdown
---
name: weekly-<repo>
description: Run the weekly feature cycle for <repo>.
---

Run the weekly cycle in agent-kit/WEEKLY.md for this repo.
Promote at most 1 feedback-candidate issue per agent-kit/AUTOMATION.md
section 3.3 before you pick.
Obey AGENTS.md and TONE.md. Stop and report if the baseline gate is red.
```

The prompt lives at `~/.claude/scheduled-tasks/weekly-<repo>/SKILL.md`.
Edit it there and commit a copy to the repo so the prompt is versioned.

Set the task time outside your working hours. A long cycle collides with
interactive work otherwise. Schedule a minute that is not `:00` or `:30`;
the scheduler adds an offset of up to 30 minutes to recurring fires.

### 4.2 Build — `/loop` plus `/goal`

The cycle's step 3 runs as a loop. Write the repo's default prompt once, at
`.claude/loop.md`, and commit it.

```markdown
Ship the top ready item in agent-kit/ROADMAP.md.
Work until its completion promise is true with the named evidence.
Run the gate before every commit. Commit at each boundary with evidence
in the message.
If the promise cannot be met this week, ship the largest gated slice and
split the remainder into a new queue item.
```

Set the promise as the stop condition:

```
/goal <the ROADMAP item's promise, verbatim>
```

Then start it:

```
/loop
```

The evaluator judges the condition against what the session has surfaced. Write
promises that name a file, a command, or a count. "The gate is green with 24
tests" resolves. "The feature works well" does not.

### 4.3 Parallel work — `/batch`

Use `/batch` only when the item decomposes into independent units that touch
disjoint files. A migration across many call sites qualifies. A single feature
does not. `/batch` opens 1 pull request per unit, which multiplies your review
load; that is the cost you are paying for the parallelism.

### 4.4 Verify — before the tell sweep

Run in order:

1. The repo gate.
2. `/run` or `/verify`, to see the change working in the app, not only in tests.
3. `/code-review high --fix` on the diff.
4. `/security-review` when the diff touches auth, input parsing, or network.
5. The tell sweep in section 5.

Record the recipe for step 2 once per repo with `/run-skill-generator`. It
writes `.claude/skills/run-<name>/SKILL.md` so every later run and every other
agent follows the same launch steps instead of rediscovering them.

`/verify` is user-invocable only. A scheduled fire will not execute it; the
text arrives as plain text instead. Keep the real gate a shell script.

---

## 5. The quality bar — no visible AI

This is the section that matters most. A user must not be able to tell. The
gate proves the code runs. This section proves it belongs in the repo.

### 5.1 The single rule

Match the file you are standing in. Naming, comment density, error style,
test shape, file length, import order. When the repo and a general best
practice disagree, the repo wins. `AGENTS.md` says this. This section names
the specific ways an agent breaks it.

### 5.2 Code tells — reject the diff if you see these

| Tell | Correct form |
|---|---|
| A comment that narrates the next line | Delete it. Comments state constraints the code cannot show |
| `# Step 1:` / `# Step 2:` block comments | Delete them |
| `try`/`except` around code that cannot fail | Let it raise |
| A new `utils` or `helpers` module | Put the function next to its caller |
| A config option nobody requested | Remove it. New knobs need a ROADMAP line |
| A wrapper class with 1 caller | Inline it |
| Docstrings on private 3-line functions when the file has none | Match the file |
| Renaming an existing concept to a "clearer" name | Keep the repo's word |
| Defensive `None` checks on values that are never `None` | Remove them |
| A test that asserts the implementation | Assert the behavior |
| Emoji or box-drawing in program output | Remove it |
| A new dependency for something the stdlib does | Remove it |

### 5.3 Interface tells

The standing rule already: no explanatory microcopy, no UI chrome. That means
no subtitles, no taglines, no status pills, no badges, no toasts narrating
state, no helper text, no empty-state paragraphs explaining what a screen is.
State the thing. Never reassure about it. Keep only the interactive elements.

Additional tells: gradients added without a request, a spinner where the work
is instant, a confirmation dialog for a reversible action, an onboarding
overlay, rounded-corner and drop-shadow defaults that no other screen uses.

### 5.4 Prose tells

Applies to release notes, README changes, error strings, commit bodies, PR
bodies, and devlog entries.

| Tell | Correct form |
|---|---|
| "seamlessly", "robust", "powerful", "simply", "just" | Delete the word |
| "It is not just X, it is Y" | State Y |
| Three-item lists used for rhythm | Say the one thing that matters |
| A header on a 2-sentence section | Delete the header |
| Bullets where 2 sentences would do | Write the sentences |
| Hedges: "should work", "may help" | State what you tested and what you did not |
| A closing paragraph restating the opening | Delete it |
| Em dashes stacked in one paragraph | Use a period |

`TONE.md` already carries the base rules. Read a sentence aloud. If a tired
person understands it on the first pass, it ships.

### 5.5 Commit and PR tells

A commit message says the real change. "feat: implement comprehensive
improvements" says nothing. First line under 65 characters. Body: what
changed, why, evidence. A PR body is What / Why / Evidence and links the
ROADMAP item. No emoji headers. No restating the diff line by line.

### 5.6 The sweep

Before merge, read the full diff once against sections 5.2 to 5.5. Ask 1
question per file: would a maintainer who has never seen this repo guess a
different author wrote this file? If yes, name the line and fix it.

This sweep is a required step in `WEEKLY.md` step 4. A cycle is not proven
until it passes.

---

## 6. Deterministic gates — hooks

Prose in a kit file is model-judged. A hook is not. Move every rule that a
script can decide into `.claude/settings.json`, so it commits with the repo and
binds every agent that opens it.

| Rule | Event | Handler |
|---|---|---|
| Never commit to `main` | `PreToolUse`, matcher `Bash` | Exit 2 when the command pushes to `main` |
| Docs move with behavior | `Stop` | Exit 2 when `src/` is staged without STATUS / CHANGELOG / ROADMAP |
| No new dependency without a queue item | `PreToolUse`, matcher `Edit\|Write` | Exit 2 on a manifest edit with no matching ROADMAP line |
| No leftover diagnostics | `PreToolUse`, matcher `Bash` | Exit 2 when the staged diff adds a debug print |

Exit code 2 means different things per event. On `PreToolUse` it blocks the
tool call. On `Stop` it forces another turn. On `PostToolUse` it undoes
nothing and only shows the message to the model. Pick the event accordingly.

Hooks run with your user permissions. Treat them as executable code.

---

## 7. Use cases

Each case has the same shape: Load, Run, Prove.

### Ship a requested feature

**Load.** The intake routine files 6 `feedback-candidate` issues for `mouse`.
The Tuesday task fires and reads them.
**Run.** One candidate has a testable promise, a matching use case, and no
conflict with the north star. It joins the queue. `/loop` builds it. The gate
runs at each boundary.
**Prove.** Gate green, `/run` shows the feature working on device, tell sweep
clean, PR merged, `CHANGELOG.md` updated in the same branch.

### Reject a popular request

**Load.** 14 users ask for a feature that contradicts the north star.
**Run.** The agent states the conflict in 1 sentence, closes the issue
`wontfix`, and adds a line to `ROADMAP.md` under "Explicitly not doing".
**Prove.** The next cycle does not re-raise it.

### Ship a slice instead of a promise

**Load.** The item's promise needs 3 subsystems. Two are done by Thursday.
**Run.** The agent ships the largest coherent gated slice and splits the
remainder into a new item with its own promise.
**Prove.** The branch merged. `ROADMAP.md` holds the split item. `DEVLOG.md`
records the shortfall plainly.

### Catch an AI tell before merge

**Load.** The gate is green. The diff adds a `helpers.py` and 11 narrating
comments.
**Run.** The tell sweep flags both. The agent inlines the function and deletes
the comments, then reruns the gate.
**Prove.** The diff reads like the surrounding files. No `helpers.py` in the
tree.

### Repair a red baseline

**Load.** Wednesday's task fires. `./verify/verify.sh` fails on `kern`.
**Run.** The red baseline becomes that week's feature. The agent fixes the
gate, ships the fix, and records it in `ROADMAP.md`.
**Prove.** Gate green. `DEVLOG.md` has an entry for the week with no shipped
feature.

### Run a week with an empty queue

**Load.** `blackarrow` has no ready item and no candidates.
**Run.** The cycle's deliverable becomes a refilled queue plus unblocking
work, per `WEEKLY.md` failure modes.
**Prove.** 3 or more ready items with testable promises, marked
`provisional: true` until you rank them.

---

## 8. Failure modes

| Symptom | Cause | Fix |
|---|---|---|
| The loop prompt keeps growing | Thin spec | Sharpen the ROADMAP promise; never fatten the prompt |
| The goal never resolves | Condition names something the transcript cannot show | Rewrite it to cite a file, command, or count |
| The agent builds the wrong thing | Candidate promoted without a use case | Enforce section 3.3 in the trigger prompt |
| Reviews pile up | `/batch` used on non-independent work | Use `/loop` for single features |
| Diffs read as generated | Tell sweep skipped | Make it a hard step in step 4 |
| The task never fires | Machine asleep | Use a cloud routine for anything that must not slip |
| Fixes stop appearing after 7 days | Recurring `/loop` tasks expire after 7 days | The Desktop task is the durable trigger, not `/loop` |

---

## 9. Install checklist

Per repo:

1. `SETUP.md` complete. Gate green. Add the repo to the section 1 register.
2. Write `.claude/loop.md`. Commit it.
3. Run `/run-skill-generator` once. Commit the recorded skill.
4. Add the section 6 hooks to `.claude/settings.json`. Commit them.
5. Create the Desktop task for that repo's day. Commit a copy of its prompt.
6. Create the cloud intake routine, scheduled 24 hours earlier.
7. Add a `ROUTING.md` row: "Scheduling cycles, intake, or the quality bar →
   `AUTOMATION.md`".
8. Add `.claude/` runtime state to `.gitignore`. The scheduled task list lives
   there.
9. Run 1 cycle by hand, start to finish, before letting the task fire
   unattended. Approve every permission prompt during that run so later runs
   do not stall.
