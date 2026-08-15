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

Escalate to the spec-driven-development skill (see SKILL.md Step 8) before writing a plan if **any** of these hold:

- Effort estimates at L or XL
- The change touches two or more independent subsystems (e.g. a database schema change *and* new frontend state, or backend execution logic *and* UI at once)
- It requires a schema migration or changes the signature of an existing Wails-bound method — those are hard to walk back once released
- The issue describes a *problem* without a specific desired *behavior*, and different reasonable engineers would build different things from it
- Reverting the change later would be expensive (data migrations, public API shape, anything users would build workflows on top of)

When none of these hold, plan directly — most issues (a UI tweak, a missing shortcut, a small bug with a clear repro) don't need the extra ceremony of a spec round-trip.
