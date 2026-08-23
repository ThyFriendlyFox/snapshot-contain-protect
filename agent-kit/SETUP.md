# SETUP.md — one-time installation into a repo

Run this once, when this kit lands in a repo and placeholders still exist.
When finished, no `{{PLACEHOLDER}}` remains anywhere in `agent-kit/`.

## 1. Survey the repo

Read the tree, the manifest(s), any existing README/docs/CI. Determine:
stack, build/test/lint commands, default branch, license, how it runs.

## 2. Fill every placeholder, in every kit file

| Placeholder | Meaning |
|---|---|
| `{{PROJECT_NAME}}` | Human name of the project |
| `{{REPO_SLUG}}` | owner/repo |
| `{{DEFAULT_BRANCH}}` | usually `main` |
| `{{BUILD_CMD}}` / `{{TEST_CMD}}` / `{{LINT_CMD}}` | canonical commands (create them if missing) |
| `{{VERIFY_CMD}}` | ONE command chaining lint→build→test (create `verify/verify.sh` if nothing exists) |
| `{{MIN_RUNTIME}}` | minimum toolchain |
| `{{SECURITY_EMAIL}}` | from the human; ask if unknown |
| `{{PKG_ECOSYSTEM}}` | dependabot ecosystem name |

## 3. Reconcile with existing files

- Repo already has README/CONTRIBUTING/SECURITY/etc.: the kit does NOT
  duplicate them. Link to them from the kit, or fold their content in —
  one source of truth per topic, never two.
- Repo has a root `AGENTS.md`/`CLAUDE.md`: merge — repo-specific
  invariants stay, kit process rules join them. Leave a pointer file so
  there is exactly one contract.

## 4. Seed the roadmap — REQUIRED, blocking

`ROADMAP.md` must end setup with a north star and **at least 3 ready
items in the Feature Queue**, each with a testable completion promise.
Derive candidates from the repo's own TODOs/issues/gaps; confirm
priorities with the human if reachable, otherwise mark the queue
`provisional: true` and proceed — the human re-orders later.

## 4b. Fill the docs layer

Populate `docs/` from the repo itself:

- `docs/ARCHITECTURE.md` — parts table, one numbered data flow, boundaries.
- `docs/CONFIGURATION.md` — every real option with its real default.
- `docs/ADAPTERS.md` — rename to the repo's word for its pluggable seam
  (providers/backends/drivers); delete only if no such seam exists.
- `docs/USE-CASES.md` — 5–10 same-shape cases. These feed the roadmap:
  every Feature Queue item must trace to a case here.
- `docs/STYLE.md` — keep the rules; seed the Terms table with the
  repo's existing vocabulary.

## 5. Wire the machinery

- Instantiate workflows from `CI.md` into `.github/workflows/`.
- Add the loop-state ignore lines from `LOOP.md` to `.gitignore`.
- Ensure `CHANGELOG.md` exists (seed is in this kit).
- Run `{{VERIFY_CMD}}`; fix until green — this is the baseline.

## 5b. Start the devlog and install the skill

- Write the first `DEVLOG.md` entry: "Installed the agent kit." State
  what the repo is and what the seeded queue holds.
- Copy `skills/blog-post/` into the repo's skill directory
  (`.claude/skills/blog-post/`) and set its output path.

## 6. Commit

One commit on a branch: `chore: install agent-kit`. Push. From now on,
`ROUTING.md` governs.
