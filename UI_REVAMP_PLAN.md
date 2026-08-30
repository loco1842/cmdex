# Cmdex UI Revamp — Implementation Plan

## Context

Cmdex's UI today is largely default shadcn/ui styling on top of a hand-written 3040-line
`frontend/src/style.css` — functional but visually generic. A "Refined IDE" direction was mocked
(Manrope UI font + JetBrains Mono, dark-first neutral-blue palette with a single violet accent,
1px borders, small radii, precise spacing — Zed/Warp/Linear in spirit) and published as a design
canvas: https://claude.ai/code/artifact/e7819dd9-1be7-43fe-bd9e-dfd4cd71ff58. Token values and
component-level decisions are recorded in `DESIGN.md` (repo root).

The user wants this to **replace** the current default look (not ship as an opt-in extra theme),
while keeping all 8 existing built-in themes selectable and unchanged in identity for anyone who
picked one deliberately. Two rounds of codebase exploration (three parallel Explore agents, one
Plan-validation agent) surfaced hard constraints and a few pre-existing bugs the user asked to fix
alongside this work; both are reflected below.

**Confirmed constraints that shape the approach:**
- `style.css` themes use semantic CSS class names (`.tab-item`, `.cmd-title`, …) that ~177 Playwright
  locators depend on directly. Restyling **in place** (new rules, same class names) keeps the
  existing 131 e2e tests green; renaming/removing classes or wrapping elements in new containers
  does not.
- The script editor (`HighlightedTextarea` inside `CommandDetail.tsx`) is a `<textarea>` + a
  scroll-synced transparent backdrop `<div>` for `{{var}}` highlighting. True inline chips would
  require replacing the editor (CodeMirror/Lexical) — **out of scope**, per user decision. The
  **read-only preview** path (`renderScriptUnified`, plain `<span>`s) can be restyled into real
  pill chips today.
- Theme colors are hex/rgba throughout (no oklch/hsl) — DESIGN.md's OKLCH values need converting.
- Terminal ANSI colors are hardcoded VS Code Dark+ in `Terminal.tsx` regardless of active theme;
  only 5 of the terminal's color keys currently react to theme changes.
- The theme-token-key list is duplicated across `theme-apply.ts`, `SettingsPage.tsx` (three times:
  `allVarKeys`, `defaultTheme`, `THEME_DOTS`), and `importexport_service.go`.
- `index.html` hardcodes `class="dark"` on `<html>`, permanently activating Tailwind `dark:`
  utilities even under light themes.
- The real default theme/font are seeded by **Go** (`db.go` `GetSettings`/`ResetAll`, and
  migration `0009_settings_json.go`'s fresh-install insert) — changing `main.tsx`'s frontend
  fallback strings alone is a no-op, since the backend always returns a populated settings row.

## Phase 0 — Foundation (no visible restyle yet; infra only)

1. **Canonical token lists.** New `frontend/src/lib/themeTokens.ts` exporting three derived
   arrays instead of one flat list (a flat merge would silently change behavior):
   - `ALL_THEME_TOKEN_KEYS` — every token a theme block defines (today's 35, growing to 43 once
     ANSI tokens are added in Phase 1). Used for auditing/completeness only.
   - `CUSTOM_OVERRIDABLE_KEYS` — today's 27 (`theme-apply.ts`'s current `CUSTOM_THEME_VAR_KEYS`).
     This is the set a *custom theme* may set and that `applyTheme`'s clear-loop must reset —
     keep `--var-filled-*`/`--var-missing-*`/`--scrollbar-*`/`--ansi-*` **out** of it, matching
     today's behavior, unless a later decision explicitly wants custom themes to control those.
   - `EXPORT_TEMPLATE_KEYS` — the 15 keys `importexport_service.go`'s `SaveThemeTemplate` writes.
   `theme-apply.ts` and `SettingsPage.tsx` (`allVarKeys`) both import from this file instead of
   keeping local copies. Add a comment in `importexport_service.go` cross-referencing it.

2. **Fix the `dark:` class bug at its single source.** Move the dark/light class decision into
   `applyTheme()` itself (`theme-apply.ts`) — derive `type` from `THEMES.find(id)?.type ??
   customTheme?.type ?? 'dark'` (the same derivation `main.tsx:90` already does inline) and call
   `document.documentElement.classList.toggle('dark', type === 'dark')` there. This collapses what
   would otherwise be six call sites needing the same fix (`App.tsx:446`, `main.tsx:71/91/166`,
   `SettingsPage.tsx:306/328`, plus the OS-color-scheme listener at `App.tsx:456`) into one.
   Leave `index.html`'s `class="dark"` as its initial value (matches the dark default, avoids a
   flash) — `applyTheme` only ever needs to *remove* it for light themes.

