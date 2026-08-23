# CI.md — source blocks for .github/

SETUP.md step 5 instantiates these into `.github/`. Adjust the toolchain
step to the repo's stack; keep the verify step identical to local.

## `.github/workflows/ci.yml`

```yaml
name: CI
on:
  push:
    branches: [{{DEFAULT_BRANCH}}]
  pull_request:
  workflow_dispatch:
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
permissions:
  contents: read
jobs:
  verify:
    runs-on: ubuntu-latest   # macos-latest for Apple-platform builds
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
      # toolchain setup for the stack goes here
      - name: Verify
        run: {{VERIFY_CMD}}
```

## `.github/workflows/nightly.yml` — catches rot between cycles

```yaml
name: Nightly
on:
  schedule:
    - cron: '17 6 * * *'
  workflow_dispatch:
permissions:
  contents: read
  issues: write
jobs:
  verify:
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v4
      - name: Verify
        run: {{VERIFY_CMD}}
      - name: File issue on failure
        if: failure()
        uses: actions/github-script@v7
        with:
          script: |
            const open = await github.rest.issues.listForRepo({ ...context.repo, labels: 'ci-failure', state: 'open' });
            if (open.data.length === 0) {
              await github.rest.issues.create({ ...context.repo,
                title: `Nightly verify failed — ${new Date().toISOString().slice(0,10)}`,
                labels: ['ci-failure'],
                body: `Run: ${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}` });
            }
```

## `.github/workflows/release.yml`

```yaml
name: Release
on:
  push:
    tags: ['v*.*.*']
permissions:
  contents: write
jobs:
  release:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
      - name: Build artifacts
        run: {{BUILD_CMD}}    # adjust to produce dist/
      - name: Publish GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
          # files: dist/*
```

## `.github/workflows/stale.yml`

```yaml
name: Stale
on:
  schedule:
    - cron: '23 7 * * 1'
permissions:
  issues: write
  pull-requests: write
jobs:
  stale:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/stale@v9
        with:
          days-before-stale: 60
          days-before-close: 14
          stale-issue-label: stale
          exempt-issue-labels: 'blocked,security,accepted'
          stale-issue-message: >
            No activity in 60 days. Comment or this closes in 14 days.
```

## `.github/dependabot.yml`

```yaml
version: 2
updates:
  - package-ecosystem: {{PKG_ECOSYSTEM}}
    directory: "/"
    schedule: { interval: weekly }
    groups:
      minor-and-patch:
        update-types: ["minor", "patch"]
  - package-ecosystem: github-actions
    directory: "/"
    schedule: { interval: weekly }
```

## `.github/PULL_REQUEST_TEMPLATE.md`

```markdown
## What

## Why
<!-- Fixes #NN / ROADMAP.md item: <name> -->

## Checklist
- [ ] `{{VERIFY_CMD}}` passes locally
- [ ] Tests added/updated for behavior changes
- [ ] STATUS/CHANGELOG/ROADMAP updated in this PR as applicable
- [ ] No secrets, scaffolding, or leftover diagnostics
```

## Branch protection on `{{DEFAULT_BRANCH}}` (repo Settings)

- Require the `verify` check; require PRs; require up-to-date branches;
  squash-merge only; auto-delete head branches.
