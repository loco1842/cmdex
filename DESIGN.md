# Cmdex UI Revamp — Design Reference

Design canvas: https://claude.ai/code/artifact/e7819dd9-1be7-43fe-bd9e-dfd4cd71ff58

Direction: **Refined IDE** — clean geometric sans UI + monospace for code/terminal, dark-first
neutral-blue palette, one violet accent, 1px borders, small radii, precise spacing. Closer in
spirit to Zed/Warp/Linear than the current default shadcn/ui look.

This file is the source of truth for token values and component decisions so they can be ported
into the real app without re-deriving them from the mockup HTML. It covers 6 of the app's screens;
see **Not yet designed** below for what's still open.

## Typography

- UI font: **Manrope** (400/500/600/700) — replaces Inter/Geist/Nunito as the primary UI face.
- Code/terminal font: **JetBrains Mono** (400/500/600) — already one of the app's real monospace
  options today, kept for continuity.
- Base UI size 13px. Command/tab titles 12.5–14.5px/600–700. Section labels 10.5px, uppercase,
  `letter-spacing: .07em`, `font-weight: 700`, muted color. Screen title 22px/700,
  `letter-spacing: -0.015em`.

## Color tokens (OKLCH, same hue per role in light and dark)

### Dark (default)
```
--bg               oklch(16% 0.014 262)
--bg-2              oklch(12% 0.012 262)   /* sidebar / tab bar / terminal recess */
--surface           oklch(19% 0.014 262)   /* cards, code block, modals */
--surface-2         oklch(22% 0.014 262)   /* nested surface, segmented control track */
--border            oklch(28% 0.014 262)
--border-strong     oklch(34% 0.016 262)   /* modal/save-bar edge */
--fg                oklch(93% 0.006 262)
--fg-muted          oklch(68% 0.012 262)
--fg-faint          oklch(50% 0.012 262)
--accent            oklch(70% 0.17 290)    /* violet */
--accent-fg         oklch(15% 0.01 290)
--accent-soft       oklch(70% 0.17 290 / 0.14)
--accent-soft-strong oklch(70% 0.17 290 / 0.26)
--success           oklch(72% 0.15 150)
--danger            oklch(66% 0.19 27)
```
Category-dot accents (sidebar): cyan `oklch(72% 0.11 220)`, orange `oklch(72% 0.15 55)`,
green `oklch(72% 0.13 150)`.

### Light
```
--bg               oklch(98% 0.004 262)
--bg-2              oklch(95% 0.006 262)
--surface           oklch(100% 0 0)
--surface-2         oklch(97% 0.005 262)
--border            oklch(90% 0.007 262)
--border-strong     oklch(82% 0.009 262)
--fg                oklch(22% 0.012 262)
--fg-muted          oklch(46% 0.012 262)
--fg-faint          oklch(62% 0.01 262)
--accent            oklch(56% 0.18 290)
--accent-fg         oklch(99% 0.002 290)
--accent-soft       oklch(56% 0.18 290 / 0.10)
--accent-soft-strong oklch(56% 0.18 290 / 0.18)
--success           oklch(52% 0.14 150)
--danger            oklch(56% 0.19 27)
```
Same hue rule flipped: cyan `oklch(52% 0.1 220)`, orange `oklch(58% 0.15 55)`,
green `oklch(52% 0.13 150)`.

Every accent/success/danger keeps its hue across both modes (290 / 150 / 27) — only lightness and
chroma shift — so a future custom-theme author (or a new preset) can follow the same rule.

## Spacing / sizing

- Sidebar width 264px. Tab bar height 40px. Terminal panel default height 262px (120px in the
  empty/no-session state). Terminal session tab bar 34px, status bar 24px.
- Control height 30–34px. Border width 1px throughout (no 2px borders anywhere).
- Radii: 4–6px small controls, 8px cards/panels, 10–12px modals/tiles.
- Active tab indicator: 2px accent bar along the top edge, not a background fill.
- Active sidebar row: 2px accent bar on the left edge + `--accent-soft` background, not a full
  accent-colored row.

## Key component decisions

- **Variable placeholders render as pill chips** (`--accent-soft-strong` bg, `--accent` text,
  fully rounded), not raw `{{name}}` text. This is a genuine UX change from today's raw-text
  rendering — it makes scripts with several variables far easier to scan — and should carry
  through the real script editor and its live preview.
- **Floating save bar**: an elevated (`--border-strong` + shadow) card, appears only while the
  active tab is dirty, unsaved-dot + Discard/Save — same dirty-tracking model as today
  (`tabDrafts`/`tabBaselines`), just restyled chrome.
- **Presets** as pill chips; selected = accent border + `--accent-soft` fill (replaces the current
  plain chip style).
- **Preview panel** uses a segmented Template/Resolved control instead of tabs.
- **Command palette** and **variable-prompt modal** share one modal language: blurred/dimmed
  backdrop, `--surface` panel, `--border-strong` edge, large soft shadow, footer kbd hints.
- **Settings window** moves from the current top `Tabs` bar to a left icon+label nav (General,
  Appearance, Terminal, Import/Export, Danger Zone at the bottom in `--danger`). Appearance keeps
  all 8 existing theme presets (VS Code Dark/Light, Monokai, Tokyo Night, One Dark, Classic,
  Catppuccin Mocha, Dracula) plus a "custom theme" tile, font pickers, and a density segmented
  control — no functional changes, just the new chrome.
- **Terminal panel** keeps monospace prompt/output distinction, session tabs, and a status bar
  (shell + cwd + exit code) — restyled chrome only; no change to the PTY/OSC-133 backend.

## Not yet designed

Only 6 screens were mocked: Main (with a command open), Welcome, Command Palette, Variable
Prompt, Settings → Appearance, and a light-theme pass of Main. Still open, using the same tokens
above:
- `CategoryEditor`, `KeyboardShortcutsDialog`
- Settings tabs: General, Terminal, Import/Export, Danger Zone (confirmation states)
- Toast/error states (sonner), empty search results, empty sidebar (no categories yet)
- Resizable-panel drag handles/affordances at real interaction size

## Next step: applying this to the app

This changes real component markup and CSS, not just an app color theme — content like the chip-
styled variables, the save bar, and the settings nav restructuring go beyond what the existing
per-theme CSS-variable system covers today. Suggested path:

1. Decide scope: ship this as a 9th selectable theme preset, or make it the new default look
   while keeping the 8 existing presets as-is for color only.
2. Add Manrope as a bundled self-hosted font (`frontend/public/fonts/manrope/`, following the
   existing `@font-face` pattern in `style.css` for Inter/Geist/Nunito), and add the new tokens
   (`--surface-2`, `--border-strong`, `--accent-soft-strong`) to the theme CSS-variable mapping.
3. Implement per component, one slice at a time, verifying each in `wails3 dev` before moving on:
   `Sidebar.tsx` → `TabBar.tsx` → `CommandDetail.tsx` (chips, presets, preview, save bar) →
   `Terminal.tsx`/`TerminalTabBar.tsx` (including the xterm.js theme object, since terminal text
   itself is drawn by xterm, not CSS) → `CommandPalette.tsx` → `VariablePrompt.tsx` →
   `SettingsPage.tsx`.
4. Run the existing Vitest + Playwright suites after each slice; add/update tests for anything
   whose DOM structure changes (data-testids, selectors.ts).
