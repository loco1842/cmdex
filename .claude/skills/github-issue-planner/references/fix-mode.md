# --fix Mode Mechanics

`--fix` implements an already-written plan and opens a draft PR. It never plans from scratch — if no plan exists *anywhere* for the target issue, stop and tell the user to run the skill without `--fix` first. "Anywhere" is load-bearing: step 1 searches past the current checkout, because a plan sitting on an unmerged branch is still a plan.

## 1. Resolve the plan

**The plan is often not in the current checkout, and its absence there means nothing.** Mode A lands plans through their own docs PR (`SKILL.md` Step 11), so "committed on a docs branch that hasn't merged yet" is a perfectly normal state — as is planning in one session and running `--fix` in a later one. Check all three locations before concluding anything:

1. **The working tree** — `ls docs/issue-plans/issue-<N>-*.md`
2. **The default branch** — the fix branch is cut from it, so anything already merged will simply be there once you branch in step 3.
3. **Any other branch**, including an unmerged docs PR or another issue's fix branch:

```bash
git fetch origin
git log --all --oneline --diff-filter=A -- 'docs/issue-plans/issue-<N>-*.md'
git branch -a --contains <sha-from-above>
```

If that turns something up, note the branch and keep going — step 6 recovers the file from it. Do **not** stop here.

In all three lookups, **exclude any `*.spec.md` file** — that's the escalation spec (see `SKILL.md` Step 8), not the implementation plan, even though it matches the same glob. When an issue was escalated both files exist side by side; this step always resolves the plan, never the spec.

If an issue number wasn't given as an argument, look for plans at status `planned` in `TRIAGE.md`:

- Exactly one → use it.
- More than one → list them and ask which to implement.
- **None, and all three lookups above came up empty** → only then tell the user there's nothing queued to fix, and point at the default (plan) mode. Reporting "no plan exists" while one sits on an unmerged branch sends the user off to re-plan work that's already done — and the plan they'd be told to write is the one they already reviewed.

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

Prefix by category from the plan's `**Category:**` header field (a markdown header line, not literal YAML frontmatter — the plan template doesn't use frontmatter):

| Category | Prefix |
|---|---|
| bug | `fix/` |
| enhancement | `feat/` |
| documentation | `docs/` |
| everything else | `chore/` |

This matches the branch-naming convention already visible in the project's git history (e.g. `fix/windows-conpty-terminal`).

## 4. Implement

Work the plan's tasks **in order**, following the incremental-implementation discipline for each one (that skill ships in the `agent-skills` plugin — if it isn't installed here, "Fallback C" in `fallback-prompts.md` restates it self-contained):

1. Implement the smallest complete slice for the task
2. Run the relevant test/build step
3. Verify it actually does what the acceptance criteria describe
4. Commit — one commit per task, conventional-commit style subject line (`fix:`, `feat:`, etc., matching this project's convention), staging only the files that task touched (never `git add -A`)
5. Move to the next task

If a task's acceptance criteria can't be met as written (the plan assumed something about the code that turns out to be wrong), stop and report the discrepancy rather than silently improvising a different approach — the plan was reviewed; a deviation from it should be too.

Do not use `/agent-skills:build auto` for this — it requires a root `SPEC.md` to exist and enforces its own clean-baseline allowlist that doesn't account for `docs/issue-plans/`. Drive the task loop directly instead.

## 5. Verify

After all tasks are implemented, run the project's actual verification gates — don't assume generic ones, and don't approximate them either. Read `.github/workflows/ci.yml` (or this project's equivalent) to see what CI actually enforces, rather than guessing from `CLAUDE.md`/`Makefile` alone — a locally "reasonable" gate that's narrower than the real CI gate just means CI catches the problem later instead of now. For this project, CI runs `go test -race ./...` (not the bare form) and the e2e suite **unconditionally on every PR**, not only when frontend files changed:

```bash
make check          # go build ./... && pnpm tsc --noEmit
make lint           # golangci-lint run
go test -race ./...
cd frontend && pnpm test:e2e
```

**If any gate fails, stop.** Report exactly what failed and why, and do not proceed to pushing. A red branch pushed as a draft PR still wastes a reviewer's time opening it.

## 6. Commit the plan (and spec) into this branch

The fix branch is cut from the default branch, so if the plan already landed there (Mode A Step 11) it's simply present — nothing to copy, skip to step 7. Otherwise it exists only as an uncommitted draft or on another branch, and won't be on the fix branch unless you put it there. The two cases need different commands; run whichever applies, and in both write **straight to the final path** rather than staging through a temp file.

**Uncommitted in another checkout** (Mode A ran earlier this session without landing its plans):

```bash
mkdir -p docs/issue-plans
SRC=<that-checkout>/docs/issue-plans
cp "$SRC/issue-<N>-<slug>.md" docs/issue-plans/
[ -f "$SRC/issue-<N>-<slug>.spec.md" ] && cp "$SRC/issue-<N>-<slug>.spec.md" docs/issue-plans/
```

**Committed on another branch** — read it out without checking that branch out:

```bash
mkdir -p docs/issue-plans
git show <branch>:docs/issue-plans/issue-<N>-<slug>.md > docs/issue-plans/issue-<N>-<slug>.md
git show <branch>:docs/issue-plans/issue-<N>-<slug>.spec.md > docs/issue-plans/issue-<N>-<slug>.spec.md 2>/dev/null || rm -f docs/issue-plans/issue-<N>-<slug>.spec.md
```