3. **Bundle Manrope** (400/500/600 — 600 covers the app's real `font-semibold`/`600` usage; skip a
   dedicated 700 file and demote the handful of `font-weight: 700` chrome sites to 600 during
   Phase 1, matching the "Refined IDE" weight palette). Add `.woff2` files under
   `frontend/public/fonts/manrope/`, `@font-face` blocks at the top of `style.css` (same pattern as
   Inter/Geist/Nunito), and an entry in `UI_FONTS` (`SettingsPage.tsx:25-31`).
   **Also backfill `Inter-SemiBold.woff2` (600)** — existing users keep `uiFont: "Inter"` after
   upgrading (see Phase 1's migration), so without a real 600 weight most upgrading users would see
   browser-synthesized bold on the new chrome. Same `@font-face` treatment.

4. **Thread the mono font into the terminal.** `TerminalComponent` currently takes no font prop
   (`Terminal.tsx:11-24`, called from `App.tsx:1762` with `theme` only) and hardcodes
   `fontFamily: 'JetBrains Mono, Fira Code, monospace'` at construction. Add a `monoFont: string`
   prop threaded from `App.tsx` (which already has `monoFont` in scope), read it at construction
   and in the existing theme-reactive `useEffect` (currently keyed `[theme]` — re-key to
   `[theme, monoFont]`), and call `fitAddonRef.current?.fit()` after mutating
   `term.options.fontFamily` (cell metrics change size, so a stale fit under-sizes the PTY).

**Phase 0 verification:** `make check`, `make lint`, full `make test` — no behavior change is
expected yet beyond the dark-class fix, so specifically re-check any Playwright assertion touching
`vscode-light`/other light themes still passes (this phase turns on previously-dead `dark:` CSS,
which is a real bugfix, not pure refactor).

## Phase 1 — New themes, ANSI colors, and the real (Go-side) default

1. **ANSI tokens, added while authoring/touching each theme block.** Add 8 new tokens per theme —
   `--ansi-black/red/green/yellow/blue/magenta/cyan/white` — **not** 6: reusing
   `--background`/`--foreground` for black/white was considered and rejected, because under
   `vscode-light` that would render ANSI black as white-on-white. All 8 must resolve to a literal
   `#rrggbb` (no `var()`-nested `color-mix()`/`oklch()` — `getComputedStyle` returns those
   unresolved, which breaks both the mixer below and the existing `hexToRgba` call). For the 8
   existing themes, seed sensible values per theme's own hue family (e.g. `vscode-dark`/`:root`
   can reuse its current hardcoded xterm hex values verbatim as a zero-risk starting point).
   In `Terminal.tsx`'s theme-reactive effect, read all 8 plus `--background`/`--foreground`, and
   compute the 8 "bright" variants by mixing ~25% toward `--foreground` (not white — mixing toward
   white destroys contrast on light themes; mixing toward foreground auto-flips with theme
   polarity). This is an intentional simplification (procedural, not 16 hand-tuned values); the
   goal is "the terminal stops looking wrong under other themes," not VS Code color parity.

2. **Author `cmdex-dark` and `cmdex-light`** as new `[data-theme="..."]` blocks in `style.css`
   (43 tokens each: today's 35 + the 8 ANSI), colors derived from `DESIGN.md`'s OKLCH palette
   converted to hex. Preserve the existing green=filled/amber=missing variable-pill convention
   (the mockup didn't render a "missing" state, so don't copy it as undifferentiated violet).

3. **Swap `:root`'s identity.** Make `:root`'s literal values equal `cmdex-dark`'s (the new
   implicit default, mirroring how `vscode-dark` is special-cased today), and give `vscode-dark`
   its own explicit `[data-theme="vscode-dark"]` block with today's exact `:root` values — so nothing
   changes for a user who has `vscode-dark` saved. Note the accepted side effect: a custom theme
   (`data-theme="custom-<id>"`, no CSS rule of its own) inherits whatever `:root` provides for any
   key it omits — after this swap that base becomes `cmdex-dark` instead of `vscode-dark`. This is
   a real, user-visible change for anyone with a partial custom theme; it's accepted as part of
   "replace the default," not something to code around.

4. **Add a radius scale** (`--radius-sm`/`-md`/`-lg` in `:root`, values from DESIGN.md) for Phase 2
   slices to adopt as they touch each section — don't mass-replace all ~43 existing hardcoded
   `border-radius: Npx` declarations in one sweep; only rules actually touched by a Phase 2 slice
   move onto the scale, keeping each diff reviewable.

