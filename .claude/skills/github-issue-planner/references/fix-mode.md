# --fix Mode Mechanics

`--fix` implements an already-written plan and opens a draft PR. It never plans from scratch — if no plan exists yet for the target issue, stop and tell the user to run the skill without `--fix` first.

## 1. Resolve the plan

Locate `docs/issue-plans/issue-<N>-*.md`. If an issue number wasn't given as an argument, look for plans at status `planned` in `TRIAGE.md`:

- Exactly one → use it.
- More than one → list them and ask which to implement.
- None → tell the user there's nothing queued to fix, and point at the default (plan) mode.

## 2. Preconditions

Before touching anything:

```bash
git status --porcelain
```

The tree should be clean, tolerating only untracked/modified files under `docs/issue-plans/` (triage index updates from a prior planning pass are fine). If there's other uncommitted work, stop and ask the user how to handle it rather than branching over it — don't stash or discard without asking.

```bash
gh auth status
```

Must pass — `--fix` will eventually need to push and open a PR.

## 3. Branch

```bash
git fetch origin
git switch -c <prefix>/issue-<N>-<slug> origin/<default-branch>
```

Prefix by category from the plan's frontmatter:

| Category | Prefix |
|---|---|
| bug | `fix/` |
| enhancement | `feat/` |
| documentation | `docs/` |
| everything else | `chore/` |

This matches the branch-naming convention already visible in the project's git history (e.g. `fix/windows-conpty-terminal`).

## 4. Implement

Work the plan's tasks **in order**, following the incremental-implementation discipline for each one:

1. Implement the smallest complete slice for the task
2. Run the relevant test/build step
3. Verify it actually does what the acceptance criteria describe
4. Commit — one commit per task, conventional-commit style subject line (`fix:`, `feat:`, etc., matching this project's convention), staging only the files that task touched (never `git add -A`)
5. Move to the next task

If a task's acceptance criteria can't be met as written (the plan assumed something about the code that turns out to be wrong), stop and report the discrepancy rather than silently improvising a different approach — the plan was reviewed; a deviation from it should be too.

Do not use `/agent-skills:build auto` for this — it requires a root `SPEC.md` to exist and enforces its own clean-baseline allowlist that doesn't account for `docs/issue-plans/`. Drive the task loop directly instead.

## 5. Verify

After all tasks are implemented, run the project's actual verification gates — don't assume generic ones. For this project (see `CLAUDE.md` and `Makefile`):

```bash
make check          # go build ./... && pnpm tsc --noEmit
make lint           # golangci-lint run
go test ./...
```

Run the frontend e2e suite too if any frontend files were touched:

```bash
cd frontend && pnpm test:e2e
```

**If any gate fails, stop.** Report exactly what failed and why, and do not proceed to pushing. A red branch pushed as a draft PR still wastes a reviewer's time opening it.

## 6. Confirm before pushing

Show the user:

- `git log --oneline origin/<default-branch>..HEAD` (the commits about to ship)
- `git diff --stat origin/<default-branch>..HEAD` (the shape of the change)

Then ask for explicit confirmation before:

```bash
git push -u origin <branch>
```

Never push directly to the default branch under any circumstances.

## 7. Open a draft PR

```bash
gh pr create --draft --base <default-branch> \
  --title "<type>: <short title> (#<N>)" \
  --body-file <tmpfile>
```

PR body contents:

```markdown
## Summary
<one paragraph — what changed and why, drawn from the plan's Approach section>

Fixes #<N>

## Plan
<link to docs/issue-plans/issue-<N>-<slug>.md>

## Task checklist
- [x] Task 1: <title> — <one-line verification evidence>
- [x] Task 2: <title> — <one-line verification evidence>

## Test plan
- [x] `make check`
- [x] `make lint`
- [x] `go test ./...`
- [ ] `pnpm test:e2e` (if applicable)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

Never pass anything other than `--draft`. Never call `gh pr merge`, `gh pr ready`, or edit the PR to remove draft status — that decision belongs entirely to the user.

## 8. Close the loop

- Update the issue's row in `docs/issue-plans/TRIAGE.md`: status → `pr-open`, add the PR link.
- Stamp the PR URL into the plan file's `**PR:**` header field.
- Report the PR URL to the user and stop. Do not comment on the GitHub issue itself unless asked.
