# CONTRIBUTING.md

## Ground rules

- Be respectful; assume good faith.
- Open an issue before large changes; small fixes go straight to PR.
- Feature-sized ideas go to ROADMAP.md's queue via an issue — that's the
  path to getting built.
- One logical change per PR; refactors separate from behavior changes.

## Workflow

1. Branch from `{{DEFAULT_BRANCH}}`: `feat/…`, `fix/…`, `docs/…`, `chore/…`.
2. Change + tests for any behavior change.
3. `{{VERIFY_CMD}}` green.
4. Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`,
   `test:`, `ci:`; `!` for breaking).
5. PR against `{{DEFAULT_BRANCH}}`; fill the template; CI green +
   comments resolved before merge. Squash-merge; PR title becomes the
   commit message, so make it a valid Conventional Commit.