5. **The real default lives in Go — fix it there, not just in `main.tsx`.** `main.tsx`'s
   `s.theme || 'vscode-dark'` fallback never fires today because `GetSettings()` always returns a
   populated row. Concretely:
   - `db.go`'s `GetSettings()` default block (~line 1312) and `ResetAll()`'s re-seed
     (~lines 1435-1437): `Theme`/`LastDarkTheme` → `"cmdex-dark"`, `LastLightTheme` →
     `"cmdex-light"`, `UIFont` → `"Manrope"`.
   - **New migration `migrations/0011_default_theme.go`** (append to `Migrations` in
     `migrations/migration.go`; do not touch `0008`/`0009` — they're applied history and `0009`'s
     `Down` is a live rollback path). Settings live as one JSON blob (`app_settings.data`, since
     migration `0009`) — **unmarshal into `map[string]interface{}`, not a narrow struct**: `0009`'s
     own `settingsV9` struct is already stale (it predates `windowX/Y/Width/Height`), which is
     exactly the trap a typed round-trip falls into. Conditionally rewrite only `theme` →
     `"cmdex-dark"` and `lastLightTheme` → `"cmdex-light"` **if and only if** the current value is
     the untouched default (`theme == "vscode-dark" && lastDarkTheme == "vscode-dark"`), leaving
     `lastDarkTheme` as `"cmdex-dark"` too. This ships the new look to everyone who never changed
     their theme while leaving anyone who deliberately picked a different theme alone (there's no
     way to distinguish "never touched the default" from "deliberately re-selected vscode-dark" —
     this predicate is the accepted approximation). Leave `uiFont` untouched by the migration
     (font is a stronger personal preference; Phase 0's Inter-SemiBold backfill keeps Inter users
     looking correct).
   - Update `importexport_service.go`'s `SaveThemeTemplate` example values to match `cmdex-dark`.
   - Add a migration test case alongside the existing ones in `db_test.go`
     (`TestFreshDBMigrations`/`TestRollbackTo` patterns).

6. **Wire up the frontend side of the new default:** add `cmdex-dark`/`cmdex-light` to `THEMES`
   (`types.ts`) and `THEME_DOTS` (`SettingsPage.tsx:39-40`); update `main.tsx`'s fallback strings
   for consistency (defense in depth, even though Go is now the real source); set the sidebar's
   default width `280 → 264` at its actual prop (`App.tsx:1585`'s `ResizablePanel defaultWidth`) —
   not a CSS constant, since width is user-resizable/persisted and the three existing
   `--sidebar-width`/`--header-height`/`--tab-bar-height` CSS vars are already dead (zero `var()`
   references) and out of scope to touch here.

7. **Update the e2e mock's seeded defaults** (`frontend/e2e/mocks/runtime.ts:87-92`,
   `theme`/`uiFont`) to `cmdex-dark`/`Manrope` — otherwise every Phase 2 verification gate exercises
   the restyle under the *old* theme/font, not the one it was designed for. `themes.spec.ts` seeds
   its own theme explicitly per test and is unaffected.

