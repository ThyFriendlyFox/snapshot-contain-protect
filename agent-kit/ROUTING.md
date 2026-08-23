# ROUTING.md — read this first

You are an AI agent pointed at this repository. This folder (`agent-kit/`)
is the operating system for working on it. This file tells you which file
governs which situation. Do not improvise a process that a file below
already defines.

## Dispatch table

| You are here because… | Go to | Notes |
|---|---|---|
| First time this kit is in this repo (placeholders like `{{THIS}}` still exist anywhere in the kit) | `SETUP.md` | One-time tailoring. Do this before anything else. |
| "Push out a feature" / "do the weekly update" / a scheduled weekly trigger fired | `WEEKLY.md` | The core cycle. Requires ROADMAP.md to have a ready queue. |
| Starting, running, or stopping an autonomous build loop (single or multi-agent) | `LOOP.md` | The ralph loop: file format, rules, stop conditions. |
| Deciding WHAT to build, or the queue is empty/stale | `ROADMAP.md` | The single source of work. Nothing gets built that isn't traceable to it. |
| Writing or changing ANY code, at any time | `AGENTS.md` | The binding contract: commands, invariants, landmines, style. Always in effect. |
| Asked "where does this project stand?" | `STATUS.md` | Answer from it; update it when state changes. |
| The gate fails, or you're adding/changing checks | `VERIFICATION.md` | One command answers "is this repo healthy". |
| Cutting a release / tagging | `RELEASING.md` | Weekly cycle ends here. |
| Triage, dependency updates, stale issues, deprecations | `MAINTENANCE.md` | The between-features runbook. |
| Vulnerability report or anything secret-shaped | `SECURITY.md` | Overrides normal process. Never open a public issue for it. |
| Human contributor questions, PR conventions | `CONTRIBUTING.md` | |
| CI is missing or broken, workflows need creating | `CI.md` | Source blocks for `.github/workflows/`. |
| Understanding the codebase before a change | `docs/ARCHITECTURE.md` | Parts, data flow, boundaries. |
| Adding/changing any config option | `docs/CONFIGURATION.md` | Every knob lands in its table, same PR. |
| Touching the pluggable seam (adapters/providers/backends) | `docs/ADAPTERS.md` | Includes the new-adapter contract. |
| Judging whether a feature is worth building, or writing roadmap items | `docs/USE-CASES.md` | Every ROADMAP item traces to a use case here. |
| Writing ANY user-facing text (docs, UI copy, errors, release notes) | `docs/STYLE.md` | One voice, enforced terms table. |
| Writing YOUR OWN words — commits, PR bodies, reports, devlog entries | `TONE.md` | The agent's voice. Always in effect, like AGENTS.md. |
| Recording what happened this week (or any notable event) | `DEVLOG.md` | Append-only plain history. Every cycle writes an entry. |
| Asked for a blog post / public write-up of recent work | `skills/blog-post/SKILL.md` | Devlog in, post out. Never publishes without a go-ahead. |

## Precedence

1. `SECURITY.md` overrides everything for security-shaped work.
2. `AGENTS.md` is binding whenever code is touched; `TONE.md` is binding whenever the agent writes words. When either conflicts with intuition, it wins.
3. `ROADMAP.md` decides what gets built. If requested work isn't on it, add it to the roadmap first (or get the human to), then build it. **The roadmap is not optional — every feature flows down from it.**
4. `VERIFICATION.md`'s gate decides what ships. No green, no push.
5. Everything else is reference.

## Standing rules (apply in every instance)

- Docs move with behavior: STATUS.md, CHANGELOG.md, ROADMAP.md updates land in the same commit/PR as the change they describe.
- Commit at boundaries with evidence in the message. Never commit directly to `{{DEFAULT_BRANCH}}`.
- Report outcomes faithfully: failing is reported as failing, with output.
- Scope is what the current roadmap item names — no side quests. Defects found on the way get fixed if they block the item, otherwise queued in ROADMAP.md.
