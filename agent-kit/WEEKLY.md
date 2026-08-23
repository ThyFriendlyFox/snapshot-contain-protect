# WEEKLY.md — the weekly feature cycle

Trigger: a human says "push the weekly feature", or a scheduled weekly
run fires. One cycle = one Feature Queue item taken from promise to
released, with evidence. This file is the ritual; `AGENTS.md` is the law
while you execute it.

## The loop

Run the implementation phase (step 3) as a ralph loop. `LOOP.md` has
the creation instruction: the `.claude/ralph-loop.local.md` file format,
its field rules, gitignore requirement, stop conditions, and the
multi-agent variant. The prompt stays 2–3 sentences; all intelligence
lives in the kit files. Do not fatten the prompt.

## The cycle

**0. Preflight.** Fetch; branch from latest `{{DEFAULT_BRANCH}}` as
`feat/<slug>`. Run `{{VERIFY_CMD}}` — the baseline must be green before
feature work starts. A red baseline is this week's feature: fix it,
ship the fix as the weekly update, and record that in ROADMAP.md.

**1. Pick.** Top unblocked item in the Feature Queue. Do not skip or
reorder silently; a reorder needs a line in "Queue changes" with a reason.

**2. Spec.** Re-read the item. If the promise is not one testable
sentence, sharpen it now — before code. Write down what evidence will
prove it and what is out of scope. Mark the item `in progress (week of
<date>)`.

**3. Build.** Create the loop file per `LOOP.md` and run it:
implement → `{{VERIFY_CMD}}` → commit at each
boundary with evidence in the message. Scope is the item's promise —
defects found en route get fixed if they block the promise, otherwise
queued in ROADMAP.md "Later". No side quests.

**4. Prove.** The promise must be demonstrably true: gates green AND the
item's named evidence produced (screenshot, output, benchmark — attach
or link it in the PR/commit).

**5. Document — same branch, before merge.** Update in one commit:
- `CHANGELOG.md` — under Unreleased, user-facing words.
- `STATUS.md` — new state, with the evidence.
- `ROADMAP.md` — item → Shipped table; promote the queue; refill to ≥3
  ready items (mark refills `provisional` if the human hasn't ranked them).
- `DEVLOG.md` — one entry in TONE.md voice: what happened this week,
  what broke, what we learned, with evidence. Failed weeks get an
  entry too.
- Any README/docs the behavior touched. Docs move with behavior:
  new config option → its row in `docs/CONFIGURATION.md`; moved
  boundary → `docs/ARCHITECTURE.md`; new/changed adapter →
  `docs/ADAPTERS.md`; new capability → its case in `docs/USE-CASES.md`.
  All user-facing words obey `docs/STYLE.md`.

**6. Ship.** Push the branch; open a PR per `CONTRIBUTING.md` (or merge
directly if the repo's policy allows agent merges). CI must be green.

**7. Release.** Follow `RELEASING.md`: the weekly update is a **minor**
version by default, **major** if the promise involved a breaking change.
Tag; the release workflow publishes; verify the artifact actually
installs/runs.

**8. Report.** End with a short summary in TONE.md voice: what shipped,
the evidence, what's next at the top of the queue, anything blocked.
If the human wants a public write-up, run `skills/blog-post/SKILL.md`.

## Failure modes

- **Promise can't be met this week:** ship the largest coherent, gated
  slice; split the remainder into a new queue item with its own promise.
  Never ship an unproven promise, and never let the week end with the
  branch unmerged AND undocumented — either merge the slice or record
  the state in ROADMAP.md.
- **Queue empty or all blocked:** the cycle's deliverable becomes a
  refilled queue (per SETUP.md step 4) plus unblocking work. Say so in
  the report.
- **Gate flaky:** fixing the gate IS in scope, always (VERIFICATION.md).
