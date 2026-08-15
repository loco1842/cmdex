---
name: github-issue-planner
description: "Scans GitHub issues with `gh issue list`, categorizes and prioritizes them, and writes a per-issue implementation plan containing concrete code changes to docs/issue-plans/. Pass --fix to implement an already-written plan and open a draft PR. Use this skill whenever the user mentions GitHub issues, issue triage, prioritizing the backlog, 'what issues are open', 'plan issue #N', 'turn this issue into a PR', or 'fix issue N'. Also triggers on 'gh issue', 'triage', 'issue backlog', and any request to work through reported bugs or feature requests."
---

# GitHub Issue Planner

Turn the GitHub issue backlog into reviewed, actionable work: scan open issues, categorize and prioritize them, then write a concrete implementation plan per issue — grounded in the real codebase, not invented. Only when explicitly asked with `--fix` does this skill touch source code, and even then it stops for confirmation before pushing anything.

**Announce at start:** "Using github-issue-planner to work through GitHub issues."

## Why plan and fix are separate modes

Writing a plan is cheap and reversible — reading, thinking, and producing markdown. Implementing it touches source, git history, and eventually a public PR. Collapsing those into one step means a misread issue or a bad assumption turns into a wrong diff before anyone reviewed the reasoning. Keeping them separate means the plan is the thing that gets reviewed, and `--fix` just executes what was already approved.

## Argument parsing

Arguments arrive as free text after the skill invocation. Parse permissively and echo back what you understood before doing anything, so a misparse is caught immediately rather than after fetching everything:

| Input | Effect |
|---|---|
| *(nothing)* | Triage all open issues, write the index, then ask which to plan |
| `56`, `#56`, `56 58` | Scope straight to those issue numbers, skip the "which ones?" question |
| `--fix` | Implementation mode — requires a plan already written for the target issue |
| `--label bug` | Passed through to `gh issue list --label` |
| `--limit N` | Default 30 |
| `--state open\|closed\|all` | Default `open` |
| `--assignee @me` | Passed through |

`--fix` with no issue number: if exactly one plan in `docs/issue-plans/` has status `planned`, use it. If more than one qualifies, list them and ask which.

## Mode A — Plan (default)

### Step 1: Preflight

Run `gh auth status` and `gh repo view --json nameWithOwner,defaultBranchRef`. If `gh` isn't installed or isn't authenticated, say so plainly and stop — don't try to scrape issues any other way. Keep the `nameWithOwner` and default branch around; both get used later, and neither should be assumed from the local directory name (it commonly differs from the repo name).

### Step 2: Discover the label taxonomy

```bash
gh label list --json name,description
```

Categorize against whatever labels actually exist in *this* repo, not a fixed list baked into this skill — that's what keeps it correct across different projects.

### Step 3: Fetch issues

```bash
gh issue list --state open --limit 30 \
  --json number,title,labels,state,createdAt,updatedAt,author,assignees,url,comments
```

Honor `--label`, `--limit`, `--state`, `--assignee` overrides from the parsed arguments.

### Step 4: Read each candidate in full

```bash
gh issue view <N> --json number,title,body,labels,comments,author,url,createdAt
```

The title is rarely the whole story — the body and comment thread usually carry the actual requirement, edge cases the reporter already ran into, and sometimes a maintainer's steer on the intended fix. Read all of it before categorizing.

### Step 5: Categorize and prioritize

