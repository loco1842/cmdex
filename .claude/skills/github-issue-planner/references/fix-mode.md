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

## 6. Commit the plan (and spec) into this branch

The plan file (and its spec, if the issue was escalated) lives under `docs/issue-plans/` wherever it was originally written — often a different branch, or still uncommitted in whatever context wrote it. It will **not** exist on the fix branch unless you copy it over explicitly:

```bash
mkdir -p docs/issue-plans
cp <path-to-original>/docs/issue-plans/issue-<N>-<slug>.md docs/issue-plans/
git add docs/issue-plans/issue-<N>-<slug>.md
git commit -m "docs: add plan for issue #<N>"
```

Do this as its own small commit, separate from the task commits — it's supporting documentation, not part of the implementation. Skipping this step means the PR body's "Plan" link (step 9) points at a file that doesn't exist on this branch.

## 7. Verify independently before asking for confirmation

Before showing the user anything, re-check the work yourself rather than trusting a summary at face value — this matters whether you implemented the tasks yourself or a subagent did:

```bash
git diff origin/<default-branch>..HEAD
go build ./... && go test ./...          # or this project's equivalent
cd frontend && pnpm tsc --noEmit
```

Read the actual diff, not just the diffstat — confirm it matches the plan's file map and doesn't touch anything the plan marked out of scope. A subagent's final report describes what it *intended* to do; rerunning the gates yourself and reading the real diff is what confirms it actually happened.

## 8. Confirm before pushing

Show the user:

- `git log --oneline origin/<default-branch>..HEAD` (the commits about to ship)
- `git diff --stat origin/<default-branch>..HEAD` (the shape of the change)
- The verification results from step 7

Then ask for explicit confirmation before:

```bash
git push -u origin <branch>
```

Never push directly to the default branch under any circumstances.

## 9. Open a draft PR

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
<link to the plan file — now a real blob URL on this branch, e.g.
https://github.com/<owner>/<repo>/blob/<branch>/docs/issue-plans/issue-<N>-<slug>.md>

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

## 10. Close the loop

- Update the issue's row in `docs/issue-plans/TRIAGE.md`: status → `pr-open`, add the PR link.
- Stamp the PR URL into the plan file's `**PR:**` header field (in whatever copy is the source of truth — the fix branch's copy doesn't need a self-referential update).
- Report the PR URL to the user and stop. Do not comment on the GitHub issue itself unless asked.

## Parallelizing across multiple issues

Fixing several issues at once means several branches with real source edits — those can't share one working directory. Two `--fix` flows in the same checkout would fight over which branch is checked out and could interleave writes to the same files.

**Isolate each issue in its own worktree, but split the work: subagents implement, you close the loop.**

Create each worktree yourself rather than relying on the Agent tool's `isolation: "worktree"` default — it gives you direct control over the exact branch name and base, which this skill's category-based prefix convention depends on:

```bash
git fetch origin
git worktree add ../<repo>-issue-<N> -b <prefix>/issue-<N>-<slug> origin/<default-branch>
```

Then spawn one subagent per worktree, each briefed with the full plan content directly in the prompt (fresh subagents have no context — don't assume they can read files from your current directory or an unrelated branch) and instructed to work **only through step 5** — resolve/implement/verify — then stop and report back its commit list, diffstat, and verification output. Explicitly tell each subagent not to push or open a PR.

Once a subagent reports back, run steps 6-10 **yourself**, from your own shell, for that worktree:

1. Commit the plan/spec into the branch (step 6).
2. Re-verify independently (step 7) — don't skip this just because it ran unattended.
3. Push and open the draft PR only after showing the user the diff and getting confirmation (step 8) — this has to reach an actual human, which a background subagent cannot obtain on its own.
4. Update `TRIAGE.md` and the plan file, then remove the worktree:
   ```bash
   git worktree remove ../<repo>-issue-<N>
   ```

If two issues' plans touch overlapping files, don't parallelize those two — implement them sequentially so the second one is grounded in the first one's actual result rather than a plan written against a codebase that already changed underneath it.
