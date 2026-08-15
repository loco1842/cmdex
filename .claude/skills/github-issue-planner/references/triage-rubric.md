# Triage Rubric

## Categories

Don't hardcode a category list — run `gh label list --json name,description` and categorize against whatever this repo actually uses. As of writing, `loco1842/cmdex` has:

- `bug` — something isn't working
- `enhancement` — new feature or request
- `documentation` — improvements or additions to docs
- `question` — further information requested
- `duplicate` — already exists elsewhere
- `invalid` — doesn't seem right
- `wontfix` — deliberately not being worked on
- `idea` — new idea, not yet committed to
- `dependencies` / `github_actions` / `javascript` — dependabot-generated, usually not worth a hand-written plan

Exclude `diffray-*` labels from triage entirely — they're review-automation status labels (`diffray-review-started`, `diffray-review-completed`, etc.), not issue categories, and will otherwise pollute the table.

If an issue carries none of these, or a repo has an entirely different taxonomy, infer the closest category from the title and body and note it plainly rather than forcing a bad fit.

**A label is a claim, not a fact.** Reporters (and auto-labelers) mis-tag issues — something filed as `question` or `enhancement` can turn out, once you actually read the code, to be a confirmed regression. If premise verification (below) contradicts the applied label, recategorize based on the evidence and say so plainly in `TRIAGE.md`'s notes, citing what you found. Don't let a pre-existing label substitute for checking.

## Priority

Assign from impact × reach first; use effort only to break ties.

| Priority | Meaning | Example |
|---|---|---|
| **P0** | Data loss, crash on launch, security issue, or the default branch is broken | DB corruption during a migration |
| **P1** | A core workflow is broken with no workaround, or this is a regression from something already shipped | Commands won't execute at all on one platform |
| **P2** | A meaningful improvement, or a bug that has a workaround | A missing keyboard shortcut in a frequently used flow |
| **P3** | Nice-to-have, cosmetic, or speculative | A theme color tweak |

**Tie-breaker:** a P2 issue at XS effort is usually worth planning before a P1 at XL effort — the small one clears the board almost for free, while the large one needs real runway regardless of urgency. Prioritization should account for this rather than sorting by priority alone.

## Effort

| Size | Files touched | Notes |
|---|---|---|
| **XS** | 1 | Single function or config change |
| **S** | 1–2 | One component or endpoint |
| **M** | 3–5 | One full feature slice |
| **L** | 5–8 | Multi-component; consider breaking down |
| **XL** | 8+ | Must be decomposed — do not plan as one unit |

## Escalation test

Escalate to the spec-driven-development skill — or "Fallback B" in `fallback-prompts.md` if that plugin isn't installed (see SKILL.md Step 8) — before writing a plan if **any** of these hold:

- Effort estimates at L or XL
- The change touches two or more independent subsystems (e.g. a database schema change *and* new frontend state, or backend execution logic *and* UI at once)
- It requires a schema migration or changes the signature of an existing Wails-bound method — those are hard to walk back once released
- The issue describes a *problem* without a specific desired *behavior*, and different reasonable engineers would build different things from it
- Reverting the change later would be expensive (data migrations, public API shape, anything users would build workflows on top of)

When none of these hold, plan directly — most issues (a UI tweak, a missing shortcut, a small bug with a clear repro) don't need the extra ceremony of a spec round-trip.

## Premise verification

Before writing a plan (SKILL.md Step 9), confirm the issue is still an accurate description of reality. What counts as sufficient evidence depends on category:

| Category | What "verified" looks like |
|---|---|
| **bug** | You read the exact code path the report implies and it plainly still produces the described behavior, *or* you reproduced it (ran the command, hit the UI flow). Check `git log --since="<issue's createdAt>" --oneline -- <path>` for commits touching that area since the issue was filed — a later change may have already fixed it. The `--since` bound matters: without it, a pre-issue commit can be mistaken for a later fix. |
| **enhancement / idea** | You searched the codebase for the requested capability under any name and confirmed it genuinely doesn't exist yet. A feature request for something already shipped is stale, not a gap. |
| **documentation** | You checked the current docs (`docs/`, `README.md`, inline comments) and confirmed the described gap or inaccuracy is still there. |
| **question / duplicate / invalid** | These rarely warrant a plan at all — confirm which one it actually is (answer it, point at the duplicate, or explain why it's invalid) rather than planning around an unclear ask. |

Evidence goes in the plan's Verification section as a citation (`file.ext:line`, a commit hash, or a described repro) — not as "confirmed" with nothing backing it up.

For anything beyond a one-line grep — an empty issue body, an unfamiliar area of the codebase, or a claim that needs git-history archaeology to confirm (was this already fixed? when did it break?) — delegate the research to a subagent rather than skimming it inline. A dedicated research pass is what turns up the things a quick look misses: the exact commit that caused a regression, a shared helper function that looks like it's part of the code being removed but isn't, or a second call site nobody thought to check.

**When verification fails** (doesn't reproduce, already fixed, already exists): don't write a plan. Mark the issue `disputed` in `TRIAGE.md` with the one-line reason, and let the user decide next steps — closing or commenting on the issue is never done unilaterally.

**When verification is inconclusive** (platform-specific, needs the reporter's environment, insufficient information): say so plainly and ask the user whether to proceed speculatively or wait for more detail. Guessing and writing a confident-looking plan anyway is the failure mode this step exists to prevent.
