# Fallback Prompts

This skill delegates three things to the `agent-skills` plugin: task decomposition (`planning-and-task-breakdown`), spec interviews (`spec-driven-development`), and implementation discipline (`incremental-implementation`). Those are the right tools when present — they're better developed than anything restated here, and they get maintained independently.

But they're a plugin, not a guarantee. If it isn't installed or enabled in the current setup, use the self-contained versions below rather than skipping the discipline entirely. **Check availability first** — look for the skill in the available-skills list; if it's there, use it and ignore this file.

This exists because the alternative is a dangling reference. This project's own `todo-scanner` skill instructs the model to "use the **writing-plans** skill," and that skill isn't installed anywhere in this setup — so that step silently degrades into improvisation every time it runs. Not repeating that here.

---

## Fallback A — Task decomposition

*Substitutes for `agent-skills:planning-and-task-breakdown` (what `/agent-skills:plan` runs).*

Stay read-only. The output is a plan document, not code.

**1. Map the dependency graph.** What has to exist before what? Implementation order follows it bottom-up — foundations first.

**2. Slice vertically, not horizontally.** Each task should deliver one complete working path through the stack, not one layer across the whole feature.

```
Bad  (horizontal):  Task 1: all the schema · Task 2: all the API · Task 3: all the UI
Good (vertical):    Task 1: user can create an account (schema + API + UI)
                    Task 2: user can log in (schema + API + UI)
```

Horizontal slices can't be verified until the last one lands; vertical slices each leave the system working.

**3. Write each task in this shape:**

```markdown
### Task N: <short descriptive title>

**Files:** create/modify `path/to/file.ext`
**Depends on:** <task numbers, or none>

- [ ] Step 1: <specific action, with real code>

**Acceptance:** <specific, testable condition — not "works correctly">
**Verify:** <the actual command to run> and <manual check, if relevant>
```

**4. Size every task.** 1 file = XS · 1-2 = S · 3-5 = M · 5-8 = L · 8+ = XL. **Anything at L or larger gets broken down further** — agents perform best on S and M. Other signals a task is too big: you can't state its acceptance criteria in three bullets, it spans two independent subsystems, or its title contains "and."

**5. Checkpoint every 2-3 tasks** with a concrete gate:

```markdown
### Checkpoint: after Task N
- [ ] Build passes, tests pass
- [ ] <the user-visible behavior that should now work end-to-end>
```

**6. Order for fail-fast.** Dependencies satisfied first, each task leaving the system working, highest-risk work early — discovering a plan is wrong is far cheaper on task 2 than task 9.

**Red flags:** a task that says "implement the feature" with no acceptance criteria; no verification step; every task sized XL; no checkpoints; dependency order ignored.

---

## Fallback B — Spec interview

*Substitutes for `agent-skills:spec-driven-development` (what `/agent-skills:spec` runs). Used by SKILL.md Step 8 when an issue is too large or ambiguous to plan directly.*

The point is to replace a guess with a decision the user actually made. Do not write the spec and then ask for a rubber stamp — ask first.

**1. Surface your assumptions before anything else.** State them plainly and invite correction:

```
ASSUMPTIONS I'M MAKING:
1. <assumption about the requirement>
2. <assumption about scope — what's explicitly not included>
3. <assumption about the approach or constraints>
→ Correct me now, or I'll proceed with these.
```

**2. Ask clarifying questions until the requirement is concrete** — specifically wherever two reasonable engineers would build different things from the same description. Prefer offering two or three concrete options with their trade-offs over an open-ended "what do you want?", which pushes the design work back onto the user.

**3. Write the spec** with these sections, and no more:

- **Objective** — what we're building and why, in a paragraph
- **Scope** — in scope / explicitly out of scope
- **Success criteria** — observable, testable conditions for "done"
- **Boundaries** — Always / Ask first / Never
- **Open questions** — anything still unresolved

**4. Gate on approval.** Do not proceed to planning until the user signs off. That gate is the entire value of this step — skipping it converts a spec into an expensive-looking guess.

Save to `docs/issue-plans/issue-<N>-<slug>.spec.md` — never a root `SPEC.md`, which usually documents something else entirely.

---

## Fallback C — Implementation discipline

*Substitutes for `agent-skills:incremental-implementation` (what `/agent-skills:build` runs). Used by `fix-mode.md` step 4.*

Per task, in order:

1. **Implement** the smallest complete slice of that task
2. **Test** — run the relevant test/build step
3. **Verify** it does what the acceptance criteria actually describe
4. **Commit** — one commit per task, conventional-commit subject, staging only the files that task touched (never `git add -A`)
5. **Move on** — carry forward, don't restart

**Keep it compilable.** The tree should build and pass at every commit, not just at the end — a branch that only works after the final commit can't be bisected, reviewed incrementally, or safely abandoned partway.

**Simplicity first.** Prefer the boring, obvious implementation. If you've written far more code than the task seems to warrant, stop and reconsider before continuing.

**Scope discipline.** Touch only what the task requires. When you notice something else worth fixing, report it instead of fixing it:

```
NOTICED BUT NOT TOUCHING:
- <file:line> — <what's wrong> (out of scope for this task)
```

That list is genuinely valuable output — it's how real issues get found — but folding those fixes into this diff makes the change harder to review and harder to revert.

**When the plan turns out to be wrong** — an acceptance criterion can't be met as written because the plan assumed something untrue about the code — stop and report the discrepancy rather than improvising a different approach. The plan was reviewed; a departure from it should be too.
