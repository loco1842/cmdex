import React, { useState, useMemo, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { type VariablePrompt as VariablePromptType, type VariablePreset } from '../types';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { Plus, Check, X, Play } from 'lucide-react';
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from '@/components/ui/alert-dialog';

interface VariablePromptProps {
  mode: 'manage' | 'fill';
  variables: VariablePromptType[];
  presets: VariablePreset[];
  defaultPresetId?: string;
  initialValues?: Record<string, string>;
  onPresetChange?: (presetId: string) => void;
  onSubmit: (values: Record<string, string>) => void;
  onCancel: () => void;
  onSavePreset: (name: string, values: Record<string, string>) => Promise<void>;
  onUpdatePreset: (presetId: string, name: string, values: Record<string, string>) => Promise<void>;
  onDeletePreset: (presetId: string) => Promise<void>;
}

const VariablePrompt: React.FC<VariablePromptProps> = ({
  mode,
  variables,
  presets,
  defaultPresetId,
  initialValues,
  onPresetChange,
  onSubmit,
  onCancel,
  onSavePreset,
  onUpdatePreset,
  onDeletePreset,
}) => {
  const { t } = useTranslation();
  const resolveInitialPreset = (): VariablePreset | undefined => {
    if (mode !== 'manage' || presets.length === 0) return undefined;
    if (defaultPresetId) {
      const found = presets.find(p => p.id === defaultPresetId);
      if (found) return found;
    }
    return presets[0];
  };

  const initialPreset = resolveInitialPreset();

  const buildDefaults = () => {
    const init: Record<string, string> = {};
    if (initialPreset) {
      variables.forEach(v => {
        init[v.name] = initialPreset.values[v.name] ?? v.defaultValue ?? '';
      });
    } else {
      variables.forEach(v => {
        init[v.name] = initialValues?.[v.name] || v.defaultValue || '';
      });
    }
    return init;
  };

  const [values, setValues] = useState<Record<string, string>>(buildDefaults);
  const [confirmDeletePreset, setConfirmDeletePreset] = useState(false);
  const [selectedPresetId, setSelectedPresetId] = useState<string>(() => initialPreset?.id ?? '');
  const [editingPresetNameId, setEditingPresetNameId] = useState<string>('');
  const [editingName, setEditingName] = useState('');
  const [toolbarName, setToolbarName] = useState(() => {
    if (initialPreset) return initialPreset.name;
    if (mode === 'manage') return 'Default';
    return '';
  });
  const nameInputRef = useRef<HTMLInputElement>(null);
  const toolbarNameInputRef = useRef<HTMLInputElement>(null);
  const [editingToolbarName, setEditingToolbarName] = useState(() => mode === 'manage' && presets.length === 0);
  const isCreatingNew = mode === 'manage' && presets.length === 0;

  const selectedPreset = presets.find(p => p.id === selectedPresetId) || null;
  const [snapshotValues, setSnapshotValues] = useState<Record<string, string>>(() => {
    if (initialPreset) {
      const snap: Record<string, string> = {};
      variables.forEach(v => { snap[v.name] = initialPreset.values[v.name] ?? v.defaultValue ?? ''; });
      return snap;
    }
    return {};
  });
  const [snapshotName, setSnapshotName] = useState(() => initialPreset?.name ?? '');

  const isDirty = useMemo(() => {
    if (isCreatingNew) return toolbarName.trim().length > 0;
    if (!selectedPreset) return false;
    const nameChanged = toolbarName.trim() !== snapshotName;
    const valuesChanged = variables.some(v => (values[v.name] || '') !== (snapshotValues[v.name] || ''));
    return nameChanged || valuesChanged;
  }, [values, snapshotValues, selectedPreset, variables, toolbarName, snapshotName, isCreatingNew]);

  const selectedPresetIdRef = useRef(selectedPresetId);
  useEffect(() => { selectedPresetIdRef.current = selectedPresetId; });
  const prevPresetsRef = useRef(presets);
  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect -- auto-select newest preset on add, fallback on delete */
    if (mode !== 'manage') return;
    const prev = prevPresetsRef.current;
    if (presets.length > prev.length) {
      const newest = presets[presets.length - 1];
      if (newest) {
        setSelectedPresetId(newest.id);
        onPresetChange?.(newest.id);
        const newValues: Record<string, string> = {};
        variables.forEach(v => {
          newValues[v.name] = newest.values[v.name] ?? v.defaultValue ?? '';
        });
        setValues(newValues);
        setSnapshotValues({ ...newValues });
        setToolbarName(newest.name);
        setSnapshotName(newest.name);
      }
    } else if (presets.length < prev.length) {
      if (!presets.find(p => p.id === selectedPresetIdRef.current)) {
        if (presets.length > 0) {
          const first = presets[0];
          setSelectedPresetId(first.id);
          onPresetChange?.(first.id);
          const newValues: Record<string, string> = {};
          variables.forEach(v => {
            newValues[v.name] = first.values[v.name] ?? v.defaultValue ?? '';
          });
          setValues(newValues);
          setSnapshotValues({ ...newValues });
          setToolbarName(first.name);
          setSnapshotName(first.name);
        } else {
          setSelectedPresetId('');
          setToolbarName('Default');
          setSnapshotValues({});
          setSnapshotName('');
          onPresetChange?.('');
        }
      }
    }
    prevPresetsRef.current = presets;
  }, [presets, mode, variables, onPresetChange]);
  /* eslint-enable react-hooks/set-state-in-effect */

  useEffect(() => {
    if (editingPresetNameId && nameInputRef.current) {
      nameInputRef.current.focus();
      nameInputRef.current.select();
    }
  }, [editingPresetNameId]);

  useEffect(() => {
    if (editingToolbarName && toolbarNameInputRef.current) {
      toolbarNameInputRef.current.focus();
      toolbarNameInputRef.current.select();
    }
  }, [editingToolbarName]);

  const handleSubmit = () => onSubmit(values);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (mode === 'manage') {
        handleSaveCurrentPreset();
      } else {
        handleSubmit();
      }
    }
  };

  const handleSelectPreset = (presetId: string) => {
    setSelectedPresetId(presetId);
    onPresetChange?.(presetId);
    const preset = presets.find(p => p.id === presetId);
    if (!preset) return;
    const newValues: Record<string, string> = {};
    variables.forEach(v => {
      newValues[v.name] = preset.values[v.name] ?? v.defaultValue ?? '';
    });
    setValues(newValues);
    setSnapshotValues({ ...newValues });
    setToolbarName(preset.name);
    setSnapshotName(preset.name);
  };

  const handleCreatePreset = async (name?: string) => {
    const presetName = name || toolbarName.trim() || 'Default';
    await onSavePreset(presetName, { ...values });
  };

  const handleSaveCurrentPreset = async () => {
    if (isCreatingNew) {
      await handleCreatePreset();
      return;
    }
    if (!selectedPreset || !isDirty) return;
    const name = toolbarName.trim() || selectedPreset.name;
    await onUpdatePreset(selectedPresetId, name, { ...values });
    setSnapshotValues({ ...values });
    setSnapshotName(name);
    setToolbarName(name);
  };

  const handleDeletePreset = async () => {
    if (!selectedPresetId) return;
    await onDeletePreset(selectedPresetId);
    setSelectedPresetId('');
    setSnapshotValues({});
  };

  const handleDoubleClickName = (preset: VariablePreset) => {
    setEditingPresetNameId(preset.id);
    setEditingName(preset.name);
  };

  const commitNameEdit = async () => {
    if (!editingPresetNameId || !editingName.trim()) {
      setEditingPresetNameId('');
      return;
    }
    const preset = presets.find(p => p.id === editingPresetNameId);
    if (preset && editingName.trim() !== preset.name) {
      await onUpdatePreset(editingPresetNameId, editingName.trim(), preset.values);
    }
    setEditingPresetNameId('');
  };

  const firstEmptyIdx = variables.findIndex(v => !values[v.name]);
  const fillFocusIdx = firstEmptyIdx >= 0 ? firstEmptyIdx : 0;

  return (
    <>
    {mode === 'fill' ? (
      <Dialog open onOpenChange={(open) => { if (!open) onCancel(); }}>
        <DialogContent className="max-w-sm p-0 gap-0" data-testid="fill-variables-dialog">
          <DialogHeader className="px-5 pt-5 pb-3">
            <DialogTitle className="text-base">{t('variablePrompt.fillTitle')}</DialogTitle>
            <DialogDescription className="sr-only">{t('variablePrompt.fillDescription')}</DialogDescription>
          </DialogHeader>
          <div className="vp-fill-vars px-5 pb-2">
            {variables.map((v, i) => (
                <div key={v.name} className="vp-fill-row" data-testid={`fill-var-row-${v.name}`}>
                  <div className="vp-fill-label">
                    <code className="vp-fill-varname">{v.name}</code>
                    {v.description && <span className="vp-fill-desc">{v.description}</span>}
                  </div>
                  <Input
                    className="vp-fill-input font-mono text-sm h-8"
                    data-testid={`fill-var-input-${v.name}`}
                    placeholder={v.example ? `e.g. ${v.example}` : ''}
                    value={values[v.name] || ''}
                    onChange={(e) => setValues({ ...values, [v.name]: e.target.value })}
                    onKeyDown={handleKeyDown}
                    autoFocus={i === fillFocusIdx}
                  />
                </div>
              ))}
          </div>
          <div className="flex items-center justify-end gap-2 px-5 py-4 border-t border-border">
            <Button variant="ghost" size="sm" onClick={onCancel} data-testid="fill-variables-cancel">{t('variablePrompt.cancel')}</Button>
            <Button variant="success" size="sm" onClick={handleSubmit} data-testid="fill-variables-execute">
              <Play className="size-3.5" /> {t('variablePrompt.execute')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    ) : (
      <Dialog open onOpenChange={(open) => { if (!open) onCancel(); }}>
        <DialogContent className="sm:max-w-xl md:max-w-3xl lg:max-w-5xl xl:max-w-7xl p-0">
          <DialogHeader className="px-6 pt-6 pb-0">
            <DialogTitle>{t('variablePrompt.manageTitle')}</DialogTitle>
            <DialogDescription className="sr-only">{t('variablePrompt.manageDescription')}</DialogDescription>
          </DialogHeader>
          <div className="vp-layout">
            <div className="vp-preset-list">
              <div className="flex items-center justify-between px-3 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('variablePrompt.presets')}</span>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon-xs" onClick={() => handleCreatePreset(t('commandDetail.newPresetName'))}>
                      <Plus />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('variablePrompt.saveAsNewPreset')}</TooltipContent>
                </Tooltip>
              </div>
              <div className="flex-1 overflow-y-auto scrollbar-thin scrollbar-w-1 scrollbar-thumb-border scrollbar-track-transparent">
                <div className="vp-preset-items">
                  {presets.length === 0 ? (
                    <div className="px-3 py-4 text-xs text-muted-foreground text-center italic">{t('variablePrompt.noPresetsYet')}</div>
                  ) : (
                    presets.map(p => (
                      <div
                        key={p.id}
                        className={`vp-preset-item ${selectedPresetId === p.id ? 'active' : ''}`}
                        onClick={() => handleSelectPreset(p.id)}
                        onDoubleClick={() => handleDoubleClickName(p)}
                      >
                        {editingPresetNameId === p.id ? (
                          <Input
                            ref={nameInputRef}
                            className="h-6 text-xs px-1"
                            value={editingName}
                            onChange={(e) => setEditingName(e.target.value)}
                            onBlur={commitNameEdit}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') commitNameEdit();
                              if (e.key === 'Escape') setEditingPresetNameId('');
                            }}
                            onClick={(e) => e.stopPropagation()}
                          />
                        ) : (
                          <span className="vp-preset-item-name">{p.name}</span>
                        )}
                        {selectedPresetId === p.id && editingPresetNameId !== p.id && (
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            className="shrink-0"
                            onClick={(e) => { e.stopPropagation(); setConfirmDeletePreset(true); }}
                          >
                            <X className="size-3" />
                          </Button>
                        )}
                      </div>
                    ))
                  )}
                </div>
              </div>
            </div>

            <div className="vp-center">
              {(selectedPreset || isCreatingNew) && (
                <div className="flex items-center gap-2 px-4 py-2 border-b border-border">
                  <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('variablePrompt.preset')}</span>
                  {editingToolbarName ? (
                    <Input
                      ref={toolbarNameInputRef}
                      className="h-7 text-sm flex-1"
                      value={toolbarName}
                      onChange={(e) => setToolbarName(e.target.value)}
                      onBlur={() => setEditingToolbarName(false)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') setEditingToolbarName(false);
                        if (e.key === 'Escape') setEditingToolbarName(false);
                      }}
                    />
                  ) : (
                    <span
                      className="text-sm flex-1 truncate cursor-text px-1"
                      onDoubleClick={() => setEditingToolbarName(true)}
                      title={t('commandDetail.doubleClickToRename')}
                    >
                      {toolbarName || 'Default'}
                    </span>
                  )}
                </div>
              )}
              <div className="vp-vars-scroll overflow-y-auto scrollbar-thin scrollbar-w-1 scrollbar-thumb-border scrollbar-track-transparent">
                <div className="space-y-3 p-4">
                  {variables.map((v, i) => (
                    <div key={v.name} className="space-y-1">
                      <Label className="text-sm font-medium">{v.name}</Label>
                      {v.description && (
                        <p className="text-xs text-muted-foreground">{v.description}</p>
                      )}
                      <Input
                        className="font-mono"
                        placeholder={v.example ? t('variablePrompt.examplePrefix', { example: v.example }) : t('variablePrompt.enterValueFor', { name: v.name })}
                        value={values[v.name] || ''}
                        onChange={(e) => setValues({ ...values, [v.name]: e.target.value })}
                        onKeyDown={handleKeyDown}
                        autoFocus={i === 0}
                      />
                      {v.defaultExpr && (
                        <p className="text-xs text-muted-foreground">
                          {t('variablePrompt.default')} <code className="bg-muted px-1 rounded">{v.defaultExpr}</code>
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              <Separator />

              <DialogFooter className="px-4 py-4">
                {!isCreatingNew && (
                  <Button variant="outline" className="group gap-0 overflow-hidden" onClick={() => handleCreatePreset(t('commandDetail.newPresetName'))}>
                    <span className="max-w-0 opacity-0 group-hover:max-w-40 group-hover:opacity-100 transition-all duration-200 whitespace-nowrap overflow-hidden">
                      {t('variablePrompt.saveAsNew')}
                    </span>
                    <Plus className="size-4 shrink-0 ml-0 group-hover:ml-1.5 transition-[margin] duration-200" />
                  </Button>
                )}
                <Button
                  variant="default"
                  onClick={handleSaveCurrentPreset}
                  disabled={!isDirty}
                >
                  <Check className="size-4" /> {t('variablePrompt.save')}
                </Button>
              </DialogFooter>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    )}

    <AlertDialog open={confirmDeletePreset} onOpenChange={setConfirmDeletePreset}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('variablePrompt.deletePresetTitle')}</AlertDialogTitle>
          <AlertDialogDescription>{t('variablePrompt.deletePresetDescription')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={() => { handleDeletePreset(); setConfirmDeletePreset(false); }}
          >
            {t('variablePrompt.deletePresetAction')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
    </>
  );
};

export default VariablePrompt;
