// CSS selectors mapped to data-testid attributes that actually exist in
// `frontend/src`. Every entry here was verified against source at the time
// it was written — this file previously drifted (11 of 24 entries referenced
// testids that did not exist anywhere in the codebase) and was imported by
// zero specs, so a test built on it would have silently matched nothing.
// mock-contract.spec.ts does not cover this file (it only checks the
// backend method-ID table); keep this list honest by hand.

export const sel = {
  // Sidebar
  sidebarAddCommand: '[data-testid="sidebar-add-command"]',
  sidebarSettings: '[data-testid="sidebar-settings"]',
  commandItem: (cmdId: string) => `[data-testid="command-item-${cmdId}"]`,

  // Tab bar (command tabs)
  tabBar: '[data-testid="tab-bar"]',
  tabItem: (tabId: string) => `[data-testid="tab-${tabId}"]`,
  tabDirtyDot: (tabId: string) => `[data-testid="tab-dirty-dot-${tabId}"]`,

  // Command detail
  commandTitle: '[data-testid="command-title"]',
  commandDescription: '[data-testid="command-description"]',
  commandScript: '[data-testid="command-script"]',
  commandScriptTextarea: '[data-testid="command-script-textarea"]',
  commandRunBtn: '[data-testid="command-run-btn"]',
  scriptEditEnterBtn: '[data-testid="script-edit-enter-btn"]',
  scriptEditSaveBtn: '[data-testid="script-edit-save-btn"]',
  scriptEditDiscardBtn: '[data-testid="script-edit-discard-btn"]',

  // Presets (CommandDetail's inline preset chips/vars)
  presetChip: (presetId: string) => `[data-testid="preset-chip-${presetId}"]`,
  presetChipRename: (presetId: string) => `[data-testid="preset-chip-rename-${presetId}"]`,
  presetChipAdd: '[data-testid="preset-chip-add"]',
  presetVarInput: (name: string) => `[data-testid="preset-var-input-${name}"]`,
  presetValuesRevert: '[data-testid="preset-values-revert"]',
  presetValuesSave: '[data-testid="preset-values-save"]',

  // Working-directory dialog
  workingDirectoryDialog: '[data-testid="working-directory-dialog"]',
  workingDirectoryInput: '[data-testid="working-directory-input"]',
  workingDirectoryBrowse: '[data-testid="working-directory-browse"]',
  workingDirectoryClear: '[data-testid="working-directory-clear"]',
  workingDirectoryApply: '[data-testid="working-directory-apply"]',

  // Script-discard dialog (CommandDetail) — NOTE inverted button semantics:
  // "discard" reverts the pending edit, "save" persists it.
  scriptDiscardDialog: '[data-testid="script-discard-dialog"]',
  scriptDiscardDiscard: '[data-testid="script-discard-discard"]',
  scriptDiscardSave: '[data-testid="script-discard-save"]',

  // Delete-preset dialog (CommandDetail)
  confirmDeletePresetDialog: '[data-testid="confirm-delete-preset-dialog"]',
  confirmDeletePresetCancel: '[data-testid="confirm-delete-preset-cancel"]',
  confirmDeletePresetConfirm: '[data-testid="confirm-delete-preset-confirm"]',

  // Fill-variables dialog (VariablePrompt mode="fill")
  fillVariablesDialog: '[data-testid="fill-variables-dialog"]',
  fillVarRow: (name: string) => `[data-testid="fill-var-row-${name}"]`,
  fillVarInput: (name: string) => `[data-testid="fill-var-input-${name}"]`,
  fillVariablesCancel: '[data-testid="fill-variables-cancel"]',
  fillVariablesExecute: '[data-testid="fill-variables-execute"]',

  // Discard-tab dialog (App.tsx — closing a dirty tab)
  confirmDiscardTabDialog: '[data-testid="confirm-discard-tab-dialog"]',
  confirmDiscardTabCancel: '[data-testid="confirm-discard-tab-cancel"]',
  confirmDiscardTabConfirm: '[data-testid="confirm-discard-tab-confirm"]',

  // Variables-will-be-removed dialog (App.tsx)
  confirmVarRemovalDialog: '[data-testid="confirm-var-removal-dialog"]',
  confirmVarRemovalCancel: '[data-testid="confirm-var-removal-cancel"]',
  confirmVarRemovalConfirm: '[data-testid="confirm-var-removal-confirm"]',

  // Delete-category dialog (Sidebar.tsx) — distinct testids from the
  // discard-tab dialog above; they used to collide under "confirm-dialog".
  confirmDeleteCategoryDialog: '[data-testid="confirm-delete-category-dialog"]',
  confirmDeleteCategoryCancel: '[data-testid="confirm-delete-category-cancel"]',
  confirmDeleteCategoryConfirm: '[data-testid="confirm-delete-category-confirm"]',

  // Category editor modal
  categoryEditor: '[data-testid="category-editor"]',
  categoryNameInput: '[data-testid="category-name-input"]',

  // Floating save bar
  floatingSaveBar: '[data-testid="floating-save-bar"]',
  saveBarSave: '[data-testid="save-bar-save"]',
  saveBarDiscard: '[data-testid="save-bar-discard"]',

  // Command palette
  commandPalette: '[data-testid="command-palette"]',
  paletteInput: '[data-testid="palette-input"]',
  paletteEmpty: '[data-testid="palette-empty"]',
  paletteItem: (cmdId: string) => `[data-testid="palette-item-${cmdId}"]`,

  // Terminal
  terminalTabBar: '[data-testid="terminal-tab-bar"]',
  terminalTab: (sessionId: string) => `[data-testid="terminal-tab-${sessionId}"]`,
  terminalTabStatus: (sessionId: string) => `[data-testid="terminal-tab-status-${sessionId}"]`,
  terminalNewSessionBtn: '[data-testid="terminal-new-session-btn"]',
  terminalContainer: (sessionId: string) => `[data-testid="terminal-container-${sessionId}"]`,

  // Settings page — Danger Zone
  dangerZoneResetButton: '[data-testid="danger-zone-reset-button"]',
  dangerZoneConfirm: '[data-testid="danger-zone-confirm"]',
  dangerZoneCancel: '[data-testid="danger-zone-cancel"]',
} as const;
