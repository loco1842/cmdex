# Plan File Template

Save to `docs/issue-plans/issue-<N>-<slug>.md`. This mirrors the existing plan style already used in this project (`docs/superpowers/plans/*.md`: Goal / Architecture / File Map / numbered tasks with checkbox steps and inline code), with issue metadata added at the top so the file is self-contained without needing to open GitHub.

````markdown
# Issue #<N>: <title, conventional-commit prefix stripped>

**Issue:** <github issue URL>
**Category:** <category> · **Priority:** P<0-3> · **Effort:** <XS-XL> · **Status:** planned
**Author:** @<username> · **Opened:** <date>
**Spec:** <link, only present when this issue was escalated>
**PR:** <filled in later by --fix, once opened>

## Problem

What the reporter actually needs, restated in your own words — not just a copy
of the issue title. Include reproduction steps for bugs, or the concrete gap
being asked for on feature requests.

## Verification

Evidence that this issue is still real *before* any code was planned against
it — not a restatement of the Problem section. Cite what you checked:

- **Reproduced/confirmed by:** <e.g. "read `Spotlight.tsx:40-58`, no keydown
  handler for Enter while a modifier is held" or "ran the repro steps, still
  fails on current `main`">
- **Checked for a prior fix:** `git log --since="<issue's createdAt>" --oneline -- <path>`
  — <found nothing relevant / found X, doesn't cover this>
- **Checked it doesn't already exist (feature requests only):** <where you
  searched and what you found>

If this section can't be filled in with real evidence, stop — don't write the
rest of the plan. Flag the issue as `disputed` in TRIAGE.md instead (see
`references/triage-rubric.md`, "Premise verification").

## Current behavior

Where this lives in the code today, with real citations: `path/to/file.tsx:123`.
This section should read like something you found by actually opening the
file, not like a plausible guess.

## Approach

One paragraph describing the chosen approach. Name at least one alternative
you considered and rejected, and why — that reasoning is the expensive part
to reconstruct later if someone questions the approach during review.

## File map

| File | Change |
|---|---|
| `path/to/file.ext` | What changes here and why |

## Tasks

Follow the planning-and-task-breakdown skill's task structure (or `fallback-prompts.md`
"Fallback A" if that plugin isn't installed) — each task
sized to land in one focused session (roughly 1-2 hours, touching at most
~5 files), with real code in the steps rather than descriptions of code.

### Task 1: <short descriptive title>

**Files:** create/modify `path/to/file.ext`
**Depends on:** none (or a prior task number)

- [ ] Step 1: <specific action>

  ```<language>
  // concrete code, grounded in the real current file — not pseudocode
  ```

**Acceptance:** <a specific, testable condition — not "works correctly">
**Verify:** <the actual command to run, e.g. `make check`> and <manual check
description, if relevant>

### Task 2: <title>
...

### Checkpoint: after Task <N>

- [ ] `make check` passes (or the project's equivalent build+typecheck gate)
- [ ] Relevant tests pass
- [ ] Behavior confirmed manually (describe how)

## Risks

| Risk | Impact | Mitigation |
|---|---|---|

## Out of scope

List things this plan deliberately does NOT do, even if they're adjacent or
tempting. This is the main defense against `--fix` quietly growing the diff
beyond what the issue actually asked for.

- <example: not refactoring the surrounding component, only adding the handler>

## Open questions

Anything that needs a human answer before or during implementation. Empty is
fine — it just means nothing came up.

- <question, if any>
````