Apply the rubric in `references/triage-rubric.md` — category (from the repo's real labels), priority P0–P3, effort XS–XL, and whether the issue needs to escalate to a spec before planning.

### Step 6: Write or update the triage index

Merge into `docs/issue-plans/TRIAGE.md` rather than overwriting it — issues already past `planned` keep their status and any links they've accumulated (plan file, spec, PR).

```markdown
# Issue Triage

_Last updated: <date> · repo: <owner>/<repo> · <N> open issues_

| # | Title | Category | Priority | Effort | Plan | Status |
|---|-------|----------|----------|--------|------|--------|
| [56](https://github.com/<owner>/<repo>/issues/56) | ctrl+enter in spotlight | enhancement | P2 | S | [plan](issue-56-ctrl-enter-spotlight.md) | planned |
```

Status progresses: `triaged` → `planned` → `in-progress` → `pr-open` → `merged` / `wontfix`.

### Step 7: Present and select

Print the table. If the user didn't name specific issue numbers, ask which ones to write full plans for — planning every open issue burns a lot of tokens on things the user may not care about right now. If they did name numbers, skip straight to Step 8 for those.

### Step 8: Escalation gate

Before writing a plan, check the escalation test in `references/triage-rubric.md`. When an issue is large, spans independent subsystems, needs an architectural or schema decision, or the requirement is genuinely ambiguous, don't guess your way through it — invoke the **spec-driven-development** skill to interview the user properly first.

When escalating, direct that skill's output to `docs/issue-plans/issue-<N>-<slug>.spec.md` — **not** a root `SPEC.md`. If the project already has a root spec file, it almost certainly documents something else, and overwriting it would destroy unrelated context. Link the spec from the plan header once it's approved, and don't proceed to Step 9 until the user has signed off on it.

### Step 9: Write the plan

For each selected issue:

1. **Ground it in real code first.** Read the actual files the change would touch. Every code snippet and file reference in the plan needs to match what's really on disk (cite as `path/file.ext:line`) — a plan built on invented function names or guessed file paths is actively worse than no plan, because it looks trustworthy and isn't.
2. **Borrow the planning-and-task-breakdown skill's decomposition discipline** — vertical slices over horizontal layers, per-task acceptance criteria and verification steps, dependencies made explicit, checkpoints every 2–3 tasks, nothing sized past ~5 files.
3. **Redirect its output.** That skill defaults to writing `tasks/plan.md` and `tasks/todo.md`. Those paths may already hold unrelated in-flight work in this project, so when invoking it, say explicitly that the plan for this issue goes to `docs/issue-plans/issue-<N>-<slug>.md` instead, and that the default `tasks/` targets are off-limits for this run.
4. Fill out `references/plan-template.md`.
5. Update the issue's row in `TRIAGE.md` to `planned`, linking the new plan file.

**Slug rule:** strip any conventional-commit prefix (`feat:`, `fix:`, etc.) from the title, kebab-case what's left, drop filler words, cap around 40 characters. `"feat: ctrl+enter shortcut in spotlight search"` → `ctrl-enter-spotlight`.

### Step 10: Report

Summarize what was written (triage index, plan files, any spec), and state the next step verbatim, e.g. `github-issue-planner --fix 56`.

## Mode B — `--fix`

Full mechanics, including verification-gate and PR-body details, live in `references/fix-mode.md`. The spine:

1. **Require an existing plan.** `docs/issue-plans/issue-<N>-*.md` must already exist. If it doesn't, stop and say to run the skill without `--fix` first — this mode never plans from scratch.
2. **Check preconditions.** Clean working tree (`git status --porcelain`, tolerating only files under `docs/issue-plans/`), and `gh auth status` passing.
3. **Branch off the default branch**, prefixed by category (`fix/` for bug, `feat/` for enhancement, `docs/` for documentation, `chore/` otherwise) — matching this repo's existing branch-naming convention.
4. **Implement task-by-task**, one commit per task, using the incremental-implementation skill's discipline: implement, test, verify, commit, move on. Never `git add -A`; stage only what the current task touched.
5. **Run the project's real verification gates** (discovered from its build tooling — for this repo, that means `make check`, `make lint`, `go test ./...`, and the frontend e2e suite when frontend files changed) after implementation. If anything fails, stop and report the failure — do not push a red branch.
6. **Show the diff and commit list, and get explicit confirmation before pushing.** A push to a shared, public repo is outward-facing and effectively irreversible — one confirmation is a small cost for that.
7. **Open a draft PR** — never non-draft, never auto-merge — linking the issue (`Fixes #<N>`) and the plan file, with the task checklist and verification evidence in the body.
8. **Close the loop**: update `TRIAGE.md` status to `pr-open` with the PR link, and stamp the PR URL back into the plan file.

## Guardrails

These aren't arbitrary restrictions — each protects something that would be expensive or embarrassing to undo:

- **Never write to `tasks/plan.md` or `tasks/todo.md`** unless the user explicitly says those are free — they're the default output of the planning skill this one borrows from, and in this project they hold unrelated live work.
- **Never overwrite an existing root `SPEC.md`** — escalated specs go under `docs/issue-plans/`.
- **Never `git add -A`** — stage exactly the files the current task touched.
- **Never push to the default branch, never merge, never mark a PR ready-for-review** — `--fix` produces a draft PR and stops.
- **Never close, label, assign, or comment on an issue without asking first** — those actions are visible to other people on the repo and can't be quietly undone.
- **Never edit source code outside `--fix` mode.** Plan mode reads and writes markdown only.

## See also

- `references/triage-rubric.md` — category mapping, priority/effort scales, escalation test
- `references/plan-template.md` — the exact plan file structure to fill in
- `references/fix-mode.md` — branch naming, verification commands, PR body template