**Phase 1 verification:** `go test ./...` (migration tests especially), full frontend `make test`,
manual `wails3 dev` check that a fresh profile boots into `cmdex-dark`/Manrope and that an existing
`vscode-dark` profile is unaffected (except via the migration's intended flip).

## Phase 2 — Component-by-component restyle

**Phase 2.0 — close a mockup gap found during planning.** The 6 published mockup screens all
assumed a command with title + description already filled in, and never showed two real,
substantial pieces of `CommandDetail.tsx`: the **tags feature** (per-command tag badges with
add/edit/remove — `.tag-badge`/`.tag-add-btn`/`.tag-edit-input`/`.tag-remove-btn`,
`CommandDetail.tsx:702-789`; not documented in `CLAUDE.md` but real, shipped code) and the
**hover-reveal "Add title" / "Add description" / "Add tags" pill affordances** shown when those
fields are empty/collapsed (`.add-title-pill`, `.add-field-pill-anchor`,
`CommandDetail.tsx:675-697, 857-864`). Before restyling `CommandDetail.tsx` (item 3 below), add a
7th artboard to the design canvas (same "Refined IDE" tokens) showing: a command with an
empty/collapsed title and description (reveal pills visible on hover) and a populated tags row
with the add-tag control — so the restyle has a real visual target instead of improvised styling.
Update `DESIGN.md`'s component-decisions and "Not yet designed" sections accordingly.

Each slice: edit only that component's CSS section/JSX (no class renames), verify visually in
`wails3 dev`, then run its Playwright spec(s) before moving to the next. Reuse the radius scale
from Phase 1.4 where a rule is touched.

| Order | Component(s) | style.css section | Verify with |
|---|---|---|---|
| 1 | `TabBar.tsx` + `TerminalTabBar.tsx` (share `.tab-bar/.tab-item/.tab-title/.tab-close/.tab-status-dot`) | Tab Bar | `tabs.spec.ts`, `terminal.spec.ts`, `palette-shortcuts.spec.ts` |
| 2 | `Sidebar.tsx` — active row as left accent bar not full fill; replace hardcoded `#7c6aef`/`#6c6c88` fallbacks with `var(--primary)`/`var(--muted-foreground)` | Sidebar | `sidebar.spec.ts`, `categories.spec.ts`, `commands.spec.ts` |
| 3 | `CommandDetail.tsx` preview-mode chips (`.var-filled`/`.var-missing`/`.var-placeholder-muted` → real pill radius/padding, keep green/amber hues), preset chips, preview panel, **tags row + add-tag control, and the hover-reveal add-title/description/tags pills** (per Phase 2.0's new mockup); restyle (don't replace) `HighlightedTextarea`'s edit-mode `.var-highlight` box | Command Detail, Preset Chips | `variables-execution.spec.ts`, `presets.spec.ts`, `commands.spec.ts` (tag-related assertions) |
| 4 | `FloatingSaveBar.tsx` — tune colors/radius only, already uses a `color-mix` glass-pill pattern | Inline edit/hover/floating save | dirty-state tests in `commands.spec.ts`/`tabs.spec.ts` (testid-only, low risk) |
| 5 | `Terminal.tsx`/`.terminal-pane`/`.terminal-container` chrome padding (ANSI/font already done in Phase 1) | Terminal Split Pane | `terminal.spec.ts` |
| 6 | `CommandPalette.tsx` (clean `.palette-*` namespace) | Command Palette | `palette-shortcuts.spec.ts` |
| 7 | `VariablePrompt.tsx` | Variable Prompt (fill/manage) | `variables-execution.spec.ts`, `presets.spec.ts` |
| 8 | `WelcomeTab.tsx` | Welcome Tab | quick manual check + `i18n.spec.ts` |
| 9 | `SettingsPage.tsx` — **migrate to vertical nav first** (`<Tabs orientation="vertical">`, keeps Radix's `role="tablist"`/`role="tab"` so `getByRole` locators keep working; only Left/Right vs Up/Down arrow-nav changes, which no test exercises), **then** restyle appearance/typography/general panels with new tokens | (new left-nav layout) | `settings.spec.ts`, settings-tab assertions in `i18n.spec.ts` |

## Phase 3 — Regression pass

- Full `make test` (`go test ./...` with `-race`, Vitest, Playwright) — all green except the 4
  pre-existing `test.fixme()`s, unchanged.
- Manual `wails3 dev` smoke test: cycle all 10 themes (8 existing + 2 new) and both density and
  all font pickers; confirm light themes now render shadcn primitives correctly (Phase 0.2's fix);
  confirm terminal ANSI colors and font follow the active theme/setting; run a command end-to-end;
  open the command palette and variable prompt; open Settings and confirm the new left nav; import
  and export a custom theme to confirm the consolidated token list still round-trips.
- Update `DESIGN.md`'s "Not yet designed" / next-steps section to reflect what's now implemented.

## Critical files

- `frontend/src/style.css` — all 10 theme blocks, `:root`, radius scale, and the ~9 component
  sections touched in Phase 2
- `frontend/src/lib/theme-apply.ts` — canonical token-list imports, dark-class toggle
- `frontend/src/lib/themeTokens.ts` — new, the three derived token lists
- `frontend/src/components/Terminal.tsx` — ANSI/mono-font theme reactivity, `fitAddon.fit()` after font change
- `frontend/src/components/SettingsPage.tsx` — `allVarKeys`/`defaultTheme`/`THEME_DOTS`, vertical-nav migration
- `frontend/src/types.ts` — `THEMES`
- `frontend/src/App.tsx` — sidebar `defaultWidth`, `monoFont` prop threading to `Terminal`
- `db.go` (`GetSettings`, `ResetAll`), `migrations/0011_default_theme.go` (new),
  `importexport_service.go` (`SaveThemeTemplate`)
- `frontend/e2e/mocks/runtime.ts` — seeded defaults for Phase 2 verification

## Verification

- `make check` and `make lint` after each phase.
- `go test ./...` (migration tests especially after Phase 1).
- `cd frontend && pnpm test` (Vitest — unaffected by styling, should stay green throughout).
- `cd frontend && pnpm test:e2e` after every Phase 2 slice, and the full suite at the end of
  Phase 3 — watching specifically for the ~177 class-based locators identified during exploration
  and the settings `getByRole('tab'/'tablist')` locators.
- Manual `wails3 dev` pass per Phase 3's checklist — this is a visual redesign, so automated tests
  proving "nothing broke" are necessary but not sufficient; eyeball the result.

## Known gap at time of writing

The design canvas currently has 6 screens; Phase 2.0 above calls for a 7th (empty title/description
+ tags row) before Phase 2 item 3 is implemented. That 7th mockup has not been created yet.
