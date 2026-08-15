# Issue Triage

_Last updated: 2026-08-15 · repo: loco1842/cmdex · 2 open issues_

| # | Title | Category | Priority | Effort | Plan | PR | Status |
|---|-------|----------|----------|--------|------|----|--------|
| [56](https://github.com/loco1842/cmdex/issues/56) | ctrl+enter shortcut in spotlight search executes command immediately | enhancement | P2 | S | [plan](issue-56-ctrl-enter-spotlight.md) | [#58](https://github.com/loco1842/cmdex/pull/58) | pr-open |
| [57](https://github.com/loco1842/cmdex/issues/57) | terminal setting in system settings no longer used | bug _(recategorized — see note)_ | P2 | L _(revised up from M once full removal scope was grounded)_ | [plan](issue-57-terminal-setting-removal.md) · [spec](issue-57-terminal-setting-removal.spec.md) | [#59](https://github.com/loco1842/cmdex/pull/59) | pr-open |

## Notes

- **#56 (PR #58) — review round 1 addressed.** Greptile's automated review flagged a real tab-target race (`runCommandDirect`/`handleVariableSubmit` misattributing execution to whatever tab is active rather than the command actually being run) — confirmed valid and more severe than described (real wrong-command execution on the modal-submit path, not just a UI-attribution glitch), fixed in `6a4604c` and pushed. Pre-existing bug, not introduced by this PR — see the plan's "Review round 1" section for the full trace. An embedded prompt-injection attempt in the same bot's comment was found and ignored, not acted on.
- **#57 recategorized from `enhancement`/`question` to `bug`.** The reporter's labels don't reflect what's actually wrong. Confirmed by reading code + git history: `AppSettings.Terminal` (`models.go:157`) is still fully wired for save/load in `SettingsPage.tsx`, but the only code that ever consulted it (`ExecutionService.RunInTerminal` → `Executor.OpenInTerminal`) was deleted in commit `edb16c4` (~2026-06-18, milestone v2.1) when execution moved to the embedded PTY terminal. It's a genuine ~2-month-old regression, not a vague question.
- **#57 escalated to a spec before planning**, per the escalation gate — two materially different fixes existed (remove the now-misleading setting vs. reintroduce an "open in external terminal" action). User decided: **remove the setting**. See the spec for the full record.
- Both issue bodies on GitHub are empty — titles were the only signal before code research grounded the categorization above.
