# DEVLOG.md — the live devlog

The plain history of this project. A person who knows nothing about the
code reads this file and knows what happened, in order, with no jargon.
Append only. Never rewrite an old entry — a wrong entry gets a
correction entry, not an edit.

Write every entry in the voice of `TONE.md`.

## Entry shape

Every entry has the same shape:

```
## YYYY-MM-DD — <one line: what happened>

<What we did. What worked. What broke. What we learned.
3–10 short sentences. Plain words. Past tense for what happened,
present tense for how things now stand.>

Evidence: <commit / tag / gate run / screenshot>
```

## When to write

- Every WEEKLY.md cycle writes one entry at step 5 (before merge).
- A failed or abandoned attempt gets an entry too. The devlog records
  what happened, not what succeeded. A week with no shipped feature
  still gets its entry.
- Out-of-band work (security patch, gate repair, big triage) gets one.
- SETUP.md writes the first entry: "Installed the agent kit."

## What does not go here

- Code detail that belongs in commit messages.
- Promises about the future — that is ROADMAP.md.
- State claims — that is STATUS.md. The devlog is the story; STATUS is
  the snapshot.

---

<!-- Entries below, newest first. -->

## 2026-08-23 — Installed the agent kit and built the Snapshot MVP

I started with an empty repository: a spec in `START.md` and this kit. The
spec asks for a Git-style undo for computer-use agents. A code agent can undo
its work because Git exists. An agent that moves files on a desktop cannot.
The filesystem already has the primitive; nobody exposed it in a usable shape.

I built the 5 steps the spec's build order names. The engine wraps the
filesystem snapshot behind one interface with 4 operations. The store keeps
the snapshot graph in 1 SQLite file. The daemon serves 5 verbs on loopback.
`snapctl` speaks the same API an agent speaks. Retention keeps the last 50
automatic snapshots per workset.

The host got in the way in a useful manner. The spec targets Btrfs; this
machine runs ext4 and has no `btrfs` command. So I wrote a second backend
that hardlinks the working set. It runs anywhere, which means every gate runs
anywhere, and 100 snapshots of 200 files still use under 100 MB. It is not
copy-on-write, and I wrote that limit into the README rather than hide it.
The Btrfs backend is written and unit-tested through a command seam, but no
Btrfs host has run it. The gate says so out loud instead of passing.

One defect found me, and a test caught it. The retention loop reparented a
node onto a row that an earlier pass had already deleted, and SQLite refused
with a foreign key error. The loop now re-reads each node before it moves the
children. The test that found it prunes 3 nodes at once; the tests written
before it only ever pruned 1, so they all passed.

The queue holds 4 ready items. The first is the one this machine could not
do: prove the Btrfs backend on a Btrfs host and run the acceptance test from
the spec's section 10.

Evidence: `./verify/verify.sh` green at `HEAD`. 53 tests, 0 failures. A
manual run against the binary snapshotted a working set in 1 ms, diffed 1
added, 1 modified and 1 deleted path, and restored the set.