Three things that bite here. Redirecting to a temp file and copying afterward is how the filename drifts away from the one the PR link expects — write the destination name directly. The `|| rm -f` isn't decoration: a failed `git show` still creates its redirect target, so without it every non-escalated issue picks up an empty `.spec.md`. And keep that `||` on one line rather than wrapping it with a `\` continuation — the continuation is easy to mangle when these commands get passed around as a single string, and a broken `||` silently deletes the file the `git show` just wrote.

Then commit, staging the spec only when there is one:

```bash
git add docs/issue-plans/issue-<N>-<slug>.md
[ -f docs/issue-plans/issue-<N>-<slug>.spec.md ] && git add docs/issue-plans/issue-<N>-<slug>.spec.md
git commit -m "docs: add plan for issue #<N>"
```

Stage the two paths as separate commands, not one `git add plan spec`. When the spec doesn't exist — the common case, since only escalated issues have one — a combined add fails on the missing pathspec with exit 128 and stages **nothing at all, including the plan**. Suppressing its stderr hides that completely, and the commit that follows then either fails or ships without the plan.

Carry the spec whenever one exists — an escalated issue's PR should have its whole decision record, not half of it. Keep this as its own small commit, separate from the task commits. Skipping this step means the PR body's "Plan" link (step 9) points at a file that isn't on the branch.

## 7. Verify independently before asking for confirmation

Before showing the user anything, re-check the work yourself rather than trusting a summary at face value — this matters whether you implemented the tasks yourself or a subagent did:

```bash
git diff origin/<default-branch>..HEAD
go build ./... && go test -race ./...    # or this project's equivalent — match step 5, not a shortcut of it
cd frontend && pnpm tsc --noEmit
```

Read the actual diff, not just the diffstat — confirm it matches the plan's file map and doesn't touch anything the plan marked out of scope. A subagent's final report describes what it *intended* to do; rerunning the gates yourself and reading the real diff is what confirms it actually happened.

## 8. Confirm before pushing

First, confirm there's nothing uncommitted that the diff below would silently miss:

```bash
git status --porcelain
```

This must be empty. `git diff origin/<default-branch>..HEAD` (below) only shows *committed* history — if anything is still staged or unstaged, the user could approve a diff that doesn't match what actually gets pushed, because tests could have passed against code that never makes it into the push. Commit it or explicitly ask the user how to handle it — never discard without asking.

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
- [x] `go test -race ./...`
- [x] `pnpm test:e2e`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

Never pass anything other than `--draft`. Never call `gh pr merge`, `gh pr ready`, or edit the PR to remove draft status — that decision belongs entirely to the user.

## 10. Close the loop

The canonical copies of `TRIAGE.md` and the plan file live on the default branch (see `SKILL.md` Step 11). The fix branch carries its own copy of the plan, but that's a snapshot the PR was opened against — it isn't the record you keep updating.

- Update the issue's row in `docs/issue-plans/TRIAGE.md`: status → `pr-open`, PR link in the `PR` column.
- Stamp the PR URL into the canonical plan file's `**PR:**` header field. The fix branch's copy doesn't need a self-referential update.
- **Land those edits on the default branch the same way Step 11 of Mode A does — via a short-lived docs branch and its own PR, with its own confirmation before pushing.** Never commit or push them straight to the default branch, and never treat the fix branch's earlier confirmation as covering this push too: it's a separate push, of different content, to a different branch, and it needs its own approval. It's easy to get this wrong because these edits are usually made outside the fix branch (in the main checkout, or wherever Mode A ran), so nothing about finishing the fix branch carries them anywhere automatically — but "it needs to get pushed" is not a license to bypass the gate.
- Until that docs PR lands, say so plainly rather than implying the loop is fully closed.
- Report the PR URL to the user and stop. Do not comment on the GitHub issue itself unless asked.

## Parallelizing across multiple issues

Fixing several issues at once means several branches with real source edits — those can't share one working directory. Two `--fix` flows in the same checkout would fight over which branch is checked out and could interleave writes to the same files.

**Isolate each issue in its own worktree, but split the work: subagents implement, you close the loop.**

Create each worktree yourself rather than relying on the Agent tool's `isolation: "worktree"` default — it gives you direct control over the exact branch name and base, which this skill's category-based prefix convention depends on:

```bash
git fetch origin
git worktree add ../<repo>-issue-<N> -b <prefix>/issue-<N>-<slug> origin/<default-branch>
```

Then spawn one subagent per worktree, each briefed with the full plan content directly in the prompt (fresh subagents have no context — don't assume they can read files from your current directory or an unrelated branch). **You already created the branch above** (step 3) — instruct the subagent to start at plan resolution and preconditions (steps 1-2), skip branch creation entirely (it's already done; redoing it would fail since the branch already exists, or move the subagent off the branch you set up), then implement and verify (steps 4-5). Explicitly tell it not to push or open a PR.

Once a subagent reports back, run steps 6-10 **yourself**, from your own shell, for that worktree:

1. Commit the plan/spec into the branch (step 6).
2. Re-verify independently (step 7) — don't skip this just because it ran unattended.
3. Push and open the draft PR only after showing the user the diff and getting confirmation (step 8) — this has to reach an actual human, which a background subagent cannot obtain on its own.
4. Update `TRIAGE.md` and the canonical plan file **in your own shell, not inside the worktree**, and land them via the docs-branch PR (step 10) — then remove the worktree:
   ```bash
   git worktree remove ../<repo>-issue-<N>
   ```
   Removing the worktree discards anything left uncommitted inside it — if the close-loop edits were made there instead of in your own shell, they're gone the moment this command runs. Batch every parallel issue's close-loop edits into one docs PR rather than opening one per issue.

If two issues' plans touch overlapping files, don't parallelize those two — implement them sequentially so the second one is grounded in the first one's actual result rather than a plan written against a codebase that already changed underneath it.
