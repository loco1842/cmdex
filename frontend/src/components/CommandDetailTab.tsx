import React, { useCallback, useMemo } from 'react';
import CommandDetail from './CommandDetail';
import FloatingSaveBar from './FloatingSaveBar';
import type { Command, TabDraft, VariablePrompt, OSPathMap, OSKey } from '../types';
import { makePlaceholderCommand } from '../utils/tabDraft';
import { variableDefinitionsToPrompts } from '../utils/templateVars';

interface CommandDetailTabProps {
  tabId: string;
  /** The saved Command, or null for a not-yet-created tab (isTabNew) — a
   * placeholder is built internally below so its reference stays stable. */
  command: Command | null;
  draft: TabDraft;
  baseline: TabDraft | undefined;
  isTabNew: boolean;
  isTabActive: boolean;
  isTabDirty: boolean;
  isExecuting: boolean;
  /** Server-resolved variable prompts (CEL defaults evaluated). Only meaningful
   * — and only read — while this tab is active; an inactive tab derives its own
   * stable prompts from draft.variables instead (see tabVariables below). */
  variables: VariablePrompt[];
  currentOS: OSKey;
  defaultWorkingDir: OSPathMap;
  onDraftChange: (tabId: string, partial: Partial<TabDraft>) => void;
  onExecute: (tabId: string, values: Record<string, string>) => void;
  onFillVariables: (tabId: string, initialValues: Record<string, string>) => void;
  onRenamePreset: (tabId: string, presetId: string, newName: string) => Promise<void>;
  onDeletePreset: (tabId: string, presetId: string) => Promise<void>;
  onAddPreset: (tabId: string, initialValues?: Record<string, string>) => Promise<string>;
  onSavePresetValues: (tabId: string, presetId: string, values: Record<string, string>) => Promise<void>;
  onReorderPresets: (tabId: string, presetIds: string[]) => Promise<void>;
  onSaveScript: (tabId: string, scriptBody: string) => Promise<void>;
  onResolvedValuesChange?: (values: Record<string, string>) => void;
  onSave: (tabId: string) => void;
  onDiscard: (tabId: string) => void;
}

const CommandDetailTab = React.memo<CommandDetailTabProps>(function CommandDetailTab({
  tabId,
  command,
  draft,
  baseline,
  isTabNew,
  isTabActive,
  isTabDirty,
  isExecuting,
  variables,
  currentOS,
  defaultWorkingDir,
  onDraftChange,
  onExecute,
  onFillVariables,
  onRenamePreset,
  onDeletePreset,
  onAddPreset,
  onSavePresetValues,
  onReorderPresets,
  onSaveScript,
  onResolvedValuesChange,
  onSave,
  onDiscard,
}) {
  const boundExecute = useCallback(
    (values: Record<string, string>) => onExecute(tabId, values),
    [tabId, onExecute],
  );
  const boundFillVariables = useCallback(
    (initialValues: Record<string, string>) => onFillVariables(tabId, initialValues),
    [tabId, onFillVariables],
  );
  const boundRenamePreset = useCallback(
    (presetId: string, newName: string) => onRenamePreset(tabId, presetId, newName),
    [tabId, onRenamePreset],
  );
  const boundDeletePreset = useCallback(
    (presetId: string) => onDeletePreset(tabId, presetId),
    [tabId, onDeletePreset],
  );
  const boundAddPreset = useCallback(
    (initialValues?: Record<string, string>) => onAddPreset(tabId, initialValues),
    [tabId, onAddPreset],
  );
  const boundSavePresetValues = useCallback(
    (presetId: string, values: Record<string, string>) => onSavePresetValues(tabId, presetId, values),
    [tabId, onSavePresetValues],
  );
  const boundReorderPresets = useCallback(
    (presetIds: string[]) => onReorderPresets(tabId, presetIds),
    [tabId, onReorderPresets],
  );
  const boundSaveScript = useCallback(
    (scriptBody: string) => onSaveScript(tabId, scriptBody),
    [tabId, onSaveScript],
  );
  const boundDraftChange = useCallback(
    (partial: Partial<TabDraft>) => onDraftChange(tabId, partial),
    [tabId, onDraftChange],
  );
  const boundSave = useCallback(() => onSave(tabId), [tabId, onSave]);
  const boundDiscard = useCallback(() => onDiscard(tabId), [tabId, onDiscard]);

  // Built here (not in App's render loop) so the reference is stable across
  // unrelated App renders — keyed on tabId + categoryId (both primitives, so
  // useMemo's own dependency comparison is all the stability this needs).
  const placeholderCommand = useMemo(
    () => makePlaceholderCommand(tabId, draft.categoryId),
    [tabId, draft.categoryId],
  );
  const resolvedCommand = isTabNew ? placeholderCommand : command;

  // Same reasoning for an inactive tab's variable prompts: derive them from
  // draft.variables (stable except at load/save) instead of recomputing a
  // fresh array from the App-level tabDrafts object on every render.
  const inactivePrompts = useMemo(
    () => variableDefinitionsToPrompts(draft.variables),
    [draft.variables],
  );
  const tabVariables = isTabActive ? variables : inactivePrompts;

  if (!resolvedCommand) return null;

  return (
    <div
      className="main-body command-tab-shell"
      style={{ display: isTabActive ? 'flex' : 'none' }}
    >
      <CommandDetail
        command={resolvedCommand}
        draft={draft}
        baselineScriptBody={baseline?.scriptBody || ''}
        onDraftChange={boundDraftChange}
        isNewCommand={isTabNew}
        isExecuting={isExecuting}
        variables={tabVariables}
        onExecute={boundExecute}
        onFillVariables={boundFillVariables}
        onRenamePreset={boundRenamePreset}
        onDeletePreset={boundDeletePreset}
        onAddPreset={boundAddPreset}
        onSavePresetValues={boundSavePresetValues}
        onReorderPresets={boundReorderPresets}
        onResolvedValuesChange={onResolvedValuesChange}
        onSaveScript={boundSaveScript}
        currentOS={currentOS}
        defaultWorkingDir={defaultWorkingDir}
      />
      <FloatingSaveBar
        visible={isTabDirty}
        saveDisabled={!draft || !draft.scriptBody.trim()}
        onSave={boundSave}
        onDiscard={boundDiscard}
      />
    </div>
  );
});

export default CommandDetailTab;
