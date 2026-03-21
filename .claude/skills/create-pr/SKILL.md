---
name: create-pr
description: Create a pull request for the mutest repository with proper conventions. Use this skill whenever the user asks to create a PR, open a pull request, submit changes for review, or says things like "PR作って", "PRお願い", "create pr", "open pr", "submit pr". Also use when the user has finished a task and wants to send it upstream.
---

# Create PR for mutest

This skill creates a pull request following mutest's established conventions. It handles branch creation, pre-flight checks, and PR submission via `gh`.

## Workflow

### 1. Analyze changes

Run these in parallel to understand the current state:

```bash
git status
git diff
git log --oneline -10
git diff main...HEAD
```

Identify:
- What files changed and why
- Whether changes are already committed or need committing
- The appropriate change type: `feat`, `fix`, `perf`, `refactor`, `chore`, `docs`, `test`

### 2. Create branch (if on main)

If still on `main`, create a branch following this naming convention:

| Type       | Branch name example              |
|------------|----------------------------------|
| Feature    | `feat/add-logical-mutator`       |
| Bug fix    | `fix/overlay-cleanup-race`       |
| Performance| `perf/parallel-discovery`        |
| Refactor   | `refactor/simplify-engine-api`   |
| Chore      | `chore/update-ci-go-version`     |

Use lowercase, hyphen-separated, concise descriptions.

### 3. Organize and commit changes

Group uncommitted changes into meaningful, logical units. Each commit should represent one coherent change — not "all files I touched" but "this set of changes achieves X".

If you find changes that are unrelated to the main task (e.g., an unrelated fix mixed in with a feature), ask the user whether they belong in this PR. If the user confirms they're unrelated, exclude them from this PR — leave them uncommitted or stash them for a separate PR.

Commit message format (conventional commits):

```
<type>: <short description>
```

Examples:
- `feat: add logical operator mutator for && and ||`
- `fix: prevent data race in progress callback`
- `perf: cache parsed ASTs across mutation points`

Keep the subject line under 72 characters. Add a body only if the "why" isn't obvious from the subject.

### 4. Run pre-flight checks

Before creating the PR, run tests and lint to catch issues early:

```bash
go vet ./...
go test ./... -race
```

If either fails, fix the issue before proceeding. Do not create a PR with failing checks.

### 5. Push and create PR

Push the branch and create the PR using `gh`:

```bash
git push -u origin HEAD
```

PR title: use the same conventional commit format as the commit message, under 70 characters.

PR body template (use a HEREDOC for correct formatting):

```bash
gh pr create --title "<type>: <description>" --body "$(cat <<'EOF'
## Summary
- <1-3 bullet points explaining what changed and why>

## Test plan
- [ ] <concrete verification steps>
- [ ] `go test ./... -race` passes
- [ ] `go vet ./...` passes
EOF
)"
```

Everything in the PR (title, body) must be written in English.

### 6. Report back

Return the PR URL to the user so they can review it.
